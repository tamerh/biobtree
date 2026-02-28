# BindingDB Dataset

## Overview
BindingDB is a public database of measured binding affinities between proteins and drug-like small molecules. It contains ~2.9 million binding data entries with experimental measurements including Ki, IC50, Kd, and EC50 values. Critical resource for drug discovery, target identification, and pharmacological research.

**Source**: https://www.bindingdb.org/
**Data Type**: Protein-ligand binding affinity measurements with chemical structures and cross-references

## Integration Architecture

### Storage Model
**Primary Entries**: BindingDB Monomer ID (numeric, e.g., "50000001")
**Searchable Text Links**: BindingDB ID, ligand name, target name, InChI Key
**Attributes Stored**: Binding affinity values (Ki, IC50, Kd, EC50), ligand info (SMILES, InChI), target organism, experimental conditions (pH, temperature), literature references
**Cross-References**: UniProt (target proteins), PubChem (compounds), ChEMBL (molecules), ChEBI (chemicals)

### Special Features
- **Bidirectional linking**: BindingDB entries link to UniProt targets and PubChem/ChEMBL compounds
- **Affinity data preservation**: Values stored with units as strings to preserve original format (e.g., ">10000 nM")
- **Multi-target support**: Single compound can have multiple target proteins (up to 50 chains)
- **Chemical structure indexing**: InChI Key searchable for structure-based queries

### UniProt ID Extraction (Fixed 2026-02-03)
BindingDB TSV has numbered chain columns for multi-chain protein complexes:
- `UniProt (SwissProt) Primary ID of Target Chain 1` through `Chain 50`
- `UniProt (TrEMBL) Primary ID of Target Chain 1` through `Chain 50`

The parser extracts UniProt IDs from all chain columns for both SwissProt and TrEMBL entries.

**Note**: TrEMBL (unreviewed) UniProt IDs are extracted but may not resolve in biobtree since only reviewed SwissProt entries are currently indexed. These IDs are still stored in BindingDB attributes for reference.

## Use Cases

**1. Target-Based Drug Discovery**
```
Query: Find all compounds binding to a specific protein target
P35354 >> bindingdb → All binding data for COX-2
Use: Identify lead compounds for drug development
```

**2. Compound Profiling**
```
Query: Get binding profile of a known drug
aspirin >> bindingdb → All target interactions
Use: Understand selectivity and off-target effects
```

**3. Affinity Comparison**
```
Query: Compare binding affinities across targets
bindingdb.ki < 100 → High-affinity binders
Use: Prioritize compounds for optimization
```

**4. Cross-Database Integration**
```
Query: Link binding data to protein structures
CHEMBL25 >> bindingdb >> uniprot >> alphafold → Structure context
Use: Structure-activity relationship analysis
```

**5. Target Identification**
```
Query: Find protein targets for a compound class
InChI Key search >> bindingdb → Associated targets
Use: Deorphanize compounds and identify mechanisms
```

**6. Organism-Specific Binding**
```
Query: Filter by target organism
bindingdb.target_source_organism == "Homo sapiens" → Human targets only
Use: Focus on human disease-relevant interactions
```

## Test Cases

**Current Tests** (10 total):
- 4 declarative tests (ID lookup, attribute check, multi-lookup, invalid ID)
- 6 custom tests (target name, ligand name, affinity data, text search by ligand/target, organism)

**Coverage**:
- Primary ID lookup
- Attribute presence validation
- Text search by ligand and target names
- Binding affinity data presence
- Organism information

**Recommended Additions**:
- CEL filter tests for affinity ranges
- InChI Key search test
- Multi-chain complex tests (verify all chains extracted)

**Cross-Reference Tests** (now working after 2026-02-03 fix):
- ✅ UniProt → BindingDB (`P00533 >>uniprot>>bindingdb`)
- ✅ BindingDB → UniProt (`>>bindingdb>>uniprot`)
- ✅ BindingDB → PubChem (`>>bindingdb>>pubchem`)
- ✅ PubChem → BindingDB (`>>pubchem>>bindingdb`)

## Performance

- **Test Build**: ~30-60s (1000 entries)
- **Data Source**: BindingDB_All_tsv.zip from bindingdb.org
- **Update Frequency**: Monthly
- **Total Entries**: ~2.9 million binding data entries
- **Special notes**: Large ZIP file (~1.5GB compressed), TSV parsing with SMILES strings

## Known Limitations

- **Large download**: Full dataset is ~1.5GB compressed
- **SMILES length**: Some SMILES strings are very long, requiring large buffer
- **Sparse affinity data**: Not all entries have all affinity types
- **Multi-value fields**: UniProt, PubChem IDs can be pipe-separated lists
- **TrEMBL not indexed**: BindingDB extracts TrEMBL (unreviewed UniProt) IDs, but biobtree currently only indexes reviewed SwissProt entries. TrEMBL targets won't resolve in `>>uniprot>>bindingdb` queries but are stored in BindingDB attributes.

## Future Work

- Add DrugBank cross-references
- Support for binding kinetics (kon/koff) filtering
- Integration with AlphaFold for structural context
- Compound similarity search via InChI Key patterns

## Maintenance

- **Release Schedule**: Monthly updates from BindingDB
- **Data Format**: Tab-separated values in ZIP archive
- **Test Data**: 1000 entries for fast testing
- **License**: Public domain for academic use

## References

- **Citation**: Liu T, Lin Y, Wen X, et al. (2007) BindingDB: a web-accessible database of experimentally determined protein-ligand binding affinities. Nucleic Acids Res.
- **Website**: https://www.bindingdb.org/
- **License**: Public domain (academic), commercial license available
