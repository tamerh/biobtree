#!/usr/bin/env python3
"""
Multi-hop "1 call vs N tools" benchmark — a curated, quantified demonstration of
biobtree's integration value. Each task is a real cross-database question answered
by ONE biobtree chain query; the `manual` field lists the separate resources/API
calls the same answer would require elsewhere (auditable, not asserted).

Run:  python usefulness_multihop.py            (uses http://localhost:9291)
Output: a markdown table (latency, targets, #tools/#calls replaced) + a per-task
manual-reproduction appendix.
"""
import json
import time
import urllib.parse
import urllib.request
import sys

BASE = "http://localhost:9291"

# Each task: a real analyst question, the biobtree chain (one call), and the
# manual equivalent (the distinct public resources + separate calls needed).
TASKS = [
    {
        "q": "What proteins does the drug imatinib target (mechanism-level)?",
        "terms": "imatinib", "chain": ">>chembl_molecule>>chembl_target>>uniprot",
        "manual": ["name→ChEMBL ID (ChEMBL search)", "ChEMBL→mechanism targets (ChEMBL API)",
                   "target→UniProt (UniProt ID mapping)"],
    },
    {
        "q": "Curated mechanism-of-action + approval for imatinib?",
        "terms": "imatinib", "chain": ">>chembl_molecule>>drugcentral",
        "manual": ["name→ChEMBL/DrugCentral ID", "DrugCentral MOA/approval lookup"],
    },
    {
        "q": "Which Reactome pathways involve TP53?",
        "terms": "TP53", "chain": ">>hgnc>>uniprot>>reactome",
        "manual": ["symbol→HGNC/UniProt (HGNC or UniProt)", "UniProt→Reactome (Reactome API)"],
    },
    {
        "q": "GO terms (function) for BRCA1?",
        "terms": "BRCA1", "chain": ">>hgnc>>uniprot>>go",
        "manual": ["symbol→UniProt (UniProt)", "UniProt→GO (QuickGO/UniProt)"],
    },
    {
        "q": "Drugs in clinical trials for Parkinson disease?",
        "terms": "parkinson disease", "chain": ">>mondo>>clinical_trials>>chembl_molecule",
        "manual": ["disease→MONDO (OLS/Mondo)", "MONDO→trials (ClinicalTrials.gov)",
                   "trial drug→ChEMBL (ChEMBL)"],
    },
    {
        "q": "Cancer-driver evidence for KRAS?",
        "terms": "KRAS", "chain": ">>hgnc>>intogen",
        "manual": ["symbol→gene id", "intOGen driver lookup (intOGen portal)"],
    },
    {
        "q": "ClinGen gene-disease validity tier for PTEN?",
        "terms": "PTEN", "chain": ">>hgnc>>clingen_gene_validity",
        "manual": ["symbol→HGNC", "ClinGen gene-validity lookup (ClinGen portal)"],
    },
    {
        "q": "Cell lines associated with the EGFR protein?",
        "terms": "EGFR", "chain": ">>hgnc>>uniprot>>cellosaurus",
        "manual": ["symbol→UniProt", "UniProt→Cellosaurus (Cellosaurus)"],
    },
    {
        "q": "Tissues expressing SCN9A (Bgee)?",
        "terms": "SCN9A", "chain": ">>hgnc>>ensembl>>bgee",
        "manual": ["symbol→Ensembl (BioMart)", "Ensembl→Bgee expression (Bgee API)"],
    },
    {
        "q": "MaveDB functional-assay scores for BRCA1 variants?",
        "terms": "BRCA1", "chain": ">>hgnc>>uniprot>>mavedb",
        "manual": ["symbol→UniProt", "UniProt→MaveDB (MaveDB search + score CSV parse)"],
    },
    {
        "q": "Pharmacology targets + affinity for the ligand quinine (GtoPdb)?",
        "terms": "quinine", "chain": ">>gtopdb_ligand>>gtopdb_interaction>>gtopdb>>uniprot",
        "manual": ["name→GtoPdb ligand", "ligand→interactions (GtoPdb)",
                   "interaction→target (GtoPdb)", "target→UniProt (UniProt)"],
    },
    {
        "q": "Diseases genetically associated with the HPO term 'Seizure' via genes?",
        "terms": "HP:0001250", "chain": ">>hpo>>gencc>>hgnc",
        "manual": ["phenotype→HPO id (HPO)", "HPO→disease/gene (GenCC)", "→HGNC (HGNC)"],
    },
]


def call(terms, chain, timeout=60):
    url = f"{BASE}/ws/map/?i={urllib.parse.quote(terms)}&m={urllib.parse.quote(chain)}"
    t0 = time.perf_counter()
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            data = json.loads(r.read().decode())
    except Exception as e:
        return None, (time.perf_counter() - t0) * 1000, str(e)
    dt = (time.perf_counter() - t0) * 1000
    stats = data.get("stats", {}) or {}
    n = stats.get("total_targets")
    if n is None:  # fall back to counting
        n = sum(len(g.get("targets", [])) for g in data.get("results", []))
    return n, dt, None


def main():
    rows = []
    for t in TASKS:
        # warm once, then time (report warm — reflects a running service)
        call(t["terms"], t["chain"])
        n, dt, err = call(t["terms"], t["chain"])
        rows.append({**t, "targets": n, "ms": dt, "err": err})

    print("# biobtree multi-hop: one call vs N tools\n")
    print("| # | Question | biobtree chain (1 call) | targets | latency | manual: #DBs/tools |")
    print("|---|----------|-------------------------|--------:|--------:|-------------------:|")
    for i, r in enumerate(rows, 1):
        n = r["targets"] if r["err"] is None else f"ERR"
        ms = f"{r['ms']:.0f} ms" if r["err"] is None else "-"
        print(f"| {i} | {r['q']} | `{r['chain']}` | {n} | {ms} | {len(r['manual'])} |")

    ok = [r for r in rows if r["err"] is None and (r["targets"] or 0) > 0]
    lat = [r["ms"] for r in ok]
    tools = [len(r["manual"]) for r in ok]
    print(f"\n**Summary:** {len(ok)}/{len(rows)} tasks answered in a single call · "
          f"median latency {sorted(lat)[len(lat)//2]:.0f} ms · "
          f"each replaces {min(tools)}–{max(tools)} separate resource lookups "
          f"(mean {sum(tools)/len(tools):.1f}).")

    print("\n## Manual-reproduction appendix (what the same answer needs elsewhere)\n")
    for i, r in enumerate(rows, 1):
        print(f"{i}. **{r['q']}** — biobtree: 1 call. Manual: " +
              " → ".join(r["manual"]) + f" ({len(r['manual'])} steps).")

    if any(r["err"] for r in rows):
        print("\n_errors:_ " + "; ".join(f"{i+1}:{r['err']}" for i, r in enumerate(rows) if r["err"]))


if __name__ == "__main__":
    main()
