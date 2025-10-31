#!/usr/bin/env python3
"""
Main Test Orchestrator

Manages biobtree web server and runs all dataset tests.
Runs biobtree from tests/tmp to avoid polluting directories with downloaded files.
"""

import sys
import os
import time
import subprocess
import signal
import shutil
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


def build_test_database(biobtree_path: Path, datasets: str, cwd: Path = None) -> bool:
    """Build test database with specified datasets"""
    print("=" * 60)
    print("  Step 1: Building Test Database")
    print("=" * 60)
    print(f"  Datasets: {datasets}")
    print()

    try:
        result = subprocess.run(
            [str(biobtree_path), "-d", datasets, "test"],
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
    script_dir = Path(__file__).parent
    project_root = script_dir.parent
    out_dir = project_root / "test_out"
    db_dir = out_dir / "db"
    biobtree_path = project_root / "biobtree"

    print("=" * 60)
    print("  Biobtree Test Suite Orchestrator")
    print("=" * 60)
    print()

    # Check biobtree exists
    if not biobtree_path.exists():
        print(f"{Colors.RED}Error:{Colors.NC} biobtree not found at {biobtree_path}")
        return 1

    # Build test database
    datasets = "hgnc,uniprot,go,taxonomy,uniparc,uniref100,uniref50,uniref90,eco,chebi,interpro"
    if not build_test_database(biobtree_path, datasets, cwd=project_root):
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

        # Run individual dataset tests
        test_scripts = [
            script_dir / "hgnc" / "test_hgnc.py",
            script_dir / "uniprot" / "test_uniprot.py",
            script_dir / "go" / "test_go.py",
            script_dir / "taxonomy" / "test_taxonomy.py",
            script_dir / "uniparc" / "test_uniparc.py",
            script_dir / "uniref100" / "test_uniref100.py",
            script_dir / "uniref50" / "test_uniref50.py",
            script_dir / "uniref90" / "test_uniref90.py",
            script_dir / "eco" / "test_eco.py",
            script_dir / "chebi" / "test_chebi.py",
            script_dir / "interpro" / "test_interpro.py",
        ]

        results = {}
        for test_script in test_scripts:
            dataset_name = test_script.parent.name
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
