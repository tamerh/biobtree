# GeneRIF Dataset

## Overview
GeneRIF (Gene Reference Into Function) is NCBI's collection of concise, expert-contributed functional claims about genes — each one anchored to a PubMed citation. It is a high-value grounding/RAG source: every claim carries its evidence.

**Source**: https://ftp.ncbi.nlm.nih.gov/gene/GeneRIF/generifs_basic.gz
**Data Type**: Per-gene one-line functional claims (TSV)
**License**: NCBI — U.S. public domain
**Dataset ID**: 142

## Integration Architecture

### Storage Model
**Primary Entries**: one per GeneRIF claim, keyed by `<gene_id>_<pmid>_<n>` (n disambiguates multiple claims sharing a gene+PMID)
**Attributes Stored**: gene_id, PMIDs, claim text, timestamp, tax_id
**Cross-References**: `entrez` (the gene), `pubmed` (the citation)
**Bucket Method**: `alphanum`

A GeneRIF is reached *via its gene* — it behaves like a content child of the gene, but it is a standalone dataset linked by ordinary cross-reference (not the parent/child link mechanism). Because `entrez` already links to `hgnc`/`ensembl`, the gene graph is reachable from any gene identifier.

## Use Cases
- **Cited functional claims for a gene (RAG/grounding)**: `entrez >> generif` → claims, each with its PMID
- **Gene → literature with context**: `generif >> pubmed`
- **From any gene id**: `>>hgnc>>entrez>>generif`

## Scope
- **Full ingest, all species** (~1.69M claims) — not human-only.
- Distinct from the existing `gene2pubmed` gene→PMID links: GeneRIF adds the **claim text**, not just the citation link.

## Known Limitations
- The file has no gene symbol column; the symbol is reached through `entrez`.
- Claim text is stored as an attribute (not used as a search key — sentences exceed sane key length).

## Maintenance
- **Update Frequency**: NCBI continuous
- **Data Format**: gzipped TSV
- **License**: U.S. public domain

## References
- **Website**: https://www.ncbi.nlm.nih.gov/gene/about-generif
