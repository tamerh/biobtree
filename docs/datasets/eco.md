# ECO (Evidence & Conclusion Ontology) Dataset

## Overview

ECO is an ontology containing terms that describe types of evidence and assertion methods used in biocuration. Essential for tracking provenance of biological annotations, enabling quality control, and supporting evidence-based queries across biological databases. Used extensively in GO annotations and biocuration workflows.

**Source**: ECO OWL file (OBO Library)
**Data Type**: Evidence codes and assertion methods in hierarchical ontology structure

## Integration Architecture

### Storage Model

**Primary Entries**:
- ECO IDs (e.g., `ECO:0000000`) stored as main identifiers

**Searchable Text Links**:
- Evidence type names → stored in attributes (not text-searchable currently)
- Synonyms → stored in attributes

**Attributes Stored** (protobuf OntologyAttr):
- Name (e.g., "inference from background scientific knowledge")
- Type (evidence namespace)
- Synonyms (e.g., "evidence code", "evidence_code")

**Cross-References**:
- **Hierarchical**: Parent-child relationships via `ecoparent` and `ecochild` virtual datasets
- **Biological databases**: Used by GO, UniProt, and other annotation databases

### Special Features

**Hierarchical Classification**:
- Parent-child relationships between evidence types
- Enables browsing from general to specific evidence codes
- Supports evidence type inheritance and specialization

**GO Evidence Code Mapping**:
- ECO terms map to traditional GO evidence codes (IDA, IMP, ISS, etc.)
- Provides standardized vocabulary across annotation resources

**Evidence Granularity**:
- Fine-grained evidence types for precise annotation tracking
- Supports computational and experimental evidence distinction

## Use Cases

**1. Annotation Provenance**
```
Query: Gene annotation → Check ECO code → Determine evidence type
Use: Assess confidence in gene function predictions
```

**2. Evidence-Based Filtering**
```
Query: GO terms with ECO:0000269 (experimental evidence) → High-confidence annotations
Use: Filter annotations by evidence quality
```

**3. Database Integration**
```
Query: ECO term → Find annotations using this evidence type
Use: Cross-database evidence standardization
```

**4. Evidence Hierarchy Navigation**
```
Query: ECO term → Navigate parent/child evidence types
Use: Understand evidence type relationships and specialization
```

**5. Quality Control**
```
Query: Annotations → Check ECO codes → Flag low-confidence evidence
Use: Biocuration validation and quality assurance
```

**6. Research Queries**
```
Query: Find all experimental vs computational evidence → Compare annotation sources
Use: Meta-analysis of annotation quality across databases
```

## Test Cases

**Current Tests** (6 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 2 custom tests (evidence name, synonyms)

**Coverage**:
- ✅ ECO ID lookup
- ✅ Attribute validation (name, type, synonyms)
- ✅ Batch lookups
- ✅ Invalid ID handling
- ✅ Evidence term names
- ✅ Synonym presence

**Recommended Additions**:
- Parent-child hierarchy navigation tests
- Evidence type classification tests
- GO evidence code mapping validation
- Cross-database evidence reference tests

## Performance

- **Test Build**: ~1s (100 ECO entries)
- **Data Source**: OWL file from OBO Library (http://purl.obolibrary.org/obo/)
- **Update Frequency**: Regular updates (several times per year)
- **Total Terms**: ~3,000+ evidence type terms

## Known Limitations

**Text Search**:
- Evidence type names not currently indexed for text search
- Must use ECO IDs directly for lookups

**Cross-References**:
- ECO provides vocabulary but doesn't store actual annotations
- Must link to annotation databases (GO, UniProt) for usage data

**Hierarchy Navigation**:
- Parent-child relationships stored via virtual datasets (`ecoparent`, `ecochild`)
- Requires additional queries to navigate hierarchy

## Future Work

- Add text search support for evidence type names (currently not indexed)
- Add comprehensive hierarchy navigation tests
- Test GO evidence code mapping validation
- Add tests for experimental vs computational evidence distinction
- Test integration with GO/UniProt annotations
- Add evidence type filtering tests

## Maintenance

- **Release Schedule**: Regular updates from ECO Consortium
- **Data Format**: OWL/XML (stable OBO format)
- **Test Data**: Fixed 100 ECO IDs spanning various evidence types
- **Namespace**: All terms use ECO: prefix

## References

- **Citation**: Giglio M et al. ECO, the Evidence & Conclusion Ontology: community standard for evidence information. Nucleic Acids Research.
- **Website**: https://www.evidenceontology.org/
- **OBO Library**: http://obofoundry.org/ontology/eco.html
- **License**: CC0 1.0 (Public Domain) - freely available for all uses
