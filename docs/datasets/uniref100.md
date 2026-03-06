# UniRef100 (UniProt Reference Clusters 100%) Dataset

## Overview

UniRef100 provides clustered sets of 100% identical protein sequences and sub-fragments, serving primarily as a duplicate removal layer. Each cluster combines identical sequences and their fragments from UniProtKB and UniParc under a single representative, reducing storage and computational overhead while preserving all unique sequences. Contains ~236 million clusters (minimal reduction from full UniProtKB), providing complete sequence coverage with only exact duplicates removed - essential for exhaustive similarity searches requiring maximum sensitivity.

**Source**: UniProt FTP (EBI)
**Data Type**: Protein sequence clusters at 100% identity threshold

## Integration Architecture

### Storage Model

**Primary Entries**:
- UniRef100 IDs (e.g., `UniRef100_P12345`) stored as main identifiers

**Attributes**: Same structure as UniRef50 (see tests/uniref50/README.md) with 100% identity clustering

**Cross-References**:
- **UniRef90**: Moderate granularity clusters (90% identity)
- **UniRef50**: Coarse clusters (50% identity)
- **UniParc**: Sequence archive entries
- **UniProtKB**: Source protein annotations
- **Taxonomy**: Organism classification

### Special Features

**100% Identity Clustering**:
- Only exact sequence matches clustered
- Sub-fragments included in parent cluster
- Minimal redundancy reduction (~0-5%)
- Maximum sequence specificity

**Hierarchical Integration**:
- **UniRef100 (100%)** → UniRef90 (90%) → UniRef50 (50%)
- Base level for clustering hierarchy
- Foundation for coarser clustering levels

## Use Cases

**1. Exhaustive Similarity Searches**
```
Query: Sequence → BLAST vs. UniRef100 → Maximum sensitivity
Use: When every unique sequence variant matters
```

**2. Exact Duplicate Identification**
```
Query: Sequence → UniRef100 cluster → Find all identical copies
Use: Identify redundant submissions and database artifacts
```

**3. Fragment Analysis**
```
Query: Partial sequence → UniRef100 → Full-length parent
Use: Identify fragments and their complete sequences
```

**4. Multi-Organism Exact Matches**
```
Query: Conserved sequence → UniRef100 cluster members
Use: Find identical sequences across multiple species
```

**5. Annotation Consolidation**
```
Query: UniRef100 cluster → All member annotations
Use: Aggregate information from identical sequence entries
```

**6. Base for Coarser Clustering**
```
Query: Start with UniRef100 → Navigate to UniRef90/50
Use: Begin with maximum specificity, then broaden search
```

## Test Cases & Performance

**Tests**: 7/7 passing (4 declarative + 3 custom) - identical structure to UniRef50
**Build Time**: ~0.3s (10 clusters)
**Total Clusters**: ~236 million (~0-5% reduction from 236M UniProtKB)
**Full Build**: 1-2 hours

## Comparison with Other UniRef Levels

| Metric | **UniRef100** | UniRef90 | UniRef50 |
|--------|---------------|----------|----------|
| **Identity** | **100%** | ≥90% | ≥50% |
| **Clusters** | **~236M** | ~79M | ~9M |
| **Reduction** | **~0%** | 68% | 94% |
| **Best For** | **Exact duplicates** | General | Families |

**UniRef100 Advantages**:
- **Maximum sensitivity** - no sequence loss
- **Exact match detection**
- **Fragment consolidation**
- **Complete sequence coverage**

**UniRef100 Disadvantages**:
- **Minimal size reduction** - nearly as large as full UniProtKB
- **Slower searches** compared to UniRef90/50
- **High redundancy** for most applications

## Known Limitations

See tests/uniref50/README.md for common UniRef limitations. Additionally:
- **Minimal redundancy reduction** makes searches slow
- **Large database size** (~236M clusters)
- **100% threshold** means even single residue differences create separate clusters
- **Not recommended** for routine similarity searches (use UniRef90 instead)

## Maintenance

- **Release Schedule**: Weekly from UniProt
- **Data Format**: Single gzipped XML file (~15GB compressed, ~180GB uncompressed)
- **License**: CC BY 4.0
- **Coordination**: Part of UniRef suite (UniRef100 → UniRef90 → UniRef50)

## When to Use UniRef100

**Use UniRef100 when**:
- Maximum sensitivity required
- Exact duplicates must be identified
- Fragment analysis needed
- Complete sequence coverage essential

**Use UniRef90 instead for**:
- Routine similarity searches (recommended default)
- Faster computational performance
- Good balance of sensitivity and speed

**Use UniRef50 instead for**:
- Protein family analysis
- Large-scale comparative genomics
- Maximum speed with acceptable sensitivity

## References

- **Citation**: UniProt Consortium (2025) UniProt: the Universal Protein Knowledgebase in 2025. Nucleic Acids Res. 53(D1):D523-D531.
- **Website**: https://www.uniprot.org/help/uniref
- **FTP**: https://ftp.uniprot.org/pub/databases/uniprot/uniref/uniref100/
- **License**: CC BY 4.0
