# ChEMBL Target Dataset

## Overview

ChEMBL Target contains biological entities (proteins, protein complexes, cell lines, organisms) against which compounds are tested in bioactivity assays. Each target record includes organism, target type classification, and detailed protein component information with UniProt accessions, gene symbols, and extensive cross-references. Essential for target-based drug discovery, with 15,000+ curated targets linking compounds to their biological mechanisms of action. Target components provide molecular details enabling protein-level analysis.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Drug targets with organism, type classification, and component details

## Integration Architecture

### Storage Model

**Primary Entries**:
- Target IDs (e.g., `CHEMBL2242`) stored as main identifiers

**Searchable Text Links**:
- Target names indexed for text search
- Protein component descriptions searchable

**Attributes Stored** (protobuf ChemblTargetAttr):
- `pref_name`: Preferred target name
- `organism`: Source organism (species)
- `tax_id`: NCBI taxonomy ID
- `target_type`: Classification (SINGLE PROTEIN, PROTEIN COMPLEX, PROTEIN FAMILY, etc.)
- `species_group_flag`: Multi-species target indicator
- `target_components`: Array of component details (see below)

**Component Attributes** (ChemblTargetComponentAttr):
- `accession`: UniProt accession
- `component_description`: Protein description
- `component_type`: PROTEIN, DNA, RNA, etc.
- `relationship`: SINGLE PROTEIN, PROTEIN COMPLEX MEMBER, etc.
- `target_component_synonyms`: Gene symbols, EC numbers, names
- `target_component_xrefs`: Cross-references to GO, AlphaFold, UniProt, etc.

**Cross-References**:
- **Activities**: chembl_activity (bioactivity measurements against this target)
- **Assays**: chembl_assay (assays testing this target)
- **UniProt**: Protein sequences and annotations
- **Gene Ontology**: Cellular component, molecular function, biological process
- **AlphaFold**: 3D structure predictions
- **Expression Atlas**: Gene expression data

### Special Features

**Hierarchical Structure**:
- Targets contain component details
- Single proteins → single component
- Protein complexes → multiple components
- Protein families → multiple related proteins

**Rich Component Data**:
- UniProt accessions for protein-level queries
- Gene symbols for genomic integration
- Extensive cross-references (GO, AlphaFold, etc.)
- Synonym lists (EC numbers, gene symbols, alternative names)

**Target Type Classification**:
- SINGLE PROTEIN: Individual protein targets
- PROTEIN COMPLEX: Multi-subunit complexes
- PROTEIN FAMILY: Groups of related proteins
- ORGANISM: Whole organism targets (antimicrobials)
- CELL LINE: Cellular targets
- And more specialized types

**Cross-Dataset Integration**:
- Links to activities and assays
- UniProt integration via accessions
- GO term enrichment via component xrefs
- AlphaFold structure predictions

## Use Cases

**1. Target-Based Drug Discovery**
```
Query: Disease → Target → Activities → Active molecules
Use: Identify compounds active against therapeutic target
```

**2. Target Selectivity Analysis**
```
Query: Molecule → All targets → Compare activities across protein family
Use: Assess selectivity and predict off-target effects
```

**3. Protein Complex Analysis**
```
Query: Target with PROTEIN COMPLEX type → Components → UniProt details
Use: Understand multi-subunit target composition
```

**4. Cross-Species Target Comparison**
```
Query: Target name → Filter by organism → Compare human vs. animal targets
Use: Assess species selectivity for toxicology
```

**5. GO Term Enrichment**
```
Query: Active molecules → Targets → Component xrefs → GO terms
Use: Identify biological pathways affected by compounds
```

**6. Structure-Based Analysis**
```
Query: Target → Component accession → AlphaFold xref → 3D structure
Use: Structure-based drug design and docking studies
```

## Test Cases

**Current Tests** (14 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 10 custom tests (name, type, organism, components, taxonomy, UniProt, description, synonyms, xrefs, component validation)

**Coverage**:
- ✅ Target ID lookup
- ✅ Target names (pref_name)
- ✅ Target type classification
- ✅ Organism tracking
- ✅ Taxonomy ID validation
- ✅ Component count validation
- ✅ UniProt accession extraction
- ✅ Component descriptions
- ✅ Component synonyms
- ✅ Component cross-references (GO, AlphaFold, etc.)

**Recommended Additions**:
- Protein complex target tests (multiple components)
- Protein family target tests
- Organism target tests (antimicrobial targets)
- Species group flag tests
- EC number extraction tests
- Gene symbol validation tests
- GO term distribution tests
- Cross-reference to activity/assay tests

## Performance

- **Test Build**: ~25.8s (20 targets + components)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Targets**: 15,000+ curated biological targets
- **Note**: Includes chembl_target_component dependency (automatically built)

## Known Limitations

**Component Complexity**:
- Protein complexes have multiple components
- Not all components have complete cross-reference data
- Some older entries lack detailed component information

**Target Type Heterogeneity**:
- Test data focuses on single proteins
- Protein complexes, families, and organism targets need separate testing
- Type definitions can overlap

**Cross-Reference Completeness**:
- Not all components have GO annotations
- AlphaFold coverage limited to reviewed proteins
- Some xref databases not universally populated

**Species Coverage**:
- Majority are human targets
- Animal model targets less represented
- Pathogen targets variable coverage

**Dependency**:
- Requires chembl_target_component dataset
- Automatically included in builds
- Cannot build target without component data

## Target Type Classification

| Type | Description | Components | Example |
|------|-------------|------------|---------|
| **SINGLE PROTEIN** | Individual protein | 1 | Kinases, receptors, enzymes |
| **PROTEIN COMPLEX** | Multi-subunit | 2+ | Ion channels, transcription factors |
| **PROTEIN FAMILY** | Related proteins | Multiple | Cytochrome P450s, GPCRs |
| **ORGANISM** | Whole organism | Varies | Bacteria, parasites |
| **CELL LINE** | Cellular target | N/A | Cancer cell lines |
| **TISSUE** | Tissue-level | N/A | Organ systems |

## Future Work

- Add protein complex target tests (multiple components)
- Test protein family targets
- Add organism target tests (antimicrobial applications)
- Test species group flag functionality
- Add EC number extraction validation
- Test gene symbol search functionality
- Add GO term enrichment tests
- Test cross-references to activities and assays
- Add target-disease association tests
- Test multi-species target handling
- Add component cross-reference distribution analysis
- Test synonym search functionality

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with nested component structure
- **Test Data**: Fixed 20 target IDs (all single proteins in test sample)
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (target links to activity↔assay↔molecule↔document)
- **Dependency**: Requires chembl_target_component (auto-built together)

## Component Cross-Reference Databases

Component xrefs provide rich external database links:
- **AlphaFoldDB**: 3D structure predictions
- **GoComponent/GoFunction/GoProcess**: Gene Ontology annotations
- **ExpressionAtlas**: Gene expression profiles
- **UniProt**: Protein sequences and functional annotations
- **PDB**: Experimental protein structures (when available)
- **Reactome/PathwayCommons**: Pathway information

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **UniProt**: https://www.uniprot.org/
- **License**: CC BY-SA 3.0
