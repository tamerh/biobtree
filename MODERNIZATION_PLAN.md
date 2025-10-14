# Biobtree Modernization Plan

**Date:** October 14, 2025
**Context:** Solo developer with limited time
**Goal:** Practical improvements with minimal disruption

---

## Current State Analysis

### Your Versions (from go.mod)

```go
go 1.13                              // Released: Sept 2019 ⚠️
github.com/google/cel-go v0.3.2      // Released: Oct 2019 ⚠️
github.com/bmatsuo/lmdb-go v1.8.0    // Last update: 2019 ⚠️
```

### Latest Versions (Oct 2025)

```go
go 1.23+                             // Current
github.com/google/cel-go v0.26.1     // Released: Aug 2025
```

**Time Gap:** ~5 years behind

---

## Issue 1: CEL-go Upgrade (v0.3.2 → v0.26.1)

### Should You Upgrade?

**Answer:** ⚠️ **Maybe** - Depends on your pain points

### What's Changed (2019 → 2025)

**Version Jump:** v0.3.2 → v0.26.1 (23 minor versions!)

**Key Improvements:**

1. **Performance** 🚀
   - ~2-3x faster compilation
   - Better memory usage
   - Optimized evaluations

2. **New Features**
   - String manipulation (starts_with, ends_with, etc.)
   - Better type checking
   - Optional types support
   - Improved error messages

3. **Bug Fixes**
   - Memory leaks fixed
   - Edge case handling
   - Stability improvements

4. **Breaking Changes** ⚠️
   - API changes (parse/check/eval flow)
   - Some function signatures changed
   - Import paths may differ

### Migration Complexity

**Effort:** 🔴 **HIGH** (1-2 weeks)

**Why Hard:**
- v0.x = no stability guarantees
- 23 versions of potential breaking changes
- No consolidated migration guide
- Need to test every query

**Code Impact:**
```go
// Your current code (v0.3.2)
env, _ := cel.NewEnv(cel.Types(...))
parsed, _ := env.Parse(query)
checked, _ := env.Check(parsed)
prg, _ := env.Program(checked)

// New code (v0.26.1) - similar but subtle differences
// Function signatures, error handling, options may differ
```

### Testing Requirements

You'd need to test **all 100+ queries** from UseCases.json:
- Protein queries (30+ cases)
- Gene queries (40+ cases)
- ChEMBL queries (25+ cases)
- Taxonomy queries (10+ cases)

**Risk:** Query breakage in production

### When to Upgrade CEL-go

✅ **Upgrade if:**
- You're hitting performance limits
- You need new CEL features
- You're already refactoring query code
- You have good test coverage

❌ **Don't upgrade if:**
- Current version works fine
- Limited time for testing
- Production system is stable
- No pressing issues

### Recommendation: ⭐⭐ **Defer**

**Why:**
- Current version works
- High risk/effort for uncertain benefit
- Better to wait for v1.0 (stable API)
- Focus on higher-value improvements first

**When to revisit:**
- When CEL-go reaches v1.0
- When you build comprehensive test suite
- When refactoring query language anyway

---

## Issue 2: Database Alternatives

### Option A: MDBX-go (LMDB Fork by Erigon)

**Repository:** https://github.com/erigontech/mdbx-go

#### What is MDBX?

**MDBX (libmdbx)** is a **fork of LMDB** by Leonid Yuriev (Erigon/Ethereum client):
- Started ~2018
- Used in Erigon (Ethereum node)
- Claims to be "better than LMDB"

#### Key Improvements over LMDB

✅ **Performance**
- 10-30% faster than LMDB (official claims)
- Better write performance
- Optimized for modern SSDs

✅ **Features**
- Better error handling
- Improved compaction
- Auto-resize support (no pre-allocation!)
- Better tooling

✅ **Reliability**
- More robust error recovery
- Better corruption detection
- Active maintenance

#### Benchmarks (from libmdbx repo)

```
MDBX vs LMDB (in-memory tmpfs):
- CRUD operations: 10-20% faster
- With optimizations: up to 30% faster
- Real-world: 163.5 Kops (MDBX) vs 136.9 Kops (LMDB)
```

#### Migration Effort

**Complexity:** 🟡 **LOW-MEDIUM** (2-3 days)

**Why Easy:**
- MDBX is LMDB-compatible (mostly)
- API is very similar
- Drop-in replacement (mostly)

**Code Changes:**
```go
// Current: LMDB
import "github.com/bmatsuo/lmdb-go/lmdb"

// New: MDBX
import "github.com/erigontech/mdbx-go/mdbx"

// API is ~95% same
env, _ := mdbx.NewEnv()
env.SetMapSize(size)  // No need! MDBX auto-resizes
env.Open(path, flags, mode)
```

#### Pros & Cons

**Pros:**
- ✅ Easy migration (similar API)
- ✅ Better performance (10-30%)
- ✅ Auto-resize (no pre-allocation headaches!)
- ✅ Active development
- ✅ Used in production (Erigon)
- ✅ Stays B-tree (same architecture)

**Cons:**
- ⚠️ Smaller community than LMDB
- ⚠️ Less mature (newer project)
- ⚠️ Go bindings less polished
- ⚠️ Still single-writer limitation

#### Recommendation: ⭐⭐⭐ **Consider**

**Why:**
- Easy migration (2-3 days)
- Real performance benefits
- Auto-resize is huge win!
- Low risk

**When:**
- After more pressing improvements
- When you have 2-3 days for testing
- Before considering bigger changes

---

### Option B: Badger (Standalone, No Dgraph)

**Repository:** https://github.com/dgraph-io/badger

#### What is Badger?

**Badger** is a **pure Go LSM-tree database** built by Dgraph team:
- Can be used standalone (without Dgraph!)
- LSM-tree (like RocksDB)
- Written in Go (no cgo)
- Embedded library

#### Architecture Difference

```
LMDB/MDBX:           Badger:
B-tree               LSM-tree
Memory-mapped        SSD-optimized
Single writer        Concurrent writers
Read-optimized       Write-optimized
```

#### Performance (2017 Benchmarks)

**vs LMDB:**
- Random writes: **1.7-22x faster**
- Range iteration: **3-6x faster**
- Random reads: Similar
- Sequential reads: Slightly slower

**vs RocksDB:**
- Random reads: **3.5x faster**
- Random writes: **0.86-14x faster** (value size dependent)
- Memory usage: Lower

#### Key Features

✅ **Pure Go**
- No cgo (easier builds)
- Better cross-platform
- No C library dependencies

✅ **MVCC & Transactions**
- Multi-version concurrency control
- ACID transactions
- Serializable snapshot isolation

✅ **Write Performance**
- Much faster writes than LMDB
- Better for high-write workloads
- Handles concurrent writes

✅ **SSD Optimized**
- Designed for modern SSDs
- Separates keys from values
- Better space utilization

#### Migration Effort

**Complexity:** 🔴 **HIGH** (1-2 weeks)

**Why Hard:**
- **Different API** (not LMDB-compatible)
- Different concepts (LSM vs B-tree)
- Need to rethink data access patterns
- Requires testing entire codebase

**Code Changes:**
```go
// Current: LMDB
env, _ := lmdb.NewEnv()
txn, _ := env.BeginTxn(nil, 0)
txn.Get(dbi, key)
txn.Put(dbi, key, value, 0)
txn.Commit()

// New: Badger - VERY DIFFERENT
db, _ := badger.Open(badger.DefaultOptions(path))
err := db.View(func(txn *badger.Txn) error {
    item, _ := txn.Get(key)
    item.Value(func(val []byte) error {
        // Use val
    })
})
err = db.Update(func(txn *badger.Txn) error {
    return txn.Set(key, value)
})
```

**Key Differences:**
1. Different transaction model
2. Different value retrieval (callbacks)
3. Different error handling
4. Different configuration

#### Badger Versions

**Current:** v4.4.0 (latest)
- ⚠️ Recent CPU usage issues reported (v4.4.0)
- v4 is stable but evolving
- v3 is more mature

**Recommendation:** Use v3 for stability

#### Pros & Cons

**Pros:**
- ✅ Much faster writes (for updates!)
- ✅ Better for incremental updates
- ✅ Concurrent writers
- ✅ Pure Go (no cgo)
- ✅ Active development
- ✅ Production-proven (Dgraph uses it)

**Cons:**
- ❌ Complex migration (1-2 weeks)
- ❌ Different API paradigm
- ❌ Slightly slower reads
- ❌ More disk space (LSM overhead)
- ❌ More complex tuning
- ⚠️ Recent version (v4) has issues

#### When Badger Makes Sense

✅ **Good for:**
- Frequent updates/writes
- Incremental data updates
- High-write workloads
- When cgo is a problem

❌ **Not ideal for:**
- Read-mostly workloads (LMDB better)
- When you need simple migration
- When disk space is limited
- Limited time for migration

#### Recommendation: ⭐⭐ **Maybe Later**

**Why:**
- High migration effort
- Current LMDB works fine
- Benefit mainly for writes (you do full rebuilds anyway)
- Better to wait until you solve incremental updates

**When to Consider:**
- After implementing incremental updates
- If write performance becomes bottleneck
- If you have 2-3 weeks for migration
- After MDBX if that's not enough

---

## Database Comparison Matrix

| Feature | LMDB (Current) | MDBX-go | Badger | RocksDB |
|---------|----------------|---------|--------|---------|
| **Migration Effort** | N/A | 🟢 Low (2-3d) | 🔴 High (1-2w) | 🔴 High (2-3w) |
| **Read Performance** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| **Write Performance** | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Memory Usage** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |
| **Disk Usage** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| **Simplicity** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ |
| **Go Native** | ❌ (cgo) | ❌ (cgo) | ✅ Pure Go | ❌ (cgo) |
| **Auto-Resize** | ❌ | ✅ | ✅ | ✅ |
| **Concurrent Writes** | ❌ | ❌ | ✅ | ✅ |
| **Maturity** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Community** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **Best For** | Reads, stable | All-around | Writes, updates | Writes, scale |

---

## Prioritized TODO List (Solo Developer)

### Phase 1: Quick Wins (1-2 weeks) ⭐⭐⭐⭐⭐

**Goal:** Improve maintainability without breaking anything

#### 1. Refactor Query Parser (2-3 days)

**Priority:** 🔴 **CRITICAL**

**Why:**
- goto-based code is hard to maintain
- Blocks future query features
- Low risk, high value

**Tasks:**
```
[ ] Remove goto statements from query/query.go
[ ] Rewrite as clean recursive descent
[ ] Add unit tests for parser
[ ] Test with all UseCases.json queries
[ ] Document query syntax
```

**Files to modify:**
- `query/query.go` (~200 lines)

**Risk:** 🟢 Low (just refactoring, same behavior)

---

#### 2. Add Query Profiling/EXPLAIN (2-3 days)

**Priority:** 🔴 **HIGH**

**Why:**
- Helps debug slow queries
- Shows where time is spent
- Enables optimization

**Tasks:**
```
[ ] Add QueryStats struct
[ ] Track timing per query step
[ ] Add EXPLAIN command
[ ] Log slow queries automatically
[ ] Add performance metrics endpoint
```

**Example Output:**
```
EXPLAIN map(uniprot).filter(...).map(hgnc)

Step 1: map(uniprot)           100ms  1000 results
Step 2: filter(...)             50ms   200 results (80% filtered)
Step 3: map(hgnc)               20ms   200 results
Total: 170ms
```

**Files to modify:**
- `service/mapfilter.go` (add timing)
- `query/query.go` (add EXPLAIN)

**Risk:** 🟢 Low (additive feature)

---

#### 3. Document CEL Expressions (1-2 days)

**Priority:** 🟡 **MEDIUM**

**Why:**
- Users need reference
- Shows query capabilities
- Reduces support questions

**Tasks:**
```
[ ] Create CEL_REFERENCE.md
[ ] Document all operators
[ ] Document custom functions (overlaps, within, covers)
[ ] Add cookbook examples
[ ] Link from API docs
```

**Risk:** 🟢 None (documentation only)

---

### Phase 2: Performance (1-2 weeks) ⭐⭐⭐⭐

**Goal:** Make existing system faster

#### 4. Parallel Filter Evaluation (2-3 days)

**Priority:** 🟡 **MEDIUM**

**Why:**
- 2-4x speedup on multi-core
- No architecture change
- Relatively easy

**Tasks:**
```
[ ] Identify parallelizable filters
[ ] Add goroutine pool for evaluation
[ ] Add concurrency config (default: NumCPU)
[ ] Benchmark before/after
[ ] Test with complex queries
```

**Files to modify:**
- `service/mapfilter.go` (filter execution)

**Risk:** 🟡 Medium (need proper synchronization)

---

#### 5. Optimize Cache Strategy (1-2 days)

**Priority:** 🟡 **MEDIUM**

**Why:**
- Cache is already there
- Can tune for better hit rates
- Low risk

**Tasks:**
```
[ ] Analyze cache hit rates
[ ] Tune cache sizes
[ ] Add cache statistics
[ ] Consider multi-tier cache
[ ] Add cache warming for common queries
```

**Files to modify:**
- `service/service.go` (cache setup)

**Risk:** 🟢 Low (tuning existing system)

---

### Phase 3: Database Migration (Optional, 1 week) ⭐⭐⭐

**Goal:** Migrate to MDBX for better performance

#### 6. Migrate LMDB → MDBX (2-3 days)

**Priority:** 🟡 **OPTIONAL**

**Why:**
- 10-30% performance improvement
- Auto-resize (huge win!)
- Easy migration

**Tasks:**
```
[ ] Add mdbx-go dependency
[ ] Create database abstraction layer
[ ] Implement MDBX backend
[ ] Test with sample data
[ ] Benchmark vs LMDB
[ ] Full migration
```

**Files to modify:**
- `db/db.go` (create interface)
- `db/lmdb.go` (extract LMDB)
- `db/mdbx.go` (new MDBX implementation)

**Risk:** 🟡 Medium (database swap)

**Decision:** Only if Phase 1+2 aren't enough

---

### Phase 4: Extended Features (2-3 weeks) ⭐⭐

**Goal:** Add missing query features

#### 7. Add Query Features (3-5 days)

**Priority:** 🟢 **NICE-TO-HAVE**

**Why:**
- Users want these
- Makes queries more powerful
- Natural extension

**Tasks:**
```
[ ] Add limit() support
[ ] Add sortBy() support
[ ] Add count() aggregation
[ ] Add distinct() operator
[ ] Update parser for new syntax
```

**Example:**
```javascript
map(uniprot)
  .filter(uniprot.organism == "human")
  .sortBy(uniprot.sequence.mass, desc)
  .limit(100)
```

**Risk:** 🟡 Medium (parser changes)

---

### Phase 5: Major Changes (1-2 months) ⭐

**Goal:** Architectural improvements (if needed)

#### 8. Consider Badger Migration (2-3 weeks)

**Priority:** 🟢 **FUTURE**

**Why:**
- Better write performance
- Enables true incremental updates
- Pure Go

**Tasks:**
```
[ ] Prototype with Badger
[ ] Benchmark read/write performance
[ ] Design migration strategy
[ ] Implement database abstraction
[ ] Migrate data
[ ] Test extensively
```

**Risk:** 🔴 High (major change)

**Decision:** Only after incremental update problem is solved

---

#### 9. CEL-go Upgrade (1-2 weeks)

**Priority:** 🟢 **FUTURE**

**Why:**
- Performance improvements
- New features
- Bug fixes

**Tasks:**
```
[ ] Wait for CEL-go v1.0 (stable API)
[ ] Review all breaking changes
[ ] Build comprehensive test suite
[ ] Upgrade incrementally (v0.4 → v0.5 → ...)
[ ] Test all 100+ use cases
[ ] Update documentation
```

**Risk:** 🔴 High (breaking changes)

**Decision:** Defer until v1.0 or critical need

---

## Recommended Timeline (Solo Developer)

### Month 1: Foundation
- ✅ Week 1: Parser refactor
- ✅ Week 2: Query profiling
- ✅ Week 3: Parallel filters
- ✅ Week 4: Documentation

**Output:** Better maintainability, faster queries

### Month 2: Performance (Optional)
- ⚠️ Week 1: MDBX evaluation
- ⚠️ Week 2: MDBX migration
- ⚠️ Week 3: Testing & optimization
- ⚠️ Week 4: Buffer for issues

**Output:** 10-30% performance boost

### Month 3+: Advanced (If Needed)
- Extended query features
- Consider Badger (if write performance critical)
- CEL-go upgrade (if v1.0 released)

---

## Decision Matrix

### What to Do Now?

| Change | Effort | Benefit | Risk | Priority |
|--------|--------|---------|------|----------|
| **Parser Refactor** | 2-3d | High | Low | ⭐⭐⭐⭐⭐ DO NOW |
| **Query Profiling** | 2-3d | High | Low | ⭐⭐⭐⭐⭐ DO NOW |
| **Documentation** | 1-2d | Medium | None | ⭐⭐⭐⭐ DO SOON |
| **Parallel Filters** | 2-3d | Medium | Medium | ⭐⭐⭐ CONSIDER |
| **MDBX Migration** | 2-3d | Medium | Medium | ⭐⭐⭐ CONSIDER |
| **Badger Migration** | 2-3w | High | High | ⭐⭐ LATER |
| **CEL-go Upgrade** | 1-2w | Medium | High | ⭐ DEFER |

---

## My Recommendation for You

### Immediate (Next 2 Weeks)

1. ✅ **Refactor Parser** - This is blocking future improvements
2. ✅ **Add Query Profiling** - Essential for optimization
3. ✅ **Document CEL** - Helps users and yourself

**Time:** ~1 week actual work
**Benefit:** Much better codebase
**Risk:** Minimal

### Soon (Next Month)

4. ⚠️ **Parallel Filters** - Good perf boost, reasonable effort
5. ⚠️ **Test MDBX** - Easy to try, good potential benefit

**Time:** ~1 week
**Benefit:** 2-4x filter speed, 10-30% overall
**Risk:** Low

### Later (When Needed)

6. ⏸️ **Badger** - Only if you solve incremental updates first
7. ⏸️ **CEL-go** - Wait for v1.0 or critical need

---

## Summary

### Your Current Stack is OK!

**Keep:**
- ✅ LMDB (works fine for read-heavy)
- ✅ CEL-go v0.3.2 (stable, works)
- ✅ map().filter() syntax (good design)

**Improve (Low-Hanging Fruit):**
- 🔧 Parser quality
- 🔧 Observability (profiling)
- 🔧 Documentation

**Consider Later:**
- 🤔 MDBX (easy upgrade, nice benefits)
- 🤔 Parallel evaluation (good perf boost)

**Defer:**
- ⏸️ CEL-go upgrade (risky, low value)
- ⏸️ Badger (only for incremental updates)

### The 80/20 Rule

**20% effort (parser + profiling + docs)** will give you **80% of maintainability benefits**

**Skip the big migrations** unless you have a specific problem they solve.

---

## Conclusion

As a solo developer, focus on:

1. **Code quality** (parser refactor)
2. **Observability** (profiling)
3. **Documentation** (CEL reference)

These give **maximum benefit** for **minimum risk**.

**Avoid:**
- Big migrations without clear need
- Upgrades for the sake of "being current"
- Solving problems you don't have

**Remember:** Your current stack works. Make it better, don't replace it (yet).

---

**Last Updated:** October 14, 2025
**Next Review:** After Phase 1 completion
**Contact:** BioYoda Development Team
