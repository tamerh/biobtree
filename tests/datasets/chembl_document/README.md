# ChEMBL Document Dataset

## Overview

ChEMBL Document contains scientific literature sources (journal articles, patents, books) reporting bioactivity data. Each document record includes title, abstract, authors, journal information, publication year, and cross-references (PubMed, DOI, patent IDs). Essential for data provenance and literature mining, linking 90,000+ publications to ChEMBL bioactivity measurements. Every activity, assay, and molecule in ChEMBL traces back to a source document, enabling verification and deeper context.

**Source**: ChEMBL RDF (EMBL-EBI)
**Data Type**: Scientific literature metadata with PubMed and DOI cross-references

## Integration Architecture

### Storage Model

**Primary Entries**:
- Document IDs (e.g., `CHEMBL1121978`) stored as main identifiers

**Searchable Text Links**:
- Document IDs indexed for lookup

**Attributes Stored** (protobuf ChemblDocumentAttr):
- `title`: Full publication title
- `abstract`: Publication abstract text
- `authors`: Author list
- `doc_type`: Document classification (PUBLICATION, PATENT, BOOK)
- `journal`: Journal abbreviation
- `journal_full_title`: Full journal name
- `year`: Publication year
- `volume`: Journal volume
- `issue`: Journal issue
- `first_page`: Starting page number
- `last_page`: Ending page number
- `pubmed_id`: PubMed identifier
- `doi`: Digital Object Identifier
- `patent_id`: Patent number (for patent documents)

**Cross-References**:
- **Activities**: chembl_activity (bioactivity measurements from this publication)
- **Assays**: chembl_assay (experimental protocols from this publication)
- **Molecules**: chembl_molecule (compounds reported in this publication)
- **Targets**: chembl_target (targets studied in this publication)
- **PubMed**: NCBI PubMed (full text and citations)
- **Patents**: Patent databases via patent_id

### Special Features

**Literature Provenance**:
- Every ChEMBL bioactivity record linked to source document
- Enables data quality assessment and verification
- Tracks data lineage from publication to database

**Multi-Format Coverage**:
- Journal articles (PUBLICATION): peer-reviewed research
- Patents (PATENT): intellectual property filings
- Books (BOOK): chapters and monographs

**Full-Text Metadata**:
- Complete abstracts for text mining
- Author lists for collaboration analysis
- Journal information for impact assessment

**PubMed Integration**:
- Direct PubMed ID links to NCBI
- DOI links to publishers

## Use Cases

**1. Data Provenance Tracking**
```
Query: Activity → Document → PubMed ID → Full text verification
Use: Validate bioactivity measurements against original publications
```

**2. Literature-Based Drug Discovery**
```
Query: Target protein → Documents → Abstracts → Text mining for SAR insights
Use: Extract structure-activity relationships from literature
```

**3. Patent Analysis**
```
Query: Molecule → Documents with doc_type=PATENT → Patent IDs
Use: Freedom-to-operate and prior art searches
```

**4. Author Collaboration Networks**
```
Query: Documents → Author lists → Co-authorship networks
Use: Identify research collaborations and expertise
```

**5. Temporal Bioactivity Trends**
```
Query: Target → Activities → Documents → Publication years
Use: Track historical development of drug discovery programs
```

**6. Journal Impact Assessment**
```
Query: Documents → Journal → Count molecules/activities per journal
Use: Identify high-impact sources for bioactivity data
```

## Test Cases

**Current Tests** (9 total):
- 4 declarative tests (ID lookup, attributes, multi-lookup, invalid ID)
- 5 custom tests (doc type, journal, PubMed ID, DOI, year)

**Coverage**:
- ✅ Document ID lookup
- ✅ Document type classification (PUBLICATION, PATENT, BOOK)
- ✅ Journal information
- ✅ PubMed ID cross-references
- ✅ DOI cross-references
- ✅ Publication year tracking
- ✅ Attribute validation

**Recommended Additions**:
- Patent document type tests
- Book document type tests
- Abstract text search tests
- Author list validation tests
- Title search tests
- Volume/issue/page validation
- Cross-reference to activity/assay/molecule tests
- Publication year distribution tests

## Performance

- **Test Build**: ~4.9s (20 documents)
- **Data Source**: ChEMBL RDF (EMBL-EBI FTP)
- **Update Frequency**: Quarterly ChEMBL releases
- **Total Documents**: 90,000+ publications (articles, patents, books)
- **Note**: Sparse RDF handling for incomplete metadata

## Known Limitations

**Data Completeness**:
- Not all documents have DOIs (especially older publications)
- Abstract text missing for some patent documents
- Page numbers sometimes incomplete
- Some journals lack full title information

**Document Type Coverage**:
- Majority are journal articles (PUBLICATION)
- Patent coverage varies by source
- Book chapters relatively rare

**PubMed Coverage**:
- Not all publications have PubMed IDs
- Depends on whether journal indexed in PubMed
- Pre-1990s literature often lacks PubMed IDs

**Abstract Searchability**:
- Abstract text stored but keyword indexing depends on configuration
- Full-text search requires external tools
- Abstracts not available for all documents

**Cross-References**:
- Requires chembl_activity, chembl_assay, chembl_molecule for functional links
- Patent database integration depends on separate patent datasets

## Future Work

- Add patent document type tests
- Test book document type handling
- Add abstract text search validation
- Test author list parsing and search
- Add title search tests
- Test volume/issue/page extraction
- Add cross-reference validation (activity, assay, molecule)
- Test publication year distribution
- Add journal impact analysis tests
- Test DOI resolution
- Add PubMed ID validation tests
- Test patent ID format validation

## Maintenance

- **Release Schedule**: Quarterly from ChEMBL (currently v34+)
- **Data Format**: RDF/XML with sparse data handling
- **Test Data**: Fixed 20 document IDs (all journal articles in test sample)
- **License**: CC BY-SA 3.0 - freely available with attribution
- **Coordination**: Part of ChEMBL suite (document links to activity↔assay↔molecule↔target)

## Document Types

| Type | Description | Key Fields | Coverage |
|------|-------------|------------|----------|
| **PUBLICATION** | Journal articles | journal, pubmed_id, doi, abstract | ~80% |
| **PATENT** | Patent filings | patent_id, title, year | ~15% |
| **BOOK** | Book chapters | title, authors, year | ~5% |

## References

- **Citation**: Zdrazil B et al. (2024) The ChEMBL Database in 2023. Nucleic Acids Res. 52(D1):D1180-D1189.
- **Website**: https://www.ebi.ac.uk/chembl/
- **API**: https://www.ebi.ac.uk/chembl/api/data/docs
- **PubMed**: https://pubmed.ncbi.nlm.nih.gov/
- **License**: CC BY-SA 3.0
