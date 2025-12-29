# PubChem Dataset

## Overview

PubChem is the world's largest open chemical database (119M+ compounds). This integration focuses on a biotech-relevant subset (~10M compounds) selected from multiple priority sources: FDA-approved drugs, literature-referenced compounds, patent-associated compounds, bioassay-tested compounds, and biologics.

**Source**: NIH National Library of Medicine - https://pubchem.ncbi.nlm.nih.gov/
**Data Type**: Chemical compound structures, properties, synonyms, and cross-references

## Integration Architecture

### Priority Sources (Phase 1)
- **P0**: FDA-approved drugs (~3K) - from Drug-Names.tsv.gz
- **P1**: Literature-referenced (~8M) - from CID-PMID.gz (creates xrefs to PubMed)
- **P2**: Patent-associated (~6M) - from CID-Patent.gz filtered by SureChEMBL (creates xrefs)
- **P3**: Bioassay-tested (~variable) - from bioactivities.tsv.gz
- **P4**: Biologics (~2.5M) - peptides, proteins, nucleotides from CID-Biologics.tsv.gz

### Storage Model
**Primary Entries**: PubChem CID (e.g., "2244" for Aspirin)
**Searchable Text Links**: InChI Key, Synonyms (all)
**Attributes Stored** (PubchemAttr protobuf):
- Core: CID, InChI Key, SMILES, IUPAC Name, Synonyms
- Molecular: Formula, Weight, Exact Mass, XLogP, HBD, HBA, Rotatable Bonds, TPSA
- Classification: Compound Type, FDA Approved, Has Literature, Has Patents
- Medical: MeSH Terms, Pharmacological Actions

**Cross-References**:
- Xrefs created: PubMed (PMIDs), Patents, MeSH, ChEBI, HMDB, ChEMBL
- Bidirectional links to ChEBI, HMDB, ChEMBL

### Special Features
- **Biotech Filtering**: Only processes ~10M biotech-relevant compounds from 119M+ total
- **Synonym Search**: All synonyms indexed for text search (not just top 20)
- **MeSH Integration**: Links to MeSH descriptors via keyword lookup
- **Pharmacological Actions**: Derived from MeSH term mappings
- **Parallel SDF Processing**: Configurable worker count via `--pubchem-sdf-workers`

## Use Cases

**1. Drug Discovery - Find Compounds by Synonym**
```
Query: "Aspirin" → Text search → PubChem CID 2244
Use: Identify compound by any known name, trade name, or synonym
```

**2. Chemical Property Analysis**
```
Query: CID → Molecular properties → Lipinski Rule of Five check
Use: Evaluate drug-likeness for pharmaceutical development
```

**3. Literature Mining**
```
Query: CID → PMID xrefs → PubMed publications
Use: Find all publications referencing a compound
```

**4. Patent Landscape Analysis**
```
Query: CID → Patent xrefs → Patent documents
Use: Identify IP coverage for compound of interest
```

**5. MeSH-based Classification**
```
Query: CID → MeSH terms → Pharmacological actions
Use: Understand drug mechanism and therapeutic category
```

**6. Cross-Database Navigation**
```
Query: CID → ChEMBL/ChEBI/HMDB xrefs → Related databases
Use: Integrate compound data across multiple resources
```

## Test Cases

**Current Tests** (8 total):
- Declarative: ID lookup, attribute validation, invalid ID handling
- Custom: FDA flag validation, molecular properties, Lipinski properties, synonym presence, ChEMBL xrefs, ChEBI xrefs, SMILES text search, InChI Key text search

**Coverage**:
- Core identifiers (CID, InChI Key, SMILES)
- Molecular properties (formula, weight, XLogP, HBD/HBA)
- Drug classification flags
- Cross-references (when target datasets present)
- Text search functionality

**Recommended Additions**:
- Synonym text search validation
- MeSH term presence and linking
- Pharmacological actions validation
- PMID and Patent xref counts

## Performance

- **Test Build**: ~30-60s (20 compounds from FDA drugs)
- **Full Build**: Several hours (10M+ compounds, 355 SDF files)
- **Data Sources**: PubChem FTP (SDF files ~200GB total)
- **Memory Optimization**:
  - PMIDs/Patents as xrefs (disk) instead of memory maps
  - Biotech CID filtering via in-memory map (~200-300MB)
  - Parallel SDF processing with configurable workers

## Known Limitations

- **SDF Processing**: Large files (355 x 500K compounds each)
- **MeSH Linking**: Uses keyword lookup (term names, not descriptor UIDs)
- **Creation Date/Parent CID**: Disabled to save memory (optional features)
- **Synonym Limits**: No limit applied (biobtree philosophy: all or nothing)

## Future Work

- Add test validation for synonym text search
- Add MeSH term and pharmacological action tests
- Consider adding compound structure similarity search
- Add bioassay activity linking tests (with pubchem_activity dataset)

## Maintenance

- **Release Schedule**: PubChem updates weekly
- **Data Format**: SDF files + TSV mapping files (gzipped)
- **Test Data**: 20 FDA-approved drug CIDs
- **License**: Public domain (NIH)

## References

- **Citation**: Kim S, et al. (2023) PubChem 2023 update. Nucleic Acids Res.
- **Website**: https://pubchem.ncbi.nlm.nih.gov/
- **FTP**: ftp://ftp.ncbi.nlm.nih.gov/pubchem/
