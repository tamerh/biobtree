# Internal: MONDO → MeSH gap for sclerosing cholangitis

Status: open (upstream data gap) · Last updated: 2026-11-06 · Context: main `db_v55`, MONDO build 2026-10-28

This note records a missing disease→MeSH mapping that breaks the natural
`MONDO → mesh → CTD` path. It is a **source-data gap**, not a biotree parser
bug; there is nothing to fix in the update code until MONDO or MeSH publishes
the mapping.

## Case

| MONDO | name | `>>mondo>>mesh` |
|---|---|---|
| `MONDO:0018646` | sclerosing cholangitis | 0 |
| `MONDO:0013433` | primary sclerosing cholangitis | 0 |

MeSH `D015209` ("Cholangitis, Sclerosing") exists and is well populated:

- `meshparent|1`
- `ctd_disease_association|2081`
- `chembl_molecule|35`
- `orphanet|1`

but its xref list contains no `mondo` entry.

## Evidence

```bash
curl -sL https://github.com/monarch-initiative/mondo/releases/latest/download/mondo.obo -o mondo.obo
grep -c D015209 mondo.obo        # 0
```

The MONDO terms themselves have no MeSH xref:

```
MONDO:0018646  xrefs: DOID:14268, SCTID:235917005, ...
MONDO:0013433  xrefs: DOID:0060643, OMIM:613806, Orphanet:171, SCTID:197441003, ...
```

The MedGen detour does not help either:

```
MONDO:0018646 >>mondo>>medgen          → 3036
MONDO:0018646 >>mondo>>medgen>>mesh    → 0
```

So both directions are absent at the source: MONDO does not assert
`skos:exactMatch MESH:D015209` for these terms, and MeSH does not point back to
MONDO.

## Impact

- `MONDO:0018646 >>mondo>>mesh` and `MONDO:0013433 >>mondo>>mesh` return 0.
- Disease → MeSH → CTD is broken for these diseases even though CTD has 2,081
  chemical–disease associations under `D015209`.
- Other diseases (T2D, Crohn, CF) work, so this is coverage, not a broken chain.

## Workarounds

1. Query `D015209` directly in `mesh`/`ctd_disease_association`.
2. Curate a small MONDO↔MeSH supplement for diseases with this gap (data
   maintenance decision).
3. Wait for MONDO/MeSH to publish the xref.

## Draft upstream report (MONDO)

Title: `Missing MeSH xref for sclerosing cholangitis (MONDO:0018646 / MONDO:0013433)`

Body:

> The latest `mondo.obo` release has no `MESH:D015209` xref on either
> `MONDO:0018646` (sclerosing cholangitis) or `MONDO:0013433` (primary
> sclerosing cholangitis). MeSH `D015209` (Cholangitis, Sclerosing) exists and
> is used by CTD for 2,081 chemical–disease associations, but it also has no
> MONDO xref. This breaks the natural disease → MeSH → CTD path. Could the
> equivalence be added? `grep -c D015209 mondo.obo` returns 0 on the current
> release.
