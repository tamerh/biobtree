"""
Biobtree Chat Endpoint

Chat with LLM using biobtree tools via OpenRouter.
"""

import json
import logging
from typing import Optional

from fastapi import APIRouter, Request
from fastapi.responses import JSONResponse

from .biobtree_client import BiobtreeClient
from .config import config
from .tools import CHAT_TOOLS, execute_tool


def _build_query_url_from_args(tool_name: str, tool_args: dict) -> str | None:
    """Build query URL from tool name and arguments (avoids parsing truncated results)."""
    public_base = config.biobtree_public_url or config.biobtree_url
    public_base = public_base.rstrip("/")

    if tool_name == "biobtree_search":
        terms = tool_args.get("terms", "")
        dataset = tool_args.get("dataset")
        url = f"{public_base}/ws/?i={terms}"
        if dataset:
            url += f"&s={dataset}"
        return url
    elif tool_name == "biobtree_map":
        terms = tool_args.get("terms", "")
        chain = tool_args.get("chain", "")
        return f"{public_base}/ws/map/?i={terms}&m={chain}"
    elif tool_name == "biobtree_entry":
        identifier = tool_args.get("identifier", "")
        dataset = tool_args.get("dataset", "")
        return f"{public_base}/ws/entry/?i={identifier}&s={dataset}"

    return None

logger = logging.getLogger(__name__)
router = APIRouter(tags=["chat"])

# Singleton client
_client: Optional[BiobtreeClient] = None


async def get_client() -> BiobtreeClient:
    """Get or create biobtree client."""
    global _client
    if _client is None:
        _client = BiobtreeClient()
    return _client


DEFAULT_SYSTEM_PROMPT_WITH_TOOLS = """You are a helpful bioinformatics assistant with access to biobtree, a biological database integrating 70+ data sources including genes, proteins, drugs, diseases, variants, pathways, interactions, expression, rare diseases, clinical trials, and more.

IMPORTANT: Before answering any question, call biobtree_help with topic="patterns" to discover the available mapping chains. Do NOT guess chains — always check what connections exist first.

When answering:
1. First call biobtree_help to find the right chain for the question
2. Use biobtree_search to find identifiers, then biobtree_map to traverse chains
3. Include specific database identifiers (IDs, accession numbers) in your answer
4. Provide clear, scientifically accurate answers based on the retrieved data"""

DEFAULT_SYSTEM_PROMPT_NO_TOOLS = """You are a helpful bioinformatics assistant. Answer questions about genes, proteins, drugs, diseases, variants, pathways, and other biological topics based on your training knowledge.

Provide clear, scientifically accurate answers. When discussing specific database entries, mention relevant identifiers if you know them (UniProt IDs, Ensembl IDs, etc.)."""


def _append_data_sources(answer: str, query_urls: list) -> str:
    """Append data sources section with unique query URLs to the answer."""
    if not query_urls or not answer:
        return answer

    # Deduplicate while preserving order
    seen = set()
    unique_urls = []
    for url in query_urls:
        if url not in seen:
            seen.add(url)
            unique_urls.append(url)

    if not unique_urls:
        return answer

    # Build data sources section
    sources_section = "\n\n---\n**Data Sources:**"
    for url in unique_urls:
        sources_section += f"\n- {url}"

    return answer + sources_section


@router.post("/chat")
async def chat_endpoint(request: Request):
    """
    Chat with LLM using biobtree tools.

    This endpoint enables conversational queries with automatic tool calling.
    Useful for web demos and benchmarking.

    Request body:
    {
        "question": "What proteins does BRCA1 encode?",
        "model": "anthropic/claude-sonnet-4",  // optional
        "with_tools": true,  // optional, default true
        "system_prompt": "..."  // optional custom system prompt
    }

    Response:
    {
        "answer": "...",
        "model": "anthropic/claude-sonnet-4",
        "tools_used": true,
        "tool_calls": [...],
        "iterations": 2
    }
    """
    # Parse request
    try:
        body = await request.json()
    except Exception:
        return JSONResponse(status_code=400, content={"error": "Invalid JSON body"})

    question = body.get("question")
    if not question:
        return JSONResponse(status_code=400, content={"error": "Missing 'question' field"})

    # Check API key
    if not config.openrouter_api_key:
        return JSONResponse(
            status_code=503,
            content={"error": "OpenRouter API key not configured. Set OPENROUTER_API_KEY environment variable."}
        )

    model = body.get("model", config.default_model)
    with_tools = body.get("with_tools", True)

    # Use appropriate default system prompt based on tools setting
    default_prompt = DEFAULT_SYSTEM_PROMPT_WITH_TOOLS if with_tools else DEFAULT_SYSTEM_PROMPT_NO_TOOLS
    system_prompt = body.get("system_prompt", default_prompt)

    # Import openai here to avoid startup dependency
    try:
        from openai import AsyncOpenAI
    except ImportError:
        return JSONResponse(
            status_code=503,
            content={"error": "openai package not installed. Run: pip install openai"}
        )

    # Create OpenRouter client
    client = AsyncOpenAI(
        api_key=config.openrouter_api_key,
        base_url=config.openrouter_base_url,
        timeout=config.chat_timeout,
        default_headers={
            "HTTP-Referer": "https://sugi.bio",
            "X-Title": "Biobtree"
        }
    )

    # Build messages
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": question}
    ]

    tools = CHAT_TOOLS if with_tools else None
    tool_calls_log = []
    query_urls = []  # Collect all query URLs from tool results
    iterations = 0
    total_tokens = {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}

    # Get biobtree client for tool execution
    biobtree = await get_client()

    try:
        while iterations < config.chat_max_iterations:
            iterations += 1

            # Call LLM
            response = await client.chat.completions.create(
                model=model,
                messages=messages,
                tools=tools,
                tool_choice="auto" if tools else None,
                temperature=0.0,
                max_tokens=4096
            )

            msg = response.choices[0].message

            # Accumulate token usage
            if hasattr(response, 'usage') and response.usage:
                total_tokens["prompt_tokens"] += getattr(response.usage, 'prompt_tokens', 0) or 0
                total_tokens["completion_tokens"] += getattr(response.usage, 'completion_tokens', 0) or 0
                total_tokens["total_tokens"] += getattr(response.usage, 'total_tokens', 0) or 0

            # Check if model wants to call tools
            if msg.tool_calls and with_tools:
                # Add assistant message with tool calls
                messages.append({
                    "role": "assistant",
                    "content": msg.content or "",
                    "tool_calls": [
                        {
                            "id": tc.id,
                            "type": "function",
                            "function": {
                                "name": tc.function.name,
                                "arguments": tc.function.arguments
                            }
                        }
                        for tc in msg.tool_calls
                    ]
                })

                # Execute each tool call
                for tool_call in msg.tool_calls:
                    tool_name = tool_call.function.name
                    try:
                        tool_args = json.loads(tool_call.function.arguments)
                    except json.JSONDecodeError:
                        tool_args = {}

                    logger.info(f"Chat tool call: {tool_name}({tool_args})")

                    # Execute tool
                    result = await execute_tool(tool_name, tool_args, biobtree, max_result_length=15000)

                    # Build query_url from tool arguments (more reliable than parsing truncated results)
                    query_url = _build_query_url_from_args(tool_name, tool_args)
                    if query_url:
                        query_urls.append(query_url)

                    # Log for response
                    tool_calls_log.append({
                        "tool": tool_name,
                        "arguments": tool_args,
                        "result_length": len(result)
                    })

                    # Add tool result to messages
                    messages.append({
                        "role": "tool",
                        "tool_call_id": tool_call.id,
                        "content": result
                    })
            else:
                # No tool calls - model is done
                # Append data sources section with all query URLs
                final_answer = _append_data_sources(msg.content, query_urls)
                return {
                    "answer": final_answer,
                    "model": model,
                    "tools_used": with_tools,
                    "tool_calls": tool_calls_log,
                    "iterations": iterations,
                    "data_sources": query_urls,
                    "usage": total_tokens
                }

        # Max iterations reached
        base_answer = msg.content if msg else "Unable to complete request within iteration limit."
        final_answer = _append_data_sources(base_answer, query_urls)
        return {
            "answer": final_answer,
            "model": model,
            "tools_used": with_tools,
            "tool_calls": tool_calls_log,
            "iterations": iterations,
            "data_sources": query_urls,
            "usage": total_tokens,
            "warning": "Max iterations reached"
        }

    except Exception as e:
        logger.exception(f"Chat endpoint error: {e}")
        return JSONResponse(
            status_code=500,
            content={
                "error": str(e),
                "model": model,
                "iterations": iterations,
                "tool_calls": tool_calls_log
            }
        )
