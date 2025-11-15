# IntAct Dataset Tests

## Overview

IntAct is EBI's manually curated database of molecular interactions. It provides experimentally validated protein-protein interactions with detailed experimental evidence.

**Website**: https://www.ebi.ac.uk/intact/
**Data Source**: EBI FTP (ftp.ebi.ac.uk)
**Format**: PSI-MITAB 2.7 (tab-delimited)
**Update**: Regular (monthly)

## Dataset Statistics

- **~1.8 million** experimentally validated interactions
- **~100,000** unique proteins
- **23,000+** publications
- **75,000+** experiments

## Data Model

### Primary Entries
- **Key**: UniProt accession (e.g., `P49418`)
- **Storage**: Bidirectional (A→B and B→A stored separately)

### Attributes

```json
{
  "protein_id": "P49418",
  "interactions": [
    {
      "interaction_id": "intact:EBI-7121552",
      "partner_uniprot": "O43426",
      "partner_gene_name": "SYNJ1",
      "detection_method": "psi-mi:\"MI:0084\"(phage display)",
      "interaction_type": "psi-mi:\"MI:0407\"(direct interaction)",
      "confidence_score": 0.67,
      "experimental_role_a": "psi-mi:\"MI:0498\"(prey)",
      "experimental_role_b": "psi-mi:\"MI:0496\"(bait)",
      "taxid_a": 9606,
      "taxid_b": 9606,
      "organism_a": "Homo sapiens",
      "organism_b": "Homo sapiens",
      "pubmed_id": "10542231",
      "first_author": "Cestra et al. (1999)",
      "source_database": "MINT",
      "is_negative": false,
      "creation_date": "2001/01/10",
      "update_date": "2025/01/15"
    }
  ],
  "interaction_count": 15,
  "unique_partners": 12,
  "high_confidence_count": 8,
  "partner_organisms": [9606]
}
```

## Cross-References

### Created by IntAct
- **Protein → Partner Protein** (uniprot)
- **Protein → PubMed** (publications)
- **Gene Name → Protein** (text search)

### Complementary Datasets
- **UniProt**: Protein sequences and annotations
- **STRING**: Protein interaction networks (predicted + experimental)
- **Reactome**: Pathway information
- **GO**: Functional annotations
- **Ensembl**: Gene information

## Test Mode

In test mode, IntAct:
- Processes **100 interactions** (configured limit)
- Filters to **human-human interactions only** (taxid:9606)
- Downloads from **real EBI FTP** (not local file)
- Typical processing time: **5-10 seconds**

## Example Queries

### Lookup protein interactions
```bash
./biobtree query "P49418"
```

### Find high-confidence interactions
```bash
./biobtree query "P49418 >> intact[intact.interactions[0].confidence_score>0.6]"
```

### Get interaction partners
```bash
./biobtree query "P49418 >> intact >> uniprot"
```

### Find interactions by detection method
```bash
./biobtree query "P49418 >> intact[intact.interactions[0].detection_method~\"two hybrid\"]"
```

### Cross-reference with STRING
```bash
./biobtree query "P49418 >> intact >> uniprot >> string"
```

## Data Quality

### PSI-MI Standards
- **Detection Methods**: Standardized PSI-MI terms (e.g., MI:0018 = two hybrid)
- **Interaction Types**: PSI-MI controlled vocabulary
- **Confidence Scores**: IntAct MIscore (0.0-1.0)

### Curation
- **Manual curation** from literature
- **Experimental evidence** required
- **Direct citations** to PubMed
- **Regular updates** from community

## Use Cases

1. **Protein Interaction Networks**: Map protein-protein interactions
2. **Drug Target Discovery**: Identify protein complexes and interactions
3. **Pathway Analysis**: Understand molecular mechanisms
4. **Systems Biology**: Build interaction networks
5. **Experimental Design**: Find validated interaction methods

## Comparison with STRING

| Feature | IntAct | STRING |
|---------|--------|--------|
| Evidence | Experimental only | Predicted + experimental |
| Curation | Manual | Automated + some manual |
| Detail Level | High (methods, roles) | Medium (confidence scores) |
| Scope | Focused quality | Broad coverage |
| Updates | Regular | Versioned releases |
| Use Case | Detailed validation | Network analysis |

**Recommendation**: Use both! IntAct for detailed experimental validation, STRING for comprehensive network coverage.

## References

- IntAct Database: https://www.ebi.ac.uk/intact/
- PSI-MI Standard: https://www.psidev.info/molecular-interactions
- NAR Database Issue: https://academic.oup.com/nar/article/50/D1/D648/6425548

## Testing

### Run Tests
```bash
# From project root
python3 tests/run_tests.py intact
```

### Test Coverage
- ✓ Protein lookup by UniProt ID
- ✓ Interaction data structure
- ✓ Confidence scores
- ✓ PubMed references
- ✓ Detection methods
- ✓ Interaction types
- ✓ Cross-references to partners
