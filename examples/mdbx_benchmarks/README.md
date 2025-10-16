# Biobtree Performance Benchmarks

This directory contains benchmark scripts used to test LMDB vs MDBX performance.

## REST API Benchmarks

### `benchmark_lmdb_vs_mdbx.sh`
**Comprehensive REST API benchmark**

Tests 6 different queries across both LMDB and MDBX backends:
- Simple parent/child lookups
- Multi-level traversals
- Filter queries

**Usage:**
```bash
./benchmark_lmdb_vs_mdbx.sh
```

**Requirements:**
- LMDB server running on port 9292
- MDBX server running on port 9293

### `compare_single_query.sh`
**Quick single query comparison**

Compare one query between LMDB and MDBX, showing full responses.

**Usage:**
```bash
./compare_single_query.sh 'i=9606&m=map(taxparent)'
```

### `benchmark_verify.sh`
**Verification benchmark with warmup**

More rigorous testing with:
- Warmup phase (5 iterations)
- More iterations (20)
- Response validation
- Full response display

**Usage:**
```bash
./benchmark_verify.sh
```

## gRPC Benchmarks

### `benchmark_grpc.sh`
**Comprehensive gRPC benchmark**

Same queries as REST benchmark but using gRPC protocol.

**Usage:**
```bash
./benchmark_grpc.sh
```

**Requirements:**
- `grpcurl` installed (`go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`)
- LMDB gRPC server on port 7777
- MDBX gRPC server on port 7778

### `test_grpc_single.sh`
**Single gRPC query test**

Quick test to verify gRPC is working and responses match.

**Usage:**
```bash
./test_grpc_single.sh
```

## Specialized Tests

### `test_query6.sh`
**Filter query specific test**

Tests Query 6 (the CPU-intensive filter query) to verify:
- Actual execution time (~3 seconds expected)
- Response sizes match
- Filter is working correctly

**Usage:**
```bash
./test_query6.sh
```

Shows side-by-side comparison of REST and gRPC responses.

### `test_build_tags.sh`
**Build size comparison**

Compares binary sizes with and without MDBX support.

**Usage:**
```bash
./test_build_tags.sh
```

**Note:** This script is historical - current builds always include both backends.

## Test Results Summary

All benchmarks showed **0% performance difference** between LMDB and MDBX:

| Test Type | Environment | Result |
|-----------|-------------|--------|
| REST API | NFS | 0% difference |
| REST API | /tmp (local) | 0% difference |
| gRPC API | NFS | 0-1% difference |
| Build/Write | NFS | 0% difference |

See `MDBX_INTEGRATION.md` for detailed analysis.

## Notes

### Cache Must Be Disabled
For accurate benchmarking, the application cache must be disabled in `service/mapfilter.go`.

The current production code has cache **enabled** (as it should be).

### Server Setup
To run benchmarks, you need two biobtree servers running simultaneously:

**LMDB server:**
```bash
# In config: "dbBackend": "lmdb" (or omit, it's default)
./biobtree --port 9292 --grpc-port 7777
```

**MDBX server:**
```bash
# In config: "dbBackend": "mdbx"
./biobtree --port 9293 --grpc-port 7778
```

### Benchmark Queries

The standard test queries are:
1. `i=9606&m=map(taxparent)` - Simple parent lookup
2. `i=9606&m=map(taxparent).map(taxparent).map(taxparent)` - Multi-level traversal
3. `i=59201&m=map(taxparent).map(taxparent).map(taxparent)` - Different identifier
4. `i=9606&m=map(taxchild)` - Child lookup
5. `i=9606&m=map(taxchild).map(taxchild)` - Multi-level child
6. `i=562&m=map(taxchild).filter(taxonomy.taxonomic_division=="BCT")` - Filter (CPU-intensive, ~3s)

## Historical Context

These benchmarks were created during MDBX integration to validate performance claims.

Initial synthetic micro-benchmarks showed 40-50% improvement, but real-world application testing revealed no measurable difference due to:
- Database operations being only 10-20% of total query time
- Application bottlenecks (CPU-bound filtering, serialization, network)
- Both databases performing well enough that they're not the limiting factor

## Contributing

When adding new benchmarks:
1. Use consistent query sets
2. Include warmup phases
3. Document requirements clearly
4. Verify response correctness, not just timing
