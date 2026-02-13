# ENCODE cCRE Dataset

## Overview
ENCODE cCRE (candidate cis-Regulatory Elements) from ENCODE SCREEN provides a comprehensive registry of 2.3M+ regulatory elements in the human genome (GRCh38). These elements include promoters, enhancers, and other regulatory regions identified through integrative analysis of chromatin accessibility and histone modification data.

**Source**: https://screen.encodeproject.org/
**Data Type**: Regulatory element annotations with genomic coordinates
**License**: ENCODE Data Use Policy

## Integration Architecture

### Storage Model
**Primary Entries**: cCRE accession format (e.g., "EH38E2776516")
**Searchable Text Links**: cCRE ID and classification indexed for text search
**Attributes Stored**: ccre_id, ccre_class, chromosome, start, end
**Cross-References**: Links to Taxonomy (human: 9606)

### Classifications
- **PLS**: Promoter-like signatures (H3K4me3 + DNase)
- **pELS**: Proximal enhancer-like signatures (<2kb from TSS)
- **dELS**: Distal enhancer-like signatures (>2kb from TSS)
- **CA-CTCF**: CTCF-bound chromatin accessible regions
- **CA-TF**: TF-bound chromatin accessible (not CTCF)
- **CA**: Chromatin accessible only
- **TF**: TF binding only (no chromatin accessibility)

### Special Features
- **Genomic coordinates**: Full chromosome, start, end positions stored
- **Classification filtering**: CEL filters for regulatory element type
- **Coordinate range queries**: Filter by genomic position
- **Text search by classification**: Search PLS, pELS, dELS, CA-CTCF, etc.

## Use Cases

**1. Find Regulatory Element**
```
Query: EH38E2776516 → Returns cCRE details
Use: Look up specific regulatory element by accession
```

**2. Search by Classification**
```
Query: PLS >> encode_ccre → Returns promoter-like sequences
Use: Find all promoter regions in the dataset
```

**3. Filter by Classification**
```
Query: EH38E2776516 >> encode_ccre[encode_ccre.ccre_class=="pELS"]
Use: Filter to proximal enhancer-like sequences only
```

**4. Filter by Chromosome**
```
Query: pELS >> encode_ccre[encode_ccre.chromosome=="chr1"]
Use: Find enhancers on specific chromosome
```

**5. Filter by Coordinate Range**
```
Query: PLS >> encode_ccre[encode_ccre.start>10000 && encode_ccre.end<100000]
Use: Find regulatory elements in genomic region
```

**6. Get Taxonomy**
```
Query: EH38E2776516 >> encode_ccre >> taxonomy
Use: Verify organism (human GRCh38)
```

## Test Cases

**Current Tests**:
- ID lookup (cCRE accession)
- Classification search (PLS, pELS, dELS, CA-CTCF)
- Attributes verification
- Multi-lookup
- Invalid ID handling

**Coverage**:
- cCRE accession format
- Classification text search
- Classification filtering
- Chromosome filtering
- Coordinate range filtering
- Taxonomy cross-references

## Performance

- **Data Source**: ENCODE S3 bucket (ENCFF420VPZ.bed.gz)
- **Update Frequency**: Periodic ENCODE releases
- **Total Entries**: ~2.3M cCREs (human GRCh38)
- **Processing**: BED file parsing, fast

## Known Limitations

- **Human only**: Currently only GRCh38 human cCREs
- **No activity scores**: Signal/score values not stored
- **No cell type data**: Cell type-specific activity not included
- **No biosample metadata**: Sample-level annotations not stored

## Future Work

- Add mouse (mm10) cCRE support
- Include cell type-specific activity data
- Add signal strength/score attributes
- Cross-reference to nearby genes (Ensembl)
- Link to FANTOM5 enhancers

## Maintenance

- **Release Schedule**: ENCODE periodic releases
- **Data Format**: BED9+1 format
- **Test Data**: 1000 entries

## References

- **Publication**: https://pubmed.ncbi.nlm.nih.gov/32728249/
- **ENCODE Portal**: https://www.encodeproject.org/
- **SCREEN**: https://screen.encodeproject.org/
- **License**: ENCODE Data Use Policy
