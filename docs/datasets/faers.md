# FAERS (FDA Adverse Event Reporting System) Dataset

## Overview
FAERS is the FDA's post-marketing pharmacovigilance database of spontaneous adverse-event and medication-error reports. biobtree ingests it through **openFDA's** `drug/event` bulk download, which exposes the cleaned, openFDA-annotated FAERS reports as partitioned, zipped JSON.

**Source (bulk data)**: https://download.open.fda.gov/drug/event/
**Manifest (partition index)**: https://api.fda.gov/download.json (`results.drug.event.partitions[]`)
**About**: https://open.fda.gov/data/faers/
**License**: **CC0 / Public Domain** — https://open.fda.gov/license/
**Data Type**: Drug → adverse-reaction co-occurrence aggregates with a disproportionality signal (PRR)
**Dataset ID**: 802

## Attribution
Data courtesy of the U.S. Food & Drug Administration via the openFDA API. openFDA data is released into the public domain (CC0). This product uses publicly available data from the FDA but is **not endorsed by and does not reflect the views of the FDA**.

## Integration Architecture

### Storage Model
- **Primary Entries**: one entry per **(drug, adverse-reaction)** aggregate, keyed `FAERS_<sha1(drug|reaction)>` (e.g. `FAERS_5AD17CF786103F8C`). `bucketMethod: alphanum`.
- **Searchable Text Links**: drug name and the reaction term both point at the aggregate record.
- **Attributes Stored**: `drug_name`, `reaction` (MedDRA Preferred Term string), `report_count`, `prr`, `serious_count`, `top_outcome`, `drug_report_total`.
- **Cross-References**: `chembl_molecule`, `pubchem` (best-effort, via drug-name resolution — see below).

### Source JSON fields parsed (per report)
- `serious` — report-level seriousness flag (`"1"` = serious).
- `patient.drug[].openfda.generic_name` / `.substance_name` / `.brand_name`, and `patient.drug[].medicinalproduct` — the drug-name normalization cascade (generic preferred).
- `patient.reaction[].reactionmeddrapt` — the MedDRA Preferred Term **string** (stored verbatim; no MedDRA dictionary imported).
- `patient.reaction[].reactionoutcome` — outcome code, aggregated into `top_outcome` (1=recovered, 2=recovering, 3=not recovered, 4=recovered with sequelae, 5=fatal, 6=unknown).

### Processing
FAERS is partitioned into ~1,700 quarterly files (~20M reports total). The parser streams each partition, and for every report folds the **cross product** of its distinct drugs × distinct reactions into `(drug, reaction)` aggregates, also tracking per-drug and per-reaction report totals. After aggregation it computes the PRR and writes records whose `report_count >= minReportCount` (default 2).

- **Full corpus** is a production-reindex concern. For development/test builds the parser caps to `testPartitions` (default 2 most-recent partitions) so only a few hundred MB is fetched. Remove the cap (or set `testPartitions` high) for a full ingest.

## CRITICAL CAVEATS — read before using

1. **Co-occurrence, not causation.** Within a single FAERS report, the listed drugs and the listed reactions are **NOT individually linked**. A `(drug, reaction)` edge is therefore **report-level co-occurrence**, not a curated causal association. A patient on five drugs reporting three reactions contributes all 15 drug×reaction pairs.

2. **PRR is a disproportionality signal, not proof.** The proportional reporting ratio
   `PRR = [a/(a+b)] / [c/(c+d)]` (a = reports with drug AND reaction, a+b = reports with drug, c = reports with reaction but not drug, c+d = reports without drug) measures whether a reaction is reported *disproportionately often* for a drug relative to background. A common signal-of-disproportionate-reporting heuristic is **PRR > 2 with report_count >= 3**. PRR is sensitive to reporting bias (notoriety, indication confounding, stimulated reporting) and does **not** establish causality.

3. **Reactions are MedDRA Preferred Term strings only.** No MedDRA dictionary/ontology is imported (MedDRA is license-restricted), so reactions are free-text PT strings — there is no MONDO/disease-ontology edge, and term spelling/casing follows the source.

4. **Drug-ID normalization is best-effort.** biobtree has no native UNII/RxNorm dataset, so the openFDA `generic_name` is resolved to `chembl_molecule`/`pubchem` by runtime name lookup against the build index. Ambiguous or unmatched names yield no edge (edges are guarded to configured, loaded datasets). Expect partial coverage.

## Use Cases

**1. Adverse events reported for a drug**
```
Search: aspirin  (dataset filter: faers)
→ FAERS (drug, reaction) records for aspirin, each with report_count and PRR
```

**2. Filter to disproportionately-reported, serious signals**
```
map(faers).filter(prr > 2 && report_count >= 3)
```

**3. Bridge a chemical to its adverse-event profile**
```
>>chembl_molecule>>faers   or   >>pubchem>>faers
→ adverse-reaction aggregates for the compound (where the drug name resolved)
```

## Configuration
```json
"faers": {
  "id": "802",
  "name": "FAERS",
  "textPriority": "40",
  "aliases": "openFDA,adverse event,adverse drug reaction,FAERS,FDA Adverse Event Reporting System,pharmacovigilance",
  "url": "https://open.fda.gov/data/faers/",
  "manifestUrl": "https://api.fda.gov/download.json",
  "path": "https://download.open.fda.gov/drug/event/",
  "useLocalFile": "no",
  "hasFilter": "yes",
  "testPartitions": "2",
  "minReportCount": "2",
  "attrs": "drug_name,reaction,report_count,prr,serious_count,top_outcome,drug_report_total",
  "compact_fields": "drug_name,reaction,report_count,prr,serious_count",
  "test_entries_count": "100",
  "bucketMethod": "alphanum",
  "xrefSort": "chembl_molecule:interactionScore;pubchem:interactionScore"
}
```
