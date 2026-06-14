# ncRNA Drug Dataset

## Overview

Curated non-coding-RNA ↔ drug associations from **ncRNADrug**: ncRNA **drug-resistance**
(DR_Curated) and ncRNA **drug-target** (DT_Curated). Surfaces, on a gene/lncRNA or a
drug page, which ncRNAs confer resistance to or are targeted by a drug.

**Source**: ncRNADrug — `data_download/DR_Curated.txt` + `DT_Curated.txt` (curated cuts;
the high-throughput GEO/CCLE/NCI60/CMap cuts are not ingested).
**License**: ncRNADrug (CC BY 4.0); cite ncRNADrug.
**Data type**: curated ncRNA-drug resistance / target associations.

## Integration Architecture

### Storage Model
**Primary entries**: one record per association, id `NCDRUG_<hash>`.

**Attributes stored** (protobuf `NcrnaDrugAttr`):
- `ncrna_name`, `ncrna_type`, `symbol`
- `drug_name`, `drugbank_id`, `fda`
- `relation` (**drug_resistance** / **drug_target**), `effect` (resistance effect /
  expression pattern), `target_gene`, `pathway`, `detection_method`, `condition`
- `source` (provenance: `ncRNADrug`)

**Cross-references** (ncRNADrug provides most ids directly, so few lookups needed):
- → `ensembl` (`ENSEMBL_ID`, direct) and `hgnc` (from `SYMBOL`) — the ncRNA gene
- → `drugbank` (`DrugBank_ID`, direct), `pubchem` (`CID`, direct),
  `chembl_molecule` (from `Drug_Name`) — the drug
- → `hgnc`/`ensembl` — the gene the ncRNA targets (`ncRNA_Target_Gene`)
- → `pubmed`

### Query paths
- `gene >> ncrna_drug >> chembl_molecule` — drugs an ncRNA resists / targets
- `chembl_molecule >> ncrna_drug >> hgnc` — ncRNAs linked to a drug
- lite-mode compact: `id|ncrna_name|ncrna_type|drug_name|relation|effect|condition`

## Use Cases
- "Which lncRNAs confer resistance to gefitinib?" / "What drugs does HOTAIR affect?"
- Drug-resistance context on disease/drug pages (cancer-focused).

## Known Limitations
- Curated cuts only; high-throughput (GEO/CCLE/NCI60/CMap) cuts deferred.
- `relation` distinguishes resistance vs target; `effect` carries the direction.

## Maintenance
- **Update**: re-download DR_Curated.txt + DT_Curated.txt and re-index.
- **License**: ncRNADrug, CC BY 4.0.
- **References**: ncRNADrug, Nucleic Acids Research 2024.
