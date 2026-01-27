# IntAct Dataset Tests

## Overview

IntAct is EBI's manually curated database of molecular interactions. It provides experimentally validated protein-protein interactions with detailed experimental evidence.

**Website**: https://www.ebi.ac.uk/intact/
**Data Source**: EBI FTP (ftp.ebi.ac.uk)
**Format**: PSI-MITAB 2.7 (42 columns)
**Update**: Regular (monthly)

## Dataset Statistics

- **~1.8 million** raw interaction rows
- **~1.3 million** saved interactions
- **~115,000** unique proteins
- **23,000+** publications
- **75,000+** experiments

## Data Model (Interaction-Centric)

### Entry Structure
Each IntAct entry represents a **single interaction**, keyed by interaction ID (e.g., `EBI-7121552`).

### Attributes

```json
{
  "interaction_id": "EBI-7121552",
  "protein_a": "O43425",
  "protein_a_gene": "SYNJ1",
  "protein_b": "P49418",
  "protein_b_gene": "AMPH",
  "detection_method": "psi-mi:\"MI:0084\"(phage display)",
  "interaction_type": "psi-mi:\"MI:0407\"(direct interaction)",
  "confidence_score": 0.67,
  "experimental_role_a": "psi-mi:\"MI:0496\"(bait)",
  "experimental_role_b": "psi-mi:\"MI:0498\"(prey)",
  "taxid_a": 9606,
  "taxid_b": 9606,
  "organism_a": "human",
  "organism_b": "human",
  "pubmed_id": "10542231",
  "first_author": "Cestra et al. (1999)",
  "source_database": "MINT",
  "creation_date": "2001/01/10",
  "update_date": "2025/01/15",
  "detection_method_parsed": {
    "mi_id": "MI:0084",
    "term_name": "phage display",
    "full_string": "psi-mi:\"MI:0084\"(phage display)"
  },
  "interaction_type_parsed": {
    "mi_id": "MI:0407",
    "term_name": "direct interaction",
    "full_string": "psi-mi:\"MI:0407\"(direct interaction)"
  },
  "biological_role_a": {
    "mi_id": "MI:0499",
    "term_name": "unspecified role"
  },
  "biological_role_b": {
    "mi_id": "MI:0499",
    "term_name": "unspecified role"
  },
  "confidence_scores": {
    "miscore": 0.67,
    "raw_string": "intact-miscore:0.67"
  },
  "host_taxid": -1,
  "host_organism_name": "in vitro",
  "features_a": [
    {
      "feature_type": "binding-associated region",
      "range_start": 1063,
      "range_end": 1070,
      "description": "binding-associated region:1063-1070"
    }
  ],
  "features_b": [],
  "stoichiometry_a": 0,
  "stoichiometry_b": 0,
  "parameters": [],
  "interaction_xrefs": [],
  "imex_id": "",
  "method_reliability_score": 0.5
}
```

### Cross-References

Each interaction creates bidirectional cross-references:
- **protein_a** → interaction (intact xref)
- **protein_b** → interaction (intact xref)
- **interaction** → pubmed (via pubmed_id)

This enables queries like:
- `P49418` (protein) → shows all linked interactions
- `EBI-7121552` (interaction) → shows both proteins and pubmed

## Enhanced Fields (2025)

### PSI-MI Term Parsing (P0)
| Field | Type | Description |
|-------|------|-------------|
| detection_method_parsed | PsiMiTerm | Structured detection method (mi_id, term_name) |
| interaction_type_parsed | PsiMiTerm | Structured interaction type |
| biological_role_a | PsiMiTerm | Biological role of protein A |
| biological_role_b | PsiMiTerm | Biological role of protein B |

### Confidence Score Components (P0)
| Field | Type | Description |
|-------|------|-------------|
| confidence_scores.miscore | double | IntAct MIscore (0-1) |
| confidence_scores.raw_string | string | Original score string |

### Host Organism (P2)
| Field | Type | Description |
|-------|------|-------------|
| host_taxid | int32 | Host organism taxonomy ID (e.g., -1 for in vitro) |
| host_organism_name | string | Host organism name (e.g., "in vivo", "in vitro") |

### Binding Site Features (P1)
| Field | Type | Description |
|-------|------|-------------|
| features_a | array | Binding features on interactor A |
| features_b | array | Binding features on interactor B |
| feature_type | string | Feature type (binding site, mutation, etc.) |
| range_start | int32 | Start residue position |
| range_end | int32 | End residue position |

### Method Reliability Score (P1)
| Field | Type | Description |
|-------|------|-------------|
| method_reliability_score | double | Pre-computed reliability (0-1) based on method |

Method reliability mapping:
- MI:0114 (X-ray crystallography): 1.0
- MI:0077 (NMR): 0.95
- MI:0107 (Surface plasmon resonance): 0.9
- MI:0019 (Co-immunoprecipitation): 0.8
- MI:0004 (Affinity chromatography): 0.7
- MI:0096 (Pull-down): 0.65
- MI:0018 (Two hybrid): 0.6

## Example Queries

### Lookup interaction by ID
```bash
curl "http://localhost:9292/ws/entry/?i=EBI-7121552&s=intact"
```

### Find all interactions for a protein
```bash
curl "http://localhost:9292/ws/entry/?i=P49418&s=intact"
```
This returns the protein entry with linked interaction entries in `entries[]`.

### Navigate from protein to interaction details
```bash
# Step 1: Get protein's interactions
curl "http://localhost:9292/ws/entry/?i=P49418&s=intact"
# Returns entries like EBI-7121552, EBI-7122727...

# Step 2: Get full interaction details
curl "http://localhost:9292/ws/entry/?i=EBI-7121552&s=intact"
# Returns full interaction data including both proteins
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
6. **Method-based Filtering**: Filter by reliability score for high-confidence interactions

## Comparison with STRING

| Feature | IntAct | STRING |
|---------|--------|--------|
| Evidence | Experimental only | Predicted + experimental |
| Curation | Manual | Automated + some manual |
| Detail Level | High (methods, roles, features) | Medium (confidence scores) |
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

### Test Coverage (12 custom tests + 10 declarative tests)
- Interaction lookup by ID
- Protein identifiers (protein_a, protein_b)
- Gene names
- Confidence scores
- PubMed references
- Detection methods
- Interaction types
- PSI-MI term parsing (P0)
- Confidence score components (P0)
- Host organism (P2)
- Binding site features (P1)
- Method reliability score (P1)
- Protein → Interaction cross-references
