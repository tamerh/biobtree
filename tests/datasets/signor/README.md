# SIGNOR Dataset

## Overview

SIGNOR (SIGnaling Network Open Resource) is a database of manually curated causal relationships between biological entities involved in signal transduction. It contains ~114,000 interactions across three organisms (human, mouse, rat), capturing how proteins, chemicals, and phenotypes regulate each other through mechanisms like phosphorylation, binding, and transcriptional regulation.

**Source**: University of Rome "Tor Vergata" - https://signor.uniroma2.it/
**Data Type**: Causal signaling interactions with regulatory effects and mechanisms

## Integration Architecture

### Storage Model

**Primary Entries**: SIGNOR interaction IDs (e.g., `SIGNOR-142566`)
- Each interaction stored as a separate entry with full causal relationship data

**Searchable Text Links**: Entity names indexed for text search
- Source entity name (ENTITYA) - proteins, complexes, chemicals
- Target entity name (ENTITYB) - proteins, phenotypes, complexes
- Internal SIGNOR complexes and phenotypes searchable by name

**Attributes Stored** (SignorAttr protobuf):
- `entity_a`, `type_a`, `id_a`, `database_a` - Source entity details
- `entity_b`, `type_b`, `id_b`, `database_b` - Target entity details
- `effect` - Regulatory effect (up-regulates, down-regulates, etc.)
- `mechanism` - Mechanism type (phosphorylation, binding, etc.)
- `residue`, `sequence` - Modification site details
- `tax_id` - Organism taxonomy ID
- `cell_data`, `tissue_data` - Experimental context
- `direct` - Direct interaction flag
- `score` - Confidence score (0-1)

**Cross-References**:
- UniProt (protein entities)
- ChEBI (chemical entities)
- PubChem (chemical entities)
- DrugBank (drug entities)
- PubMed (literature references)
- Taxonomy (organism: 9606, 10090, 10116)

### Special Features

- **Multi-organism support**: Human (~42K), Mouse (~37K), Rat (~35K) interactions
- **Organism filtering**: Query by tax_id attribute to filter species
- **Bidirectional protein links**: Both source and target proteins cross-referenced
- **Mechanism filtering**: Filter by phosphorylation, binding, transcriptional regulation, etc.
- **Confidence scoring**: Filter by score threshold for high-confidence interactions
- **Effect filtering**: Filter by regulatory direction (up/down-regulates)

## Use Cases

**1. Drug Target Signaling**
```
Query: What signaling pathways does my drug target affect?
Flow: Drug → DrugBank → SIGNOR → downstream targets
Use: Understanding drug mechanism of action and off-target effects
```

**2. Phosphorylation Networks**
```
Query: Which kinases phosphorylate my protein of interest?
Flow: Protein → UniProt → SIGNOR[mechanism=="phosphorylation"]
Use: Mapping kinase-substrate relationships for phosphoproteomics
```

**3. Disease-Associated Signaling**
```
Query: How do disease-associated proteins signal?
Flow: Disease gene → Ensembl → UniProt → SIGNOR → phenotypes
Use: Understanding how mutations affect cellular signaling
```

**4. Cross-Species Conservation**
```
Query: Is this signaling interaction conserved across species?
Flow: Human protein → SIGNOR → compare with mouse/rat SIGNOR entries
Use: Validating model organism relevance for human disease
```

**5. Chemical Perturbation Effects**
```
Query: What proteins does this chemical compound affect?
Flow: Chemical → ChEBI → SIGNOR → target proteins
Use: Toxicology and drug discovery target identification
```

**6. Phenotype Regulation**
```
Query: Which proteins regulate apoptosis or cell proliferation?
Flow: "apoptosis" (text search) → SIGNOR phenotypes → upstream regulators
Use: Identifying therapeutic targets for cancer or degenerative diseases
```

## Test Cases

**Current Tests** (8 total):
- 8 custom Python tests

**Coverage**:
- UniProt cross-reference validation
- ChEBI cross-reference for chemicals
- PubMed literature reference linking
- Taxonomy cross-reference by organism
- Multiple entry lookup verification
- Cross-reference count validation
- Dataset presence verification
- DrugBank cross-reference for drugs

**Recommended Additions**:
- Filter tests for mechanism types
- Filter tests for effect direction
- Score threshold filtering
- Multi-organism comparison tests
- Text search for entity names

## Performance

- **Test Build**: ~2.5s (500 entries)
- **Data Source**: Local TSV files from SIGNOR download
- **Update Frequency**: SIGNOR updates periodically
- **Total Entries**: ~114,000 interactions (42K human, 37K mouse, 35K rat)
- **Special notes**: Three separate organism files processed sequentially

## Known Limitations

- **Internal SIGNOR IDs**: Complexes and phenotypes use internal SIGNOR identifiers that don't map to external databases
- **RNAcentral coverage**: Limited miRNA entries (~300) with RNAcentral cross-references
- **BTO cell types**: Cell type data stored as BTO IDs but BTO ontology not integrated
- **Tissue data**: Tissue annotations present but not cross-referenced to tissue ontology

## Future Work

- Add BTO (BRENDA Tissue Ontology) integration for cell type cross-references
- Add Uberon integration for tissue cross-references
- Index mechanism types for direct filtering
- Add pathway-level aggregation for signaling cascades
- Support for SIGNOR pathway annotations

## Maintenance

- **Release Schedule**: SIGNOR updated periodically (check website)
- **Data Format**: Tab-separated values (TSV) with 28 columns
- **Test Data**: 500 entries (configurable via test_entries_count)
- **License**: Creative Commons Attribution 4.0

## References

- **Citation**: Licata L, et al. (2020) SIGNOR 2.0, the SIGnaling Network Open Resource 2.0: 2019 update. Nucleic Acids Res.
- **Website**: https://signor.uniroma2.it/
- **License**: CC BY 4.0
