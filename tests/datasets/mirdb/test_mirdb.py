#!/usr/bin/env python3
"""
miRDB dataset integration tests.
Tests microRNA target prediction data from miRDB.
"""

import json
import sys
import os

# Add parent directory for common test utilities
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.dirname(__file__))))
from test_utils import BiobtreeTestClient, load_test_ids


def test_search_human_mirna(client):
    """Test searching for a human miRNA."""
    result = client.search("hsa-miR-21-5p")
    assert result is not None, "Search returned None"
    assert len(result) > 0, "No results for hsa-miR-21-5p"

    # Check that we got miRDB results
    mirdb_results = [r for r in result if r.get("dataset") == "mirdb"]
    assert len(mirdb_results) > 0, "No miRDB results found"
    print(f"  Found {len(mirdb_results)} miRDB results for hsa-miR-21-5p")
    return True


def test_search_mouse_mirna(client):
    """Test searching for a mouse miRNA."""
    result = client.search("mmu-let-7a")
    assert result is not None, "Search returned None"
    assert len(result) > 0, "No results for mmu-let-7a"
    print(f"  Found {len(result)} results for mmu-let-7a")
    return True


def test_mirna_attributes(client):
    """Test that miRNA entries have expected attributes."""
    result = client.search("hsa-miR-21-5p", dataset="mirdb")
    assert result is not None and len(result) > 0, "No results"

    entry = result[0]
    attrs = entry.get("attributes", {})

    # Check required attributes
    assert "mirna_id" in attrs or "mirnaId" in attrs, "Missing mirna_id attribute"
    assert "species" in attrs, "Missing species attribute"
    assert "target_count" in attrs or "targetCount" in attrs, "Missing target_count attribute"

    # Check species value for human miRNA
    species = attrs.get("species", "")
    assert species == "hsa", f"Expected species 'hsa', got '{species}'"

    print(f"  Attributes verified: species={species}")
    return True


def test_map_to_refseq(client):
    """Test mapping miRNA to RefSeq targets."""
    result = client.map("hsa-let-7a-5p", ">>mirdb>>refseq")
    assert result is not None, "Map returned None"

    refseq_results = [r for r in result if r.get("dataset") == "refseq"]
    assert len(refseq_results) > 0, "No RefSeq targets found"

    print(f"  Found {len(refseq_results)} RefSeq targets for hsa-let-7a-5p")
    return True


def test_target_predictions(client):
    """Test that targets array is populated."""
    result = client.search("hsa-miR-21-5p", dataset="mirdb", mode="full")
    assert result is not None and len(result) > 0, "No results"

    entry = result[0]
    attrs = entry.get("attributes", {})

    targets = attrs.get("targets", [])
    assert len(targets) > 0, "No targets in entry"

    # Check target structure
    first_target = targets[0]
    assert "refseq_id" in first_target or "refseqId" in first_target, "Target missing refseq_id"
    assert "score" in first_target, "Target missing score"

    print(f"  Found {len(targets)} target predictions")
    return True


def test_species_distribution(client):
    """Test that we have miRNAs from different species."""
    species_prefixes = ["hsa", "mmu", "rno", "cfa", "gga"]
    found_species = []

    for prefix in species_prefixes:
        test_mirna = f"{prefix}-let-7a"
        result = client.search(test_mirna, dataset="mirdb")
        if result and len(result) > 0:
            found_species.append(prefix)

    assert len(found_species) >= 3, f"Expected at least 3 species, found {len(found_species)}: {found_species}"
    print(f"  Found miRNAs from {len(found_species)} species: {', '.join(found_species)}")
    return True


def main():
    """Run all miRDB tests."""
    # Default to localhost:9292
    base_url = os.environ.get("BIOBTREE_URL", "http://localhost:9292")
    client = BiobtreeTestClient(base_url)

    tests = [
        ("Search human miRNA", test_search_human_mirna),
        ("Search mouse miRNA", test_search_mouse_mirna),
        ("miRNA attributes", test_mirna_attributes),
        ("Map to RefSeq", test_map_to_refseq),
        ("Target predictions", test_target_predictions),
        ("Species distribution", test_species_distribution),
    ]

    passed = 0
    failed = 0

    print("\nmiRDB Integration Tests")
    print("=" * 50)

    for name, test_func in tests:
        try:
            print(f"\nRunning: {name}")
            if test_func(client):
                print(f"  PASSED")
                passed += 1
            else:
                print(f"  FAILED")
                failed += 1
        except Exception as e:
            print(f"  FAILED: {e}")
            failed += 1

    print("\n" + "=" * 50)
    print(f"Results: {passed} passed, {failed} failed")

    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
