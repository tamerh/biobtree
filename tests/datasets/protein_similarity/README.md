# Protein Similarity Dataset

## Overview

DIAMOND BLASTP protein sequence similarity database enabling fast homology searches across the proteome. Provides all-vs-all similarity results for UniProt proteins with comprehensive alignment statistics.

**Source**: DIAMOND (fast BLAST alternative) all-vs-all protein similarity search
**Data Type**: Protein sequence alignments with identity scores, e-values, bitscores, and alignment positions

## Integration Architecture

### Storage Model
**Primary Entries**: DIAMOND IDs with "d" prefix (e.g., `dP01942` for UniProt P01942)
**Searchable Text Links**: DIAMOND IDs indexed for text search
**Attributes Stored**: protein_id, similarities array (target proteins with alignment stats), similarity_count, top_identity, top_bitscore
**Cross-References**:
- DIAMOND ID → Original UniProt protein
- DIAMOND ID → Similar UniProt proteins (top N hits)

### Special Features
- **Unique ID Prefix**: "d" prefix prevents collisions with UniProt entries
- **Variable Hit Counts**: Each protein has 10-100 similar proteins (not fixed)
- **Bidirectional Xrefs**: Links both to original protein and all similar proteins
- **Alignment Metrics**: Full DIAMOND output preserved (12 fields per hit)
- **Fast Similarity Search**: Enables homology queries without BLAST

## Use Cases

**1. Cross-Organism Homology Discovery**
```
Query: Find human BRCA1 orthologs in model organisms → HBA1 >> uniprot >> protein_similarity >> uniprot >> taxonomy
Use: Identify functional homologs for comparative studies
```

**2. Protein Family Expansion**
```
Query: Discover related proteins by sequence similarity → dP01942 >> uniprot
Use: Build protein families from seed sequences
```

**3. Functional Annotation Transfer**
```
Query: Find well-characterized similar proteins → Unknown protein >> protein_similarity >> uniprot[reviewed=true]
Use: Transfer annotations from reviewed to unreviewed proteins
```

**4. Evolutionary Analysis**
```
Query: Map sequence conservation across species → Protein >> protein_similarity[identity>80] >> taxonomy
Use: Identify conserved vs. divergent protein regions
```

**5. Drug Target Homolog Identification**
```
Query: Find similar proteins for off-target prediction → Drug target >> protein_similarity >> uniprot >> chembl_target
Use: Predict potential drug binding sites in related proteins
```

**6. Structural Template Discovery**
```
Query: Find proteins with known structures → Novel protein >> protein_similarity >> alphafold
Use: Identify structural templates for homology modeling
```

## Test Cases

**Current Tests** (13 total):
- 5 declarative tests (lookup, attribute_exists checks)
- 8 custom tests (ID format, similarity data, alignment stats, cross-references, top scores)

**Coverage**:
- ✅ DIAMOND ID format validation ("d" prefix)
- ✅ Similarity array presence and structure
- ✅ Alignment statistics (identity, e-value, bitscore, positions)
- ✅ Target protein information (UniProt ID, name)
- ✅ Cross-references to original and similar proteins
- ✅ Top score calculations (top_identity, top_bitscore)

**Recommended Additions**:
- Identity threshold filtering tests
- E-value range validation
- Multi-hop similarity chains (A→B→C)
- Coverage percentage tests

## Performance

- **Test Build**: ~10-30 seconds (100 proteins with ~1000 total hits)
- **Data Source**: DIAMOND BLASTP all-vs-all on UniProt SwissProt + TrEMBL
- **Update Frequency**: When UniProt releases new data
- **Total Entries**: ~400,000-500,000 proteins (estimated)
- **Storage**: ~2-5 GB database size (estimated)
- **Processing**: Streaming TSV parser handles 40M lines without loading into memory

## Known Limitations

- **No bidirectional storage**: Similar proteins stored unidirectionally (A→B but not automatically B→A)
- **Self-hits included**: Query protein may appear in its own similarity list (100% identity)
- **Fixed snapshot**: Similarity pre-computed, not real-time BLAST
- **Top-N only**: Only stores top 10-100 hits per protein (configurable)
- **No alignment visualization**: Raw statistics only (no FASTA alignments)

## Future Work

- Add filtering by identity threshold
- Implement similarity score validation
- Add alignment coverage percentage
- Create protein family clustering
- Enable multi-hop similarity searches
- Add phylogenetic distance estimates

## Maintenance

- **Release Schedule**: Updated with each UniProt release
- **Data Format**: TSV (12 columns, DIAMOND BLASTP outfmt 6)
- **Test Data**: 100 proteins from filtered_top10.tsv (test mode)
- **License**: Data follows UniProt license terms
- **Dependencies**: Requires UniProt dataset for cross-references

## References

- **DIAMOND**: Buchfink B, Xie C, Huson DH. (2015) Fast and sensitive protein alignment using DIAMOND. Nat Methods. 12(1):59-60.
- **UniProt**: https://www.uniprot.org
- **License**: Free for academic use (follows UniProt terms)
