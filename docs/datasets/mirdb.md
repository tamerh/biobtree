# miRDB Dataset Tests

## Dataset Overview
- **Name**: miRDB
- **ID**: 450
- **Source**: https://mirdb.org/
- **Data Version**: 6.0
- **License**: Free for academic use

## Data Description
miRDB is a database for miRNA target prediction using the MirTarget algorithm. It contains predicted miRNA-target interactions across five species:
- Human (hsa): ~2,656 miRNAs
- Mouse (mmu): ~1,978 miRNAs
- Chicken (gga): ~1,235 miRNAs
- Rat (rno): ~764 miRNAs
- Dog (cfa): ~453 miRNAs

Total: ~7,086 unique miRNAs with ~6.8 million target predictions.

## Data Format
Source file: Tab-separated with 3 columns:
```
miRNA_ID<tab>RefSeq_ID<tab>Score
hsa-miR-21-5p	NM_001234	95.5
```

## Entry ID Format
- **Primary key**: miRNA ID (e.g., `hsa-miR-21-5p`, `mmu-let-7a`)
- **Format**: `{species}-{miRNA_name}` or `{species}-{miRNA_name}-{arm}`
- **Species prefixes**: hsa (human), mmu (mouse), rno (rat), cfa (dog), gga (chicken)

## Attributes Stored
- `mirna_id`: miRNA identifier
- `species`: Species prefix
- `target_count`: Number of predicted targets
- `avg_score`, `max_score`, `min_score`: Score statistics
- `targets[]`: Array of target predictions with RefSeq ID and score

## Cross-References Created
- **RefSeq**: Each miRNA links to all its predicted target RefSeq transcripts
- **hgnc / entrez**: Each miRNA also links from the target **gene**, so
  `hgnc >> mirdb` (and `entrez >> mirdb`) lists the miRNAs predicted to target
  that gene. This is the reliable gene-level chain; `hgnc >> refseq >> mirdb`
  only sees the single RefSeq transcript carried by the HGNC→RefSeq edge and
  can miss targets.
- **Text search**: miRNA IDs and short names (without species prefix)

## Example Queries

### Search for a miRNA
```
curl "http://localhost:9292/ws/?i=hsa-miR-21-5p"
```

### Map miRNA to RefSeq targets
```
curl "http://localhost:9292/ws/map/?i=hsa-miR-21-5p&m=>>mirdb>>refseq"
```

### Map a gene to its predicted miRNAs
```
curl "http://localhost:9292/ws/map/?i=HECTD1&m=>>hgnc>>mirdb"
```

Use `>>hgnc>>mirdb` rather than `>>hgnc>>refseq>>mirdb`: the HGNC→RefSeq edge
carries one transcript, while miRDB is keyed at the transcript level and can
target a different isoform.

### Filter by target count
```
curl "http://localhost:9292/ws/map/?i=hsa-miR-21-5p&m=>>mirdb[target_count>500]"
```

### Filter by score
```
curl "http://localhost:9292/ws/map/?i=hsa-miR-21-5p&m=>>mirdb[max_score>90.0]"
```

## Known Limitations
- Score thresholds: Only predictions with score >= 50 are included in miRDB source data
- Source predictions are RefSeq transcript IDs; the gene-level hgnc/entrez
  edges are resolved from those transcripts, so a gene is present when at
  least one of its RefSeq targets resolves.
- No experimental validation flags: All predictions are computational

## Test Cases
See `test_cases.json` for declarative tests.

## References
- Wong N, Wang X. (2020) miRDB: an online database for prediction of functional microRNA targets. Nucleic Acids Research, 48(D1):D127-D131.
