# BRENDA Dataset

## Overview
BRENDA (BRaunschweig ENzyme DAtabase) is the world's most comprehensive enzyme information system. It contains enzyme functional data, including kinetic parameters, for classified enzymes.

**Source**: https://www.brenda-enzymes.org/
**Data Type**: Enzyme classification and functional data (EC numbers)

## Integration Architecture

### Storage Model
**Primary Entries**: EC numbers (e.g., "1.1.1.1", "3.4.21.5")
**Subdatasets**:
- `brenda_kinetics`: EC|substrate entries with Km, kcat values
- `brenda_inhibitor`: EC|inhibitor entries with Ki, IC50 values

**Searchable Text Links**: EC number, recommended_name, systematic_name, synonyms, organism names

**Attributes Stored** (brenda):
- recommended_name, systematic_name, synonyms
- reactions, cofactors, reaction_types
- organisms (top 50), organism_count
- substrate_count, inhibitor_count, km_count, kcat_count, reference_count

**Cross-References**:
- brenda → pubmed (literature references)
- brenda ↔ brenda_kinetics (bidirectional)
- brenda ↔ brenda_inhibitor (bidirectional)

### Special Features
- **Child Datasets**: Running `-d brenda` automatically builds brenda_kinetics and brenda_inhibitor
- **Detailed Kinetics**: Per-substrate Km/kcat measurements in brenda_kinetics
- **Inhibitor Data**: Per-inhibitor Ki/IC50 measurements in brenda_inhibitor
- **Filter Support**: CEL filters on kinetic/inhibitor counts and values

## Use Cases

**1. Find Enzymes by Name**
```
Query: "alcohol dehydrogenase" → brenda text search → EC entries
Use: Identify EC classification for enzymes by name
```

**2. Get Kinetic Parameters**
```
Query: 1.1.1.1 → brenda → brenda_kinetics → Km values for each substrate
Use: Compare Km across substrates for enzyme characterization
```

**3. Find Enzyme Inhibitors**
```
Query: 1.1.1.1 → brenda → brenda_inhibitor[ki_count>0] → Inhibitors with Ki data
Use: Drug discovery - identify known inhibitors with quantitative data
```

**4. Literature Mining**
```
Query: 1.1.1.1 → brenda → pubmed → Related publications
Use: Find primary literature for enzyme studies
```

**5. Filter by Data Availability**
```
Query: EC → brenda_kinetics[brenda_kinetics.km_count>50] → Well-characterized substrates
Use: Focus on substrates with extensive kinetic data
```

**6. Organism-Specific Enzymes**
```
Query: "Homo sapiens" → brenda text search → Human enzymes
Use: Species-specific enzyme discovery
```

## Test Cases

**Current Tests** (25+ total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 9 custom declarative tests (subdataset lookups, mappings, text search, filters)
- 20+ custom Python tests

**Coverage**:
- ✅ EC number lookup
- ✅ Attribute validation (recommended_name, systematic_name, counts)
- ✅ Subdataset lookups (brenda_kinetics, brenda_inhibitor)
- ✅ Cross-reference validation (pubmed, kinetics, inhibitors)
- ✅ Text search (enzyme names, synonyms, substrates, inhibitors)
- ✅ Filter validation (km_count, ki_count)
- ✅ Removed attributes verification (pubmed_ids, top_substrates, top_inhibitors)

## Performance

- **Test Build**: ~7s (100 EC entries + 500 kinetics + 500 inhibitors)
- **Data Source**: Local JSON file (brenda_2025_1.json, 693 MB)
- **Update Frequency**: Annual releases
- **Total Entries**: ~8,055 EC numbers, 107K kinetics, 386K inhibitors

## Known Limitations

- Organism names stored as text (no taxonomy ID resolution)
- Long inhibitor names truncated in entry IDs (full name in attributes)
- No direct API for reference data extraction

## Future Work

- Taxonomy cross-references via organism name lookup
- Rhea reaction cross-references
- ChEBI cross-references for substrates/inhibitors

## Maintenance

- **Release Schedule**: Annual (e.g., 2025.1)
- **Data Format**: JSON
- **Test Data**: 100 EC entries (brenda), 500 entries each (kinetics, inhibitors)
- **License**: CC BY 4.0

## References

- **Citation**: Schomburg et al., BRENDA in 2024, Nucleic Acids Research, 2024
- **Website**: https://www.brenda-enzymes.org/
- **License**: CC BY 4.0
