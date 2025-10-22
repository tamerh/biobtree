.PHONY: build proto clean run help

BINARY_NAME=biobtree
SRC_DIR=src

help:
	@echo "BiobtreeV2 Build Commands:"
	@echo "  make build   - Build biobtree binary"
	@echo "  make proto   - Regenerate protobuf code (only needed when .proto files change)"
	@echo "  make run     - Build and run biobtree"
	@echo "  make clean   - Clean build artifacts"
	@echo ""
	@echo "Direct usage:"
	@echo "  cd src && go build -o ../biobtree"
	@echo "  ./biobtree <args>        - Run binary"

build:
	@echo "Building $(BINARY_NAME)..."
	cd $(SRC_DIR) && go build -o ../$(BINARY_NAME)
	@echo "✓ Built successfully: ./$(BINARY_NAME)"

# Only run this when you modify .proto files in src/pbuf/
# The generated .pb.go files are already in git, so this is rarely needed
proto:
	@echo "Generating protobuf code from .proto files..."
	cd $(SRC_DIR)/pbuf && ./gen.sh
	@echo "✓ Protobuf code generated in src/pbuf/"

run: build
	./$(BINARY_NAME)

clean:
	@echo "Cleaning build artifacts..."
	rm -f $(BINARY_NAME)
	@echo "✓ Cleaned"
