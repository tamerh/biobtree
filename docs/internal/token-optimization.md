# Internal: token optimization backlog

Status: open (future work, on hold) · Last updated: 2026-11-06 · Context: v2.19.x, main `db_v55`

This note lists ways to reduce the context/token cost of agent and human work on
this repo. Nothing here is required for correctness; it is deferred until it is
convenient. Two items are already implemented and are recorded so they are not
"rediscovered" later.

The audience is whoever picks this up next. Measurements are from this repo at
the date above and are approximate (≈4 chars/token).

## Where the cost is

| Sink | Size | Approx tokens |
|---|---|---|
| `tests/datasets/mondo/mondo.obo` | 52 MB | ~13 M |
| `logs/` (all runs) | 272 MB | ~68 M |
| generated `src/pbuf/*.pb*.go` | 5.2 MB | ~1.3 M |
| `out_prod/dataset_state.json` | 118 KB | ~30 K |
| `bb.sh --status` output | 139 lines | ~2.5 K |
| `bb.sh --db-versions` output | 51 lines | ~1 K |
| MCP `max_result_length` default | 50 K chars | ~12 K per call |
| `AGENTS.md` (injected every session) | ~330 lines | ~4 K per session |

Two patterns cause most of the waste: commands that print all datasets when one
was asked for, and reads of generated/data files that should never be text.

## Already done

- `.ignore` — ripgrep-based tools skip generated protobuf, generated genome path
  manifests, and large test fixtures on directory searches. Explicit file paths
  still work, so a deliberate read is not blocked.
- `.gitattributes` — `-diff` on generated protobuf and large fixtures, so
  `git diff` reports "Binary files differ" instead of dumping megabytes.
- `AGENTS.md` records the read/diff/search rules, and `make env` prints the
  resolved toolchain so agents do not probe paths by trial and error.
- Generated protobuf is inspected with `go doc ./pbuf <Type>` from `src/`; the
  `.pb.go` files are never read or edited.

## Proposed work

Ordered by payoff/effort. None of these change query results.

### 1. Dataset-scoped status output (low effort, recurring win)

`./bb.sh --status` always renders every configured dataset, and `--db-versions`
does the same for every federation.

Add:

```bash
./bb.sh out_prod --status <dataset>[,<dataset>...]
./bb.sh out_prod --status --json
./bb.sh out_prod --db-versions <federation>
```

Expected: 139 lines → 1–5; 51 lines → ~6. `--json` is the most useful for agents
because it can be projected with `jq` and never needs text parsing.

Note: `bb.sh` re-execs itself for builds and may still be executing during a
long generate. **Do not edit it while a build is running** — bash reads the
script incrementally. Make the change between builds.

### 2. Small helper scripts (low effort)

Add tracked scripts (no machine-specific paths):

- `tools/state.sh <dataset>...` — compact `{status,edges,keys}` from
  `$OUT_DIR/dataset_state.json`, defaulting to `out_prod`.
- `tools/verify.sh <id> <chain> [port]` — `curl` + `jq` projection for a map
  chain, defaulting to the staging port.

Expected: replaces the 2–4 exploratory calls agents usually need to get the
`jq`/`curl` invocation right.

### 3. Lower the MCP result cap (low effort, needs MCP restart)

`execute_tool(..., max_result_length=50000)` in `mcp_srv/tools.py` is the budget
for every tool result. Most responses are far smaller; the cap only matters when
truncation happens.

Proposal: default to 20 K chars and let the caller pass a larger
`max_result_length` explicitly. Keeps normal calls cheap without hiding data.

Note: the MCP servers are long-running; the change only takes effect after a
restart, which needs owner approval.

### 4. Field projection on the HTTP API (medium effort, biggest API win)

`/ws/map/` and `/ws/entry/` return full rows/attributes. High-fanout datasets
produce pages that exceed any reasonable budget even in `mode=lite`.

Proposal: add `fields=<comma-separated>` selecting output columns, e.g.
`fields=id,name,af`. Applied after the existing filter/limit so pagination
semantics do not change. Document it next to `mode`.

Expected: a 10-gene, 100-target page can shrink by an order of magnitude for
agents that only need IDs and one attribute.

### 5. Separate progress logs from result logs (medium effort)

Dataset logs are dominated by `Processing...Elapsed Ns ...` lines written at a
fixed interval. The useful lines (start, counts, errors, duration) are a small
fraction of a multi-megabyte file.

Proposal: write progress to `logs/<dataset>.progress` (or only when a TTY) and
keep `logs/<dataset>.log` for events. `tail`/`grep` then returns signal without
filtering.

### 6. Quiet test output (medium effort)

`tests/run_tests.py` prints per-test detail. For agent verification the useful
output is the summary plus failures.

Proposal: `--quiet` (one line per dataset) and `--json` (machine-readable
summary). Keep the current output as the default for humans.

### 7. Trim `AGENTS.md` (low effort, fixed per-session cost)

`AGENTS.md` is injected into every session. The "state at time of writing"
section goes stale fastest and costs the most per freshness.

Proposal: move volatile state to a separate internal note loaded on demand, and
keep `AGENTS.md` to rules, map, workflow, and gotchas. Target under ~200 lines.

## Measurement

To check a change actually helps, compare the same command before/after:

```bash
./bb.sh out_prod --status | wc -l
wc -c out_prod/dataset_state.json
curl -s -G http://localhost:9291/ws/map/ \
  --data-urlencode "i=TP53" --data-urlencode "m=>>hgnc>>go" \
  --data-urlencode "mode=lite" | wc -c
```

## Deliberately not proposed

- Splitting `dataset_state.json` per federation, or moving it out of JSON. The
  file is cheap when projected and the split would complicate state handling for
  a small gain.
- Reducing the service's per-input result cap. That changes query behavior, not
  just transport size, and belongs in the map-budget discussion
  (`docs/internal/map-output-budget.md`).
- Log rotation/archival. It saves disk, not context, because ignore rules already
  keep agents out of `logs/`.
