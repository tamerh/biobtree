"""
Biobtree HTTP Client

Simple HTTP client for biobtree REST API endpoints.
"""

import httpx
from typing import Optional
from urllib.parse import urlencode


class BiobtreeClient:
    """HTTP client for biobtree REST API."""

    def __init__(self, base_url: str = "http://localhost:9291"):
        self.base_url = base_url.rstrip("/")
        self.client = httpx.AsyncClient(timeout=60.0)

    async def close(self):
        """Close the HTTP client."""
        await self.client.aclose()

    async def search(
        self,
        terms: str,
        dataset: Optional[str] = None,
        page: Optional[str] = None,
        filter_expr: Optional[str] = None,
        mode: str = "lite"
    ) -> dict:
        """
        Search for identifiers.

        Args:
            terms: Comma-separated identifiers to search
            dataset: Filter to specific dataset (optional)
            page: Pagination token (optional)
            filter_expr: Filter expression (optional)
            mode: Response mode - "lite" or "full"

        Returns:
            Search results with matching entries
        """
        params = {"i": terms, "mode": mode}
        if dataset:
            params["s"] = dataset
        if page:
            params["p"] = page
        if filter_expr:
            params["f"] = filter_expr

        url = f"{self.base_url}/ws/"
        response = await self.client.get(url, params=params)
        response.raise_for_status()
        return response.json()

    async def map(
        self,
        terms: str,
        chain: str,
        page: Optional[str] = None,
        mode: str = "lite"
    ) -> dict:
        """
        Map identifiers through dataset chain.

        Args:
            terms: Comma-separated identifiers to map
            chain: Mapping chain (e.g., ">> ensembl >> uniprot")
            page: Pagination token (optional)
            mode: Response mode - "lite" or "full"

        Returns:
            Mapping results with source and target entries
        """
        params = {"i": terms, "m": chain, "mode": mode}
        if page:
            params["p"] = page

        url = f"{self.base_url}/ws/map/"
        response = await self.client.get(url, params=params)
        response.raise_for_status()
        return response.json()

    async def entry(
        self,
        identifier: str,
        dataset: str
    ) -> dict:
        """
        Get full entry details.

        Args:
            identifier: The identifier to look up
            dataset: The dataset containing the entry

        Returns:
            Full entry with all attributes
        """
        params = {"i": identifier, "s": dataset}
        url = f"{self.base_url}/ws/entry/"
        response = await self.client.get(url, params=params)
        response.raise_for_status()
        return response.json()

    async def meta(self) -> dict:
        """
        Get metadata about available datasets.

        Returns:
            Dataset metadata including names, IDs, and statistics
        """
        url = f"{self.base_url}/ws/meta"
        response = await self.client.get(url)
        response.raise_for_status()
        return response.json()

    async def health_check(self) -> bool:
        """
        Check if biobtree server is running.

        Returns:
            True if server is accessible
        """
        try:
            await self.meta()
            return True
        except Exception:
            return False
