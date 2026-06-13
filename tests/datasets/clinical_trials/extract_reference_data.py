#!/usr/bin/env python3
"""
Extract reference data for Clinical Trials tests.

Reads the pinned trial IDs from clinical_trials_ids.txt and pulls those trials
out of the canonical source file raw_data/clinical_trials/trials.json, writing
reference_data.json (the source-shaped fixture the tests validate against).

The source file is large (multi-GB) and wrapped as {"trials":[...]}, so it is
streamed object-by-object rather than loaded whole; extraction stops once every
pinned ID has been found.

Regenerate after a clinical_trials snapshot refresh:
  1. build the test DB:   ./biobtree -d clinical_trials --lookupdb test
  2. sync the pinned IDs: cp test_out/reference/clinical_trials_ids.txt \
                             tests/datasets/clinical_trials/clinical_trials_ids.txt
  3. run this script:     python tests/datasets/clinical_trials/extract_reference_data.py
"""

import json
from pathlib import Path


def stream_trials(source_file, wanted):
    """Yield trial objects whose nct_id is in `wanted`, streaming the wrapped
    {"trials":[...]} array from the front without loading the whole file."""
    dec = json.JSONDecoder()
    found = {}
    with open(source_file) as f:
        head = f.read(len('{"trials":['))
        if not head.startswith('{"trials":['):
            raise ValueError(f"unexpected source header: {head!r}")
        buf = ""
        while len(found) < len(wanted):
            if len(buf) < 65536:
                chunk = f.read(1 << 20)
                if not chunk and not buf.strip(", \n\r\t]"):
                    break
                buf += chunk
            buf = buf.lstrip(", \n\r\t")
            if buf.startswith("]") or buf == "":
                break
            try:
                obj, idx = dec.raw_decode(buf)
            except json.JSONDecodeError:
                more = f.read(1 << 20)
                if not more:
                    break
                buf += more
                continue
            buf = buf[idx:]
            nct = obj.get("nct_id")
            if nct in wanted and nct not in found:
                found[nct] = obj
    return found


def main():
    script_dir = Path(__file__).parent
    ids_file = script_dir / "clinical_trials_ids.txt"
    source_file = script_dir.parent.parent.parent / "raw_data" / "clinical_trials" / "trials.json"
    output_file = script_dir / "reference_data.json"

    print(f"Reading trial IDs from {ids_file}")
    ids_order = [line.strip() for line in open(ids_file) if line.strip()]
    wanted = set(ids_order)
    print(f"Looking for {len(wanted)} trial IDs")

    print(f"Streaming trials from {source_file}")
    found = stream_trials(source_file, wanted)

    missing = wanted - set(found)
    if missing:
        print(f"\n⚠ Warning: {len(missing)} trial IDs not found: {sorted(missing)}")

    # Preserve the build's ID order
    reference_data = [found[n] for n in ids_order if n in found]

    print(f"\nWriting {len(reference_data)} trials to {output_file}")
    with open(output_file, "w") as f:
        json.dump(reference_data, f, indent=2)

    print("✓ Reference data extracted successfully")
    print(f"  Total trials: {len(reference_data)}")


if __name__ == "__main__":
    main()
