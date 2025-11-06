# Ensembl Bacteria (Ensembl Genomes Division)

## Overview

Ensembl Bacteria provides comprehensive genome annotations for bacterial and archaeal organisms, part of the Ensembl Genomes project. Contains 50,000+ bacterial genomes with gene annotations including genomic coordinates, biotypes, and functional descriptions. Data structure mirrors main Ensembl but uses different ID formats and API endpoints specific to bacterial genomes.

**Source**: Ensembl Genomes REST API (rest.ensemblgenomes.org)
**Data Type**: Bacterial gene annotations with genomic coordinates
**Division**: Bacteria (one of 5 Ensembl Genomes divisions)

## Current Status

⚠️ **Testing Limitations**: The Ensembl Genomes REST API (rest.ensemblgenomes.org) currently has SSL certificate issues, preventing automatic extraction of reference test data. As a result:

- Test data builds successfully from Ensembl Genomes
- Data is correctly stored in biobtree database
- Reference data extraction for validation is not possible
- Tests currently fail due to empty reference_data.json

## Integration Architecture

### Storage Model

**Same as Main Ensembl** (see tests/ensembl/README.md):
- Gene IDs with different format (e.g., `E1CxwDWD5uDlWpi`)
- ID prefix format: `{species_name}:{division}:{gene_id}`
- Example: `escherichia_coli_110957_gca_000485615:ENSB:E1CxwDWD5uDlWpi`
- All same attributes as main Ensembl (display_name, biotype, coordinates, etc.)

**Key Differences from Main Ensembl**:
- Different API endpoint (rest.ensemblgenomes.org vs rest.ensembl.org)
- Different ID format (encoded IDs vs ENSG-style)
- Species names include assembly identifiers
- Division prefix "ENSB" (Ensembl Bacteria)

### Special Features

**Extensive Bacterial Coverage**:
- 50,000+ bacterial and archaeal genomes
- Model organisms and pathogens well-represented
- Pan-genome analyses supported
- Strain-level annotations

**Shared with Main Ensembl**:
- Same data model and attributes
- Compatible biotype classifications
- Cross-references to UniProt, RefSeq, GO, etc.
- REST API structure (when SSL issues resolved)

## Use Cases

**Same as Main Ensembl** (see tests/ensembl/README.md):
1. Gene ID to symbol mapping
2. Genomic coordinate lookup
3. Biotype-based filtering
4. Cross-database integration
5. Variant annotation
6. Comparative genomics (bacterial focus)

**Additional Bacterial-Specific**:
- Pan-genome analyses across bacterial strains
- Pathogen gene identification
- Antibiotic resistance gene discovery
- Metabolic pathway reconstruction

## Test Cases

**Current Status**: 7 tests configured, 6 failing due to empty reference data
- 4 declarative tests (JSON-based)
- 3 custom tests (Python logic)

**Why Tests Fail**:
- reference_data.json is empty `[]`
- Cannot extract data from Ensembl Genomes API (SSL issues)
- Tests require reference data for validation

**Test Coverage (when functional)**:
- ✅ Basic gene ID lookup
- ✅ Attribute presence validation
- ✅ Multiple ID batch lookup
- ✅ Invalid ID handling
- ✅ Gene symbol check
- ✅ Biotype annotation
- ✅ Strand orientation

## Performance

- **Test Build**: ~3s (20 genes from E. coli)
- **Data Source**: REST API (currently unavailable)
- **Full Build**: Hours (for selected bacterial genomes)
- **Total Genes**: Millions (depends on genome selection)
- **Test Organism**: Escherichia coli (1268975)
- **Test Database**: Shared with other Ensembl divisions

## Known Limitations

**API Access Issues** (Critical):
- Ensembl Genomes REST API has SSL certificate problems
- Cannot fetch reference data for test validation
- Issue affects all 5 Ensembl Genomes divisions
- Workaround: Use biobtree API directly (requires running server)

**Data Build Works**:
- Despite API issues, data builds successfully
- biobtree integrates data correctly
- Problem is only with test reference extraction

**Shared Limitations with Main Ensembl**:
- Assembly-specific coordinates
- Annotation quality varies by organism
- Some genomes lack rich cross-references

**Division-Specific**:
- Bacterial gene IDs less human-readable than ENSG format
- Species names include assembly info (longer strings)
- Not all bacterial genomes included (selective coverage)

## Future Work

**Immediate (SSL Issue Resolution)**:
- Monitor Ensembl Genomes SSL certificate fix
- Update extract_reference_data.py when API accessible
- Re-run reference extraction
- Validate all tests pass

**Alternative Approaches**:
- Extract reference data from biobtree database directly
- Use main Ensembl test data as template
- Create minimal synthetic reference data
- Document SSL workaround procedures

**Long-term**:
- Add bacteria-specific tests (pathogen genes, resistance markers)
- Pan-genome analysis test cases
- Strain comparison tests
- Metabolic pathway annotation tests

## Maintenance

- **Release Schedule**: Quarterly (synced with Ensembl)
- **Current Version**: Ensembl Genomes 60 (November 2024)
- **Data Format**: REST API JSON (when accessible)
- **Test Data**: E. coli genes (taxonomy 1268975)
- **License**: Open data
- **API Docs**: https://rest.ensemblgenomes.org/

## Relationship to Other Ensembl Datasets

Ensembl Bacteria is one of 5 Ensembl Genomes divisions:
1. **ensembl** - Main Ensembl (vertebrates) ✅ Working
2. **ensembl_bacteria** - Bacteria & Archaea ⚠️ SSL issues
3. **ensembl_fungi** - Fungi ⚠️ SSL issues
4. **ensembl_metazoa** - Invertebrate animals ⚠️ SSL issues
5. **ensembl_plants** - Plants ⚠️ SSL issues
6. **ensembl_protists** - Protists ⚠️ SSL issues

**Shared Features**:
- All use same data model
- Built together with --genome-taxids flag
- Share database in biobtree
- Only API endpoints differ

**Testing Strategy**:
- Main Ensembl has full test coverage
- Genomes divisions await SSL fix
- Consider merged testing approach

## References

- **Citation**: Yates AD, et al. (2022) Ensembl Genomes 2022: an expanding genome resource for non-vertebrates. Nucleic Acids Res. 50(D1):D996-D1003.
- **Website**: https://bacteria.ensembl.org/
- **REST API**: https://rest.ensemblgenomes.org/ (SSL issues)
- **FTP**: https://ftp.ensemblgenomes.ebi.ac.uk/pub/bacteria/
- **License**: Open data
