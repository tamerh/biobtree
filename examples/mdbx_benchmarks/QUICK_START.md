# Quick Start - LMDB vs MDBX Benchmarks

## Prerequisites

1. **Build biobtree:**
   ```bash
   cd /path/to/biobtreev2
   go build
   ```

2. **Prepare two database builds:**
   ```bash
   # LMDB database (default)
   # In conf/application.param.json: "dbBackend": "lmdb" (or omit)
   ./biobtree update --dataset=taxonomy

   # MDBX database
   # In conf/application.param.json: "dbBackend": "mdbx"
   ./biobtree update --dataset=taxonomy
   ```

3. **For gRPC tests, install grpcurl:**
   ```bash
   go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
   export PATH="$HOME/go/bin:$PATH"
   ```

## Running Benchmarks

### Quick REST API Test (5 minutes)

**Terminal 1 - LMDB server:**
```bash
# Use LMDB database
./biobtree --port 9292
```

**Terminal 2 - MDBX server:**
```bash
# Use MDBX database
./biobtree --port 9293
```

**Terminal 3 - Run benchmark:**
```bash
cd examples/mdbx_benchmarks
./benchmark_lmdb_vs_mdbx.sh
```

### Quick gRPC Test (5 minutes)

**Terminal 1 - LMDB server:**
```bash
./biobtree --port 9292 --grpc-port 7777
```

**Terminal 2 - MDBX server:**
```bash
./biobtree --port 9293 --grpc-port 7778
```

**Terminal 3 - Run benchmark:**
```bash
cd examples/mdbx_benchmarks
./benchmark_grpc.sh
```

### Single Query Comparison

**Quick test without full benchmark:**
```bash
cd examples/mdbx_benchmarks
./compare_single_query.sh 'i=9606&m=map(taxparent)'
```

### Test Specific Heavy Query

**Verify filter query (CPU-intensive, ~3 seconds):**
```bash
cd examples/mdbx_benchmarks
./test_query6.sh
```

## Expected Results

All benchmarks should show **~0% performance difference** between LMDB and MDBX.

Example output:
```
Query 1: LMDB 0.4ms, MDBX 0.4ms (0% difference)
Query 2: LMDB 0.6ms, MDBX 0.6ms (0% difference)
...
Query 6: LMDB 3.2s, MDBX 3.2s (0% difference)
```

## Troubleshooting

### "Connection refused"
- Servers not running
- Wrong ports
- Check with: `curl http://localhost:9292/ws/meta/`

### "grpcurl not found"
- Install: `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`
- Add to PATH: `export PATH="$HOME/go/bin:$PATH"`

### gRPC "server does not support reflection API"
- Fixed in current scripts (uses proto files)
- If error persists, check pbuf/app.proto exists

### Query 6 takes <1 second
- Cache not disabled in code
- For accurate benchmarking, cache must be disabled in service/mapfilter.go
- Production code has cache enabled (correct)

## Understanding Results

### Why No Performance Difference?

Database operations are only 10-20% of total query time:
- **70-80%** - Network, serialization, app logic
- **10-20%** - Database reads
- **CPU-bound queries** - Database irrelevant (e.g., Query 6 filter)

Even if MDBX were 50% faster at database operations:
- Total improvement: 50% × 20% = **10% overall**
- Within measurement noise

### What This Means

For biobtree's workload:
- ✅ Both databases perform excellently
- ✅ Neither is a bottleneck
- ✅ Choose based on operational needs:
  - **LMDB** - Proven stability (default)
  - **MDBX** - Auto-growing database

## Full Documentation

See `README.md` in this directory for complete documentation.
