#!/usr/bin/env python3
import pandas as pd
import json
from pathlib import Path

base_dir = Path("biobtreev2/test_data/surechembl")

print("Converting parquet to JSON for biobtree (jsparser format)...")
print("Note: jsparser expects wrapped format like {\"arrayname\": [...]}")

# Patents - wrapped with "patents" property
print("\n1. Converting patents...")
patents = pd.read_parquet(base_dir / "patents_sample.parquet")
patents_json = patents.to_dict(orient='records')
patents_wrapped = {"patents": patents_json}
with open(base_dir / "patents.json", 'w') as f:
    json.dump(patents_wrapped, f, indent=2, default=str)
print(f"   ✓ patents.json ({len(patents)} records, wrapped with 'patents' key)")

# Compounds - wrapped with "compounds" property
print("\n2. Converting compounds...")
compounds = pd.read_parquet(base_dir / "compounds_sample.parquet")
compounds_json = compounds.to_dict(orient='records')
compounds_wrapped = {"compounds": compounds_json}
with open(base_dir / "compounds.json", 'w') as f:
    json.dump(compounds_wrapped, f, indent=2, default=str)
print(f"   ✓ compounds.json ({len(compounds)} records, wrapped with 'compounds' key)")

# Mapping - wrapped with "mappings" property
print("\n3. Converting mapping...")
mapping = pd.read_parquet(base_dir / "mapping_sample.parquet")
mapping_json = mapping.to_dict(orient='records')
mapping_wrapped = {"mappings": mapping_json}
with open(base_dir / "mapping.json", 'w') as f:
    json.dump(mapping_wrapped, f, indent=2, default=str)
print(f"   ✓ mapping.json ({len(mapping)} records, wrapped with 'mappings' key)")

# Summary
print("\n4. Creating summary...")
summary = {
    "created": "2025-10-20",
    "source": "test_out/raw_data/patents/surechembl/2025-10-01/",
    "sample_size": {
        "patents": len(patents),
        "compounds": len(compounds),
        "mappings": len(mapping)
    },
    "example_patent": {
        "id": int(patents.iloc[0]['id']),
        "patent_number": str(patents.iloc[0]['patent_number']),
        "country": str(patents.iloc[0]['country']),
        "title": str(patents.iloc[0]['title']),
        "assignees": list(patents.iloc[0]['asignee']),
        "ipc_codes": list(patents.iloc[0]['ipc'])
    },
    "example_compound": {
        "id": int(compounds.iloc[0]['id']),
        "smiles": str(compounds.iloc[0]['smiles']),
        "inchi_key": str(compounds.iloc[0]['inchi_key']),
        "mol_weight": float(compounds.iloc[0]['mol_weight'])
    },
    "example_mapping": {
        "patent_id": int(mapping.iloc[0]['patent_id']),
        "compound_id": int(mapping.iloc[0]['compound_id']),
        "field_id": int(mapping.iloc[0]['field_id'])
    }
}

with open(base_dir / "summary.json", 'w') as f:
    json.dump(summary, f, indent=2)
print(f"   ✓ summary.json")

print("\n" + "="*80)
print("SUCCESS!")
print("="*80)
