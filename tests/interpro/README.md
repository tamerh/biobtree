# InterPro Dataset

## Overview

InterPro is a comprehensive resource for protein families, domains, and functional sites. Integrates predictive information from multiple member databases (Pfam, PROSITE, SMART, etc.) into unified entries with detailed annotations. Essential for protein function prediction, sequence analysis, and comparative genomics.

**Source**: InterPro XML dump (FTP download from EMBL-EBI)
**Data Type**: Protein signatures with hierarchical classification and cross-database integration

## Integration Architecture

### Storage Model

**Primary Entries**:
- InterPro IDs (e.g., `IPR000001`) stored as main identifiers

**Searchable Text Links**:
- Short names (e.g., `Kringle`) → self-referencing keywords
- Full names → stored in attributes

**Attributes Stored** (protobuf):
- Short name and full names
- Entry type (domain, family, homologous_superfamily, repeat, site, conserved_site, binding_site, active_site, PTM)
- Protein count (number of proteins containing this signature)
- Member database information (Pfam, PROSITE, SMART, etc.)

**Cross-References**:
- **Member databases**: Pfam, PROSITE, SMART, PRINTS, ProDom, CDD, etc.
- **Literature**: PubMed references
- **Structural**: PDB entries
- **Hierarchical**: Parent-child InterPro relationships

### Entry Types

InterPro classifies protein signatures into distinct types:

1. **Domain** - Structural/functional unit (e.g., Kringle domain)
2. **Family** - Group of proteins sharing common evolutionary origin
3. **Homologous Superfamily** - Proteins with distant evolutionary relationship
4. **Repeat** - Short sequence recurring multiple times
5. **Site** - Functional regions:
   - **Conserved Site** - Evolutionarily conserved pattern
   - **Active Site** - Catalytic residues
   - **Binding Site** - Ligand interaction region
   - **PTM** (Post-Translational Modification) - Modification sites

### Special Features

**Member Database Integration**:
- Combines signatures from 13+ prediction methods
- Each InterPro entry integrates related signatures from member databases
- Provides unified annotation layer over multiple sources

**Protein Coverage Statistics**:
- Each entry annotated with protein count
- Indicates how many proteins contain that signature
- Useful for assessing signature prevalence

**Hierarchical Classification**:
- Parent-child relationships between entries
- Enables browsing from general to specific classifications

## Use Cases

**1. Protein Function Prediction**
```
Query: Protein sequence → BLAST/scan → InterPro domains → Predict function
Use: Annotate novel proteins with predicted functional domains
```

**2. Domain Architecture Analysis**
```
Query: InterPro:IPR000001 (Kringle) → Get protein count → Find proteins with Kringle domains
Use: Study domain distribution and multi-domain architectures
```

**3. Signature Database Integration**
```
Query: InterPro entry → Get member database signatures (Pfam, SMART, etc.)
Use: Cross-reference predictions from multiple methods
```

**4. Functional Site Identification**
```
Query: InterPro entries with type=active_site → Identify catalytic residues
Use: Enzyme mechanism studies, site-directed mutagenesis planning
```

**5. Evolutionary Analysis**
```
Query: InterPro homologous_superfamily → Trace evolutionary relationships
Use: Comparative genomics, protein evolution studies
```

**6. Structural Annotation**
```
Query: InterPro domain → PDB cross-references → 3D structures
Use: Structure-function relationship analysis
```

## Test Cases

**Current Tests** (9 total):
- 4 declarative tests (lookup, attributes, multi-lookup, invalid ID)
- 5 custom tests (entry type, protein count, member databases, short name, relationships)

**Coverage**:
- ✅ InterPro ID lookup
- ✅ Attribute validation
- ✅ Entry type classification (domain, family, site, etc.)
- ✅ Protein count statistics
- ✅ Member database cross-references (Pfam, SMART, PROSITE, etc.)
- ✅ Short name searchability
- ✅ Cross-database relationships

**Recommended Additions**:
- Parent-child hierarchy navigation tests
- Entry type filtering (domains vs families vs sites)
- Literature reference validation

## Performance

- **Test Build**: ~2.3s (20 InterPro entries)
- **Data Source**: XML dump from EMBL-EBI (FTP download)
- **Update Frequency**: Regular releases (every 2-3 months)
- **Total Entries**: ~40,000+ InterPro entries integrating 13+ member databases

## Known Limitations

**Member Database Completeness**:
- Not all member databases may be configured in biobtree
- Cross-references only stored for databases in configuration

**Hierarchy Navigation**:
- Parent-child relationships stored but not heavily tested
- Hierarchical queries may require additional implementation

**Protein Sequences**:
- InterPro stores signatures, not actual protein sequences
- Must link to UniProt/other databases for sequence data

## Future Work

- Add comprehensive hierarchy navigation tests (parent → children, child → parents)
- Test filtering by entry type (domain, family, site, repeat)
- Add tests for different member database cross-references
- Test literature reference validation
- Add GO term association tests (InterPro entries linked to GO)

## Maintenance

- **Release Schedule**: Every 2-3 months from InterPro
- **Data Format**: XML (stable format)
- **Test Data**: Fixed 20 InterPro IDs spanning various entry types
- **Member Databases**: Pfam, PROSITE, SMART, PRINTS, ProDom, CDD, HAMAP, PANTHER, PIRSF, SFLD, SUPERFAMILY, TIGRFAMs, NCBIfam

## References

- **Citation**: Paysan-Lafosse T et al. InterPro in 2022. Nucleic Acids Research.
- **Website**: https://www.ebi.ac.uk/interpro/
- **License**: CC0 (Public Domain) - freely available for all uses
