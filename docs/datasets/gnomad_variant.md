# gnomAD Variant (`gnomad_variant`, id 809)

Per-variant, per-ancestry allele frequencies from the **gnomAD v4** sites VCF.

## Why

Before this dataset, biobtree carried only a single global allele frequency
(`dbsnp.gnomad_frequency`) and gene-level constraint (`gnomad_constraint`, id
800). `gnomad_variant` adds the **ACMG BA1 / BS1 / PM2 evidence layer**: the
per-population allele frequencies used to call a variant "too common to be
pathogenic" (BA1/BS1) or "absent/rare in population databases" (PM2). It is a
**separate dataset** from the gene-level `gnomad_constraint` (id 800).

## Source & format

- **Release:** gnomAD v4 (GRCh38).
- **Files:** per-chromosome, bgzipped **sites VCFs**
  (`https://gnomad.broadinstitute.org/downloads`, ODC-ODbL). Sites VCFs are
  split so each ALT is on its own row.
- **Browser / entry URL:** `https://gnomad.broadinstitute.org/variant/{chr-pos-ref-alt}?dataset=gnomad_r4`

### VCF INFO fields consumed

| INFO key            | Attr field        | Meaning                                                        |
|---------------------|-------------------|---------------------------------------------------------------|
| `AF`                | `af`              | Global allele frequency                                       |
| `AF_grpmax`         | `af_grpmax`       | Group-max AF. **grpmax = popmax renamed in v4.**              |
| `grpmax`            | `grpmax_ancestry` | Ancestry group holding the grpmax AF                          |
| `fafmax_faf95_max`  | `faf`             | Filtering allele frequency (grpmax faf95); the BA1/BS1 metric |
| `AF_afr`            | `af_afr`          | African / African-American                                    |
| `AF_amr`            | `af_amr`          | Admixed American                                              |
| `AF_eas`            | `af_eas`          | East Asian                                                    |
| `AF_nfe`            | `af_nfe`          | Non-Finnish European                                          |
| `AF_sas`            | `af_sas`          | South Asian                                                   |
| `AF_fin`            | `af_fin`          | Finnish                                                       |
| `AF_asj`            | `af_asj`          | Ashkenazi Jewish                                              |
| `AF_ami`            | `af_ami`          | Amish (genomes)                                               |
| `AF_mid`            | `af_mid`          | Middle Eastern                                                |
| `AF_remaining`      | `af_remaining`    | Remaining / unassigned ancestry                              |

v4 genetic-ancestry groups: exomes `afr, amr, asj, eas, fin, mid, nfe,
remaining, sas`; genomes add `ami`. The faf calculation uses `afr, amr, eas,
mid, nfe, sas`.

> **Note — regional missense constraint is NOT in v4.0.** v4.0 dropped the
> regional missense constraint (RMC) tracks; a genome-wide missense
> depletion / missense-z metric returned in **v4.1**. Those are gene/region
> annotations (see `gnomad_constraint`), not per-variant, and are out of scope
> for `gnomad_variant`.

## Key scheme

`chr:pos:ref:alt` (GRCh38), identical to `alphamissense` and `spliceai`
(e.g. `1:69094:G:A`). Chromosome is normalized by stripping a leading `chr`.

## Cross-references

- **→ `dbsnp` by rsID.** Where the VCF `ID` column carries an `rs...` id, the
  variant is xref'd to `dbsnp` so the frequency reaches the rsID hub. rsIDs are
  validated (`rs` + digits) before the edge is created.

## License & KG export

**ODC-ODbL** (Open Data Commons Open Database License): open, requires
attribution **and share-alike**. Ingest is fine. The share-alike clause makes
it incompatible with the CC BY-NC-SA KG export — `gnomad_variant` must be
**EXCLUDED from the KG export**, the same treatment as `spliceai` /
`alphamissense`. (Documented here only; no export logic lives in this parser.)

## Production-scale note (for the coordinator)

Real gnomAD v4 is **federation-scale (~786M variants)** across per-chromosome
VCFs. This dataset ships as a **normal main-federation scaffold** with a tiny
hand-crafted fixture VCF (`tests/datasets/gnomad_variant/gnomad_variant_fixture.vcf`).
Whether production lives in the **main federation or its own federation** (like
`dbsnp`) is decided at production-ingest time by the coordinator. Do **not** run
a real gnomAD download or `--generate` from this scaffold.
