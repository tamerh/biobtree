# OBI (Ontology for Biomedical Investigations) Dataset

## Overview
The Ontology for Biomedical Investigations (OBI) describes biomedical
investigations: assays, study designs, instruments, specimens, devices and the
processes that relate them. In biobtree it is loaded as a standalone, browsable
ontology — fully searchable and navigable via its internal hierarchy.

## Data Source
- **Source**: OBO Foundry
- **URL**: http://purl.obolibrary.org/obo/obi.obo
- **Format**: OBO
- **License**: CC BY 4.0

## Dataset Characteristics
- **ID Format**: `OBI:XXXXXXX` (e.g., `OBI:0001146` = "binding assay")
- **ID Count**: ~5,200 OBI terms
- **Attributes**: type, name, synonyms
- **Relationships**: parent/child hierarchy via `obiparent` / `obichild` (`is_a`)

## Sample Queries

```bash
# Lookup an OBI term
curl "http://localhost:9292/ws/?i=OBI:0001146"

# Text search by term name / synonym
curl "http://localhost:9292/ws/?i=binding%20assay"

# Navigate to parent terms
curl "http://localhost:9292/ws/map/?i=OBI:0001146&m=obi>>obiparent"

# Navigate to child terms
curl "http://localhost:9292/ws/map/?i=OBI:0000070&m=obi>>obichild"

# Filter (hasFilter: yes)
curl "http://localhost:9292/ws/filter?i=OBI:0000070&e=name==\"assay\""
```

## Running Tests
```bash
python3 tests/run_tests.py obi
```

## Known Limitations
1. **No cross-references to other datasets**: OBI is queryable as a standalone
   ontology (search, filter, hierarchy navigation). It has no cross-reference
   edges to or from other biobtree datasets.
2. **Definitions not extracted**: only name, synonyms and type are indexed.
