# NCBI Taxonomy Dataset

## Overview

NCBI Taxonomy is the authoritative classification system for all organisms represented in molecular databases. Provides scientific names, common names, taxonomic ranks, and hierarchical relationships from domains down to strains. Essential for organizing biological data across species and enabling comparative genomics.

**Source**: NCBI Taxonomy XML dump
**Data Type**: Hierarchical taxonomic classification with parent-child relationships

## Integration Architecture

### Storage Model

**Primary Entries**:
- Taxonomy IDs (e.g., `9606` for Homo sapiens) stored as main identifiers

**Searchable Text Links**:
- Scientific names (e.g., `Homo sapiens`) → self-referencing keywords
- Scientific names with underscores (e.g., `Homo_sapiens`) → self-referencing keywords
- Common names (e.g., `human`) → text search

**Hierarchical Relationships**:
- Parent-child links stored as cross-references within taxonomy namespace
- Virtual datasets: `taxparent` and `taxchild` for navigating hierarchy
- Bidirectional: taxon → parents and taxon → children

**Attributes Stored** (protobuf):
- Scientific name (required)
- Common name (when available)
- Rank (domain, kingdom, phylum, class, order, family, genus, species, subspecies, strain)
- Taxonomic division (groups like bacteria, mammals, plants, viruses)

### Taxonomic Hierarchy

Standard Linnaean ranks stored as numeric codes:
- **Domain** (e.g., Bacteria, Archaea, Eukaryota)
- **Kingdom** → **Phylum** → **Class** → **Order** → **Family**
- **Genus** → **Species** → **Subspecies** → **Strain**

### Special Features

**Name Normalization**:
- Spaces automatically converted to underscores for compatibility with Ensembl naming
- Both `Homo sapiens` and `Homo_sapiens` searchable

**Lineage Navigation**:
- Virtual datasets enable queries like: species → genus → family → order
- Enables filtering by taxonomic groups in cross-dataset queries

## Use Cases

**1. Species Identification**
```
Query: Scientific name "Escherichia coli" → Find taxon:562
Use: Standardize species names across datasets
```

**2. Taxonomic Lineage Traversal**
```
Query: Human (9606) → Navigate to Mammalia (40674) → Vertebrata → Eukaryota
Use: Study evolutionary relationships and comparative genomics
```

**3. Common Name Resolution**
```
Query: "mouse" → Find Mus musculus (taxon:10090)
Use: Convert colloquial names to standardized taxonomy IDs
```

**4. Cross-Species Analysis**
```
Query: All species under Primates (9443) → Get all primate taxon IDs
Use: Comparative genomics across related species
```

**5. Data Filtering by Organism**
```
Query: Proteins with taxonomy filter → Restrict to specific organisms or clades
Use: Focus analysis on specific taxonomic groups (e.g., only bacteria)
```

**6. Taxonomic Classification**
```
Query: Novel sequence → BLAST → Get taxonomy ID → Classify organism
Use: Assign sequences to taxonomic groups in metagenomics
```

## Test Cases

**Current Tests** (12 total):
- 6 declarative tests (lookup, attributes, names, multi-lookup, invalid ID)
- 6 custom tests (common names, ranks, taxonomic division, parent relationship, hierarchy)

**Coverage**:
- ✅ Taxonomy ID and scientific name lookup
- ✅ Name with underscores (Ensembl compatibility)
- ✅ Common name availability
- ✅ Domain and species rank validation
- ✅ Taxonomic division field
- ✅ Parent-child relationship storage
- ✅ Hierarchical relationships

**Recommended Additions**:
- Lineage traversal (species → genus → family chain)
- Taxonomic division filtering
- Virtual dataset queries (taxparent, taxchild)

## Performance

- **Test Build**: ~2.3s (100 taxonomy entries)
- **Data Source**: XML dump from NCBI (FTP download)
- **Update Frequency**: Daily releases from NCBI
- **Total Taxa**: ~2.5 million+ taxa spanning all life

## Known Limitations

**Lineage Information**:
- Full lineage paths not stored (skipped during parsing)
- Lineage must be reconstructed by traversing parent relationships

**Relationship Validation**:
- Parent-child relationships stored but not heavily tested yet in isolated builds
- Virtual datasets (`taxparent`, `taxchild`) enable navigation

**Taxonomic Division**:
- Division field available but not present in all reference test data
- Broader groupings (bacteria, mammals, etc.) may vary in completeness

## Future Work

- Add comprehensive lineage traversal tests (species → domain path)
- Test virtual dataset queries (`taxparent`, `taxchild`)
- Add tests for different rank levels (genus, family, order, class, phylum)
- Test taxonomic division filtering
- Add synonym/common name search tests

## Maintenance

- **Release Schedule**: Daily updates from NCBI Taxonomy
- **Data Format**: XML dump (stable format)
- **Test Data**: Fixed 100 taxonomy IDs spanning multiple ranks and divisions

## References

- **Citation**: Schoch CL et al. NCBI Taxonomy: a comprehensive update on curation, resources and tools. Database (Oxford). 2020.
- **Website**: https://www.ncbi.nlm.nih.gov/taxonomy
- **License**: Public domain (US Government work)
