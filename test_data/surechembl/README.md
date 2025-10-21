# SureChEMBL Test Data

**Created**: October 20, 2025
**Source**: `/test_out/raw_data/patents/surechembl/2025-10-01/`
**Sample Size**: 10 patents, 50 compounds, 100 mappings

---

## Files

### Human-Readable Formats
- **summary.json** - Quick overview with examples
- **patents_sample.json** - All 10 patents in JSON
- **compounds_sample.json** - All 50 compounds in JSON
- **mapping_sample.json** - All 100 mappings in JSON

### Machine-Readable Formats (Parquet)
For Go/Python processing:

### 1. patents_sample.parquet (12 KB)
**10 patent records**

**Columns**:
- `id` (int) - Internal patent ID (1-10)
- `patent_number` (string) - Patent number (e.g., "US-5153197-A")
- `country` (string) - Patent office country code (US, EP, WO, etc.)
- `publication_date` (string) - Publication date (YYYY-MM-DD)
- `family_id` (string) - Patent family identifier
- `cpc` (list of strings) - CPC classification codes
- `ipcr` (list of strings) - IPCR classification codes
- `ipc` (list of strings) - IPC classification codes
- `ecla` (list of strings) - ECLA classification codes
- `asignee` (list of strings) - Assignees/companies
- `title` (string) - Patent title

**Sample Patents**:
1. US-5153197-A: "Treatment of hypertension with angiotensin II blocking imidazoles" (E. I. DU PONT)
2. US-5360800-A: "Tetrahydro-1H-pyrido[4,3-b]indol-1-one derivatives" (GLAXO GROUP)
3. US-4650884-A: "Citalopram diol intermediate" (H. LUNDBECK A/S)

### 2. compounds_sample.parquet (7.8 KB)
**50 compound records**

**Columns**:
- `id` (int) - Internal compound ID
- `smiles` (string) - SMILES chemical structure
- `inchi` (string) - InChI string
- `inchi_key` (string) - InChI Key for lookups
- `mol_weight` (float) - Molecular weight

**Sample Compounds**:
- ID 1: `Cc1ccccc1` (Toluene, MW: 92.14)
- ID 2: `NC(=O)O` (Carbamic acid, MW: 61.04)
- ID 3: `OCCO` (Ethylene glycol, MW: 62.07)

### 3. mapping_sample.parquet (2.8 KB)
**100 patent-compound relationships**

**Columns**:
- `patent_id` (int) - Foreign key to patents.id
- `compound_id` (int) - Foreign key to compounds.id
- `field_id` (int) - Field where compound was found (1=title, 2=claims, etc.)

**Relationship**:
- Patent 1 contains compounds: 1, 2, 3, 4, 5, 6, 7, 8, ...
- Patent 2 contains compounds: ...
- etc.

---

## Cross-References to Implement

Based on this data structure:

### Priority 1: Core Xrefs
1. **Patent ↔ Compound** (bidirectional)
   - `patent.id` ↔ `mapping.patent_id` ↔ `mapping.compound_id` ↔ `compound.id`
   - Query: "Find compounds in patent 1" or "Find patents containing compound 5"

2. **InChI Key → Compound** (structure lookup)
   - `compound.inchi_key` → `compound.id`
   - Query: "Find compound by structure"

### Priority 2: Patent Metadata
3. **Patent ↔ Family** (bidirectional)
   - `patent.id` ↔ `patent.family_id`
   - Query: "Find all patents in same family"

4. **Patent ↔ IPC Code** (bidirectional)
   - `patent.id` ↔ `patent.ipc[*]`
   - Query: "Find patents in classification A61K31"

5. **Patent ↔ Assignee** (bidirectional)
   - `patent.id` ↔ `patent.asignee[*]`
   - Query: "Find all patents by PFIZER"

---

## Notes

### Column Name Discrepancy
- **Patents table** uses `id` (not `patent_id`)
- **Compounds table** uses `id` (not `surechembl_id`)
- **Mapping table** uses `patent_id` and `compound_id` (foreign keys)

This is important for the Go implementation!

### Missing Fields
Compared to production SureChEMBL, this test data does NOT have:
- SureChEMBL IDs (e.g., "SCHEMBL123456") - only internal integer IDs
- Full patent text (abstracts, claims, descriptions)
- Inventor names
- Citations

### List Fields
Several fields are arrays/lists:
- `cpc`, `ipcr`, `ipc`, `ecla` - classification codes
- `asignee` - multiple assignees possible

These will need special handling in Go (iterate over arrays).

---

## Usage

```bash
# View sample data
conda run -n bioyoda python << 'EOF'
import pandas as pd

patents = pd.read_parquet('biobtreev2/test_data/surechembl/patents_sample.parquet')
print(patents[['id', 'country', 'patent_number', 'title']].to_string())
EOF
```

---

## Next Steps

1. ✅ Test data created
2. ⏭️ Document expected xrefs for specific examples
3. ⏭️ Implement `biobtreev2/update/patents.go`
4. ⏭️ Test with this small dataset
5. ⏭️ Run on full production data

---

## Location

**Test Data**: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/biobtreev2/test_data/surechembl/`

**Full Data**: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/test_out/raw_data/patents/surechembl/2025-10-01/`
