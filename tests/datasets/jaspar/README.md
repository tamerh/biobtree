# JASPAR Dataset

## Overview
JASPAR is an open-access database of transcription factor (TF) binding profiles stored as position frequency matrices (PFMs). It provides curated, non-redundant TF binding profiles for six taxonomic groups.

**Source**: https://jaspar.elixir.no/
**Data Type**: TF binding profile metadata
**License**: CC BY 4.0

## Integration Architecture

### Storage Model
**Primary Entries**: matrix_id format (e.g., "MA0004.1", "UN0875.1")
**Searchable Text Links**: TF name/gene symbol indexed for text search
**Attributes Stored**: matrix_id, name, collection, class, family, tax_group, type, species, version
**Cross-References**: Links to UniProt, PubMed, and Taxonomy

### Collections
- **CORE**: ~4,572 curated, non-redundant profiles
- **UNVALIDATED**: ~1,363 profiles lacking orthogonal validation

### Special Features
- **Heterodimer handling**: Names like "MAX::MYC" are split to index both symbols
- **Multiple UniProt IDs**: Field may contain "P30561::P53762" - split and create xref for each
- **Taxonomic groups**: vertebrates, plants, insects, fungi, nematodes, urochordates

## Use Cases

**1. Find TF Binding Profile**
```
Query: MA0004.1 → Returns Arnt binding profile
Use: Look up specific TF motif metadata
```

**2. Search by TF Name**
```
Query: RUNX1 >> jaspar → Returns MA0002.1
Use: Find binding profiles by gene symbol
```

**3. Map to Protein**
```
Query: MA0004.1 >> jaspar >> uniprot → P53762
Use: Link binding profile to UniProt protein entry
```

**4. Filter by Collection**
```
Query: MYC >> jaspar[jaspar.collection=="CORE"]
Use: Find only curated, validated profiles
```

**5. Filter by Taxonomic Group**
```
Query: TP53 >> jaspar[jaspar.tax_group=="vertebrates"]
Use: Find vertebrate-specific binding profiles
```

**6. Filter by Experiment Type**
```
Query: BRCA1 >> jaspar[jaspar.type=="ChIP-seq"]
Use: Find profiles from specific experimental methods
```

## Test Cases

**Current Tests**:
- ID lookup (matrix_id)
- TF name search (gene symbol)
- Attributes verification
- Multi-lookup
- Case-insensitive search
- Invalid ID handling

**Coverage**:
- Matrix ID format (MA/UN prefix)
- Gene symbol text search
- Collection filtering (CORE/UNVALIDATED)
- Taxonomic group filtering
- UniProt cross-references
- PubMed cross-references
- Taxonomy cross-references

## Performance

- **Data Source**: TSV metadata files from JASPAR
- **Update Frequency**: Annual releases (JASPAR 2024, 2026, etc.)
- **Total Entries**: ~5,935 profiles (CORE + UNVALIDATED)
- **Processing**: Simple TSV parsing, fast

## Known Limitations

- **No PFM data**: Only metadata stored; actual position frequency matrices not included
- **Gene symbol based**: Cross-references depend on gene symbol matching
- **Multi-species**: Some profiles represent multiple species

## Future Work

- Add base_id indexing for version-agnostic searches
- Support Deep Learning collection
- Add TFFM (Transcription Factor Flexible Model) integration

## Maintenance

- **Release Schedule**: Annual JASPAR releases
- **Data Format**: Tab-separated values (TSV)
- **Test Data**: 200 entries

## References

- **Publication**: https://pubmed.ncbi.nlm.nih.gov/37962376/
- **Website**: https://jaspar.elixir.no/
- **License**: CC BY 4.0
