# PubChem Activity Dataset

## Overview

PubChem BioActivity contains bioassay activity measurements linking compounds to biological targets. This dataset provides IC50, EC50, Ki, and other activity values from PubChem BioAssays, connecting compounds to proteins, genes, and assays.

**Source**: NIH National Library of Medicine - https://pubchem.ncbi.nlm.nih.gov/
**Data Type**: Bioactivity measurements with target annotations

## Integration Architecture

### Storage Model
**Primary Entries**: Activity ID (format: `CID_AID_index`, e.g., "2244_12345_1")
**Searchable Text Links**: None (linked via xrefs)
**Attributes Stored** (PubchemActivityAttr protobuf):
- Identifiers: ActivityId, CID (compound), AID (assay)
- Activity Data: Outcome, Type (IC50/EC50/Ki/etc), Qualifier, Value, Unit
- Target Info: Protein Accession, Gene ID, Target Taxonomy ID
- Literature: PMID

**Cross-References**:
- Activity → PubChem Compound (CID)
- Activity → PubChem Assay (AID)
- Activity → UniProt (protein accession, when UniProt format)
- Activity → PDB (protein accession, when PDB format)
- Activity → Ensembl Gene (via Entrez Gene ID lookup)

### Special Features
- **Streaming Processing**: No memory accumulation - entries streamed directly to database
- **Retry Mechanism**: Configurable retries for network/corruption errors (`pubchemRetryCount`, `pubchemRetryWaitMinutes`)
- **Target Resolution**: Automatically detects UniProt vs PDB protein accession format
- **Gene Mapping**: Links to Ensembl genes via Entrez Gene ID lookup
- **Unique Activity IDs**: CID_AID_index format handles multiple measurements per compound-assay pair

## Use Cases

**1. Target-Based Drug Discovery**
```
Query: UniProt ID → Activity xrefs → Active compounds with IC50 values
Use: Find compounds active against a specific protein target
```

**2. Compound Activity Profile**
```
Query: PubChem CID → Activity xrefs → All bioassay results
Use: Understand complete activity profile of a compound
```

**3. Assay Analysis**
```
Query: Assay AID → Activity xrefs → All tested compounds and results
Use: Analyze screening results from a specific bioassay
```

**4. Gene-Compound Relationships**
```
Query: Ensembl Gene → Activity xrefs → Compounds affecting gene product
Use: Find chemical modulators of a gene of interest
```

**5. Structure-Activity Relationships**
```
Query: Activity Type (IC50) + Target → Related activities
Use: Compare potency across compound series
```

**6. Literature-Linked Activities**
```
Query: Activity → PMID → Publication details
Use: Find primary literature source for activity data
```

## Test Cases

**Current Tests** (0 total):
- No tests implemented yet

**Coverage**:
- Not yet tested

**Recommended Tests**:
- Activity ID lookup and attribute validation
- Compound (CID) cross-reference verification
- Assay (AID) cross-reference verification
- UniProt protein cross-reference (when present)
- PDB cross-reference (when present)
- Ensembl gene cross-reference via Entrez lookup
- Activity value and unit parsing
- Activity outcome validation (Active, Inactive, etc.)

## Performance

- **Test Build**: TBD (depends on test limit)
- **Full Build**: Several hours (3GB compressed, ~300M+ activity records)
- **Data Source**: bioactivities.tsv.gz from PubChem Bioassay/Extras
- **Memory Usage**: Minimal - streaming architecture, no accumulation
- **Retry Config**: 2 retries default, 2 minute wait between attempts

## Known Limitations

- **Large Dataset**: 3GB compressed, requires significant processing time
- **Network Sensitivity**: FTP download can fail - retry mechanism mitigates
- **Gene Mapping**: Requires Entrez Gene dataset for Ensembl linking
- **Protein Format Detection**: Heuristic-based (UniProt: letter+digit, PDB: digit+alphanum)
- **No Direct Text Search**: Activities found via compound/target xrefs

## Future Work

- Implement test suite with declarative and custom tests
- Add activity type filtering tests (IC50, EC50, Ki)
- Validate protein target cross-references
- Test gene-to-activity relationship via Entrez
- Add activity value range validation
- Consider activity aggregation by target

## Maintenance

- **Release Schedule**: PubChem updates weekly
- **Data Format**: TSV (tab-delimited), gzipped
- **Test Data**: TBD
- **License**: Public domain (NIH)
- **Dependencies**: Entrez Gene dataset for gene linking

## References

- **Citation**: Kim S, et al. (2023) PubChem 2023 update. Nucleic Acids Res.
- **Website**: https://pubchem.ncbi.nlm.nih.gov/
- **BioAssay Data**: https://ftp.ncbi.nlm.nih.gov/pubchem/Bioassay/Extras/
