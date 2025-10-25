# STRING Test Dataset

**Created**: 2025-10-24
**STRING Version**: 12.0
**Organism**: Human (Taxonomy ID: 9606)
**Source**: Extracted from full human dataset in `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/analysis/string_raw/`

## Dataset Overview

This test dataset contains protein-protein interaction data for 50 well-known cancer and pathway genes plus all their interaction partners.

### Files

| File | Size | Lines | Description |
|------|------|-------|-------------|
| `protein_links_test.txt` | 3.2 MB | 71,099 | Protein-protein interactions (score ≥ 400) |
| `protein_aliases_test.txt` | 76 MB | 1,534,858 | Identifier mappings to external databases |
| `protein_info_test.txt` | 2.8 MB | 6,902 | Protein metadata and annotations |

### Statistics

- **Proteins**: 6,901 unique proteins
- **Seed Proteins**: 50 well-known genes (see below)
- **Interactions**: 71,098 protein-protein links
- **Score Filter**: ≥ 400 (medium+ confidence)
- **Average interactions per protein**: ~10 (for seed proteins: ~1,400)
- **UniProt mappings**: Available for identifier conversion

### Seed Proteins (50 Well-Known Genes)

These are clinically and biologically important genes commonly found in cancer research, drug discovery, and pathway analysis:

**Receptor Tyrosine Kinases & Growth Factors**:
- EGFR, ERBB2, KDR, FLT1, ALK, RET, MET

**Oncogenes & Tumor Suppressors**:
- TP53, BRCA1, BRCA2, RB1, PTEN, APC, TP63, TP73

**RAS/RAF/MAPK Pathway**:
- KRAS, NRAS, HRAS, BRAF, RAF1, MAP2K1, MAPK1, MAPK3

**PI3K/AKT/MTOR Pathway**:
- PIK3CA, AKT1, MTOR

**Cell Cycle Regulators**:
- CDK4, CDK2, CCND1, CCNE1, CDKN2A, RBL1, E2F1, PCNA

**Transcription Factors**:
- MYC, JUN, FOS, STAT3, HIF1A, CTNNB1

**DNA Damage Response**:
- ATM, CHEK2, MDM2

**Hormone Receptors**:
- ESR1, AR

**Signal Transduction**:
- JAK2, SMAD4, TGFB1

**Chromatin Modifiers**:
- EP300, CREBBP

**Angiogenesis**:
- VEGFA

### Data Format

#### protein_links_test.txt
```
protein1 protein2 combined_score
9606.ENSP00000275493 9606.ENSP00000011653 640
```

- **Format**: Space-separated values
- **Columns**:
  - `protein1`: STRING protein ID (format: `9606.ENSP_ID`)
  - `protein2`: STRING protein ID (format: `9606.ENSP_ID`)
  - `combined_score`: Confidence score (400-999 in this dataset)

#### protein_aliases_test.txt
```
#string_protein_id	alias	source
9606.ENSP00000275493	P00533	UniProt_AC
9606.ENSP00000275493	EGFR	Ensembl_HGNC_symbol
```

- **Format**: Tab-separated values
- **Columns**:
  - `string_protein_id`: STRING protein ID
  - `alias`: External identifier or name
  - `source`: Database/source of the alias

#### protein_info_test.txt
```
#string_protein_id	preferred_name	protein_size	annotation
9606.ENSP00000275493	EGFR	1210	Epidermal growth factor receptor...
```

- **Format**: Tab-separated values
- **Columns**:
  - `string_protein_id`: STRING protein ID
  - `preferred_name`: Preferred gene/protein name
  - `protein_size`: Amino acid length
  - `annotation`: Functional description

### Example Proteins in Dataset

| Gene | STRING ID | UniProt | Interactions | Description |
|------|-----------|---------|--------------|-------------|
| EGFR | 9606.ENSP00000275493 | P00533 | 1,144 | Epidermal growth factor receptor |
| TP53 | 9606.ENSP00000269305 | P04637 | 2,317 | Tumor protein p53 |
| BRCA1 | 9606.ENSP00000418960 | P38398 | 1,891 | Breast cancer type 1 susceptibility protein |
| KRAS | 9606.ENSP00000256078 | P01116 | 982 | GTPase KRas |
| MYC | 9606.ENSP00000478887 | P01106 | 1,542 | Myc proto-oncogene protein |

### Score Distribution

| Score Range | Count | Percentage |
|-------------|-------|------------|
| 900-999 | ~500 | 0.7% |
| 700-899 | ~5,000 | 7.0% |
| 400-699 | ~65,598 | 92.3% |

**Average Score**: ~480

### Use Cases

This test dataset is ideal for:

1. **Testing STRING integration** into biobtree
2. **Validating cross-references** (STRING ↔ UniProt ↔ HGNC)
3. **Query testing**:
   - `HGNC:EGFR >> uniprot >> string`
   - `P00533 >> string >> uniprot` (find EGFR interaction partners)
   - `BRCA1 >> uniprot >> string >> uniprot >> hgnc`
4. **Performance benchmarking** (manageable size for quick tests)
5. **Biological validation** (well-characterized proteins with known interactions)

### Expected Query Results

**Example: Find EGFR Interaction Partners**
```bash
# Query: EGFR → UniProt → STRING interactions → back to UniProt
# Expected: ~1,144 interaction partners

# Some known partners with high confidence:
- EGF (Epidermal growth factor)
- GRB2 (Growth factor receptor-bound protein 2) - score: 999
- SHC1 (SHC-transforming protein 1) - score: 999
- PIK3CA (Phosphatidylinositol 4,5-bisphosphate 3-kinase) - score: 998
- STAT3 (Signal transducer and activator of transcription 3) - score: 987
```

**Example: TP53 Network**
```bash
# Query: TP53 → UniProt → STRING interactions
# Expected: ~2,317 interaction partners (largest in seed set)

# Known partners:
- MDM2 (E3 ubiquitin-protein ligase) - score: 999
- ATM (Ataxia telangiectasia mutated) - score: 998
- BRCA1 (Breast cancer type 1) - score: 997
- CDKN2A (Cyclin-dependent kinase inhibitor 2A) - score: 995
```

### Validation Checks

After implementing STRING integration, verify:

1. **Identifier Mapping**: All 50 seed genes should map to UniProt
2. **Interaction Count**: EGFR should have ~1,144 partners, TP53 ~2,317
3. **Bidirectionality**: Query A→B and B→A should both work
4. **Score Filtering**: All interactions should have score ≥ 400
5. **Cross-references**: STRING_ID ↔ UniProt_AC ↔ HGNC_Symbol chains work

### Data Extraction Commands

For reference, this test dataset was created with:

```bash
# 1. Select 50 seed genes
cat > /tmp/test_genes.txt << 'EOF'
EGFR, BRCA1, TP53, KRAS, MYC, AKT1, MTOR, PIK3CA, PTEN, ...
EOF

# 2. Find their STRING IDs
while read gene; do
  grep -w "$gene" 9606.protein.aliases.v12.0.txt | grep "Ensembl_HGNC_symbol" | head -n 1
done < /tmp/test_genes.txt > /tmp/test_string_ids.txt

# 3. Extract interactions (score ≥ 400)
grep -F -f /tmp/string_ids_only.txt 9606.protein.links.v12.0.txt | \
  awk '$3 >= 400' > /tmp/test_links_step1.txt

# 4. Get all proteins (seeds + partners)
awk '{print $1"\n"$2}' /tmp/test_links_step1.txt | sort -u > /tmp/all_proteins_in_test.txt

# 5. Extract aliases and info for all proteins
grep -F -f /tmp/all_proteins_in_test.txt 9606.protein.aliases.v12.0.txt > protein_aliases_test.txt
grep -F -f /tmp/all_proteins_in_test.txt 9606.protein.info.v12.0.txt > protein_info_test.txt
```

### Notes

- This dataset represents ~0.5% of the full human STRING database
- Interactions are filtered for quality (score ≥ 400)
- All interaction partners are included to maintain network completeness
- Suitable for development and testing; for production, use full dataset

### Next Steps

1. Implement STRING processor in `src/update/string.go`
2. Add configuration to `conf/source.dataset.json`
3. Build biobtree with this test dataset
4. Verify cross-references and queries work
5. Expand to full human dataset when validated

---

**Data Source**: https://string-db.org/
**License**: CC BY 4.0
**Full Analysis**: `/data/scc/ag-gruber/GROUP/tgur/x/bioyoda_dev2/analysis/STRING_DATA_ANALYSIS.md`
