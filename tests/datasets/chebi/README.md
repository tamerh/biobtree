# ChEBI (Chemical Entities of Biological Interest) Dataset

## Overview

ChEBI is a freely available dictionary of molecular entities focused on small chemical compounds. biobtree now provides **complete ChEBI integration** with full compound metadata including chemical structures, nomenclature, and ontology classifications.

**Source**: ChEBI database flat files (FTP download)
**Data Type**: **Full compound database** with attributes and cross-references

## Integration Architecture

### Storage Model - Full Compound Database

ChEBI is integrated as a complete compound database with:

**Primary Searchable Entries**:
```
ChEBI ID → Compound entry with full attributes → Cross-references to other databases
```

**What's Stored**:
- ✅ **Names**: Compound names, synonyms, IUPAC names, INN names, brand names
- ✅ **Chemical Identifiers**: Molecular formula, SMILES, InChI, InChI Key
- ✅ **Properties**: Molecular weight (average and monoisotopic), charge
- ✅ **Classifications**: ChEBI ontology roles and parent relationships
- ✅ **Metadata**: Definition, star rating (quality indicator), source
- ✅ **Cross-References**: Links to UniProt, GO, and other biological databases

### Data Processing

**Primary Data Sources** (all from ChEBI FTP):
1. **compounds.tsv.gz** - Basic compound information (ID, name, source, status, stars)
2. **names.tsv.gz** - Synonyms, IUPAC names, INN names, brand names
3. **chemical_data.tsv.gz** - Molecular formula, mass, charge
4. **structures.csv.gz** - SMILES, InChI, InChI Key
5. **relation.tsv.gz** - Ontology relationships (roles, parent compounds)
6. **database_accession.tsv.gz** - Cross-references to external databases

**Processing Pipeline**:
1. Load and map all data files using internal ChEBI IDs
2. Build complete compound attributes from merged data
3. Create primary searchable entries with full attributes
4. Generate text search keywords (name, synonyms, formula, InChI Key)
5. Process cross-references to external databases
6. Store ontology relationships (roles, parents)

**Deterministic Test Mode**:
- Test IDs are sorted to ensure reproducible test builds
- Same 100 compounds selected every time for consistent testing

### Search Capabilities

ChEBI compounds are searchable by:
- **ChEBI ID**: Direct lookup (e.g., `CHEBI:15377`)
- **Compound Name**: Primary name (e.g., `"water"`)
- **Synonyms**: All synonyms including IUPAC, INN, brand names
- **Formula**: Molecular formula (e.g., `"C16H14O4"`)
- **InChI Key**: Structure identifier (case-sensitive)

### Special Features

**Property-Only Entry Support**:
- Some ChEBI compounds have no external cross-references
- These are handled via special merge logic in `mergeg.go`
- Entries retrievable even without xrefs (attributes alone)

**Ontology Integration**:
- **Roles**: Chemical roles (e.g., inhibitor, metabolite)
- **Parents**: Parent compounds in chemical classification hierarchy
- Stored as ChEBI IDs for ontology navigation

**Quality Indicators**:
- **Star Rating**: 1-3 stars indicating data quality/curation level
- **Status**: Compound status (active, obsolete, etc.)
- **Source**: Original database (KEGG, ChEMBL, manually curated)

## Use Cases

**1. Compound Lookup and Information**
```
Query: CHEBI:15377 → Get complete compound data
Result: Name, formula, structure, mass, classifications
Use: Chemical information retrieval, structure browsing
```

**2. Chemical Structure Search**
```
Query: InChI Key → Find compound with matching structure
Result: Full compound entry with all metadata
Use: Structure-based compound identification
```

**3. Synonym-Based Discovery**
```
Query: Brand name or common name → Find official compound
Result: ChEBI entry with standardized identifiers
Use: Drug name resolution, compound standardization
```

**4. Formula-Based Searching**
```
Query: C16H14O4 → Find all compounds with this formula
Result: List of isomers and related compounds
Use: Isomer discovery, formula-based compound finding
```

**5. Ontology Navigation**
```
Query: ChEBI compound → Get roles and parent compounds
Result: Chemical classification and relationships
Use: Understanding compound function and classification
```

**6. Cross-Database Linking**
```
Query: ChEBI ID → Get cross-references to UniProt/GO
Result: Related proteins, pathways, biological processes
Use: Connecting chemistry to biology
```

**7. Metabolic Network Construction**
```
Query: Multiple ChEBI compounds → Build chemical networks
Result: Compound relationships via ontology
Use: Metabolism modeling, pathway analysis
```

## Test Cases

**Declarative Tests** (9 total):
1. **ID Lookup**: Retrieve compound by ChEBI ID
2. **Name Lookup**: Search by compound name
3. **Synonym Lookup**: Find by synonym
4. **Formula Lookup**: Search by molecular formula
5. **InChI Key Lookup**: Find by structure identifier
6. **Attribute Check**: Verify attributes present
7. **Multi-Lookup**: Batch retrieval of multiple IDs
8. **Case-Insensitive**: Test case-insensitive search
9. **Invalid ID**: Error handling for non-existent IDs

**Custom Tests** (3 total):
1. **Full Attributes**: Verify name, definition, formula, structures
2. **IUPAC Names**: Check IUPAC nomenclature storage
3. **Cross-References**: Validate external database links

**Coverage**:
- ✅ Primary entry retrieval
- ✅ Text search (names, synonyms, formulas)
- ✅ Attribute completeness
- ✅ Structure identifiers (SMILES, InChI)
- ✅ Chemical properties (mass, formula)
- ✅ Ontology relationships
- ✅ Cross-reference integrity

## Performance

- **Test Build**: ~8s (100 compounds with full data)
- **Data Processing**:
  - Loads ~62K base compounds
  - Processes ~204K ID mappings
  - Merges ~66K names
  - Loads ~55K chemical formulas
  - Processes ~54K structures
  - Handles ~97K ontology relationships
- **Cross-References**: ~285 xrefs created in test mode
- **Update Frequency**: Monthly releases from ChEBI
- **Total Compounds**: 200,000+ chemical entities

## Data Quality

**Star Ratings**:
- ⭐⭐⭐ (3 stars): Manually annotated, highest quality
- ⭐⭐ (2 stars): Imported from trusted sources
- ⭐ (1 star): Preliminary or computationally derived

**Sources**:
- Manual curation by ChEBI team
- KEGG COMPOUND
- ChEMBL
- Other chemical databases

## Known Limitations

**InChI Key Case Sensitivity**:
- InChI Keys are case-sensitive in biobtree
- Ensure exact case when searching by structure identifier

**Ontology Depth**:
- Only direct roles and parents stored
- Full ontology traversal requires recursive queries
- Grandparent/ancestor relationships not pre-computed

**External Cross-Reference Coverage**:
- Not all compounds have cross-references to biological databases
- Some compounds are chemistry-only (no protein/GO links)
- Cross-reference availability depends on compound's biological relevance

**Test Mode Limitations**:
- Only 100 compounds in test database
- May not cover all compound types
- Cross-reference testing limited to test set

## Implementation Details

**Source Code**:
- Parser: `src/update/chebi.go`
- Protobuf: `src/pbuf/attr.proto` (ChebiAttr message)
- Merge Logic: `src/generate/mergeg.go` (property-only entry handling)
- Filter Support: `src/service/service.go`, `src/service/mapfilter.go`

**Key Features**:
- Deterministic test mode with sorted IDs
- Memory-efficient streaming of large files
- Comprehensive error handling
- Progress reporting during data load

**Property-Only Entry Handling**:
- Special case in merge logic (line 1071-1075 in mergeg.go)
- Enables retrieval of compounds without external xrefs
- Uses `Xref_Chebi` attribute wrapper

## Reference Data

**Test Reference Data**: `reference_data.json`
- Source: biobtree API (local test database)
- 20 representative compounds with full attributes
- Used for automated test validation

**Extraction**: `extract_reference_data.py`
- Queries biobtree API for test IDs
- Saves complete JSON responses
- Run after test build to regenerate reference data

## Maintenance

**Update Process**:
1. Download latest ChEBI release from FTP
2. Run data processing: `./biobtree update`
3. Regenerate test data: `./biobtree -d chebi test`
4. Update reference: `cd tests/datasets/chebi && python3 extract_reference_data.py`
5. Run tests: `python3 tests/validate_biobtree.py`

**Release Schedule**: Monthly updates from ChEBI

**Test Data**:
- Generate: `./biobtree -d chebi test`
- Copy IDs: `cp test_out/reference/chebi_ids.txt tests/datasets/chebi/`
- Extract reference: `python3 extract_reference_data.py`

## References

- **Citation**: Hastings J et al. The ChEBI reference database and ontology for biologically relevant chemistry. Nucleic Acids Research.
- **Website**: https://www.ebi.ac.uk/chebi/
- **FTP**: ftp://ftp.ebi.ac.uk/pub/databases/chebi/Flat_file_tab_delimited/
- **License**: CC BY 4.0 (freely available)
- **Ontology**: Chemical ontology with roles and classification

## Future Enhancements

### Potential Improvements

**Advanced Querying**:
- Substructure search (requires chemical fingerprinting)
- Similarity search based on structure
- Mass range queries
- Formula pattern matching

**Ontology Expansion**:
- Pre-compute full ancestor paths
- Add sibling relationship queries
- Enable ontology-based filtering

**Enhanced Cross-References**:
- Bi-directional navigation to biological entities
- Cross-reference enrichment statistics
- Pathway integration via compound roles

**Performance Optimization**:
- Index chemical formulas for faster searching
- Cache common structure queries
- Optimize memory usage for full database builds

---

## Migration from Previous Version

**What Changed**:
- ❌ OLD: Cross-reference-only dataset (no compound metadata)
- ✅ NEW: Full compound database with complete attributes

**Backward Compatibility**:
- Cross-references still maintained (external database links)
- ChEBI IDs still searchable
- Enhanced with compound metadata and search capabilities

**Benefits of New Integration**:
- Direct compound lookup without external queries
- Rich chemical structure information
- Ontology navigation within biobtree
- Synonym-based chemical search
- Better integration for drug discovery and metabolism research
