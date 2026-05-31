#!/usr/bin/env python3
"""
Extract GeneRIF reference data for tests.

Streams generifs_basic.gz and samples human (tax 9606) gene ids that have a
GeneRIF claim with a PubMed citation, giving the test suite known-good gene ids
to validate the gene->generif->pubmed path. NCBI public domain.
"""

import gzip
import io
import json
import urllib.request
from pathlib import Path

URL = "https://ftp.ncbi.nlm.nih.gov/gene/GeneRIF/generifs_basic.gz"
N = 30


def main():
    out = Path(__file__).parent / "reference_data.json"
    genes = []
    seen = set()
    with urllib.request.urlopen(URL, timeout=300) as resp:
        with gzip.GzipFile(fileobj=io.BufferedReader(resp)) as gz:
            for raw in gz:
                line = raw.decode("utf-8", "replace").rstrip("\n")
                if not line or line.startswith("#"):
                    continue
                f = line.split("\t", 4)
                if len(f) < 5:
                    continue
                tax, gid, pmids, _, text = f
                if tax != "9606" or not gid or gid in seen or not pmids.strip():
                    continue
                seen.add(gid)
                genes.append({
                    "gene_id": gid,
                    "pmid": pmids.split(",")[0].strip(),
                    "text_snippet": text[:60],
                })
                if len(genes) >= N:
                    break

    with open(out, "w") as fh:
        json.dump({"genes": genes}, fh, indent=2)
    print(f"Wrote {len(genes)} GeneRIF genes to {out}")


if __name__ == "__main__":
    main()
