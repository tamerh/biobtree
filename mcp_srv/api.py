"""
Biobtree REST API Endpoints

FastAPI router for /api/* endpoints.
"""

from typing import Optional

from fastapi import APIRouter, Query
from fastapi.responses import JSONResponse

from .biobtree_client import BiobtreeClient, BiobtreeError
from .schema import get_schema

router = APIRouter(prefix="/api", tags=["api"])

# Singleton client
_client: Optional[BiobtreeClient] = None


async def get_client() -> BiobtreeClient:
    """Get or create biobtree client."""
    global _client
    if _client is None:
        _client = BiobtreeClient()
    return _client


@router.get("/search")
async def api_search(
    i: str = Query(..., description="Comma-separated identifiers to search"),
    s: Optional[str] = Query(None, description="Filter to specific dataset"),
    mode: Optional[str] = Query(None, description="Response mode: lite or full"),
    p: Optional[str] = Query(None, description="Pagination token")
):
    """
    Search for biological identifiers.

    Finds entries matching the given terms across 70+ integrated databases.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.search(terms=i, dataset=s, mode=mode, page=p)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@router.get("/map")
async def api_map(
    i: str = Query(..., description="Comma-separated identifiers to map"),
    m: str = Query(..., description="Mapping chain (e.g., '>>ensembl>>uniprot')"),
    mode: Optional[str] = Query(None, description="Response mode: lite or full"),
    p: Optional[str] = Query(None, description="Pagination token")
):
    """
    Map identifiers through dataset chains.

    The core endpoint for cross-database queries.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.map(terms=i, chain=m, mode=mode, page=p)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@router.get("/entry")
async def api_entry(
    i: str = Query(..., description="The identifier to look up"),
    s: str = Query(..., description="The dataset containing the entry")
):
    """
    Get full entry details.

    Retrieves complete information for a specific identifier in a dataset.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.entry(identifier=i, dataset=s)
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@router.get("/meta")
async def api_meta():
    """
    Get metadata about available datasets.

    Returns information about all integrated datasets.
    """
    try:
        biobtree = await get_client()
        result = await biobtree.meta()
        return result
    except BiobtreeError as e:
        return JSONResponse(status_code=503, content={"error": str(e)})


@router.get("/help")
async def api_help(
    topic: str = Query("all", description="Topic: edges, filters, hierarchies, patterns, examples, filter_syntax, disease_ontology, or all")
):
    """
    Get biobtree schema reference.

    Returns dataset connections, filters, and query patterns.
    """
    return get_schema(topic)
