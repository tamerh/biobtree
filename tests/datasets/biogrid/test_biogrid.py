#!/usr/bin/env python3
"""
BioGRID dataset integration tests.

This module tests the BioGRID protein-protein and genetic interaction database integration.
"""

import json
import urllib.request
import urllib.parse
import sys
import os

# Default biobtree URL
BIOBTREE_URL = os.environ.get("BIOBTREE_URL", "http://localhost:9292")


def fetch_json(url):
    """Fetch JSON from URL."""
    try:
        with urllib.request.urlopen(url, timeout=30) as response:
            return json.loads(response.read().decode('utf-8'))
    except Exception as e:
        print(f"Error fetching {url}: {e}")
        return None


def test_biogrid_basic_lookup():
    """Test basic BioGRID interactor lookup."""
    # Use a known BioGRID ID from test mode
    biogrid_id = "112315"

    url = f"{BIOBTREE_URL}/ws/?i={biogrid_id}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch BioGRID entry {biogrid_id}")
        return False

    results = result.get("results", [])
    if not results:
        print(f"FAIL: No results for BioGRID ID {biogrid_id}")
        return False

    # Check that we got a biogrid entry
    found_biogrid = False
    for entry in results:
        if entry.get("dataset_name") == "biogrid":
            found_biogrid = True
            break

    if not found_biogrid:
        print(f"FAIL: No biogrid entry found for {biogrid_id}")
        return False

    print(f"PASS: Basic BioGRID lookup for {biogrid_id}")
    return True


def test_biogrid_attributes():
    """Test BioGRID entry has expected attributes."""
    biogrid_id = "112315"

    url = f"{BIOBTREE_URL}/ws/?i={biogrid_id}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch BioGRID entry {biogrid_id}")
        return False

    results = result.get("results", [])

    # Find the biogrid entry
    biogrid_entry = None
    for entry in results:
        if entry.get("dataset_name") == "biogrid":
            biogrid_entry = entry
            break

    if not biogrid_entry:
        print(f"FAIL: No biogrid entry found for {biogrid_id}")
        return False

    # Check for biogrid attribute
    biogrid_attr = biogrid_entry.get("biogrid")
    if not biogrid_attr:
        print(f"FAIL: BioGRID entry missing 'biogrid' attribute")
        return False

    # Check required fields
    required_fields = ["biogrid_id", "interaction_count", "unique_partners"]
    missing = []
    for field in required_fields:
        if field not in biogrid_attr:
            missing.append(field)

    if missing:
        print(f"FAIL: BioGRID entry missing fields: {missing}")
        return False

    print(f"PASS: BioGRID attributes present for {biogrid_id}")
    return True


def test_biogrid_interactions():
    """Test BioGRID entry has interactions list."""
    biogrid_id = "112315"

    url = f"{BIOBTREE_URL}/ws/?i={biogrid_id}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch BioGRID entry {biogrid_id}")
        return False

    results = result.get("results", [])

    # Find the biogrid entry
    biogrid_entry = None
    for entry in results:
        if entry.get("dataset_name") == "biogrid":
            biogrid_entry = entry
            break

    if not biogrid_entry:
        print(f"FAIL: No biogrid entry found for {biogrid_id}")
        return False

    biogrid_attr = biogrid_entry.get("biogrid", {})
    interactions = biogrid_attr.get("interactions", [])

    if not interactions:
        print(f"FAIL: BioGRID entry has no interactions")
        return False

    # Check first interaction has expected fields
    first = interactions[0]
    expected_fields = ["interaction_id", "partner_biogrid_id", "experimental_system"]
    missing = [f for f in expected_fields if f not in first]

    if missing:
        print(f"FAIL: Interaction missing fields: {missing}")
        return False

    print(f"PASS: BioGRID has {len(interactions)} interactions for {biogrid_id}")
    return True


def test_biogrid_to_entrez_mapping():
    """Test mapping from BioGRID to Entrez Gene."""
    biogrid_id = "112315"

    chain = urllib.parse.quote(">>biogrid>>entrez")
    url = f"{BIOBTREE_URL}/ws/map/?i={biogrid_id}&m={chain}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch mapping for {biogrid_id}")
        return False

    results = result.get("results", [])
    if not results:
        print(f"FAIL: No mapping results for {biogrid_id}")
        return False

    # Check that we have entrez targets
    has_entrez = False
    for mapping in results:
        targets = mapping.get("targets", [])
        for target in targets:
            if target.get("dataset_name") == "entrez":
                has_entrez = True
                break

    if not has_entrez:
        print(f"FAIL: No Entrez Gene mappings found for {biogrid_id}")
        return False

    print(f"PASS: BioGRID -> Entrez mapping works for {biogrid_id}")
    return True


def test_biogrid_to_pubmed_mapping():
    """Test mapping from BioGRID to PubMed."""
    biogrid_id = "112315"

    chain = urllib.parse.quote(">>biogrid>>pubmed")
    url = f"{BIOBTREE_URL}/ws/map/?i={biogrid_id}&m={chain}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch mapping for {biogrid_id}")
        return False

    results = result.get("results", [])
    if not results:
        print(f"FAIL: No mapping results for {biogrid_id}")
        return False

    # Check that we have pubmed targets
    has_pubmed = False
    for mapping in results:
        targets = mapping.get("targets", [])
        for target in targets:
            if target.get("dataset_name") == "pubmed":
                has_pubmed = True
                break

    if not has_pubmed:
        print(f"FAIL: No PubMed mappings found for {biogrid_id}")
        return False

    print(f"PASS: BioGRID -> PubMed mapping works for {biogrid_id}")
    return True


def test_biogrid_partner_mapping():
    """Test mapping from BioGRID to partner BioGRID entries."""
    biogrid_id = "112315"

    chain = urllib.parse.quote(">>biogrid>>biogrid")
    url = f"{BIOBTREE_URL}/ws/map/?i={biogrid_id}&m={chain}"
    result = fetch_json(url)

    if not result:
        print(f"FAIL: Could not fetch mapping for {biogrid_id}")
        return False

    results = result.get("results", [])
    if not results:
        print(f"FAIL: No mapping results for {biogrid_id}")
        return False

    # Check that we have biogrid partner targets
    has_partner = False
    for mapping in results:
        targets = mapping.get("targets", [])
        for target in targets:
            if target.get("dataset_name") == "biogrid" and target.get("identifier") != biogrid_id:
                has_partner = True
                break

    if not has_partner:
        print(f"FAIL: No BioGRID partner mappings found for {biogrid_id}")
        return False

    print(f"PASS: BioGRID -> BioGRID partner mapping works for {biogrid_id}")
    return True


def main():
    """Run all tests."""
    print("=" * 60)
    print("BioGRID Dataset Integration Tests")
    print("=" * 60)
    print(f"Biobtree URL: {BIOBTREE_URL}")
    print()

    tests = [
        test_biogrid_basic_lookup,
        test_biogrid_attributes,
        test_biogrid_interactions,
        test_biogrid_to_entrez_mapping,
        test_biogrid_to_pubmed_mapping,
        test_biogrid_partner_mapping,
    ]

    passed = 0
    failed = 0

    for test in tests:
        try:
            if test():
                passed += 1
            else:
                failed += 1
        except Exception as e:
            print(f"ERROR in {test.__name__}: {e}")
            failed += 1

    print()
    print("=" * 60)
    print(f"Results: {passed} passed, {failed} failed")
    print("=" * 60)

    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
