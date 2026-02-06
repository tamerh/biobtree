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
    biobtree_public_url: Optional[str] = None  # Public URL for query links (if different from internal)
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

    # Chat/LLM settings (for /chat endpoint)
    openrouter_api_key: Optional[str] = None
    openrouter_base_url: str = "https://openrouter.ai/api/v1"
    default_model: str = "anthropic/claude-sonnet-4"
    chat_max_iterations: int = 20
    chat_timeout: float = 120.0

    @classmethod
    def from_env(cls) -> "Config":
        """Load configuration from environment variables."""
        log_dir_str = os.getenv("BIOBTREE_LOG_DIR")
        log_dir = Path(log_dir_str) if log_dir_str else DEFAULT_LOG_DIR

        return cls(
            # Biobtree backend
            biobtree_url=os.getenv("BIOBTREE_URL", "http://localhost:9291"),
            biobtree_public_url=os.getenv("BIOBTREE_PUBLIC_URL"),  # e.g., "https://biobtree.org"
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

            # Chat/LLM settings
            openrouter_api_key=os.getenv("OPENROUTER_API_KEY"),
            openrouter_base_url=os.getenv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
            default_model=os.getenv("BIOBTREE_DEFAULT_MODEL", "anthropic/claude-sonnet-4"),
            chat_max_iterations=int(os.getenv("BIOBTREE_CHAT_MAX_ITERATIONS", "20")),
            chat_timeout=float(os.getenv("BIOBTREE_CHAT_TIMEOUT", "120.0")),
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
