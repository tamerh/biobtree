# Patent Database (SureChEMBL)

## Overview

The Patent database integrates patent information from SureChEMBL, providing access to chemical compounds, sequences, and bibliographic data extracted from patent documents. Contains millions of patents worldwide with extracted chemical structures, classification codes, assignees, and linkages to chemical compounds. Essential for intellectual property analysis, drug discovery, competitive intelligence, and tracking innovation trends in pharmaceutical and biotech industries.

**Source**: SureChEMBL / European Bioinformatics Institute (EBI)
**Data Type**: Patent metadata with chemical compound extraction
**Coverage**: Worldwide patent documents (US, EP, WO, etc.)

## Integration Architecture

### Storage Model

**Primary Entries**:
- Patent IDs (e.g., `US-5153197-A`) serve as primary keys
- Format: `{Country}-{Number}-{Kind Code}`
- Examples: US-5153197-A (US patent), EP-1234567-B1 (European patent)

**Searchable Attributes**:
- Patent IDs indexed as identifiers for direct lookup
- Title, country, publication date stored as attributes
- Patent family IDs for grouping related patents

**Attributes Stored** (protobuf PatentAttr):
- `title`: Patent title/invention description
- `country`: Filing country code (US, EP, WO, etc.)
- `publication_date`: Date patent was published (YYYY-MM-DD)
- `family_id`: Patent family identifier (groups related patents)
- `cpc`: Cooperative Patent Classification codes (array)
- `ipcr`: International Patent Classification Revised codes (array)
- `ipc`: International Patent Classification codes (array)
- `ecla`: European Classification codes (array)
- `asignee`: Assignees/patent holders (array, normalized)

**Cross-References**:
- **Patent Families**: Groups of related patents (same invention, different jurisdictions)
- **Patent Compounds**: Chemical structures extracted from patents
- **Classification Codes**: IPC, CPC hierarchical technology classifications
- **Assignees**: Companies, universities, inventors (name-normalized)

### Related Datasets

**Patent Compound (dataset 352)**:
- Individual chemical structures extracted from patents
- Includes InChI Keys and SMILES representations
- Links to ChEMBL molecules via InChI Key matching
- Bidirectional cross-references: Patent ↔ Compound

**Patent Family (dataset 353)**:
- Groups patents protecting the same invention across jurisdictions
- Enables tracking of global patent portfolio
- Family ID links all related patent documents

### Special Features

**Company Name Normalization**:
- Intelligent assignee name normalization (see `src/update/patents.go:402-451`)
- Removes country codes: "(US)", "(GB)", "(DE)", etc.
- Strips legal suffixes: "Inc.", "Ltd.", "LLC", "GmbH", "AG", "SA", etc.
- Handles abbreviations: "E. I." → "EI"
- Uppercases for consistency
- Example: "E. I. DU PONT DE NEMOURS AND COMPANY (US)" → "EI DU PONT DE NEMOURS"

**Classification Code Coverage**:
- **IPC (International Patent Classification)**: Technology-based hierarchical system
- **CPC (Cooperative Patent Classification)**: Joint USPTO/EPO system, more granular
- **IPCR (IPC Revised)**: Updated IPC version
- **ECLA (European Classification)**: EPO-specific system
- Multiple codes per patent enable technology landscape analysis

**Chemical Structure Extraction**:
- Automated extraction of compounds from patent images and text
- InChI Key standardization for structure matching
- SMILES notation for chemical representation
- Links to ChEMBL via shared InChI Keys (when ChEMBL dataset present)

**Patent Family Grouping**:
- Single invention can have 10+ family members across jurisdictions
- Tracks priority dates and geographic coverage
- Essential for freedom-to-operate analyses

## Use Cases

**1. Drug Discovery**
```
Query: Compound InChI Key → Patents → View related patents and assignees
Use: Identify IP landscape around lead compounds
```

**2. Competitive Intelligence**
```
Query: Company name (assignee) → All patents → Analyze technology areas via CPC codes
Use: Track competitor R&D focus and patent strategy
```

**3. Freedom-to-Operate Analysis**
```
Query: Technology area (CPC code) → Active patents → Check assignees and dates
Use: Assess patent risks before product launch
```

**4. Patent Family Analysis**
```
Query: Patent ID → Patent family → All family members
Use: Understand global protection strategy for invention
```

**5. Technology Landscaping**
```
Query: Classification code (e.g., "C07D") → All patents → Temporal analysis
Use: Track innovation trends in pharmaceutical chemistry
```

**6. Prior Art Search**
```
Query: Compound structure (SMILES/InChI) → Patents mentioning compound
Use: Identify prior art for patentability assessment
```

**7. Licensing Opportunities**
```
Query: University assignee → Patents → Publication dates and status
Use: Identify technologies available for licensing
```

## Test Cases

**Current Tests** (10 tests):
- 4 declarative tests (JSON-based)
- 6 custom tests (Python logic)

**Coverage**:
- ✅ Patent ID lookup and validation
- ✅ Attribute presence (title, country, date, family_id, etc.)
- ✅ Multiple patent batch lookup
- ✅ Invalid ID handling
- ✅ Title extraction
- ✅ Country and publication date
- ✅ Patent family ID in attributes
- ✅ CPC classification codes
- ✅ IPC classification codes
- ✅ Assignee information

**Note on Cross-Reference Tests**:
Patent compound cross-references (dataset 352) are validated in full builds but may not be fully accessible via web API in test mode. The CLI query command shows that cross-references exist (44 compound entries + 1 family entry for test patent US-5153197-A).

**Recommended Additions**:
- Patent family member count validation
- Publication date range checks (valid dates)
- Assignee normalization verification
- Multi-jurisdiction family coverage
- Classification code hierarchy validation
- Compound extraction completeness

## Performance

- **Test Build**: ~5-8s (20 patents)
- **Data Source**: SureChEMBL bulk download (JSON format)
- **Full Build**: Hours (millions of patents)
- **Test Data**: ~20 patents spanning multiple years and assignees
- **Database Size**: Large (patents + compounds + mappings)
- **Update Frequency**: Weekly/monthly (SureChEMBL releases)

## Known Limitations

**Classification Code Storage**:
- IPC, CPC, IPCR, ECLA stored as attributes only (not separate cross-reference entities)
- Not directly searchable by code → patent (requires full-text or specialized indexing)
- For code-based search, would need text link index

**Assignee Matching**:
- Name normalization is heuristic-based
- Same company may have variations despite normalization
- Acquisitions/mergers not tracked
- Subsidiary relationships not captured

**Chemical Structure Coverage**:
- Not all patents have extractable compounds
- Image-based structures may have OCR errors
- Markush structures (generic formulas) not fully captured
- Biological sequences tracked separately

**Patent Status**:
- No legal status tracking (granted, abandoned, expired)
- No litigation or opposition data
- Publication date ≠ grant date
- Requires external validation for FTO analysis

**Data Completeness**:
- Older patents may lack chemical extraction
- Some jurisdictions better covered than others
- Patent families may be incomplete for recent filings

## Future Work

- Add legal status tracking (granted/abandoned/expired)
- Patent citation network analysis
- Inventor tracking (separate from assignees)
- Link to PubMed for patent-publication connections
- Sequence data integration (protein/DNA from patents)
- Patent landscape visualization
- Technology evolution tracking via classification codes
- Geographic heat maps of patent activity
- Assignee relationship graph (subsidiaries, collaborations)

## Maintenance

- **Release Schedule**: Periodic updates from SureChEMBL
- **Current Patents**: Millions of documents
- **Data Format**: JSON (patents.json, compounds.json, mapping.json)
- **Test Data**: 20 representative patents with diverse attributes
- **License**: SureChEMBL data is freely available
- **Documentation**: https://www.surechembl.org/

## References

- **Citation**: Papadatos G, et al. (2016) SureChEMBL: a large-scale, chemically annotated patent document database. Nucleic Acids Res. 44(D1):D1220-D1228.
- **Website**: https://www.surechembl.org/
- **Data Download**: ftp://ftp.ebi.ac.uk/pub/databases/chembl/SureChEMBL/
- **IPC Classification**: https://www.wipo.int/classifications/ipc/
- **CPC Classification**: https://www.cooperativepatentclassification.org/
- **License**: Open data (SureChEMBL)
