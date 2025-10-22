# MDBX Integration Guide

## Overview

Biobtree supports **both LMDB and MDBX** database backends through a clean abstraction layer. You can easily switch between them via configuration.

## Performance Results

**Extensive testing shows no significant performance difference in real-world biobtree workloads.**

### Synthetic Micro-Benchmark (1,000 records, isolated DB operations):

| Backend | Write Speed | Read Speed | Difference |
|---------|-------------|------------|------------|
| **LMDB** | 411K rec/sec | 768K rec/sec | Baseline |
| **MDBX** | 578K rec/sec | 1.14M rec/sec | 40-48% faster |

### Real-World Application Testing:

| Test Type | Environment | Result |
|-----------|-------------|--------|
| REST API | NFS | 0% difference |
| REST API | /tmp (local) | 0% difference |
| gRPC API | NFS | 0-1% difference |
| Build/Write | NFS | 0% difference |

**Conclusion:** In biobtree's actual workload, the database is only 10-20% of total query time. The real bottlenecks are:
- CPU-bound filter evaluation (CEL-go)
- Network/HTTP serialization overhead
- Application logic (query parsing, result building)

Even making the database 50% faster only improves overall performance by 5-10%.

## How to Use

### Default Behavior

**LMDB is the default backend** (proven stability, mature codebase).

No configuration needed - it just works!

### Switch to MDBX (optional)

To use MDBX backend, add to your `conf/application.param.json`:

```json
{
  "dbBackend": "mdbx"
}
```

### Method 2: Programmatically

```go
import "biobtree/db"

// Use the new abstraction-based methods
d := db.DB{}
env, dbi := d.OpenDBNew(write, totalKV, appconf)
defer env.Close()

// The backend is automatically selected from appconf["dbBackend"]
// Falls back to LMDB if not specified
```

### Method 3: Direct Backend Selection

```go
import "biobtree/db"

// Explicitly choose backend
backend := db.BackendMDBX  // or db.BackendLMDB
env, dbi, err := d.OpenDBWithBackend(backend, write, totalKV, appconf)
```

## Implementation Details

### Files Created

- `db/interface.go` - Database abstraction interface
- `db/lmdb_backend.go` - LMDB implementation
- `db/mdbx_backend.go` - MDBX implementation
- `db/factory.go` - Factory functions and configuration
- `db/db.go` - Updated with new `OpenDBNew()` methods

### Backward Compatibility

- **New methods** (`OpenDBNew()`, `OpenAliasDBNew()`): Use LMDB by default (can be overridden)
- **Old methods** (`OpenDB()`, `OpenAliasDB()`): Deprecated, migrate to new methods

All code has been migrated to use the new abstraction-based methods.

## MDBX-Specific Notes

### NFS Compatibility

MDBX works on NFS filesystems using the **`Exclusive` flag**, which:
- Ensures single-process exclusive access
- Bypasses file locking issues on NFS
- Perfect for biobtree's single-process build/generate workflow

### Key Differences from LMDB

1. **Auto-growing database**: No need to pre-calculate exact map size (main benefit)
2. **Performance**: Identical in biobtree workloads (extensively tested)
3. **Exclusive mode**:
   - Required for NFS compatibility
   - Single process can access DB (typical for biobtree)
   - Multiple reads/writes within that process work fine
   - **Does not affect normal operations**
4. **No ReaderCheck**: MDBX doesn't have reader lock issues
5. **Binary size**: +1MB compared to LMDB-only build

### Moving to SSD in the Future

When you migrate to local SSD storage:
- ✅ **No changes needed** - Both LMDB and MDBX work perfectly on SSD
- ✅ **Performance** - Testing shows no difference on local storage either
- ✅ **Exclusive flag** - Still works (or can be removed)
- ✅ **No code migration** - Just move the database files

**Note:** Testing on /tmp (local disk) showed identical performance, so SSD migration is unlikely to reveal performance differences either.

## Migration Path

### To Enable MDBX

1. Add `"dbBackend": "mdbx"` to your config file
2. Rebuild your database using the build command
3. Test with your datasets

### To Rollback to LMDB

1. Change config to `"dbBackend": "lmdb"` (or remove the setting)
2. Rebuild database
3. Everything works as before

## Testing

### Abstraction Layer Tests

The main abstraction test is in `examples/db_tests/`:

```bash
cd examples/db_tests
go run test_abstraction.go
```

### Performance Benchmarks

Comprehensive benchmark scripts are available in `examples/mdbx_benchmarks/`:

**REST API Benchmarks:**
```bash
cd examples/mdbx_benchmarks
./benchmark_lmdb_vs_mdbx.sh      # Full REST benchmark
./compare_single_query.sh '...'  # Single query comparison
```

**gRPC Benchmarks:**
```bash
./benchmark_grpc.sh              # Full gRPC benchmark
./test_grpc_single.sh            # Single gRPC test
```

**Specialized Tests:**
```bash
./test_query6.sh                 # Filter query verification
```

See `examples/mdbx_benchmarks/README.md` for detailed documentation.

**Requirements for benchmarks:**
- Two biobtree servers running (LMDB on ports 9292/7777, MDBX on 9293/7778)
- For gRPC: `grpcurl` installed

## Requirements

- Go 1.15+ (minimum for mdbx-go)
- Both LMDB and MDBX libraries are now dependencies
- Local or NFS storage (both supported)

## Recommendations

### Use LMDB (Default) ✅

**Best for most users:**
- Proven stability and maturity
- Widely used and well-tested
- Zero performance difference in biobtree workloads

### Use MDBX (Optional)

**Consider MDBX if you want:**
- ✅ **Auto-growing database** - No need to pre-calculate map size
- ✅ **Easier operational management** - Database grows as needed
- ✅ **More modern codebase** - Actively maintained
- ❌ **No performance benefit** - Extensively tested, identical speed

### Migration Path

**To Try MDBX:**
1. Add `"dbBackend": "mdbx"` to config
2. Rebuild your database
3. Test with your datasets

**To Switch Back to LMDB:**
1. Change config to `"dbBackend": "lmdb"` (or remove the setting)
2. Rebuild database
3. Everything works as before

## Dependencies

```go
require (
    github.com/bmatsuo/lmdb-go v1.8.0
    github.com/erigontech/mdbx-go v0.40.0
)
```

## Status

✅ **Production Ready**
- Abstraction layer tested with both backends
- NFS and local storage compatibility verified
- Extensive performance testing completed (REST, gRPC, NFS, /tmp)
- **Conclusion:** LMDB and MDBX provide identical real-world performance
- Easy switching via configuration
- Rollback path available

---

**Last Updated:** October 15, 2025
**Version:** v2.0-mdbx-integration
