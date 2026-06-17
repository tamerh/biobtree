# FAERS (FDA Adverse Event Reporting System) Dataset

## Overview
FAERS is the FDA's post-marketing pharmacovigilance database of spontaneous adverse-event and medication-error reports. biobtree ingests it through **openFDA's** `drug/event` bulk download, which exposes the cleaned, openFDA-annotated FAERS reports as partitioned, zipped JSON.

**Source (bulk data)**: https://download.open.fda.gov/drug/event/
**Manifest (partition index)**: https://api.fda.gov/download.json (`results.drug.event.partitions[]`)
**About**: https://open.fda.gov/data/faers/
**License**: **CC0 / Public Domain** — https://open.fda.gov/license/
**Data Type**: Drug → adverse-reaction co-occurrence aggregates with a disproportionality signal (PRR)
**Dataset IDs**: `faers` (master, **802**), `faers_reaction` (child, **804**)

## Attribution
Data courtesy of the U.S. Food & Drug Administration via the openFDA API. openFDA data is released into the public domain (CC0). This product uses publicly available data from the FDA but is **not endorsed by and does not reflect the views of the FDA**.

## Integration Architecture

### Storage Model (master / child)
FAERS uses a **master/child** layout so the drug↔compound linkage is consolidated to one node per drug and the per-reaction detail hangs off it:

- **`faers` (MASTER, id 802)** — one entry per **drug**, keyed `FAERS_<sha1(drug)>` (e.g. `FAERS_80AE44850DC40259`). `bucketMethod: alphanum`.
  - **Attributes**: `drug_name`, `total_reports` (total report mentions of this drug), `distinct_reactions`, `serious_reports`.
  - **Cross-References**: `chembl_molecule`, `pubchem` — resolved **once per drug** (best-effort, via drug-name resolution — see below). This is the single place the drug is normalized to a compound.
  - **Edges to children**: `faers >> faers_reaction`, **sorted by `report_count` DESC** (so the most-reported reactions survive the per-query result cap).
  - **Searchable Text Link**: the drug name points at the master record.
- **`faers_reaction` (CHILD, id 804)** — one entry per **(drug × reaction)** co-occurrence, keyed `FAERS_RX_<sha1(drug|reaction)>`. `bucketMethod: alphanum`.
  - **Attributes**: `reaction` (MedDRA Preferred Term string), `report_count`, `prr`, `serious_count`, `outcome`.
  - **Edge**: linked back to its parent `faers` master (bidirectional with the sorted master→child edge).
  - **Searchable Text Link**: the reaction term points at the child record.

So the access path is **`chembl_molecule >> faers >> faers_reaction`** (compound → drug AE summary → per-reaction detail, most-reported first). **Individual reports are never stored** — only the per-drug and per-(drug,reaction) aggregates.

### Source JSON fields parsed (per report)
- `serious` — report-level seriousness flag (`"1"` = serious).
- `patient.drug[].openfda.generic_name` / `.substance_name` / `.brand_name`, and `patient.drug[].medicinalproduct` — the drug-name normalization cascade (generic preferred).
- `patient.reaction[].reactionmeddrapt` — the MedDRA Preferred Term **string** (stored verbatim; no MedDRA dictionary imported).
- `patient.reaction[].reactionoutcome` — outcome code, aggregated per child reaction into `outcome` (the most common code: 1=recovered, 2=recovering, 3=not recovered, 4=recovered with sequelae, 5=fatal, 6=unknown).

### Processing
FAERS is partitioned into ~1,700 quarterly files (~20M reports total). The parser streams each partition, and for every report folds the **cross product** of its distinct drugs × distinct reactions into `(drug, reaction)` aggregates, also tracking per-drug and per-reaction report totals. After aggregation it:

1. writes one **`faers_reaction` child** per `(drug, reaction)` pair whose `report_count >= minReportCount` (default 2), with its PRR / serious_count / outcome, and links it to its parent master sorted by `report_count` DESC;
2. writes one **`faers` master** per drug, summing `total_reports` / `serious_reports` from the report-level marginals and counting `distinct_reactions` from the children that passed the threshold, and resolves the drug **once** to `chembl_molecule` / `pubchem`.

- **Full corpus**: the config no longer caps partitions, so a production re-index ingests the full ~1,700-partition corpus. In **test mode** the parser auto-caps to the 2 most-recent partitions (`resolvePartitions` fallback) so the focused build only fetches a few hundred MB. A `testPartitions` config key, if set, overrides the cap in either mode.

## CRITICAL CAVEATS — read before using

1. **Co-occurrence, not causation.** Within a single FAERS report, the listed drugs and the listed reactions are **NOT individually linked**. A `(drug, reaction)` edge is therefore **report-level co-occurrence**, not a curated causal association. A patient on five drugs reporting three reactions contributes all 15 drug×reaction pairs.

2. **PRR is a disproportionality signal, not proof.** The proportional reporting ratio
   `PRR = [a/(a+b)] / [c/(c+d)]` (a = reports with drug AND reaction, a+b = reports with drug, c = reports with reaction but not drug, c+d = reports without drug) measures whether a reaction is reported *disproportionately often* for a drug relative to background. A common signal-of-disproportionate-reporting heuristic is **PRR > 2 with report_count >= 3**. PRR is sensitive to reporting bias (notoriety, indication confounding, stimulated reporting) and does **not** establish causality.

3. **Reactions are MedDRA Preferred Term strings only.** No MedDRA dictionary/ontology is imported (MedDRA is license-restricted), so reactions are free-text PT strings — there is no MONDO/disease-ontology edge, and term spelling/casing follows the source.

4. **Drug-ID normalization is best-effort.** biobtree has no native UNII/RxNorm dataset, so the openFDA `generic_name` is resolved to `chembl_molecule`/`pubchem` by runtime name lookup against the build index. Ambiguous or unmatched names yield no edge (edges are guarded to configured, loaded datasets). Expect partial coverage.

## Use Cases

**1. A drug's adverse-event summary, then its top reactions**
```
aspirin >> faers                  → the drug's master (total_reports, distinct_reactions, serious_reports)
aspirin >> faers >> faers_reaction → its reactions, most-reported first (report_count, PRR, serious_count)
```

**2. Filter to disproportionately-reported, serious signals (on the child)**
```
map(faers_reaction).filter(prr > 2 && report_count >= 3)
```

**3. Bridge a chemical to its adverse-event profile**
```
>>chembl_molecule>>faers>>faers_reaction   or   >>pubchem>>faers>>faers_reaction
→ the compound's drug master and its per-reaction detail (where the drug name resolved)
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
  "minReportCount": "2",
  "attrs": "drug_name,total_reports,distinct_reactions,serious_reports",
  "compact_fields": "drug_name,total_reports,distinct_reactions",
  "test_entries_count": "100",
  "bucketMethod": "alphanum",
  "childDatasets": "faers_reaction",
  "xrefSort": "chembl_molecule:interactionScore;pubchem:interactionScore;faers_reaction:cellCount"
},
"faers_reaction": {
  "id": "804",
  "name": "FAERS Reaction",
  "textPriority": "40",
  "aliases": "FAERS reaction,adverse reaction,adverse event reaction,MedDRA preferred term",
  "url": "https://open.fda.gov/data/faers/",
  "useLocalFile": "no",
  "hasFilter": "yes",
  "attrs": "reaction,report_count,prr,serious_count,outcome",
  "compact_fields": "reaction,report_count,prr",
  "test_entries_count": "100",
  "bucketMethod": "alphanum"
}
```

> The `faers_reaction:cellCount` entry in the master's `xrefSort` is what orders the `faers >> faers_reaction` edges by `report_count` (descending), so the most-reported reactions survive the per-query result cap.
