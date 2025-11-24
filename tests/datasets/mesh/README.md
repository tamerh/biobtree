# MeSH (Medical Subject Headings) Dataset

## Overview

MeSH is the U.S. National Library of Medicine's controlled vocabulary thesaurus used for indexing biomedical literature. With over 30,000 descriptors and 200,000 supplementary concepts (primarily chemicals), MeSH provides standardized terminology for diseases, drugs, anatomy, organisms, and biomedical concepts. Updated annually, it serves as the primary indexing system for PubMed and other NLM databases.

**Source**: NLM MeSH Files (https://www.nlm.nih.gov/mesh/)
**Data Type**: Controlled vocabulary with descriptors, supplementary concepts, tree numbers, and pharmacological actions

## Integration Architecture

### Storage Model

**Primary Entries**:
- Descriptors (D-IDs like D000001) - Main controlled vocabulary terms
- Supplementary Concepts (C-IDs like C000002) - Additional terms, mostly chemicals

**Searchable Text Links**:
- Descriptor names (e.g., "Calcimycin" → D000001)
- All entry terms/synonyms (e.g., "A23187" → D000001)
- Supplementary concept names and synonyms

**Attributes Stored** (MeshAttr protobuf):
- descriptor_ui, descriptor_name, descriptor_class
- entry_terms[] (synonyms)
- tree_numbers[] (hierarchical classification)
- scope_note (definition/description)
- annotation (usage notes)
- allowable_qualifiers[] (MeSH subheadings)
- pharmacological_actions[] (drug action classifications)
- is_supplementary (descriptor vs supplementary concept)
- registry_number (chemical registry IDs)
- heading_mapped_to (for supplementary concepts)
- date_created, date_established, date_revised

**Cross-References**:
- Pharmacological actions: Drug/chemical → action class descriptors (e.g., Aspirin → Anti-Inflammatory Agents)
- Future: Parent-child hierarchy via tree numbers (meshparent/meshchild datasets)

### Special Features

1. **ASCII Format Parser**: Uses `.bin` files instead of XML for reliability and performance
2. **Tab Sanitization**: Cleans embedded tab characters in terms to prevent index corruption
3. **Dual Vocabulary**: Handles both descriptors (controlled terms) and supplementary concepts (chemicals)
4. **Hierarchical Classification**: Tree numbers enable category-based browsing (not yet implemented)
5. **Extensive Synonyms**: Most entries have 5-15 synonyms/entry terms
6. **Pharmacological Action Links**: Connects drugs to their mechanism descriptors

## Use Cases

**1. Drug Mechanism Discovery**
```
Query: aspirin → D001241 (MeSH descriptor)
Result: 5 pharmacological actions (Anti-Inflammatory, Antipyretics, Cyclooxygenase Inhibitors, etc.)
Use: Understand multi-target drug effects and repurposing opportunities
```

**2. Chemical Substance Identification**
```
Query: bevonium methylsulfate → C000002 (supplementary concept)
Result: Registry number 34B0471E08, mapped to descriptor D001561 (Benzilates)
Use: Link chemical synonyms to standardized descriptors for literature search
```

**3. Synonym-Based Literature Search**
```
Query: A23187 (synonym) → D000001 (Calcimycin)
Result: Official descriptor with scope note and classification
Use: Normalize variant drug names for comprehensive literature retrieval
```

**4. Drug Class Navigation**
```
Query: Anti-Inflammatory Agents → Find all drugs with this action
Result: Aspirin, other NSAIDs via pharmacological action links
Use: Identify alternative drugs in same therapeutic class
```

**5. Chemical Registry Cross-Reference**
```
Query: Registry number 52665-69-7 → Calcimycin
Result: MeSH descriptor with chemical properties
Use: Connect chemical databases (PubChem, ChEMBL) to medical literature
```

**6. Hierarchical Disease Classification**
```
Query: Tree number C10.* → Nervous system diseases
Result: All neurological condition descriptors
Use: Systematic disease categorization for clinical studies (future work)
```

## Test Cases

**Current Tests** (To be implemented):

**Recommended Test Coverage**:
- ✅ Descriptor ID lookup (D000001 → Calcimycin)
- ✅ Supplementary concept lookup (C000002 → bevonium)
- ✅ Text search by descriptor name ("aspirin")
- ✅ Text search by synonym ("A23187")
- ✅ Pharmacological action cross-references
- ✅ Chemical registry number presence
- ✅ Tree number extraction
- ✅ Entry terms (synonyms) completeness
- ✅ Scope note presence
- ✅ Case-insensitive search
- ⬜ Heading mapped to validation (supplementary → descriptor)
- ⬜ Allowable qualifiers validation
- ⬜ Multi-word search improvement ("diabetes" should find "diabetes mellitus")
- ⬜ Tree number hierarchy navigation (parent-child)
- ⬜ Batch lookup performance
- ⬜ Invalid ID handling

## Performance

- **Test Build**: ~35s (100 entries from both descriptors and supplementary files)
- **Full Build**: ~30s update + ~15s generate (217,159 total entries)
- **Data Source**: NLM MeSH ASCII files (d2025.bin, c2025.bin)
- **Update Frequency**: Annual major release (usually December), supplementary daily updates
- **Total Entries**:
  - 30,956 Descriptors
  - 186,203 Supplementary Concepts
  - 354,912 keyword index entries (names + all synonyms)
- **File Size**: ~10-15MB per ASCII file (descriptors ~8MB, supplementary ~50MB)

## Known Limitations

1. **Single-Word Search**: "diabetes" doesn't match "diabetes mellitus" - requires exact phrase or partial word matching
2. **Tree Number Hierarchy**: Not yet implemented - meshparent/meshchild datasets defined but saveHierarchyRelations() stubbed
3. **Qualifiers Not Separate**: Allowable qualifiers stored as attributes but not as separate searchable entities
4. **No PubMed Integration**: MeSH-PubMed article links not included (external to MeSH files)
5. **Chemical Structure Search**: InChI/SMILES not available in MeSH files (requires PubChem integration)
6. **Date Field Parsing**: Only date_established extracted, date_created/date_revised partially implemented

## Future Work

1. **Partial Word Matching**: Implement tokenization for single-word searches (e.g., "diabetes" finds "diabetes mellitus")
2. **Tree Number Hierarchy**: Activate parent-child navigation via tree number parsing
3. **PubChem CID Mapping**: Cross-reference supplementary concepts to PubChem compounds
4. **ChEMBL Integration**: Link MeSH pharmacological actions to ChEMBL drug targets
5. **MONDO Cross-References**: Connect MeSH disease terms to MONDO disease ontology
6. **Qualifier Dataset**: Separate dataset for MeSH subheadings/qualifiers
7. **Historical MeSH Versions**: Support for previous year vocabularies
8. **Entry Term Weights**: Prioritize preferred terms in search results
9. **Chemical Structure Search**: If PubChem integrated, enable structure-based search via registry numbers

## Maintenance

- **Release Schedule**: Annual updates (December), supplementary concepts updated daily (Monday-Friday)
- **Data Format**: ASCII `.bin` files (tab-separated field=value pairs)
- **Test Data**: 100 entries (mix of descriptors and supplementary concepts)
- **License**: Public domain (U.S. government work, no restrictions)
- **Version**: 2025 MeSH vocabulary
- **Special Notes**:
  - ASCII format preferred over XML (simpler parsing, no DOCTYPE issues)
  - Tab characters in entry terms must be sanitized to prevent index corruption
  - Supplementary concepts (C-IDs) updated more frequently than descriptors (D-IDs)

## References

- **Citation**: NLM. Medical Subject Headings. Bethesda, MD: National Library of Medicine
- **Website**: https://www.nlm.nih.gov/mesh/
- **Download**: https://www.nlm.nih.gov/databases/download/mesh.html
- **Documentation**: https://www.nlm.nih.gov/mesh/intro_record_types.html
- **License**: Public domain
