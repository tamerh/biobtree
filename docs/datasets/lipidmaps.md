# LIPID MAPS Structure Database (LMSD) Dataset

## Overview

LIPID MAPS provides comprehensive lipid structure and classification data across 8 major lipid categories. Contains ~49,000 curated lipid structures with systematic nomenclature, chemical properties, and hierarchical classification. Essential for lipidomics research, lipid metabolism studies, and systems biology linking lipids to proteins, genes, and diseases.

**Source**: LIPID MAPS SDF file download (ZIP archive)
**Data Type**: Lipid structures with 4-level hierarchical classification and cross-database mappings

## Integration Architecture

### Storage Model

**Primary Entries**:
- LIPID MAPS IDs (e.g., `LMFA01010001`) stored as main identifiers
- ID format: LM + Category Code (FA/GL/GP/SP/ST/PR/SL/PK) + numeric ID

**Searchable Text Links**:
- Common names (e.g., `Palmitic acid`) → text search
- Systematic names (e.g., `hexadecanoic acid`) → text search
- Abbreviations (e.g., `FA 16:0`) → text search
- Synonyms (multiple per lipid) → text search
- InChI Keys → text search (enables structure-based lookup)

**Attributes Stored** (protobuf):
- Names: common name, systematic name, abbreviation, synonyms
- Classification: category (8 types), main_class, sub_class, class_level4
- Chemical properties: formula, exact_mass, SMILES, InChI, InChI Key

**Cross-References**:
- **Chemistry**: ChEBI (~25K), HMDB (~14K), KEGG (~15K), PubChem (~30K)
- **Lipid DBs**: SwissLipids, LipidBank, PlantFA
- Enables mapping: Lipids → Metabolites → Pathways → Proteins → Genes → Diseases

### Special Features

**Hierarchical Classification**:
- 4-level lipid taxonomy: Category → Main Class → Sub Class → Level 4
- Enables category-based filtering (e.g., `lipidmaps[category=='Sphingolipids [SP]']`)

**Multiple Name Forms**:
- Common name + systematic name + abbreviation + synonyms all searchable
- Systematic nomenclature follows LIPID MAPS standards

**Chemical Structure Search**:
- InChI Key enables structure-based lookups
- SMILES notation stored for cheminformatics integration
- Molecular formula searchable

**Lipid Categories**:
- 8 major categories: Fatty Acyls, Glycerolipids, Glycerophospholipids, Sphingolipids, Sterol Lipids, Prenol Lipids, Saccharolipids, Polyketides
- Each with complete sub-classification hierarchy

## Use Cases

**1. Lipid Metabolism Pathway Analysis**
```
Query: Sphingolipid LM_ID → HMDB → Reactome pathways → Enzymes
Use: Map lipid metabolism to protein networks and identify metabolic enzymes
```

**2. Disease-Associated Lipid Discovery**
```
Query: Disease term → HMDB metabolites → Filter lipidmaps by category
Use: Identify lipid biomarkers for neurodegenerative diseases, metabolic disorders
```

**3. Lipidomics Data Annotation**
```
Query: InChI Key from mass spec → Find LIPID MAPS entry → Get classification
Use: Annotate unknown lipid peaks with systematic nomenclature and chemical properties
```

**4. Drug Target Identification**
```
Query: Lipid class → HMDB → Reactome → UniProt → ChEMBL
Use: Find drugs targeting lipid metabolism pathways for therapeutic development
```

**5. Comparative Lipidomics**
```
Query: Filter by lipid category → Map to multiple organisms via taxonomy
Use: Study lipid composition differences across species in evolutionary research
```

**6. Systems Biology Integration**
```
Query: Glycerophospholipids → Reactome pathways → Protein networks → Gene regulation
Use: Build comprehensive models linking lipid metabolism to cellular processes
```

## Test Cases

**Current Tests** (TBD):
- Declarative tests: ID lookup, name search, synonym search, abbreviation search, attributes, category filter, invalid ID
- Custom tests: Chemical properties, InChI Key search, hierarchical classification, cross-references (ChEBI, HMDB, KEGG), category-based filtering, systematic name search

**Coverage** (To implement):
- ✅ LIPID MAPS ID lookup
- ✅ Common name and synonym search
- ✅ Systematic name search
- ✅ Abbreviation search
- ✅ InChI Key searchability (structure-based lookup)
- ✅ Chemical properties (formula, exact mass, SMILES)
- ✅ Hierarchical classification (category, main_class, sub_class, class_level4)
- ✅ Cross-references to ChEBI, HMDB, KEGG, PubChem
- ✅ Category-based filtering

**Recommended Additions**:
- Bidirectional cross-reference validation (HMDB → LIPID MAPS)
- Multi-category filtering tests
- Lipid class hierarchy navigation
- Formula-based search validation
- Cross-database integration tests (LIPID MAPS + HMDB + Reactome)

## Performance

- **Test Build**: ~TBD (20-100 lipid entries)
- **Data Source**: SDF file in ZIP archive (~20 MB compressed)
- **Update Frequency**: Regular updates from LIPID MAPS (weekly/monthly)
- **Total Lipids**: ~49,000 curated lipid structures
- **Download Time**: Fast (~5-10s for 20MB file)

## Known Limitations

**Cross-Reference Completeness**:
- Not all lipids have mappings to all external databases
- ChEBI, HMDB, KEGG coverage varies by lipid class
- PubChem cross-references stored but may not be in isolated test builds

**Name Field Variations**:
- SDF may use either `NAME` or `COMMON_NAME` field
- Parser handles both variants automatically

**Lipid Coverage**:
- Focuses on well-characterized lipids
- May not include all theoretical/computational lipid structures
- SwissLipids has broader coverage (780K) but requires separate integration

**Classification Codes**:
- Classification strings include codes (e.g., "[FA01]") for programmatic parsing
- Filter queries must match exact string including brackets

## Future Work

- Add hierarchical navigation (lipidmapsparent/lipidmapschild datasets)
- Implement mass range filtering for MS/MS data matching
- Add SwissLipids integration for expanded coverage (780K structures)
- Test LMPD (LIPID MAPS Proteome Database) for protein-lipid associations
- Add structure similarity search via InChI/SMILES comparison
- Test fatty acid chain pattern matching (e.g., all C16 lipids)
- Add multi-database integration tests with HMDB, ChEBI, Reactome

## Maintenance

- **Release Schedule**: Regular updates from LIPID MAPS (follows community contributions)
- **Data Format**: SDF (Structure-Data File) - stable cheminformatics format
- **Test Data**: Fixed 20-100 lipid IDs spanning all 8 major categories
- **License**: CC BY 4.0 - free for all uses with attribution
- **Version**: Using LMSD 2025-11-10 format
- **SDF Parsing**: Custom parser handles MOL blocks and `> <FIELD_NAME>` data items

## References

- **Citation**: Sud M, Fahy E, Cotter D, et al. LMSD: LIPID MAPS structure database. Nucleic Acids Research. 2007;35:D527-32.
- **Website**: https://www.lipidmaps.org/
- **License**: CC BY 4.0 - https://creativecommons.org/licenses/by/4.0/
- **Classification**: https://www.lipidmaps.org/resources/tutorials/lipid_classification.html
