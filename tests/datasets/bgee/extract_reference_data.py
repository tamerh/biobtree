#!/usr/bin/env python3
"""
Extract Bgee reference data from TSV file for test gene IDs.

Bgee doesn't have a public REST API, so we extract data directly from the
TSV file that was used to build the test database.
"""

import gzip
import json
from pathlib import Path
import sys

def parse_float(s):
    """Parse float safely"""
    try:
        return float(s)
    except (ValueError, TypeError):
        return 0.0

def parse_int(s):
    """Parse int safely"""
    try:
        return int(s)
    except (ValueError, TypeError):
        return 0

def parse_bool(s):
    """Parse boolean safely"""
    return s in ("yes", "t", "true", "1")

def parse_data_type(fields):
    """Parse 9-column data type expression block"""
    if len(fields) < 9 or fields[0] in ("", "no data"):
        return None

    return {
        "expression": fields[0],
        "call_quality": fields[1],
        "fdr": parse_float(fields[2]),
        "expression_score": parse_float(fields[3]),
        "expression_rank": parse_float(fields[4]),
        "weight": parse_float(fields[5]),
        "including_observed_data": parse_bool(fields[6]),
        "self_observation_count": parse_int(fields[7]),
        "descendant_observation_count": parse_int(fields[8])
    }

def extract_reference_data():
    """Extract reference data for test gene IDs from Bgee TSV file"""

    script_dir = Path(__file__).parent
    ids_file = script_dir / "bgee_ids.txt"
    output_file = script_dir / "reference_data.json"

    # Find the Bgee TSV file
    bgee_dir = Path(__file__).parent.parent.parent.parent / "bgee"
    tsv_file = bgee_dir / "Homo_sapiens_expr_advanced.tsv"

    if not tsv_file.exists():
        print(f"Error: Bgee TSV file not found: {tsv_file}")
        print("Expected location: bgee/Homo_sapiens_expr_advanced.tsv")
        return 1

    if not ids_file.exists():
        print(f"Error: {ids_file} not found")
        print("Run test build first: ./biobtree -d 'bgee' test")
        return 1

    # Load test IDs
    with open(ids_file) as f:
        test_ids = set(line.strip() for line in f if line.strip())

    print(f"Loaded {len(test_ids)} test gene IDs")
    print(f"Extracting reference data from {tsv_file}...")

    reference_data = []
    genes_found = {}

    # Read TSV file and extract data for our test genes
    with open(tsv_file, 'r') as f:
        # Skip header
        header = f.readline()

        for line_num, line in enumerate(f, 2):
            if line_num % 100000 == 0:
                print(f"  Processed {line_num:,} lines, found {len(genes_found)} genes...")

            parts = line.strip().split('\t')
            if len(parts) < 57:
                continue

            gene_id = parts[0]
            if gene_id not in test_ids:
                continue

            gene_name = parts[1].strip('"')

            # Initialize gene if first time seeing it
            if gene_id not in genes_found:
                genes_found[gene_id] = {
                    "id": gene_id,
                    "gene_name": gene_name,
                    "expression_conditions": []
                }

            # Parse expression condition
            condition = {
                "anatomical_entity_id": parts[2],
                "anatomical_entity_name": parts[3].strip('"'),
                "expression": parts[4],
                "call_quality": parts[5],
                "fdr": parse_float(parts[6]),
                "expression_score": parse_float(parts[7]),
                "expression_rank": parse_float(parts[8]),
                "including_observed_data": parse_bool(parts[9]),
                "self_observation_count": parse_int(parts[10]),
                "descendant_observation_count": parse_int(parts[11])
            }

            # Parse per-data-type information
            condition["affymetrix"] = parse_data_type(parts[12:21])
            condition["est"] = parse_data_type(parts[21:30])
            condition["in_situ"] = parse_data_type(parts[30:39])
            condition["rna_seq"] = parse_data_type(parts[39:48])
            condition["single_cell"] = parse_data_type(parts[48:57])

            genes_found[gene_id]["expression_conditions"].append(condition)

            # Early exit if we've found all genes
            if len(genes_found) == len(test_ids):
                print(f"  Found all {len(test_ids)} genes at line {line_num:,}")
                break

    # Convert to list and add summary statistics
    for gene_id in test_ids:
        if gene_id in genes_found:
            gene = genes_found[gene_id]
            conditions = gene["expression_conditions"]

            # Add summary stats
            gene["total_conditions"] = len(conditions)
            gene["present_count"] = sum(1 for c in conditions if c["expression"] == "present")
            gene["absent_count"] = sum(1 for c in conditions if c["expression"] == "absent")
            gene["gold_quality_count"] = sum(1 for c in conditions if c["call_quality"] == "gold quality")

            # Get top expressed tissues
            present_conditions = [c for c in conditions if c["expression"] == "present"]
            present_conditions.sort(key=lambda x: x["expression_score"], reverse=True)
            gene["top_expressed_tissues"] = [
                {
                    "tissue": c["anatomical_entity_name"],
                    "tissue_id": c["anatomical_entity_id"],
                    "score": c["expression_score"]
                }
                for c in present_conditions[:5]
            ]

            # Count data types
            gene["data_type_counts"] = {
                "affymetrix": sum(1 for c in conditions if c.get("affymetrix")),
                "rna_seq": sum(1 for c in conditions if c.get("rna_seq")),
                "single_cell": sum(1 for c in conditions if c.get("single_cell")),
                "est": sum(1 for c in conditions if c.get("est")),
                "in_situ": sum(1 for c in conditions if c.get("in_situ"))
            }

            reference_data.append(gene)

    # Sort by gene ID
    reference_data.sort(key=lambda x: x["id"])

    # Save reference data (JSON)
    with open(output_file, 'w') as f:
        json.dump(reference_data, f, indent=2)

    print(f"\n✓ Extracted data for {len(reference_data)} genes")
    print(f"✓ Saved to {output_file}")

    # Also save as TSV with all expression conditions (raw reference data)
    tsv_file = script_dir / "reference_data.tsv"
    with open(tsv_file, 'w') as f:
        # Header matching advanced format columns
        f.write("gene_id\tgene_name\tanatomical_entity_id\tanatomical_entity_name\t")
        f.write("expression\tcall_quality\tfdr\texpression_score\texpression_rank\t")
        f.write("including_observed_data\tself_observation_count\tdescendant_observation_count\t")
        f.write("affymetrix_expression\taffymetrix_call_quality\taffymetrix_fdr\t")
        f.write("affymetrix_expression_score\taffymetrix_expression_rank\taffymetrix_weight\t")
        f.write("affymetrix_including_observed_data\taffymetrix_self_observation_count\taffymetrix_descendant_observation_count\t")
        f.write("est_expression\test_call_quality\test_fdr\t")
        f.write("est_expression_score\test_expression_rank\test_weight\t")
        f.write("est_including_observed_data\test_self_observation_count\test_descendant_observation_count\t")
        f.write("in_situ_expression\tin_situ_call_quality\tin_situ_fdr\t")
        f.write("in_situ_expression_score\tin_situ_expression_rank\tin_situ_weight\t")
        f.write("in_situ_including_observed_data\tin_situ_self_observation_count\tin_situ_descendant_observation_count\t")
        f.write("rna_seq_expression\trna_seq_call_quality\trna_seq_fdr\t")
        f.write("rna_seq_expression_score\trna_seq_expression_rank\trna_seq_weight\t")
        f.write("rna_seq_including_observed_data\trna_seq_self_observation_count\trna_seq_descendant_observation_count\t")
        f.write("single_cell_expression\tsingle_cell_call_quality\tsingle_cell_fdr\t")
        f.write("single_cell_expression_score\tsingle_cell_expression_rank\tsingle_cell_weight\t")
        f.write("single_cell_including_observed_data\tsingle_cell_self_observation_count\tsingle_cell_descendant_observation_count\n")

        # Data rows - one row per expression condition
        for gene in reference_data:
            for cond in gene['expression_conditions']:
                f.write(f"{gene['id']}\t{gene['gene_name']}\t")
                f.write(f"{cond['anatomical_entity_id']}\t{cond['anatomical_entity_name']}\t")
                f.write(f"{cond['expression']}\t{cond['call_quality']}\t{cond['fdr']}\t")
                f.write(f"{cond['expression_score']}\t{cond['expression_rank']}\t")
                f.write(f"{cond['including_observed_data']}\t{cond['self_observation_count']}\t")
                f.write(f"{cond['descendant_observation_count']}\t")

                # Affymetrix data
                affy = cond.get('affymetrix')
                if affy:
                    f.write(f"{affy['expression']}\t{affy['call_quality']}\t{affy['fdr']}\t")
                    f.write(f"{affy['expression_score']}\t{affy['expression_rank']}\t{affy['weight']}\t")
                    f.write(f"{affy['including_observed_data']}\t{affy['self_observation_count']}\t")
                    f.write(f"{affy['descendant_observation_count']}\t")
                else:
                    f.write("\t\t\t\t\t\t\t\t\t")

                # EST data
                est = cond.get('est')
                if est:
                    f.write(f"{est['expression']}\t{est['call_quality']}\t{est['fdr']}\t")
                    f.write(f"{est['expression_score']}\t{est['expression_rank']}\t{est['weight']}\t")
                    f.write(f"{est['including_observed_data']}\t{est['self_observation_count']}\t")
                    f.write(f"{est['descendant_observation_count']}\t")
                else:
                    f.write("\t\t\t\t\t\t\t\t\t")

                # In situ data
                in_situ = cond.get('in_situ')
                if in_situ:
                    f.write(f"{in_situ['expression']}\t{in_situ['call_quality']}\t{in_situ['fdr']}\t")
                    f.write(f"{in_situ['expression_score']}\t{in_situ['expression_rank']}\t{in_situ['weight']}\t")
                    f.write(f"{in_situ['including_observed_data']}\t{in_situ['self_observation_count']}\t")
                    f.write(f"{in_situ['descendant_observation_count']}\t")
                else:
                    f.write("\t\t\t\t\t\t\t\t\t")

                # RNA-Seq data
                rnaseq = cond.get('rna_seq')
                if rnaseq:
                    f.write(f"{rnaseq['expression']}\t{rnaseq['call_quality']}\t{rnaseq['fdr']}\t")
                    f.write(f"{rnaseq['expression_score']}\t{rnaseq['expression_rank']}\t{rnaseq['weight']}\t")
                    f.write(f"{rnaseq['including_observed_data']}\t{rnaseq['self_observation_count']}\t")
                    f.write(f"{rnaseq['descendant_observation_count']}\t")
                else:
                    f.write("\t\t\t\t\t\t\t\t\t")

                # Single-cell data
                sc = cond.get('single_cell')
                if sc:
                    f.write(f"{sc['expression']}\t{sc['call_quality']}\t{sc['fdr']}\t")
                    f.write(f"{sc['expression_score']}\t{sc['expression_rank']}\t{sc['weight']}\t")
                    f.write(f"{sc['including_observed_data']}\t{sc['self_observation_count']}\t")
                    f.write(f"{sc['descendant_observation_count']}\n")
                else:
                    f.write("\t\t\t\t\t\t\t\t\n")

    print(f"✓ Saved raw reference data to {tsv_file}")

    # Print summary
    if reference_data:
        total_conditions = sum(g["total_conditions"] for g in reference_data)
        print(f"\nSummary:")
        print(f"  Total expression conditions: {total_conditions:,}")
        print(f"  Avg conditions per gene: {total_conditions / len(reference_data):.1f}")
        print(f"\nExample gene:")
        example = reference_data[0]
        print(f"  {example['id']} ({example['gene_name']})")
        print(f"  {example['total_conditions']} conditions ({example['present_count']} present, {example['absent_count']} absent)")
        if example['top_expressed_tissues']:
            print(f"  Top tissue: {example['top_expressed_tissues'][0]['tissue']} (score: {example['top_expressed_tissues'][0]['score']:.2f})")

    return 0

if __name__ == "__main__":
    sys.exit(extract_reference_data())
