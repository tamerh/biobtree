# GO (Gene Ontology) Dataset

## Overview

Gene Ontology (GO) provides standardized vocabulary for describing gene and gene product functions across all species. Organized into three hierarchical ontologies: Molecular Function, Biological Process, and Cellular Component. The world's largest source of computational models for biological systems.

**Source**: Gene Ontology Consortium OWL/XML format
**Data Type**: Controlled vocabulary with hierarchical parent-child relationships

## Integration Architecture

### Storage Model

**Primary Entries**:
- GO IDs (e.g., `GO:0000001`) stored as main identifiers

**Hierarchical Relationships**:
- Parent-child links stored as cross-references within GO namespace
- Virtual datasets: `goparent` and `gochild` for navigating hierarchy
- Bidirectional: term → parents and term → children

**Searchable Text Links**:
- Term names (synonyms) → self-referencing keywords
- Exact synonyms searchable

**Attributes Stored** (protobuf):
- Term names, synonyms, definitions
- Aspect (biological_process, molecular_function, cellular_component)
- Obsolete flag

### Three Ontology Aspects

1. **Molecular Function** (GO:0003674 root):
   - Biochemical activities (e.g., "kinase activity", "DNA binding")
   - Elemental activities performed by individual gene products

2. **Biological Process** (GO:0008150 root):
   - Larger biological objectives (e.g., "cell division", "signal transduction")
   - Ordered assemblies of molecular functions

3. **Cellular Component** (GO:0005575 root):
   - Locations where gene products act (e.g., "nucleus", "mitochondrion")
   - Anatomical structures and complexes

### Special Features

**Hierarchical Navigation**:
- Each term connected to parent terms (is-a, part-of relationships)
- Enables traversing from specific to general concepts
- Example: "mitochondrial ATP synthesis" → "ATP synthesis" → "nucleotide metabolism"

**Obsolete Terms**:
- Deprecated terms retained for backward compatibility
- Marked with `isObsolete: true`
- Names prefixed with "obsolete"

## Use Cases

**1. Functional Gene Annotation**
```
Query: GO:0006915 (apoptotic process) → Get term definition and hierarchy
Use: Annotate genes involved in programmed cell death
```

**2. Ontology Hierarchy Navigation**
```
Query: GO:0000001 → Get parent terms → Navigate to broader biological processes
Use: Understand relationships between related biological concepts
```

**3. Enrichment Analysis Preparation**
```
Query: List of GO terms → Resolve current IDs, check obsolete status
Use: Prepare GO term sets for gene set enrichment analysis
```

**4. Synonym Resolution**
```
Query: "mitochondrial inheritance" → Find GO:0000001 (canonical term)
Use: Map alternative term names to standard GO IDs
```

**5. Aspect-Specific Queries**
```
Query: Terms by aspect (molecular_function, biological_process, cellular_component)
Use: Filter GO terms by ontology domain
```

**6. Cross-Species Functional Comparison**
```
Query: GO terms shared across species → Compare functional annotations
Use: Study conserved biological processes and molecular functions
```

## Test Cases

**Current Tests** (11 total):
- 4 declarative tests (lookup, attributes, multi-lookup, invalid ID)
- 7 custom tests (3 aspects, obsolete handling, synonyms, definition, relationships)

**Coverage**:
- ✅ GO ID lookup
- ✅ Three ontology aspects (biological_process, molecular_function, cellular_component)
- ✅ Synonym handling
- ✅ Obsolete term retrieval
- ✅ Term definitions
- ✅ Parent-child relationship storage

**Recommended Additions**:
- Hierarchical traversal (parent/child navigation)
- Synonym search
- Aspect filtering

## Performance

- **Test Build**: ~3.5s (100 term entries)
- **Data Source**: OWL/XML format from Gene Ontology Consortium
- **Update Frequency**: Regular releases (monthly)
- **Total Terms**: ~44,000+ GO terms across three ontologies

## Known Limitations

**Hierarchy Navigation**:
- Parent/child relationships stored but not heavily tested yet
- Virtual datasets (`goparent`, `gochild`) enable navigation

**Cross-References**:
- GO terms don't link to external databases in biobtree
- GO is used *by* other datasets for functional annotation

**Search**:
- Only exact synonyms searchable as text keywords
- Definitions not indexed for text search

## Future Work

- Add comprehensive hierarchical navigation tests (parent → children, child → parents)
- Test virtual dataset queries (`goparent`, `gochild`)
- Add synonym-based search tests
- Test filtering by aspect (BP, MF, CC)

## Maintenance

- **Release Schedule**: Regular updates from GO Consortium
- **Data Format**: OWL/XML (standard ontology format)
- **Test Data**: Fixed 100 GO IDs spanning all three aspects
- **Obsolete Terms**: Included in test data to verify backward compatibility

## References

- **Citation**: The Gene Ontology Consortium. The Gene Ontology resource. Nucleic Acids Research.
- **Website**: http://geneontology.org/
- **License**: CC BY 4.0 (freely available for all uses)
