# ncRNA Disease Dataset

## Overview

Curated non-coding-RNA → disease associations. This is the disease layer that
RNAcentral's Rfam/GO annotations cannot give bare lncRNAs — most lncRNAs are not
in Rfam, so their clinical/disease story comes from curation like this. Multiple
sources are unified into this one dataset, each association tagged with its `source`.

**Sources** (experimentally supported):
- **LncRNADisease v3.0** — `website_alldata.tsv` — lncRNA + circRNA → disease
  (`source="LncRNADisease"`). License: redistribution with citation.
- **HMDD v4** — `alldata_v4.txt` — miRNA → disease (`source="HMDD"`,
  `ncrna_category="miRNA"`). License: CC BY-NC (acceptable; attribute).
**Data type**: experimentally-supported ncRNA-disease associations

## Integration Architecture

### Storage Model
**Primary entries**: one record per association, id `LNCRD_<hash>` (deterministic
from ncRNA symbol + disease + PubMed + method; references are via edges).

**Attributes stored** (protobuf `NcrnaDiseaseAttr`):
- `ncrna_symbol`, `ncrna_category` (LncRNA / circRNA), `species`
- `disease_name`, `dysfunction_pattern`, `validated_method`, `causality` (Yes/No),
  `clinical_application`, `description`
- `source` (provenance: `LncRNADisease`)

**Cross-references**:
- → `hgnc` / `ensembl` — the ncRNA gene (resolved from the symbol via the lookup DB)
- → `mondo` / `efo` — the disease (shared disease-name matcher; comma-inverted MeSH
  names like "Carcinoma, Hepatocellular" are de-inverted so they map)
- → `pubmed` — the supporting citation

### Query paths
- `gene >> ncrna_disease >> mondo` — diseases associated with an ncRNA gene
- `mondo >> ncrna_disease >> hgnc` — ncRNAs implicated in a disease
- lite-mode compact: `id|ncrna_symbol|ncrna_category|disease_name|causality|validated_method`

## Use Cases
- Surface the disease associations of an lncRNA on its gene page (e.g. CAHM, HOTAIR).
- Enumerate the ncRNAs implicated in a disease, with causality + evidence method.

## Known Limitations
- Experimentally-supported only; the predicted LncRNADisease cut is deferred (would
  be added with an explicit `predicted` flag).
- Disease coverage depends on the name→MONDO/EFO matcher; some free-text disease
  names won't map (the association + gene/pubmed edges still resolve).
- For HMDD miRNA rows the miRBase name → HGNC/Ensembl gene mapping is best-effort
  (miRNA→gene is fuzzy); the disease + PubMed edges are the reliable value there.

## Maintenance
- **Update**: re-download `website_alldata.tsv` and re-index.
- **License**: LncRNADisease v3.0 (redistribution with citation).
- **References**: Lin et al. *LncRNADisease v3.0* Nucleic Acids Research 2024.
