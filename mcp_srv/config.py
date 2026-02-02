"""
Biobtree MCP Server Configuration

Configuration management via environment variables with sensible defaults.
"""

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


# Default log directory relative to mcp_srv package
DEFAULT_LOG_DIR = Path(__file__).parent.parent / "logs"


@dataclass
class Config:
    """Server configuration."""

    # Biobtree backend
    biobtree_url: str = "http://localhost:9291"
    biobtree_timeout: float = 60.0

    # Server settings
    host: str = "0.0.0.0"
    port: int = 8000

    # Logging
    log_level: str = "INFO"
    log_dir: Path = DEFAULT_LOG_DIR
    log_requests: bool = True  # Log incoming requests with IP

    # MCP settings
    mcp_server_name: str = "biobtree"

    @classmethod
    def from_env(cls) -> "Config":
        """Load configuration from environment variables."""
        log_dir_str = os.getenv("BIOBTREE_LOG_DIR")
        log_dir = Path(log_dir_str) if log_dir_str else DEFAULT_LOG_DIR

        return cls(
            # Biobtree backend
            biobtree_url=os.getenv("BIOBTREE_URL", "http://localhost:9291"),
            biobtree_timeout=float(os.getenv("BIOBTREE_TIMEOUT", "60.0")),

            # Server settings
            host=os.getenv("BIOBTREE_HOST", "0.0.0.0"),
            port=int(os.getenv("BIOBTREE_PORT", "8000")),

            # Logging
            log_level=os.getenv("BIOBTREE_LOG_LEVEL", "INFO").upper(),
            log_dir=log_dir,
            log_requests=os.getenv("BIOBTREE_LOG_REQUESTS", "true").lower() == "true",

            # MCP settings
            mcp_server_name=os.getenv("BIOBTREE_MCP_NAME", "biobtree"),
        )

    @property
    def log_file(self) -> Path:
        """Main log file path."""
        return self.log_dir / "mcp_server.log"

    @property
    def access_log_file(self) -> Path:
        """Access log file path (requests with IPs)."""
        return self.log_dir / "access.log"


# Global config instance
config = Config.from_env()
