.PHONY: build proto clean run help env release release-publish version

BINARY_NAME=biobtree
SRC_DIR=src

# Version stays "dev" for plain `make build`; `make release` stamps it.
# Commit + date are stamped so even local builds are traceable.
# NOTE: named GO_LDFLAGS (not LDFLAGS) on purpose — LDFLAGS is exported by the
# conda env and read by the CGO compiler; reusing it would feed Go's -X linker
# flags to cc and break the build.
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%d)
GO_LDFLAGS := -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE)

# ---------------------------------------------------------------------------
# Toolchain
# The biobtree conda env supplies gcc (required for CGO/LMDB) and protoc;
# $(go env GOPATH)/bin supplies protoc-gen-go and ffjson.
#
# Resolution order (first non-empty wins):
#   1. CONDA_BIN / CONDA_BASE from the environment or a local .env.mk
#   2. CONDA_PREFIX (an activated conda env)
#   3. `conda info --base` + CONDA_ENV
# Machine-specific paths belong in the git-ignored .env.mk, never here.
# ---------------------------------------------------------------------------
-include .env.mk

CONDA_ENV ?= biobtree
CONDA_BASE ?= $(shell conda info --base 2>/dev/null)
CONDA_BIN ?= $(if $(CONDA_PREFIX),$(CONDA_PREFIX)/bin,$(if $(CONDA_BASE),$(CONDA_BASE)/envs/$(CONDA_ENV)/bin))
GO_BIN := $(shell go env GOPATH)/bin
TOOL_PATH := $(CONDA_BIN):$(GO_BIN):$(PATH)
CONDA_CC ?= $(firstword $(wildcard $(CONDA_BIN)/x86_64-conda-linux-gnu-cc $(CONDA_BIN)/gcc))

help:
	@echo "BiobtreeV2 Build Commands:"
	@echo "  make build              - Build biobtree binary (stamps commit + date)"
	@echo "  make version            - Build and print version info"
	@echo "  make release VERSION=vX.Y.Z - Gate, tag+push, package linux binary (no gh)"
	@echo "  make release-publish VERSION=vX.Y.Z - Upload dist/ to GitHub (needs gh)"
	@echo "  make proto              - Regenerate protobuf code (only when .proto files change)"
	@echo "  make env                - Show the resolved conda/protoc toolchain"
	@echo "  make run                - Build and run biobtree"
	@echo "  make clean              - Clean build artifacts"
	@echo ""
	@echo "The biobtree conda env is resolved automatically (CONDA_PREFIX or"
	@echo "conda info --base). Override in a git-ignored .env.mk:"
	@echo "  CONDA_BIN := /path/to/envs/biobtree/bin"
	@echo ""
	@echo "Direct usage:"
	@echo "  cd src && go build -o ../biobtree"
	@echo "  ./biobtree <args>        - Run binary"

env:
	@echo "CONDA_BASE = $(CONDA_BASE)"
	@echo "CONDA_BIN  = $(CONDA_BIN)"
	@echo "CC         = $(CONDA_CC)"
	@echo "protoc     = $(if $(CONDA_BIN),$(CONDA_BIN)/protoc,not configured)"
	@echo "GOPATH/bin = $(GO_BIN)"

build:
	@test -n "$(CONDA_BIN)" || { \
		echo "✗ No conda env configured."; \
		echo "  Activate the biobtree env, or set CONDA_BIN/CONDA_BASE"; \
		echo "  (e.g. in a git-ignored .env.mk)."; \
		exit 1; }
	@test -n "$(CONDA_CC)" || { \
		echo "✗ C compiler not found in $(CONDA_BIN)"; \
		echo "  Set CONDA_BIN to the biobtree conda env's bin directory."; \
		exit 1; }
	@echo "Building $(BINARY_NAME)..."
	cd $(SRC_DIR) && PATH="$(TOOL_PATH)" CGO_ENABLED=1 CC="$(CONDA_CC)" \
		go build -ldflags "$(GO_LDFLAGS)" -o ../$(BINARY_NAME)
	@echo "✓ Built successfully: ./$(BINARY_NAME)"

version: build
	@./$(BINARY_NAME) --version

# Cut a release WITHOUT gh (runs on this build machine — CGO/LMDB must build
# here). Gates on a clean build, tags + pushes (plain git), and packages the
# linux/amd64 binary into dist/. The GitHub upload is a separate step
# (release-publish) so it can run wherever gh is available.
#   make release VERSION=v2.0.0
release:
	@test -n "$(VERSION)" || { echo "Usage: make release VERSION=vX.Y.Z"; exit 1; }
	@case "$(VERSION)" in v*) ;; *) echo "✗ VERSION must start with 'v' (e.g. v2.0.0)"; exit 1;; esac
	@git diff --quiet && git diff --cached --quiet || { echo "✗ working tree dirty — commit before releasing"; exit 1; }
	@echo "==> [1/3] Build gate ($(VERSION))..."
	cd $(SRC_DIR) && PATH="$(TOOL_PATH)" CGO_ENABLED=1 CC="$(CONDA_CC)" \
		go build -ldflags "-X main.Version=$(VERSION) $(GO_LDFLAGS)" -o ../$(BINARY_NAME)
	@./$(BINARY_NAME) --version
	@echo "==> [2/3] Tag + push (git, no gh)..."
	@git rev-parse "$(VERSION)" >/dev/null 2>&1 && echo "  tag $(VERSION) already exists — skipping" || { git tag "$(VERSION)" && git push origin "$(VERSION)"; }
	@echo "==> [3/3] Package linux/amd64..."
	rm -rf dist && mkdir -p dist
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)
	cd dist && sha256sum -- *.tar.gz > SHA256SUMS
	@echo "✓ $(VERSION) tagged, pushed, packaged in dist/:"
	@ls -1 dist/
	@echo "  Final step (where gh is available, with this dist/):"
	@echo "    make release-publish VERSION=$(VERSION)"

# Publish the GitHub release from the artifacts in dist/. Needs gh authenticated;
# the tag must already be pushed (by `make release`). Run on your laptop after
# copying dist/ over (or wherever gh + dist/ live).
#   make release-publish VERSION=v2.0.0
release-publish:
	@test -n "$(VERSION)" || { echo "Usage: make release-publish VERSION=vX.Y.Z"; exit 1; }
	@command -v gh >/dev/null 2>&1 || { echo "✗ gh CLI not found — run this where gh is installed"; exit 1; }
	@test -d dist || { echo "✗ dist/ not found — run 'make release VERSION=$(VERSION)' first and copy dist/ here"; exit 1; }
	gh release create "$(VERSION)" dist/* --title "$(VERSION)" --notes "See commit history for details."
	@echo "✓ $(VERSION) published to GitHub"

# Only run this when you modify .proto files in src/pbuf/
# The generated .pb.go files are already in git, so this is rarely needed
proto:
	@test -n "$(CONDA_BIN)" || { \
		echo "✗ No conda env configured."; \
		echo "  Activate the biobtree env, or set CONDA_BIN/CONDA_BASE"; \
		echo "  (e.g. in a git-ignored .env.mk)."; \
		exit 1; }
	@test -x "$(CONDA_BIN)/protoc" || { \
		echo "✗ protoc not found in $(CONDA_BIN)"; \
		echo "  Set CONDA_BIN to the biobtree conda env's bin directory."; \
		exit 1; }
	@echo "Generating protobuf code from .proto files..."
	cd $(SRC_DIR)/pbuf && PATH="$(TOOL_PATH)" ./gen.sh
	@echo "✓ Protobuf code generated in src/pbuf/"

run: build
	./$(BINARY_NAME)

clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	@echo "✓ Cleaned"
