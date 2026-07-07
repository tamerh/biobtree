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

- **Release:** gnomAD **v4.1 genomes** (GRCh38), whole-genome, ~759M variants.
- **Files:** one bgzipped **sites VCF per chromosome** (autosomes 1–22 + X, Y),
  on the AWS Registry of Open Data (no auth), ODC-ODbL:
  ```
  https://gnomad-public-us-east-1.s3.amazonaws.com/release/4.1/vcf/genomes/gnomad.genomes.v4.1.sites.chr{CHR}.vcf.bgz
  ```
  Sites VCFs are split so each ALT is on its own row. INFO field names were
  verified against the real v4.1 genomes VCF header (incl. `AF_grpmax`, which
  the browser also exposes). `.bgz` (BGZF) is gzip-compatible and read directly.
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

## Production ingest

**Decision (2026-07):** lives in the **main federation** (~759M genomes variants,
est. ~130 GB / ~2 B KV added to main). It is *not* a separate federation —
coordinate keys (`chr:pos:ref:alt`) can't be pattern-routed to a non-main
federation, since main already owns that key format via `alphamissense`/
`spliceai` (a dbsnp co-location attempt was reverted for exactly this reason).

The parser is production-ready: `update()` expands a `{CHR}` placeholder in the
conf `path` over chromosomes 1–22, X, Y and streams each per-chromosome bgz in
turn (validated end-to-end against the real chrY file). The default conf keeps
the **local fixture** so unit tests stay offline; switching to production is a
one-time conf edit:

```jsonc
"gnomad_variant": {
  ...
  "path": "https://gnomad-public-us-east-1.s3.amazonaws.com/release/4.1/vcf/genomes/gnomad.genomes.v4.1.sites.chr{CHR}.vcf.bgz",
  "useLocalFile": "no",
  ...
}
```

Then run the targeted ingest (main federation):
```
./bb.sh out_prod --only gnomad_variant --force --generate-after-main
```
Note this is a large, multi-hour, ~130 GB ingest that inflates main's generate
time and pushes the in-memory assemble/validate set toward the billion-key
range. The scaffold default (fixture + `useLocalFile: yes`) is what ships in
git so tests remain fast and offline.
