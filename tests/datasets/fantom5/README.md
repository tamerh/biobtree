# FANTOM5 Dataset

## Overview

FANTOM5 (Functional Annotation of the Mammalian Genome 5) is a comprehensive atlas of human promoters and enhancers based on CAGE (Cap Analysis of Gene Expression) technology. Contains ~185K promoters, ~65K enhancers, and ~20K gene-level expression profiles across 1,800+ human samples.

**Source**: RIKEN FANTOM Consortium
**Data Type**: CAGE expression data (promoters, enhancers, gene-level TPM)

## Integration Architecture

### Storage Model

**Primary Entries**: Three datasets with numeric IDs (1, 2, 3...)
- `fantom5_promoter` (ID: 126) - CAGE peaks/TSS regions
- `fantom5_enhancer` (ID: 127) - Active enhancer regions
- `fantom5_gene` (ID: 128) - Gene-level expression aggregation

**Searchable Text Links**:
- Promoter: peak ID (chr:start..end,+), peak name (p1@TP53), gene symbol
- Enhancer: coordinate ID (chr:start-end), **associated gene symbols** (within 500kb)
- Gene: gene symbol, gene ID

**Attributes Stored** (protobuf):
- Promoter: coordinates, strand, gene annotations, TPM stats, expression breadth, top tissues/cell types
- Enhancer: coordinates, TPM stats, **associated_genes** (nearby genes within 500kb)
- Gene: symbol, TPM stats, expression breadth, top tissues

**Cross-References**:
- All datasets: taxonomy (9606), uberon (tissue expression)
- Promoter: ensembl, hgnc, entrez, uniprot, cl (cell types)
- Enhancer: ensembl, hgnc, entrez (via proximity-based gene mapping)
- Gene: ensembl, entrez

### Special Features

1. **Parent/Child Dataset Pattern**: `fantom5_promoter` triggers processing of all three datasets
2. **Proximity-Based Enhancer-Gene Mapping**: Enhancers linked to genes within 500kb threshold
3. **Expression Breadth Classification**: ubiquitous, broad, tissue_specific, not_expressed
4. **Bidirectional Gene Lookup**: Search enhancers by gene symbol (TP53 >> fantom5_enhancer)

## Use Cases

**1. Gene Regulatory Landscape**
```
Query: "What are the promoters for TP53?" -> TP53 >> fantom5_promoter
Use: Identify TSS regions and alternative promoters for a gene
```

**2. Enhancer-Gene Associations**
```
Query: "Which enhancers might regulate BRCA1?" -> BRCA1 >> hgnc >> fantom5_enhancer
Use: Find regulatory elements near a gene of interest (within 500kb)
```

**3. Tissue-Specific Expression**
```
Query: "Where is this gene highly expressed?" -> entry(1, fantom5_gene) -> top_tissues
Use: Identify tissue-specific expression patterns for biomarker discovery
```

**4. Expression Breadth Analysis**
```
Query: "Find tissue-specific promoters" -> filter by expression_breadth=="tissue_specific"
Use: Identify cell-type-specific regulatory regions for targeted therapies
```

**5. Multi-Omics Integration**
```
Query: "TF targets with expression" -> TP53 >> collectri >> ensembl >> fantom5_gene
Use: Combine TF regulation data with expression profiles
```

**6. Reverse Lookup**
```
Query: "Which genes are near this enhancer?" -> entry(1, fantom5_enhancer) -> associated_genes
Use: Identify potential target genes for a regulatory element
```

## Test Cases

**Current Tests** (19 total):
- 10 declarative tests (search, map, entry for all 3 datasets)
- 9 custom tests (coordinates, gene associations, xrefs, expression data)

**Coverage**:
- Search by gene symbol (promoter, enhancer, gene)
- Mapping through HGNC/Ensembl to all datasets
- Entry retrieval with attribute validation
- Enhancer-gene associations and xrefs
- Expression breadth classification

**Recommended Additions**:
- Filter tests (TPM thresholds, expression breadth)
- UBERON/CL tissue mapping tests
- Combined queries with CollecTRI

## Performance

- **Test Build**: ~30s (500 entries per dataset)
- **Data Source**: FANTOM5 reprocessed hg38 data
- **Update Frequency**: Static dataset (2014 release, hg38 lift-over 2019)
- **Total Entries**: ~185K promoters, ~65K enhancers, ~20K genes
- **Special notes**: Downloads ~200MB of expression matrices

## Known Limitations

1. **No correlation-based enhancer-gene links**: Original FANTOM5 correlation data unavailable (slidebase.binf.ku.dk down). Using proximity-only (500kb threshold).
2. **Mouse data not included**: Only human (hg38) currently integrated.
3. **No TF binding site predictions**: JASPAR motif scanning on enhancers not implemented.
4. **Expression values are TPM**: Not raw counts or normalized values.

## Future Work

1. **JASPAR Integration**: Scan enhancer sequences for TF binding motifs
2. **Correlation Data**: If original FANTOM5 enhancer-TSS correlations become available
3. **Mouse Support**: Add mm10 FANTOM5 data
4. **ENCODE Integration**: Link enhancers to ENCODE cCRE regulatory elements

## Maintenance

- **Release Schedule**: Static (original FANTOM5 2014, hg38 reprocessed 2019)
- **Data Format**: Gzipped TSV/BED files
- **Test Data**: 500 entries per dataset
- **License**: CC BY 4.0
- **Gene Coordinates**: Uses Ensembl GFF3 for proximity mapping

## References

- **Citation**: FANTOM Consortium, Nature 507:462-470 (2014)
- **Website**: https://fantom.gsc.riken.jp/5/
- **License**: CC BY 4.0
