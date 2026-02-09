# CollecTRI Dataset

## Overview
CollecTRI is a comprehensive collection of transcription factor (TF) - target gene regulatory interactions curated from multiple databases. It provides evidence-backed relationships showing which transcription factors regulate which target genes, including regulation direction (activation/repression) and supporting literature.

**Source**: https://zenodo.org/record/7773985
**Data Type**: TF-target gene regulatory network

## Integration Architecture

### Storage Model
**Primary Entries**: TF:TG format (e.g., "MYC:TERT") - unique identifier for each regulatory interaction
**Searchable Text Links**: Both TF gene symbol and target gene symbol are indexed for text search
**Attributes Stored**: tf_gene, target_gene, sources (evidence databases), pmids, regulation (Activation/Repression/Unknown), confidence (High/Low)
**Cross-References**: Links to HGNC via gene symbols for both TF and target genes

### Special Features
- **Multi-source evidence**: Each interaction aggregates evidence from up to 12 databases (ExTRI, HTRI, TRRUST, TFactS, GOA, IntAct, SIGNOR, CytReg, GEREDB, NTNU Curated, Pavlidis2021, DoRothEA_A)
- **TRED exclusion**: TRED source is excluded for licensing reasons; entries with only TRED evidence are not included
- **Bidirectional gene search**: Searching for any gene finds all interactions where it's either TF or target
- **Regulation direction**: Captures whether TF activates or represses target gene expression
- **Literature support**: PubMed IDs for experimental evidence

## Use Cases

**1. Find TF Regulators of a Gene**
```
Query: What transcription factors regulate TERT? → TERT >> hgnc >> collectri → MYC, SP1, etc.
Use: Identify upstream regulators for gene therapy or drug targeting
```

**2. Identify TF Target Genes**
```
Query: What genes does MYC regulate? → MYC >> hgnc >> collectri → TERT, CDKN1A, etc.
Use: Understand TF function and downstream effects
```

**3. Filter by Regulation Type**
```
Query: Which TFs activate a gene? → gene >> hgnc >> collectri[collectri.regulation=="Activation"]
Use: Find activators for gene upregulation studies
```

**4. High-Confidence Interactions**
```
Query: Get well-supported interactions → gene >> hgnc >> collectri[collectri.confidence=="High"]
Use: Focus on experimentally validated relationships
```

**5. Multi-Evidence Interactions**
```
Query: Find interactions with multiple sources → Review sources array in results
Use: Prioritize interactions confirmed by independent studies
```

**6. Literature Mining**
```
Query: Get PubMed references for interaction → Check pmids attribute
Use: Access original experimental evidence
```

## Test Cases

**Current Tests** (16 total):
- 7 declarative tests (ID lookup, TF search, target search, attributes, multi-lookup, case-insensitive, invalid ID)
- 9 custom tests (TF:TG structure, activation regulation, repression regulation, high confidence, multiple sources, TRED exclusion verification, PMIDs, TF gene search, HGNC cross-references)

**Coverage**:
- TF:TG identifier format
- Gene symbol text search
- Regulation direction attributes
- Confidence levels
- Evidence source aggregation
- TRED licensing exclusion
- HGNC cross-references

**Recommended Additions**:
- Mapping chain tests (gene >> hgnc >> collectri)
- Filter expression tests
- Large-scale TF network queries

## Performance

- **Test Build**: ~0.5s (100 entries)
- **Data Source**: Zenodo (static TSV file)
- **Update Frequency**: Dataset version-based (currently v7773985)
- **Total Entries**: ~43,000 TF-target interactions
- **Special notes**: Single TSV download, fast parsing

## Known Limitations

- **TRED excluded**: Entries with only TRED as evidence source are not included (licensing)
- **Gene symbol based**: Cross-references depend on HGNC gene symbol matching
- **Human-focused**: Primarily human TF-target interactions
- **Static dataset**: Not automatically updated; requires manual version check

## Future Work

- Add Ensembl gene ID cross-references
- Support species-specific filtering
- Add TF family annotations
- Include interaction confidence scores from individual sources

## Maintenance

- **Release Schedule**: Zenodo versioned releases
- **Data Format**: Tab-separated values (TSV)
- **Test Data**: 100 entries
- **License**: Check Zenodo record for specific terms (TRED excluded for commercial reasons)

## References

- **Citation**: CollecTRI - Collection of Transcriptional Regulatory Interactions
- **Website**: https://zenodo.org/record/7773985
- **License**: See Zenodo record (TRED source excluded)
