"""
Biobtree MCP Server

Combined FastAPI REST API + MCP over SSE server for biobtree.

Provides:
- REST API endpoints at /api/*
- Chat endpoint at /chat (LLM + tools)
- MCP over SSE endpoint at /mcp for Claude Desktop/CLI
- Health check at /health

Usage:
    # Run server
    python -m mcp_srv.server

    # Or with uvicorn directly
    uvicorn mcp_srv.server:app --host 0.0.0.0 --port 8000

Configuration (environment variables):
    BIOBTREE_URL=http://localhost:9291
    BIOBTREE_PORT=8000
    BIOBTREE_LOG_LEVEL=INFO
    OPENROUTER_API_KEY=your-key (for /chat endpoint)
"""

import logging
import sys
import time
from contextlib import asynccontextmanager
from logging.handlers import RotatingFileHandler
from typing import Optional

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware

from .biobtree_client import BiobtreeClient
from .config import config


# =============================================================================
# Logging Setup
# =============================================================================

def setup_logging():
    """Configure logging with file rotation."""
    # Create log directory
    config.log_dir.mkdir(parents=True, exist_ok=True)

    log_format = "%(asctime)s %(levelname)s [%(name)s] %(message)s"

    # Console handler
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setFormatter(logging.Formatter(log_format))

    # Main log file (rotating, 10MB max, keep 5 backups)
    file_handler = RotatingFileHandler(
        config.log_file,
        maxBytes=10*1024*1024,
        backupCount=5
    )
    file_handler.setFormatter(logging.Formatter(log_format))

    logging.basicConfig(
        level=getattr(logging, config.log_level),
        handlers=[console_handler, file_handler]
    )

    # Access logger (separate file for request logs)
    access_logger = logging.getLogger("access")
    access_handler = RotatingFileHandler(
        config.access_log_file,
        maxBytes=10*1024*1024,
        backupCount=5
    )
    access_handler.setFormatter(logging.Formatter("%(asctime)s %(message)s"))
    access_logger.addHandler(access_handler)
    access_logger.setLevel(logging.INFO)


setup_logging()
logger = logging.getLogger(__name__)
access_logger = logging.getLogger("access")


# =============================================================================
# Request Logging Middleware
# =============================================================================

def get_client_ip(request: Request) -> str:
    """Extract client IP from request, handling proxies."""
    # Check X-Forwarded-For header (set by nginx/proxies)
    forwarded = request.headers.get("X-Forwarded-For")
    if forwarded:
        return forwarded.split(",")[0].strip()
    # Check X-Real-IP header
    real_ip = request.headers.get("X-Real-IP")
    if real_ip:
        return real_ip
    # Fall back to direct client
    return request.client.host if request.client else "unknown"


class RequestLoggingMiddleware(BaseHTTPMiddleware):
    """Log all incoming requests with client IP."""

    async def dispatch(self, request: Request, call_next):
        start_time = time.perf_counter()

        response = await call_next(request)

        # Calculate request duration
        duration_ms = (time.perf_counter() - start_time) * 1000

        # Log request details
        if config.log_requests:
            client_ip = get_client_ip(request)
            path = request.url.path
            query = f"?{request.url.query}" if request.url.query else ""
            access_logger.info(
                f"{client_ip} {request.method} {path}{query} {response.status_code} {duration_ms:.1f}ms"
            )

        return response


# =============================================================================
# Biobtree Client (singleton)
# =============================================================================

_client: Optional[BiobtreeClient] = None


async def get_client() -> BiobtreeClient:
    """Get or create biobtree client."""
    global _client
    if _client is None:
        _client = BiobtreeClient()
    return _client


# =============================================================================
# FastAPI App Setup
# =============================================================================

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Handle startup and shutdown events."""
    logger.info(f"Starting Biobtree MCP Server on {config.host}:{config.port}")
    logger.info(f"Biobtree backend: {config.biobtree_url}")

    # Verify biobtree connection on startup
    try:
        client = await get_client()
        if await client.health_check():
            logger.info("Biobtree connection verified")
        else:
            logger.warning("Biobtree health check failed - server may not be running")
    except Exception as e:
        logger.warning(f"Could not connect to biobtree: {e}")

    yield

    # Cleanup
    if _client:
        await _client.close()
    logger.info("Server shutdown complete")


app = FastAPI(
    title="Biobtree MCP Server",
    description="REST API and MCP server for biobtree biological database",
    version="1.0.0",
    lifespan=lifespan
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Add request logging middleware
app.add_middleware(RequestLoggingMiddleware)


# =============================================================================
# Include Routers
# =============================================================================

from .api import router as api_router
from .chat import router as chat_router
from .mcp_handlers import router as mcp_router

app.include_router(api_router)
app.include_router(chat_router)
app.include_router(mcp_router)


# =============================================================================
# Health Check
# =============================================================================

@app.get("/health")
async def health_check():
    """Health check endpoint."""
    try:
        client = await get_client()
        biobtree_ok = await client.health_check()
    except Exception:
        biobtree_ok = False

    return {
        "status": "healthy" if biobtree_ok else "degraded",
        "biobtree": "connected" if biobtree_ok else "disconnected",
        "biobtree_url": config.biobtree_url
    }


# =============================================================================
# Main Entry Point
# =============================================================================

def run_http_server():
    """Run the HTTP server with uvicorn (for remote access)."""
    import uvicorn
    uvicorn.run(
        "mcp_srv.server:app",
        host=config.host,
        port=config.port,
        log_level=config.log_level.lower()
    )


def main():
    """Main entry point with mode selection."""
    import argparse

    parser = argparse.ArgumentParser(description="Biobtree MCP Server")
    parser.add_argument(
        "--mode",
        choices=["http", "stdio"],
        default="stdio",
        help="Server mode: 'stdio' for local Claude CLI (default), 'http' for remote access"
    )
    args = parser.parse_args()

    if args.mode == "http":
        run_http_server()
    else:
        # stdio mode for local MCP
        import asyncio
        from .mcp_handlers import run_stdio_server
        asyncio.run(run_stdio_server())
