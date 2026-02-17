"""
Biobtree HTTP Client

Async HTTP client for biobtree REST API with logging and error handling.
"""

import logging
import time
from typing import Optional

import httpx

from .config import config

logger = logging.getLogger(__name__)


class BiobtreeError(Exception):
    """Base exception for biobtree client errors."""
    pass


class BiobtreeConnectionError(BiobtreeError):
    """Raised when connection to biobtree fails."""
    pass


class BiobtreeAPIError(BiobtreeError):
    """Raised when biobtree API returns an error."""
    def __init__(self, message: str, status_code: int = None):
        super().__init__(message)
        self.status_code = status_code


class BiobtreeClient:
    """Async HTTP client for biobtree REST API."""

    def __init__(self, base_url: str = None, timeout: float = None):
        """
        Initialize biobtree client.

        Args:
            base_url: Biobtree server URL (default from config)
            timeout: Request timeout in seconds (default from config)
        """
        self.base_url = (base_url or config.biobtree_url).rstrip("/")
        self.timeout = timeout or config.biobtree_timeout
        self._client: Optional[httpx.AsyncClient] = None

    async def _get_client(self) -> httpx.AsyncClient:
        """Get or create HTTP client."""
        if self._client is None or self._client.is_closed:
            self._client = httpx.AsyncClient(timeout=self.timeout)
        return self._client

    async def close(self):
        """Close the HTTP client."""
        if self._client and not self._client.is_closed:
            await self._client.aclose()
            self._client = None

    async def _request(self, endpoint: str, params: dict) -> dict:
        """
        Make HTTP request with logging and error handling.

        Args:
            endpoint: API endpoint path
            params: Query parameters

        Returns:
            JSON response

        Raises:
            BiobtreeConnectionError: Connection failed
            BiobtreeAPIError: API returned error
        """
        url = f"{self.base_url}{endpoint}"
        start_time = time.perf_counter()

        try:
            client = await self._get_client()
            response = await client.get(url, params=params)
            elapsed_ms = (time.perf_counter() - start_time) * 1000

            # Log request
            logger.info(
                f"[{endpoint}] params={params} status={response.status_code} time={elapsed_ms:.1f}ms"
            )

            response.raise_for_status()
            return response.json()

        except httpx.ConnectError as e:
            elapsed_ms = (time.perf_counter() - start_time) * 1000
            logger.error(f"[{endpoint}] Connection failed: {e} time={elapsed_ms:.1f}ms")
            raise BiobtreeConnectionError(
                f"Cannot connect to biobtree at {self.base_url}. Is the server running?"
            ) from e

        except httpx.TimeoutException as e:
            elapsed_ms = (time.perf_counter() - start_time) * 1000
            logger.error(f"[{endpoint}] Timeout after {elapsed_ms:.1f}ms")
            raise BiobtreeConnectionError(
                f"Request to biobtree timed out after {self.timeout}s"
            ) from e

        except httpx.HTTPStatusError as e:
            elapsed_ms = (time.perf_counter() - start_time) * 1000
            logger.error(
                f"[{endpoint}] HTTP error: {e.response.status_code} time={elapsed_ms:.1f}ms"
            )
            raise BiobtreeAPIError(
                f"Biobtree API error: {e.response.status_code}",
                status_code=e.response.status_code
            ) from e

    def _build_query_url(self, endpoint: str, params: dict) -> str:
        """Build a user-friendly query URL for full data access."""
        # Use public URL if configured, otherwise use base_url
        public_base = config.biobtree_public_url or self.base_url
        query_parts = []
        for k, v in params.items():
            if v is not None:
                query_parts.append(f"{k}={v}")
        query_string = "&".join(query_parts)
        return f"{public_base}{endpoint}?{query_string}"

    async def search(
        self,
        terms: str,
        dataset: Optional[str] = None,
        page: Optional[str] = None,
        filter_expr: Optional[str] = None,
        mode: Optional[str] = None
    ) -> dict:
        """
        Search for identifiers.

        Args:
            terms: Comma-separated identifiers to search
            dataset: Filter to specific dataset (optional)
            page: Pagination token (optional)
            filter_expr: Filter expression (optional)
            mode: Response mode - "lite" or "full" (optional, uses biobtree default if not set)

        Returns:
            Search results with matching entries and query_url for full access
        """
        params = {"i": terms}
        if mode:
            params["mode"] = mode
        if dataset:
            params["s"] = dataset
        if page:
            params["p"] = page
        if filter_expr:
            params["f"] = filter_expr

        result = await self._request("/ws/", params)

        # Add query URL at START of response (survives truncation)
        url_params = {"i": terms}
        if dataset:
            url_params["s"] = dataset
        query_url = self._build_query_url("/ws/", url_params)

        # Return with query_url first (Python 3.7+ preserves dict order)
        return {"query_url": query_url, **result}

    async def map(
        self,
        terms: str,
        chain: str,
        page: Optional[str] = None,
        mode: Optional[str] = None
    ) -> dict:
        """
        Map identifiers through dataset chain.

        Args:
            terms: Comma-separated identifiers to map
            chain: Mapping chain (e.g., ">> ensembl >> uniprot")
            page: Pagination token (optional)
            mode: Response mode - "lite" or "full" (optional, uses biobtree default if not set)

        Returns:
            Mapping results with source and target entries, plus query_url for full access
        """
        # Validate and auto-correct chain syntax - must start with >>
        chain_stripped = chain.strip()
        if not chain_stripped.startswith(">>"):
            # Auto-correct common mistake: ">dataset" -> ">>dataset"
            if chain_stripped.startswith(">"):
                corrected_chain = ">" + chain_stripped  # Add missing >
                logger.info(f"Auto-corrected chain syntax: '{chain}' -> '{corrected_chain}'")
                chain_stripped = corrected_chain
            else:
                raise BiobtreeAPIError(
                    f"Invalid chain syntax: '{chain}'. Chain must start with '>>'. "
                    f"Example: >>ensembl>>uniprot"
                )

        params = {"i": terms, "m": chain_stripped}
        if mode:
            params["mode"] = mode
        if page:
            params["p"] = page

        result = await self._request("/ws/map/", params)

        # Add query URL at START of response (survives truncation)
        query_url = self._build_query_url("/ws/map/", {"i": terms, "m": chain_stripped})

        # Return with query_url first (Python 3.7+ preserves dict order)
        return {"query_url": query_url, **result}

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
            Full entry with all attributes, plus query_url
        """
        params = {"i": identifier, "s": dataset}
        result = await self._request("/ws/entry/", params)

        # Wrap result in dict if it's a list (entry endpoint returns list)
        if isinstance(result, list):
            result = {"results": result}

        # Add query URL at START of response (survives truncation)
        query_url = self._build_query_url("/ws/entry/", params)

        # Return with query_url first (Python 3.7+ preserves dict order)
        return {"query_url": query_url, **result}

    async def meta(self) -> dict:
        """
        Get metadata about available datasets.

        Returns:
            Dataset metadata including names, IDs, and statistics
        """
        return await self._request("/ws/meta", {})

    async def health_check(self) -> bool:
        """
        Check if biobtree server is running.

        Returns:
            True if server is accessible
        """
        try:
            await self.meta()
            return True
        except BiobtreeError:
            return False
