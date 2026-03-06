#!/usr/bin/env python3
"""
Query Helpers

Helper functions for quick querying and experimentation during development.
Makes it easy to test queries without writing full test cases.
"""

from typing import Dict, List, Optional, Any
import requests


class QueryHelper:
    """Helper class for easy API queries"""

    def __init__(self, api_url: str):
        self.api_url = api_url.rstrip('/')

    def lookup(self, identifier: str, timeout: int = 10) -> Optional[Dict]:
        """
        Look up an entry by ID or symbol

        Args:
            identifier: ID or symbol to look up
            timeout: Request timeout in seconds

        Returns:
            Response data or None if not found
        """
        try:
            response = requests.get(f"{self.api_url}/ws/?i={identifier}&d=1", timeout=timeout)

            if response.status_code == 200:
                data = response.json()
                if data.get("results"):
                    return data
            return None

        except Exception as e:
            print(f"Error looking up '{identifier}': {e}")
            return None

    def lookup_symbol(self, symbol: str, timeout: int = 10) -> Optional[Dict]:
        """
        Look up by symbol (alias for lookup)

        Args:
            symbol: Symbol to search for
            timeout: Request timeout in seconds

        Returns:
            Response data or None if not found
        """
        return self.lookup(symbol, timeout)

    def check_xref(self, identifier: str, xref_type: str = None, timeout: int = 10) -> Optional[Dict]:
        """
        Check cross-references for an entry

        Args:
            identifier: ID to look up
            xref_type: Optional - filter by xref type
            timeout: Request timeout in seconds

        Returns:
            Response data with xrefs or None
        """
        data = self.lookup(identifier, timeout)

        if not data:
            return None

        # For now, return the full data
        # In the future, we could parse and filter xrefs
        return data

    def has_results(self, identifier: str, timeout: int = 10) -> bool:
        """
        Check if an identifier returns any results

        Args:
            identifier: ID or symbol to check
            timeout: Request timeout in seconds

        Returns:
            True if results found, False otherwise
        """
        data = self.lookup(identifier, timeout)
        return data is not None

    def batch_lookup(self, identifiers: List[str], timeout: int = 10) -> Dict[str, Optional[Dict]]:
        """
        Look up multiple identifiers

        Args:
            identifiers: List of IDs/symbols to look up
            timeout: Request timeout per lookup

        Returns:
            Dictionary mapping identifier to response data
        """
        results = {}
        for identifier in identifiers:
            results[identifier] = self.lookup(identifier, timeout)
        return results

    def get_first_result(self, identifier: str, timeout: int = 10) -> Optional[Dict]:
        """
        Get the first result for an identifier

        Args:
            identifier: ID or symbol to look up
            timeout: Request timeout in seconds

        Returns:
            First result entry or None
        """
        data = self.lookup(identifier, timeout)
        if data and data.get("results"):
            return data["results"][0]
        return None

    def query(self, path: str, max_depth: int = 3, timeout: int = 30) -> Optional[Dict]:
        """
        Execute a path query (future implementation)

        Example: "HGNC:5 > uniprot > chembl"

        Args:
            path: Query path
            max_depth: Maximum depth to traverse
            timeout: Request timeout in seconds

        Returns:
            Query results or None

        Note: This is a placeholder for future implementation
        """
        print(f"Query functionality not yet implemented: {path}")
        return None

    def filter(self, filter_expr: str, timeout: int = 30) -> Optional[List[Dict]]:
        """
        Filter entries (future implementation)

        Example: "locus_type:gene AND status:Approved"

        Args:
            filter_expr: Filter expression
            timeout: Request timeout in seconds

        Returns:
            List of matching entries or None

        Note: This is a placeholder for future implementation
        """
        print(f"Filter functionality not yet implemented: {filter_expr}")
        return None

    def has_path(self, from_id: str, to_dataset: str, max_depth: int = 3, timeout: int = 30) -> bool:
        """
        Check if there's a path from an ID to another dataset (future implementation)

        Example: has_path("HGNC:5", "chembl")

        Args:
            from_id: Starting ID
            to_dataset: Target dataset
            max_depth: Maximum depth to search
            timeout: Request timeout in seconds

        Returns:
            True if path exists, False otherwise

        Note: This is a placeholder for future implementation
        """
        print(f"Path checking not yet implemented: {from_id} -> {to_dataset}")
        return False

    def meta(self, timeout: int = 10) -> Optional[Dict]:
        """
        Get metadata about the database

        Args:
            timeout: Request timeout in seconds

        Returns:
            Metadata dictionary or None
        """
        try:
            response = requests.get(f"{self.api_url}/ws/meta", timeout=timeout)

            if response.status_code == 200:
                return response.json()
            return None

        except Exception as e:
            print(f"Error fetching metadata: {e}")
            return None

    def print_result(self, data: Optional[Dict], indent: int = 2):
        """
        Pretty print a result

        Args:
            data: Response data to print
            indent: Indentation level
        """
        if not data:
            print("No results")
            return

        import json
        print(json.dumps(data, indent=indent, ensure_ascii=False))
