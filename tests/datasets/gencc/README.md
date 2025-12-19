# GenCC Dataset

## Overview
GenCC (Gene Curation Coalition) provides standardized gene-disease validity classifications from multiple authoritative sources. Each entry represents a curated assertion about the relationship between a gene and a disease, including the evidence classification level.

**Source**: https://thegencc.org
**Data Type**: Gene-disease validity curations with classifications

## Integration Architecture

### Storage Model
**Primary Entries**: UUID-based entries (e.g., GENCC_000106-HGNC_1100-OMIM_604370-HP_0000006-GENCC_100002)
**Searchable Text Links**: Gene symbol, disease title, classification title
**Attributes Stored**: UUID, gene curie/symbol, disease curie/title, classification, mode of inheritance, submitter info, PMIDs
**Cross-References**: Links to HGNC (genes), MONDO/OMIM/Orphanet (diseases), HPO (inheritance mode), PubMed (citations)

### Special Features
- Multiple classification levels (Definitive, Strong, Moderate, Limited, Supportive)
- Mode of inheritance tracking (autosomal dominant/recessive, X-linked, etc.)
- Multi-source curation (Ambry, ClinGen, Genomics England, Orphanet, etc.)
- Evidence tracking via PubMed references

## Use Cases

**1. Gene-Disease Validity**
```
Query: Is BRCA1 associated with breast cancer? → Search BRCA1 in GenCC → Classification levels from multiple curators
Use: Clinical interpretation of variants
```

**2. Disease Gene Discovery**
```
Query: What genes cause Fanconi anemia? → Search disease in GenCC → Gene list with classifications
Use: Diagnostic panel design
```

**3. Evidence Assessment**
```
Query: How strong is gene-disease evidence? → Check classification_title → Definitive/Strong/Moderate/Limited
Use: Variant classification support
```

**4. Inheritance Pattern**
```
Query: What's the inheritance mode? → Check moi_title → Autosomal dominant/recessive
Use: Genetic counseling
```

**5. Source Comparison**
```
Query: Do curators agree? → Compare classifications across submitters → Consensus assessment
Use: Quality assurance
```

**6. Literature Review**
```
Query: What papers support this? → Get PMIDs → PubMed cross-references
Use: Evidence review
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 6 custom tests (gene symbol, disease, classification, text search, PMIDs)

**Coverage**:
- ID-based lookup
- Attribute verification
- Text search by gene symbol and disease title
- Cross-reference validation

## Performance

- **Test Build**: ~3s (100 entries)
- **Data Source**: GenCC TSV export
- **Update Frequency**: Ongoing updates from curators
- **Total Entries**: ~35,000 curations

## Known Limitations

- PMIDs sometimes contain suffix notation (e.g., "29133208[PMID]")
- Some multiline notes fields in TSV require special handling

## Future Work

- Add filter tests for classification levels
- Test cross-references to HGNC, MONDO, HPO datasets
- Add submitter-specific queries

## Maintenance

- **Release Schedule**: Continuous updates
- **Data Format**: TSV export
- **Test Data**: 100 entries
- **License**: CC BY 4.0

## References

- **Citation**: DiStefano MT, et al. (2022) The Gene Curation Coalition
- **Website**: https://thegencc.org
- **License**: Creative Commons Attribution 4.0
