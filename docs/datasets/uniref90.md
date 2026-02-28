# UniRef90 (UniProt Reference Clusters 90%) Dataset

## Overview

UniRef90 provides clustered sets of protein sequences with ≥90% sequence identity and ≥80% overlap, offering a balance between redundancy reduction and sequence specificity. Each cluster contains a representative sequence and members from UniProtKB and UniParc, ideal for detailed comparative genomics and protein family studies. Contains ~79 million clusters from 600M+ sequences, providing 68% size reduction while maintaining high sequence resolution for distinguishing closely related proteins and isoforms.

**Source**: UniProt FTP (EBI)
**Data Type**: Protein sequence clusters at 90% identity threshold

## Integration Architecture

### Storage Model

**Primary Entries**:
- UniRef90 IDs (e.g., `UniRef90_A0A009IHW8`) stored as main identifiers

**Attributes**: Same structure as UniRef50 (see tests/uniref50/README.md) with 90% identity clustering

**Cross-References**:
- **UniRef50**: Coarser clusters (50% identity)
- **UniRef100**: Finer clusters (100% identity)
- **UniParc**: Sequence archive entries
- **UniProtKB**: Source protein annotations
- **Taxonomy**: Organism classification

### Special Features

**90% Identity Clustering**:
- Sequences ≥90% identical and ≥80% overlap
- Moderate redundancy reduction (68%)
- Good balance between diversity and specificity
- Distinguishes closely related paralogs and isoforms

**Hierarchical Integration**:
- UniRef100 (100%) → **UniRef90 (90%)** → UniRef50 (50%)
- Ideal intermediate clustering level
- Most commonly used for similarity searches

## Use Cases

**1. Homology Searches with Good Specificity**
```
Query: Sequence → BLAST vs. UniRef90 → Precise functional annotation
Use: Balance between speed and distinguishing closely related proteins
```

**2. Paralog Distinction**
```
Query: Gene family → UniRef90 clusters → Separate paralogs
Use: Distinguish between closely related family members
```

**3. Isoform Analysis**
```
Query: Protein → UniRef90 → Member isoforms
Use: Identify alternative splicing products and variants
```

**4. Species-Specific Analysis**
```
Query: Organism-specific proteins → UniRef90 clusters
Use: Study lineage-specific protein evolution
```

**5. Annotation Transfer**
```
Query: Novel sequence → UniRef90 (90% match) → Detailed annotation
Use: Transfer annotations with high confidence
```

**6. Standard Similarity Searches**
```
Query: Use UniRef90 for routine BLAST searches
Use: Recommended default for most applications (68% faster)
```

## Test Cases & Performance

**Tests**: 7/7 passing (4 declarative + 3 custom) - identical structure to UniRef50
**Build Time**: ~0.3s (10 clusters)
**Total Clusters**: ~79 million (68% reduction from 236M UniProtKB)
**Full Build**: 30-60 minutes

## Comparison with Other UniRef Levels

| Metric | UniRef100 | **UniRef90** | UniRef50 |
|--------|-----------|--------------|----------|
| **Identity** | 100% | **≥90%** | ≥50% |
| **Clusters** | ~236M | **~79M** | ~9M |
| **Reduction** | ~0% | **68%** | 94% |
| **Best For** | Duplicates | **General use** | Families |

**UniRef90 Advantages**:
- **Most commonly used** clustering level
- Good redundancy reduction (68%)
- Maintains sequence specificity
- Distinguishes isoforms and close paralogs
- Recommended default for similarity searches

## Known Limitations

See tests/uniref50/README.md for common UniRef limitations. Additionally:
- **90% threshold** may still group some divergent isoforms
- **Computational cost** higher than UniRef50 but acceptable
- **Cluster sizes** vary widely (1 to 100,000+ members)

## Maintenance

- **Release Schedule**: Weekly from UniProt
- **Data Format**: Single gzipped XML file (~8GB compressed, ~100GB uncompressed)
- **License**: CC BY 4.0
- **Coordination**: Part of UniRef suite (UniRef50 ← UniRef90 → UniRef100)

## References

- **Citation**: UniProt Consortium (2025) UniProt: the Universal Protein Knowledgebase in 2025. Nucleic Acids Res. 53(D1):D523-D531.
- **Website**: https://www.uniprot.org/help/uniref
- **FTP**: https://ftp.uniprot.org/pub/databases/uniprot/uniref/uniref90/
- **License**: CC BY 4.0
