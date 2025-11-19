#!/usr/bin/env python3
"""
Main Test Orchestrator

Manages biobtree web server and runs all dataset tests.
Runs biobtree from tests/tmp to avoid polluting directories with downloaded files.

Usage:
  python3 run_tests.py                    # Run all tests
  python3 run_tests.py hmdb               # Run only HMDB tests
  python3 run_tests.py hgnc,uniprot       # Run HGNC and UniProt tests
  python3 run_tests.py hmdb,go,taxonomy   # Run multiple specific tests
"""

import sys
import os
import time
import subprocess
import signal
import shutil
import argparse
from pathlib import Path

try:
    import requests
except ImportError:
    print("Error: requests library not found")
    print("Install with: pip install requests")
    sys.exit(1)


class Colors:
    GREEN = '\033[0;32m'
    RED = '\033[0;31m'
    YELLOW = '\033[1;33m'
    BLUE = '\033[0;34m'
    NC = '\033[0m'


class BiobtreeWebServer:
    """Manage biobtree web server for testing"""

    def __init__(self, out_dir: str, port: int = 9292):
        self.out_dir = out_dir
        self.port = port
        self.process = None
        self.base_url = f"http://localhost:{port}"

    def start(self) -> bool:
        """Start biobtree web server"""
        biobtree_path = Path(__file__).parent.parent / "biobtree"

        if not biobtree_path.exists():
            print(f"Error: biobtree not found at {biobtree_path}")
            return False

        print(f"Starting biobtree web server (port {self.port})...")

        try:
            self.process = subprocess.Popen(
                [str(biobtree_path), "--out-dir", self.out_dir, "web"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True
            )

            # Wait for server to be ready
            for i in range(30):
                try:
                    response = requests.get(f"{self.base_url}/ws/meta", timeout=2)
                    if response.status_code == 200:
                        print(f"{Colors.GREEN}✓{Colors.NC} Server started (PID: {self.process.pid})")
                        return True
                except:
                    pass
                time.sleep(1)
                if i % 5 == 0:
                    print(".", end="", flush=True)

            print(f"\n{Colors.RED}✗{Colors.NC} Server failed to start")
            # Print stderr for debugging
            if self.process.stderr:
                stderr = self.process.stderr.read()
                if stderr:
                    print(f"Server stderr: {stderr[:500]}")
            return False

        except Exception as e:
            print(f"{Colors.RED}✗{Colors.NC} Error starting server: {e}")
            return False

    def stop(self):
        """Stop biobtree web server"""
        if self.process:
            print(f"Stopping server (PID: {self.process.pid})...")
            try:
                self.process.send_signal(signal.SIGTERM)
                self.process.wait(timeout=5)
                print(f"{Colors.GREEN}✓{Colors.NC} Server stopped")
            except subprocess.TimeoutExpired:
                print(f"{Colors.YELLOW}⚠{Colors.NC} Server didn't stop gracefully, forcing...")
                self.process.kill()
                self.process.wait()
                print(f"{Colors.GREEN}✓{Colors.NC} Server killed")
            except Exception as e:
                print(f"{Colors.RED}✗{Colors.NC} Error stopping server: {e}")

    def is_running(self) -> bool:
        """Check if server is running"""
        try:
            response = requests.get(f"{self.base_url}/ws/meta", timeout=2)
            return response.status_code == 200
        except:
            return False


def run_dataset_tests(test_script: Path, api_url: str) -> int:
    """Run tests for a specific dataset"""
    if not test_script.exists():
        print(f"{Colors.YELLOW}⚠{Colors.NC} Test script not found: {test_script}")
        return 1

    print(f"\n{Colors.BLUE}Running {test_script.parent.name} tests...{Colors.NC}")
    print("─" * 60)

    try:
        # Set API_URL environment variable for test script
        env = os.environ.copy()
        env['BIOBTREE_API_URL'] = api_url

        result = subprocess.run(
            [sys.executable, str(test_script)],
            env=env,
            cwd=str(test_script.parent),
            capture_output=False
        )

        return result.returncode

    except Exception as e:
        print(f"{Colors.RED}✗{Colors.NC} Error running tests: {e}")
        return 1


def build_test_database(biobtree_path: Path, datasets: str, cwd: Path = None, genome_taxids: str = None) -> bool:
    """Build test database with specified datasets"""
    print("=" * 60)
    print("  Step 1: Building Test Database")
    print("=" * 60)
    print(f"  Datasets: {datasets}")
    if genome_taxids:
        print(f"  Genome taxids: {genome_taxids}")
    print()

    try:
        cmd = [str(biobtree_path), "-d", datasets,"--keep"]
        if genome_taxids:
            cmd.extend(["--genome-taxids", genome_taxids])
        cmd.append("test")

        result = subprocess.run(
            cmd,
            capture_output=False,
            text=True,
            cwd=str(cwd) if cwd else None
        )

        if result.returncode == 0:
            print()
            print(f"{Colors.GREEN}✓{Colors.NC} Test database built successfully")
            return True
        else:
            print()
            print(f"{Colors.RED}✗{Colors.NC} Test database build failed")
            return False

    except Exception as e:
        print(f"{Colors.RED}✗{Colors.NC} Error building test database: {e}")
        return False


def main():
    # Parse command-line arguments
    parser = argparse.ArgumentParser(
        description='Run biobtree test suite',
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s                    # Run all tests
  %(prog)s hmdb               # Run only HMDB tests
  %(prog)s hgnc,uniprot       # Run HGNC and UniProt tests
  %(prog)s hmdb,go,taxonomy   # Run multiple specific tests

Available datasets:
  hgnc, uniprot, go, taxonomy, eco, efo, chebi, interpro, hmdb, lipidmaps, swisslipids, chembl_document, chembl_molecule, chembl_activity, chembl_assay, chembl_target, chembl_cell_line, ensembl, mondo, hpo, uberon, cl, bgee, patent, clinical_trials, clinvar, string, reactome, rhea, alphafold, rnacentral, uniparc, uniref50, uniref90, uniref100, gwas_study, gwas, dbsnp, intact, protein_similarity

  Temporarily disabled (Ensembl Genomes API SSL issues):
  ensembl_bacteria, ensembl_fungi, ensembl_metazoa, ensembl_plants, ensembl_protists
        """
    )
    parser.add_argument(
        'datasets',
        nargs='?',
        default='all',
        help='Comma-separated list of datasets to test (default: all)'
    )
    args = parser.parse_args()

    script_dir = Path(__file__).parent
    datasets_dir = script_dir / "datasets"  # All dataset tests now under datasets/
    project_root = script_dir.parent
    out_dir = project_root / "test_out"
    db_dir = out_dir / "db"
    biobtree_path = project_root / "biobtree"

    # clean the out directory
    if out_dir.exists() and out_dir.is_dir():
        shutil.rmtree(out_dir)

    # Recreate the directory after cleaning
    out_dir.mkdir(parents=True, exist_ok=True)

    # Define all available datasets and their test scripts
    all_datasets = {
        'hgnc': datasets_dir / "hgnc" / "test_hgnc.py",
        'uniprot': datasets_dir / "uniprot" / "test_uniprot.py",
        'go': datasets_dir / "go" / "test_go.py",
        'taxonomy': datasets_dir / "taxonomy" / "test_taxonomy.py",
        'eco': datasets_dir / "eco" / "test_eco.py",
        'efo': datasets_dir / "efo" / "test_efo.py",
        'chebi': datasets_dir / "chebi" / "test_chebi.py",
        'interpro': datasets_dir / "interpro" / "test_interpro.py",
        #'hmdb': datasets_dir / "hmdb" / "test_hmdb.py",
        'lipidmaps': datasets_dir / "lipidmaps" / "test_lipidmaps.py",
        'swisslipids': datasets_dir / "swisslipids" / "test_swisslipids.py",
        'chembl_document': datasets_dir / "chembl_document" / "test_chembl_document.py",
        'chembl_molecule': datasets_dir / "chembl_molecule" / "test_chembl_molecule.py",
        'chembl_activity': datasets_dir / "chembl_activity" / "test_chembl_activity.py",
        'chembl_assay': datasets_dir / "chembl_assay" / "test_chembl_assay.py",
        'chembl_target': datasets_dir / "chembl_target" / "test_chembl_target.py",
        'chembl_cell_line': datasets_dir / "chembl_cell_line" / "test_chembl_cell_line.py",
        'ensembl': datasets_dir / "ensembl" / "test_ensembl.py",
        # Temporarily disabled due to Ensembl Genomes API SSL issues
        # 'ensembl_bacteria': datasets_dir / "ensembl_bacteria" / "test_ensembl_bacteria.py",
        # 'ensembl_fungi': datasets_dir / "ensembl_fungi" / "test_ensembl_fungi.py",
        # 'ensembl_metazoa': datasets_dir / "ensembl_metazoa" / "test_ensembl_metazoa.py",
        # 'ensembl_plants': datasets_dir / "ensembl_plants" / "test_ensembl_plants.py",
        # 'ensembl_protists': datasets_dir / "ensembl_protists" / "test_ensembl_protists.py",
        'mondo': datasets_dir / "mondo" / "test_mondo.py",
        'hpo': datasets_dir / "hpo" / "test_hpo.py",
        'uberon': datasets_dir / "uberon" / "test_uberon.py",
        'cl': datasets_dir / "cl" / "test_cl.py",
        'bgee': datasets_dir / "bgee" / "test_bgee.py",
        'patent': datasets_dir / "patent" / "test_patent.py",
        'clinical_trials': datasets_dir / "clinical_trials" / "test_clinical_trials.py",
        'clinvar': datasets_dir / "clinvar" / "test_clinvar.py",
        'string': datasets_dir / "string" / "test_string.py",
        'reactome': datasets_dir / "reactome" / "test_reactome.py",
        'rhea': datasets_dir / "rhea" / "test_rhea.py",
        'alphafold': datasets_dir / "alphafold" / "test_alphafold.py",
        'rnacentral': datasets_dir / "rnacentral" / "test_rnacentral.py",
        'uniparc': datasets_dir / "uniparc" / "test_uniparc.py",
        'uniref50': datasets_dir / "uniref50" / "test_uniref50.py",
        'uniref90': datasets_dir / "uniref90" / "test_uniref90.py",
        'uniref100': datasets_dir / "uniref100" / "test_uniref100.py",
        'gwas_study': datasets_dir / "gwas_study" / "test_gwas_study.py",
        'gwas': datasets_dir / "gwas" / "test_gwas.py",
        'dbsnp': datasets_dir / "dbsnp" / "test_dbsnp.py",
        'intact': datasets_dir / "intact" / "test_intact.py",
        'protein_similarity': datasets_dir / "protein_similarity" / "test_protein_similarity.py",
    }

    # Parse dataset selection
    if args.datasets.lower() == 'all':
        selected_datasets = list(all_datasets.keys())
    else:
        selected_datasets = [d.strip().lower() for d in args.datasets.split(',')]
        # Validate dataset names
        invalid = [d for d in selected_datasets if d not in all_datasets]
        if invalid:
            print(f"{Colors.RED}Error:{Colors.NC} Unknown dataset(s): {', '.join(invalid)}")
            print(f"Available datasets: {', '.join(all_datasets.keys())}")
            return 1

    print("=" * 60)
    print("  Biobtree Test Suite Orchestrator")
    print("=" * 60)
    print(f"  Selected datasets: {', '.join(selected_datasets)}")
    print("=" * 60)
    print()

    # Check biobtree exists
    if not biobtree_path.exists():
        print(f"{Colors.RED}Error:{Colors.NC} biobtree not found at {biobtree_path}")
        return 1

    # Add dataset dependencies for database build
    # (tests may validate data from related datasets)
    build_datasets = selected_datasets.copy()
    if 'chembl_target' in selected_datasets and 'chembl_target_component' not in build_datasets:
        build_datasets.append('chembl_target_component')

    # Handle Ensembl datasets: when any Ensembl division is selected, build all with genome-taxids
    ensembl_datasets = {'ensembl', 'ensembl_bacteria', 'ensembl_fungi', 'ensembl_metazoa', 'ensembl_plants', 'ensembl_protists'}
    selected_ensembl = [d for d in selected_datasets if d in ensembl_datasets]

    genome_taxids = None
    if selected_ensembl:
        # When any Ensembl division is selected, build all divisions with their respective genome taxids
        # Taxids: homo_sapiens (9606), escherichia_coli (1268975), aspergillus_fumigatus (330879),
        #         drosophila_melanogaster (7227), arabidopsis_thaliana (3702), plasmodium_falciparum (36329)
        genome_taxids = "9606,1268975,330879,7227,3702,36329"

        # Ensure all Ensembl divisions are built together (they share genomes)
        for ensembl_ds in ensembl_datasets:
            if ensembl_ds not in build_datasets:
                build_datasets.append(ensembl_ds)

    # Handle STRING dataset: requires taxonomy ID (human: 9606)
    if 'string' in selected_datasets:
        # STRING test data is for human only
        if genome_taxids and genome_taxids != "9606":
            # Already has taxids from Ensembl - keep them
            pass
        else:
            genome_taxids = "9606"
        # STRING requires UniProt for mapping
        if 'uniprot' not in build_datasets:
            build_datasets.append('uniprot')

    # Build test database with selected datasets (including dependencies)
    datasets_str = ','.join(build_datasets)
    if not build_test_database(biobtree_path, datasets_str, cwd=project_root, genome_taxids=genome_taxids):
        return 1

    print()
    print("=" * 60)
    print("  Step 2: Running Test Suites")
    print("=" * 60)
    print()

    # Start server
    server = BiobtreeWebServer(str(out_dir), port=9292)

    all_tests_passed = True

    try:
        if not server.start():
            return 1

        print()

        # Run selected dataset tests
        results = {}
        for dataset_name in selected_datasets:
            test_script = all_datasets[dataset_name]
            exit_code = run_dataset_tests(test_script, server.base_url)
            results[dataset_name] = exit_code
            if exit_code != 0:
                all_tests_passed = False

        # Print summary
        print()
        print("=" * 60)
        print("  OVERALL TEST SUMMARY")
        print("=" * 60)

        for dataset_name, exit_code in results.items():
            status = f"{Colors.GREEN}✓ PASSED{Colors.NC}" if exit_code == 0 else f"{Colors.RED}✗ FAILED{Colors.NC}"
            print(f"  {dataset_name.upper()}: {status}")

        print("=" * 60)

        if all_tests_passed:
            print(f"{Colors.GREEN}✓ ALL TEST SUITES PASSED{Colors.NC}")
            exit_code = 0
        else:
            print(f"{Colors.RED}✗ SOME TEST SUITES FAILED{Colors.NC}")
            exit_code = 1

    except KeyboardInterrupt:
        print("\n\nInterrupted by user")
        exit_code = 1

    finally:
        # IMPORTANT: Always stop server
        print()
        server.stop()

    return exit_code


if __name__ == "__main__":
    sys.exit(main())
