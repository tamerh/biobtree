# CellPhoneDB Dataset

## Overview

CellPhoneDB is a publicly available repository of curated receptors, ligands, and their interactions. It focuses on cell-cell communication and contains ~2,911 manually curated ligand-receptor interactions, making it essential for single-cell transcriptomics analyses of cellular crosstalk.

**Source**: Vento-Tormo Lab - https://www.cellphonedb.org/
**Data Type**: Ligand-receptor and cell-cell communication interactions

## Integration Architecture

### Storage Model

**Primary Entries**: CellPhoneDB interaction IDs (e.g., `CPI-SC0A2DB962D`)
- Each interaction stored as a separate entry with full metadata
- ID format: CPI-SC* (single protein) or CPI-SS* (complex-involving)

**Searchable Text Links**: Gene symbols indexed for text search
- Partner A gene symbols (genes_a)
- Partner B gene symbols (genes_b)
- All gene symbols from complexes are expanded and indexed

**Attributes Stored** (CellphonedbAttr protobuf):
- `partner_a`, `partner_b` - Partner names (UniProt ID or complex name)
- `directionality` - "Ligand-Receptor", "Adhesion-Adhesion", etc.
- `classification` - Signaling type (e.g., "Signaling by WNT")
- `source` - Evidence source (PMID, UniProt, Reactome)
- `is_ppi` - Protein-protein interaction flag
- `receptor_a`, `receptor_b` - Receptor flags
- `secreted_a`, `secreted_b` - Secreted protein flags
- `is_complex_a`, `is_complex_b` - Complex flags
- `genes_a`, `genes_b` - Gene symbols (repeated strings)
- `is_integrin` - Integrin involvement flag

**Cross-References**:
- UniProt (~1,352 protein accessions)
- Ensembl (~1,545 gene IDs)
- HGNC (~1,354 gene symbols via text search)
- PubMed (~127 literature references)

### Special Features

- **Ligand-receptor directionality**: Track which partner is ligand vs receptor
- **Complex support**: Multi-protein complexes with all subunits indexed
- **Secreted/receptor classification**: Filter by secreted proteins or receptors
- **Integrin interactions**: Flag for integrin-involving interactions
- **Classification filtering**: Filter by signaling pathway classification

## Use Cases

**1. Cell-Cell Communication Analysis**
```
Query: What receptors interact with my ligand of interest?
Flow: Gene symbol (text search) -> CellPhoneDB interactions
Use: Identify potential cell-cell communication partners in scRNA-seq
```

**2. Receptor-Ligand Mapping**
```
Query: Map genes to their ligand-receptor interactions
Flow: Gene -> HGNC -> CellPhoneDB
Use: Annotate cell type interactions in spatial transcriptomics
```

**3. Signaling Pathway Analysis**
```
Query: Find all WNT signaling interactions
Flow: CellPhoneDB[classification~"WNT"]
Use: Focus on specific signaling pathways
```

**4. Secretome Analysis**
```
Query: Find interactions involving secreted proteins
Flow: CellPhoneDB[secreted_a==true || secreted_b==true]
Use: Identify potential paracrine signaling
```

**5. Drug Target Communication**
```
Query: How does my drug target communicate with other cells?
Flow: Drug -> UniProt -> CellPhoneDB -> interacting partners
Use: Understand drug effects on cellular communication
```

**6. Integrin Interactions**
```
Query: Find all integrin-mediated cell adhesion interactions
Flow: CellPhoneDB[is_integrin==true]
Use: Study cell adhesion and extracellular matrix interactions
```

## Test Cases

**Current Tests** (18 total):
- 7 declarative JSON tests
- 11 custom Python tests

**Coverage**:
- Interaction ID lookup
- Gene symbol search (partner A and B)
- Attribute presence verification
- Multi-entry lookup
- Case-insensitive search
- Invalid ID handling
- Ligand-Receptor directionality
- Adhesion-Adhesion directionality
- Complex interactions
- Integrin interactions
- Receptor/secreted partner detection
- UniProt cross-references
- Ensembl cross-references
- Classification (signaling) filtering

## Performance

- **Test Build**: ~1-2s (100 entries)
- **Data Source**: Local CSV files from CellPhoneDB GitHub
- **Update Frequency**: CellPhoneDB updates periodically
- **Total Entries**: ~2,911 interactions
- **Special notes**: Processes 4 CSV files (interactions, multidata, genes, complex_composition)

## Known Limitations

- **Gene coverage**: Not all partners have gene symbols (complexes may have partial coverage)
- **Classification field**: Not all interactions have classification annotations
- **Source citations**: Some interactions lack PubMed references
- **Complex expansion**: Complex subunits are all linked, but complex-level metadata is primary

## Data Files

- `interaction_table.csv` - Core interactions (2,911 entries)
- `multidata_table.csv` - Proteins and complexes metadata (1,717 entries)
- `gene_table.csv` - Gene annotations with Ensembl/HGNC (1,545 entries)
- `complex_composition_table.csv` - Complex subunit mappings (708 entries)

## Maintenance

- **Release Schedule**: CellPhoneDB updated periodically (check GitHub)
- **Data Format**: CSV files from official repository
- **Test Data**: 100 entries (configurable via test_entries_count)
- **License**: MIT License

## References

- **Citation**: Efremova M, Vento-Tormo M, Teichmann SA, Vento-Tormo R. CellPhoneDB: inferring cell-cell communication from combined expression of multi-subunit ligand-receptor complexes. Nat Protoc. 2020
- **Website**: https://www.cellphonedb.org/
- **GitHub**: https://github.com/ventolab/cellphonedb-data
- **License**: MIT
