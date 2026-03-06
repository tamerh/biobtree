#!/usr/bin/env python3
"""
Extract reference data from SC Expression Atlas API for testing.

This script fetches experiment metadata from the public SCXA API
and saves a subset for use in test validation.
"""

import json
import requests
from pathlib import Path

def fetch_scxa_experiments():
    """Fetch experiments from SCXA API"""
    url = "https://www.ebi.ac.uk/gxa/sc/json/experiments"
    print(f"Fetching experiments from {url}...")

    response = requests.get(url, timeout=30)
    response.raise_for_status()

    data = response.json()
    experiments = data.get("experiments", [])
    print(f"Found {len(experiments)} experiments")

    return experiments

def transform_experiment(exp):
    """Transform experiment to reference data format"""
    return {
        "id": exp.get("experimentAccession"),
        "description": exp.get("experimentDescription"),
        "species": exp.get("species"),
        "kingdom": exp.get("kingdom"),
        "load_date": exp.get("loadDate"),
        "last_update": exp.get("lastUpdate"),
        "experiment_type": exp.get("experimentType"),
        "raw_experiment_type": exp.get("rawExperimentType"),
        "technology_types": exp.get("technologyType", []),
        "number_of_cells": exp.get("numberOfAssays", 0),
        "experimental_factors": exp.get("experimentalFactors", []),
        "experiment_projects": exp.get("experimentProjects", []),
    }

def main():
    script_dir = Path(__file__).parent
    output_file = script_dir / "reference_data.json"

    # Fetch all experiments
    experiments = fetch_scxa_experiments()

    # Transform and filter for test data
    # Take a diverse sample: first 20 experiments
    reference_data = []
    for exp in experiments[:20]:
        ref = transform_experiment(exp)
        if ref.get("id"):
            reference_data.append(ref)

    # Sort by ID for consistency
    reference_data.sort(key=lambda x: x["id"])

    # Save reference data
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"Saved {len(reference_data)} experiments to {output_file}")

    # Print some statistics
    total_cells = sum(e.get("number_of_cells", 0) for e in reference_data)
    species_counts = {}
    for e in reference_data:
        sp = e.get("species", "unknown")
        species_counts[sp] = species_counts.get(sp, 0) + 1

    print(f"\nStatistics:")
    print(f"  Total cells: {total_cells:,}")
    print(f"  Species distribution:")
    for sp, count in sorted(species_counts.items(), key=lambda x: -x[1]):
        print(f"    {sp}: {count}")

if __name__ == "__main__":
    main()
