"""Entry point for running MCP server as module."""

from .server import main
import asyncio

if __name__ == "__main__":
    asyncio.run(main())
