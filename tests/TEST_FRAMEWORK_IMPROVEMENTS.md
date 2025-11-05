# Test Framework Future Improvements

**Status:** Priorities 1 & 2 completed (helper methods + XrefExistsTest fix)

---

## 🟡 Priority 3: Add Skip Status Support (After 5-10 Datasets)

Currently tests return `(bool, str)` which doesn't distinguish between test failure vs test skip.

Add `TestStatus.SKIP` support so test summary can show: X passed, Y failed, Z skipped.

---

## 🟢 Priority 4: API Response Enhancement (Backend Change)

Add semantic fields to API responses:
- `dataset_name` field alongside numeric `dataset` ID
- `xrefs` as alias for `entries` field

Makes responses self-documenting. Requires backend changes.

---

## 🟢 Priority 5: Dataset Configuration File

Centralized dataset configuration accessible from both Go and Python. Low priority.
