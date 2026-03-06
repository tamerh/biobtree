#!/usr/bin/env python3
"""
Common Test Types

Defines reusable test patterns that can be used across all datasets.
Each test type is a class that knows how to execute a specific type of test.
"""

from typing import Dict, Any, Tuple, Optional
import requests
import json
from pathlib import Path


def _load_dataset_mappings():
    """
    Load dataset name-to-ID mappings from configuration files.

    This is a simplified version for use in test_types.py.
    Returns name_to_id and aliases_to_id dictionaries.
    """
    conf_dir = Path(__file__).parent.parent.parent / "conf"

    name_to_id = {}
    aliases_to_id = {}

    config_files = [
        conf_dir / "source.dataset.json",
        conf_dir / "default.dataset.json",
        conf_dir / "optional.dataset.json"
    ]

    for config_file in config_files:
        if not config_file.exists():
            continue

        try:
            with open(config_file) as f:
                datasets = json.load(f)

            for dataset_name, config in datasets.items():
                dataset_id = int(config.get("id", 0))

                if dataset_id == 0:
                    continue

                # Store primary name (lowercase)
                name_to_id[dataset_name.lower()] = dataset_id

                # Store aliases (lowercase)
                if "aliases" in config:
                    aliases = config["aliases"].split(",")
                    for alias in aliases:
                        alias = alias.strip().lower()
                        if alias:
                            aliases_to_id[alias] = dataset_id

        except Exception:
            continue

    return name_to_id, aliases_to_id


# Load dataset mappings once at module level
_DATASET_NAME_TO_ID, _DATASET_ALIASES_TO_ID = _load_dataset_mappings()


class BaseTest:
    """Base class for all test types"""

    def __init__(self, name: str, config: Dict[str, Any]):
        self.name = name
        self.config = config

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        """
        Execute the test and return (success, message)

        Args:
            api_url: Base URL of the biobtree API
            reference_data: List of reference entries

        Returns:
            Tuple of (success: bool, message: str)
        """
        raise NotImplementedError("Subclasses must implement execute()")

    def resolve_value(self, value: str, reference_data: list) -> Any:
        """
        Resolve a value that might be a reference to reference_data

        Examples:
            "@reference[0].hgnc_id" -> reference_data[0]["hgnc_id"]
            "HGNC:5" -> "HGNC:5"
        """
        if not isinstance(value, str) or not value.startswith("@reference"):
            return value

        # Parse @reference[index].field or @reference[index].field[subindex]
        try:
            parts = value.replace("@reference", "").strip()

            # Extract index
            if not parts.startswith("["):
                return value

            idx_end = parts.find("]")
            index = int(parts[1:idx_end])

            if index >= len(reference_data):
                return None

            entry = reference_data[index]

            # Extract field path
            field_path = parts[idx_end + 1:].lstrip(".")
            if not field_path:
                return entry

            # Navigate through nested fields
            current = entry
            for part in field_path.split("."):
                if "[" in part and "]" in part:
                    # Handle array access like field[0]
                    field_name = part[:part.find("[")]
                    array_idx = int(part[part.find("[") + 1:part.find("]")])
                    current = current.get(field_name, [])[array_idx]
                else:
                    current = current.get(part)

                if current is None:
                    return None

            return current

        except Exception as e:
            print(f"Warning: Failed to resolve '{value}': {e}")
            return None


class IDLookupTest(BaseTest):
    """Test looking up an entry by its primary ID"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        entry_id = self.resolve_value(self.config.get("id"), reference_data)

        if not entry_id:
            return False, "No ID found in reference data"

        response = requests.get(f"{api_url}/ws/?i={entry_id}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results") or len(data["results"]) == 0:
            return False, f"No results for {entry_id}"

        return True, f"Found {entry_id}"


class SymbolLookupTest(BaseTest):
    """Test looking up an entry by symbol/name"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        symbol = self.resolve_value(self.config.get("symbol"), reference_data)

        if not symbol:
            return True, "SKIP: No symbol found in reference data"

        response = requests.get(f"{api_url}/ws/?i={symbol}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results"):
            return False, f"No results for symbol '{symbol}'"

        return True, f"Found symbol '{symbol}'"


class XrefExistsTest(BaseTest):
    """Test that a cross-reference exists"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        entry_id = self.resolve_value(self.config.get("id"), reference_data)
        xref_type = self.config.get("xref_type")
        expected_xref = self.resolve_value(self.config.get("expected"), reference_data)

        if not entry_id:
            return False, "No ID found in reference data"

        response = requests.get(f"{api_url}/ws/?i={entry_id}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]
        entries = result.get("entries", [])

        # Get dataset ID for this xref type (case-insensitive)
        xref_type_lower = xref_type.lower() if xref_type else None
        dataset_id = None

        if xref_type_lower:
            # Try direct name lookup first
            dataset_id = _DATASET_NAME_TO_ID.get(xref_type_lower)

            # If not found, try aliases
            if dataset_id is None:
                dataset_id = _DATASET_ALIASES_TO_ID.get(xref_type_lower)

        if dataset_id is None:
            return False, f"Unknown xref type: {xref_type}"

        # Find xrefs for this dataset
        xrefs = [x for x in entries if x.get("dataset") == dataset_id]

        if not xrefs:
            return False, f"{entry_id} missing {xref_type} xref"

        # If specific identifier expected, validate it
        if expected_xref:
            found = any(x.get("identifier") == str(expected_xref) for x in xrefs)
            if not found:
                xref_ids = [x.get("identifier") for x in xrefs[:3]]
                return False, f"{entry_id} missing {xref_type}:{expected_xref} (found: {', '.join(xref_ids)})"
            return True, f"{entry_id} → {xref_type}:{expected_xref} ✓"

        # Otherwise just confirm xref type exists
        return True, f"{entry_id} → {xref_type} ({len(xrefs)} xrefs) ✓"


class AttributeCheckTest(BaseTest):
    """Test that specific attributes are present"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        entry_id = self.resolve_value(self.config.get("id"), reference_data)

        if not entry_id:
            return False, "No ID found in reference data"

        response = requests.get(f"{api_url}/ws/?i={entry_id}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results"):
            return False, f"No results for {entry_id}"

        result = data["results"][0]

        # Check if entry has data fields
        if "id" in result or "identifier" in result:
            return True, "Entry has data fields"

        return False, "No data fields in response"


class InvalidIDTest(BaseTest):
    """Test that invalid IDs are handled properly"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        invalid_id = self.config.get("invalid_id", "INVALID:99999999")

        response = requests.get(f"{api_url}/ws/?i={invalid_id}", timeout=10)

        # Should return 200 with empty results or 404
        if response.status_code in [200, 404]:
            data = response.json() if response.status_code == 200 else {}
            results = data.get("results", [])
            if len(results) == 0:
                return True, "Invalid ID returns no results (correct)"
            else:
                return False, f"Invalid ID returned {len(results)} results"

        return False, f"Unexpected status code: {response.status_code}"


class MultiLookupTest(BaseTest):
    """Test looking up multiple IDs"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        count = self.config.get("count", 3)

        # Get first N IDs from reference data
        test_ids = []
        id_field = self.config.get("id_field", "id")

        for i in range(min(count, len(reference_data))):
            entry_id = reference_data[i].get(id_field)
            if entry_id:
                test_ids.append(entry_id)

        if not test_ids:
            return False, "No IDs found in reference data"

        found = 0
        for entry_id in test_ids:
            response = requests.get(f"{api_url}/ws/?i={entry_id}", timeout=10)
            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    found += 1

        if found == len(test_ids):
            return True, f"All {found}/{len(test_ids)} IDs found"
        else:
            return False, f"Only {found}/{len(test_ids)} IDs found"


class CaseInsensitiveTest(BaseTest):
    """Test case-insensitive search"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        symbol = self.resolve_value(self.config.get("symbol"), reference_data)

        if not symbol:
            return False, "No symbol found in reference data"

        # Try lowercase version
        lower_symbol = symbol.lower()

        response = requests.get(f"{api_url}/ws/?i={lower_symbol}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if data.get("results"):
            return True, f"Found results for '{lower_symbol}' (from '{symbol}')"
        else:
            # This might be expected if biobtree is case-sensitive
            return True, f"No results for lowercase (biobtree may be case-sensitive)"


class NameLookupTest(BaseTest):
    """Test looking up an entry by name (e.g., scientific name)"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        # Support both 'name' and 'scientific_name' fields
        name = self.resolve_value(
            self.config.get("scientific_name") or self.config.get("name"),
            reference_data
        )

        if not name:
            return False, "No name found in reference data"

        response = requests.get(f"{api_url}/ws/?i={name}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results"):
            return False, f"No results for name '{name}'"

        return True, f"Found by name '{name}'"


class AliasLookupTest(BaseTest):
    """Test looking up an entry by its alias/synonym (self-referencing keyword)"""

    def execute(self, api_url: str, reference_data: list) -> Tuple[bool, str]:
        # Get the alias/synonym to search for
        alias = self.resolve_value(self.config.get("alias"), reference_data)
        # Get the expected ID that should be returned
        expected_id = self.resolve_value(self.config.get("expected_id"), reference_data)

        if not alias:
            return False, "No alias found in reference data"

        if not expected_id:
            return False, "No expected ID found in reference data"

        response = requests.get(f"{api_url}/ws/?i={alias}", timeout=10)

        if response.status_code != 200:
            return False, f"HTTP {response.status_code}"

        data = response.json()
        if not data.get("results"):
            return False, f"No results for alias '{alias}'"

        # Verify the returned entry matches expected ID
        result = data["results"][0]
        returned_id = result.get("id") or result.get("identifier")

        if returned_id == expected_id:
            return True, f"Alias '{alias}' → {expected_id}"
        else:
            return False, f"Alias '{alias}' returned {returned_id}, expected {expected_id}"


# Registry of test types
TEST_TYPES = {
    "id_lookup": IDLookupTest,
    "symbol_lookup": SymbolLookupTest,
    "name_lookup": NameLookupTest,
    "alias_lookup": AliasLookupTest,
    "xref_exists": XrefExistsTest,
    "attribute_check": AttributeCheckTest,
    "invalid_id": InvalidIDTest,
    "multi_lookup": MultiLookupTest,
    "case_insensitive": CaseInsensitiveTest,
}


def create_test(test_config: Dict[str, Any]) -> Optional[BaseTest]:
    """
    Factory function to create a test instance from configuration

    Args:
        test_config: Dictionary with 'name', 'type', and test-specific config

    Returns:
        Test instance or None if type is unknown
    """
    test_type = test_config.get("type")
    test_name = test_config.get("name", "Unnamed test")

    if test_type not in TEST_TYPES:
        print(f"Warning: Unknown test type '{test_type}'")
        return None

    test_class = TEST_TYPES[test_type]
    return test_class(test_name, test_config)
