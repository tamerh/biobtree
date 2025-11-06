# MONDO (Monarch Disease Ontology) Dataset

## Overview

MONDO is a logic-based disease ontology that unifies multiple disease classification systems into a coherent structure. Addresses the fragmentation problem where overlapping disease definitions exist across OMIM, Orphanet, SNOMED CT, ICD, HPO, and other resources. Provides precise 1:1 disease equivalence mappings with 129,000+ cross-references across 25,000+ disease concepts. Essential for rare disease research, clinical genomics, and precision medicine.

**Source**: MONDO OWL file (Monarch Initiative)
**Data Type**: Unified disease ontology with hierarchical classification and extensive cross-database mappings

## Integration Architecture

### Storage Model

**Primary Entries**:
- MONDO IDs (e.g., `MONDO:0000001`) stored as main identifiers

**Searchable Text Links**:
- Disease names indexed for text search
- Synonyms indexed for alternative name lookups
- Enables name-based disease queries

**Attributes Stored** (protobuf OntologyAttr):
- Name (primary disease name)
- Type (disease classification)
- Synonyms (alternative disease names with scope and provenance)

**Cross-References**:
- **Hierarchical**: Parent-child relationships via `mondoparent` and `mondochild` virtual datasets
- **Disease databases**: OMIM, Orphanet, DOID, EFO, NCIT, GARD, MedGen
- **Clinical vocabularies**: SNOMED CT, ICD-10, ICD-11, MedDRA
- **Phenotype ontologies**: HPO (Human Phenotype Ontology)
- **Research resources**: ClinVar, GeneReviews

### Special Features

**Unified Disease Mappings**:
- Precise 1:1 equivalence axioms (not just loose cross-references)
- Safe data propagation across disease resources
- Resolves conflicting disease definitions across databases

**Text Search Enabled**:
- Disease names and synonyms fully indexed
- Supports lookup by disease name, synonym, or abbreviation
- Facilitates clinical terminology mapping

**Rare Disease Focus**:
- 15,857 rare disease terms (from Orphanet integration)
- 11,601 Mendelian disease terms (from OMIM integration)
- Specialized coverage for genetic and rare conditions

**Hierarchical Classification**:
- Multi-level disease taxonomy from general to specific
- Parent-child relationships enable disease grouping queries
- Supports subsumption reasoning for disease classification

## Use Cases

**1. Rare Disease Diagnosis**
```
Query: Patient phenotypes (HPO terms) → MONDO disease → Treatment protocols
Use: Clinical diagnosis support for rare genetic disorders
```

**2. Cross-Database Integration**
```
Query: OMIM disease → MONDO ID → Map to Orphanet, GARD, ClinVar
Use: Link genetic databases using unified disease terminology
```

**3. Clinical Genomics**
```
Query: MONDO disease → Gene associations → Variant interpretations
Use: Precision medicine and variant classification workflows
```

**4. Disease Text Search**
```
Query: "lymphoblastic leukemia" → Find MONDO terms and synonyms
Use: Clinical terminology lookup and standardization
```

**5. Disease Hierarchy Navigation**
```
Query: MONDO term → Parent diseases → Child disease subtypes
Use: Explore disease relationships and classification
```

**6. Phenotype-to-Disease Mapping**
```
Query: HPO phenotype terms → Associated MONDO diseases
Use: Differential diagnosis based on patient symptoms
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 6 custom tests (name, synonyms, parents, xrefs, text search by name/synonym)

**Coverage**:
- ✅ MONDO ID lookup
- ✅ Attribute validation (name, type, synonyms)
- ✅ Disease hierarchy (parent relationships)
- ✅ Cross-references to external databases
- ✅ Text search by disease name
- ✅ Text search by synonym
- ✅ Batch lookups
- ✅ Invalid ID handling

**Recommended Additions**:
- OMIM/Orphanet cross-reference validation
- Disease classification type tests (Mendelian, rare, infectious, etc.)
- HPO phenotype association tests
- Multi-level hierarchy navigation tests
- Synonym scope validation (EXACT, BROAD, NARROW, RELATED)

## Performance

- **Test Build**: ~5s (100 MONDO entries)
- **Data Source**: OWL file from Monarch Initiative
- **Update Frequency**: Regular monthly releases
- **Total Diseases**: 25,880 concepts (22,919 human, 2,960 non-human)
- **Rare Diseases**: 15,857 rare disease terms
- **Mendelian Diseases**: 11,601 Mendelian disease terms
- **Cross-References**: 129,785 database mappings

## Known Limitations

**Hierarchy Navigation**:
- Parent-child relationships via virtual datasets (`mondoparent`, `mondochild`)
- Requires separate queries to navigate disease hierarchy

**External Database Dependencies**:
- Cross-references depend on target databases being configured
- Some OMIM/Orphanet mappings may not be available in isolated builds

**Disease Definitions**:
- Definitions not stored in current OntologyAttr structure
- Only name, type, and synonyms preserved in attributes

## Future Work

- Add OMIM cross-reference validation tests
- Add Orphanet cross-reference validation tests
- Test disease classification types (rare, Mendelian, infectious, neoplastic)
- Add HPO phenotype association tests
- Test synonym scope metadata (EXACT, BROAD, NARROW, RELATED)
- Add multi-level hierarchy navigation tests (grandparents, siblings)
- Test integration with clinical genomics workflows
- Add ClinVar/GeneReviews cross-reference tests

## Maintenance

- **Release Schedule**: Monthly releases from Monarch Initiative
- **Data Format**: OWL (complex logic-based structure)
- **Test Data**: Fixed 100 MONDO IDs spanning disease categories
- **License**: CC-BY 4.0 - freely available with attribution
- **Namespace**: All terms use MONDO: prefix
- **Coordination**: Aligned with HPO, GARD, Orphanet, OMIM

## References

- **Citation**: Vasilevsky NA et al. Mondo: Unifying diseases for the world, by the world. medRxiv (2022).
- **Website**: https://mondo.monarchinitiative.org/
- **GitHub**: https://github.com/monarch-initiative/mondo
- **OBO Foundry**: http://obofoundry.org/ontology/mondo.html
- **Documentation**: Part of the Monarch Initiative suite of resources
