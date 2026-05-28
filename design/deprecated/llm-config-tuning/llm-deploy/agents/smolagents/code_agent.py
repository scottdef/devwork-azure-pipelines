#!/usr/bin/env python3
"""
code_agent.py — smolagents CodeAgent with local Ollama.

smolagents is the best agent framework for small (3B-8B) models because:
  1. CodeAgent emits Python code instead of JSON tool calls — more
     in-distribution for trained code models.
  2. Minimal overhead (~1 kLOC framework), no hidden prompt bloat.
  3. Native LiteLLM integration for any backend.

Hardware: Quadro P1000 4 GB · i9-10900 · 32 GB RAM

Usage:
    python code_agent.py "Find the top 5 most starred RISC-V repos on GitHub"
"""
from __future__ import annotations

import os
import sys

from smolagents import (
    CodeAgent,
    ToolCallingAgent,
    LiteLLMModel,
    Tool,
    tool,
)


# ──────────────────────────────────────────────────────────────────────
#  Model Configuration — VRAM-constrained
# ──────────────────────────────────────────────────────────────────────

OLLAMA_BASE = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")

# smolagents uses LiteLLM model strings: "ollama_chat/<model>"
# The temperature, num_ctx etc. are set in the Ollama Modelfile.
# Additional overrides go in model_kwargs.

fast_model = LiteLLMModel(
    model_id="ollama_chat/p1000-agent",
    api_base=OLLAMA_BASE,
    api_key="not-needed",
    temperature=0.0,
    # LiteLLM passes extra kwargs to Ollama's options
    num_ctx=4096,
    num_predict=1024,
    num_gpu=99,
    num_thread=10,
)

code_model = LiteLLMModel(
    model_id="ollama_chat/p1000-coder",
    api_base=OLLAMA_BASE,
    api_key="not-needed",
    temperature=0.1,
    num_ctx=8192,
    num_predict=4096,
    num_gpu=99,
    num_thread=10,
)

heavy_model = LiteLLMModel(
    model_id="ollama_chat/p1000-heavy",
    api_base=OLLAMA_BASE,
    api_key="not-needed",
    temperature=0.3,
    num_ctx=8192,
    num_predict=2048,
    num_gpu=16,        # partial offload
    num_thread=10,
)


# ──────────────────────────────────────────────────────────────────────
#  Tools (using @tool decorator — smolagents style)
# ──────────────────────────────────────────────────────────────────────

@tool
def web_search(query: str) -> str:
    """Search the web using SearXNG.  Returns top results with titles
    and snippets.

    Args:
        query: the search query string
    """
    import httpx
    searxng_url = os.getenv("SEARXNG_URL", "http://127.0.0.1:8888")
    try:
        resp = httpx.get(
            f"{searxng_url}/search",
            params={"q": query, "format": "json", "language": "en"},
            headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"},
            timeout=15.0,
        )
        results = resp.json().get("results", [])[:5]
        return "\n".join(
            f"- {r['title']}: {r.get('content', '')[:150]} ({r['url']})"
            for r in results
        ) or "No results found."
    except Exception as e:
        return f"Search error: {e}"


@tool
def read_webpage(url: str) -> str:
    """Fetch and extract the main text content from a URL.

    Args:
        url: the full URL to read
    """
    try:
        import trafilatura
        html = trafilatura.fetch_url(url)
        text = trafilatura.extract(html) if html else ""
        return text[:3000] if text else "Could not extract content."
    except Exception as e:
        return f"Read error: {e}"


@tool
def run_python(code: str) -> str:
    """Execute a Python code snippet in a sandboxed subprocess and
    return stdout.  Use for calculations, data processing, or testing.

    Args:
        code: Python code to execute (must be self-contained)
    """
    import subprocess
    try:
        result = subprocess.run(
            [sys.executable, "-c", code],
            capture_output=True, text=True, timeout=30,
            env={**os.environ, "PYTHONDONTWRITEBYTECODE": "1"},
        )
        output = result.stdout[:2000]
        if result.returncode != 0:
            output += f"\nERROR:\n{result.stderr[:1000]}"
        return output or "(no output)"
    except subprocess.TimeoutExpired:
        return "Code execution timed out after 30 seconds."
    except Exception as e:
        return f"Execution error: {e}"


@tool
def check_gpu() -> str:
    """Return current GPU status: name, VRAM used/total, temperature."""
    import subprocess
    try:
        result = subprocess.run(
            ["nvidia-smi",
             "--query-gpu=name,memory.used,memory.total,temperature.gpu,utilization.gpu",
             "--format=csv,noheader,nounits"],
            capture_output=True, text=True, timeout=5,
        )
        return result.stdout.strip() or "nvidia-smi returned empty."
    except Exception as e:
        return f"GPU check failed: {e}"


# ──────────────────────────────────────────────────────────────────────
#  Agent construction
# ──────────────────────────────────────────────────────────────────────

TOOLS = [web_search, read_webpage, run_python, check_gpu]


def build_code_agent(model=None):
    """
    CodeAgent — emits Python code to call tools.
    Best choice for 3B models: code is more in-distribution than
    JSON function-call blobs, and parsing errors are rarer.
    """
    return CodeAgent(
        tools=TOOLS,
        model=model or code_model,
        max_steps=8,              # step budget — prevents runaway
        additional_authorized_imports=[
            "json", "os", "re", "math", "datetime", "pathlib",
            "collections", "itertools", "functools", "typing",
            "httpx", "subprocess",
        ],
    )


def build_tool_agent(model=None):
    """
    ToolCallingAgent — uses native function-calling API.
    Better for models with strong tool-call templates (Qwen2.5, Llama3.2).
    Falls back gracefully if the model struggles with JSON.
    """
    return ToolCallingAgent(
        tools=TOOLS,
        model=model or fast_model,
        max_steps=8,
    )


if __name__ == "__main__":
    task = " ".join(sys.argv[1:]) or (
        "Check this machine's GPU VRAM usage, then search for "
        "'best quantized LLMs for 4GB VRAM' and summarize the results."
    )

    agent_type = "code"  # or "tool"
    if "--tool" in sys.argv:
        agent_type = "tool"

    print(f"Agent: {agent_type} | Task: {task}\n{'─' * 60}")

    if agent_type == "code":
        agent = build_code_agent()
    else:
        agent = build_tool_agent()

    result = agent.run(task)
    print(f"\n{'═' * 60}\n{result}")
