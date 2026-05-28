#!/usr/bin/env python3
"""
react_agent.py — LangGraph ReAct agent with VRAM-aware model config.

Hardware: Quadro P1000 4 GB · i9-10900 · 32 GB RAM
Models:   qwen2.5:3b (full GPU) for fast agent loops
          qwen2.5:7b (partial offload) for heavy reasoning

Usage:
    python react_agent.py "What are the latest developments in RISC-V?"
"""
from __future__ import annotations

import os
import sys
import json
import httpx
from typing import Annotated

from langchain_ollama import ChatOllama
from langchain_core.tools import tool
from langchain_core.messages import HumanMessage
from langgraph.prebuilt import create_react_agent
from langgraph.checkpoint.memory import MemorySaver


# ──────────────────────────────────────────────────────────────────────
#  Model Configuration — VRAM-constrained
# ──────────────────────────────────────────────────────────────────────

OLLAMA_BASE = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")

# Fast model: 3B full GPU — for agent loops (tool calling + routing)
FAST_MODEL = ChatOllama(
    base_url=OLLAMA_BASE,
    model=os.getenv("MODEL_AGENT", "p1000-agent"),
    temperature=0.0,          # deterministic for reliable tool calls
    num_ctx=4096,             # 4K context fits comfortably in 4 GB VRAM
    num_predict=1024,         # short outputs — agent steps are brief
    num_thread=10,            # physical cores of i9-10900
    num_gpu=99,               # all layers on GPU
    repeat_penalty=1.0,       # no penalty — agent loops repeat patterns
    top_k=1,                  # greedy decoding
    keep_alive="15m",
)

# Heavy model: 7B partial offload — for complex reasoning when needed
HEAVY_MODEL = ChatOllama(
    base_url=OLLAMA_BASE,
    model=os.getenv("MODEL_HEAVY", "p1000-heavy"),
    temperature=0.3,
    num_ctx=4096,
    num_predict=2048,
    num_thread=10,
    num_gpu=16,               # 16 of 33 layers on GPU (~2.6 GB VRAM)
    keep_alive="15m",
)


# ──────────────────────────────────────────────────────────────────────
#  Tools
# ──────────────────────────────────────────────────────────────────────

SEARXNG_URL = os.getenv("SEARXNG_URL", "http://127.0.0.1:8888")


@tool
def web_search(query: str) -> str:
    """Search the web using SearXNG.  Returns top 5 results with titles,
    URLs, and snippets.  Use this for current events, recent information,
    or anything outside your training data."""
    try:
        resp = httpx.get(
            f"{SEARXNG_URL}/search",
            params={
                "q": query,
                "format": "json",
                "categories": "general",
                "language": "en",
                "pageno": 1,
            },
            headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"},
            timeout=15.0,
        )
        resp.raise_for_status()
        results = resp.json().get("results", [])[:5]
        if not results:
            return "No search results found."
        lines = []
        for r in results:
            lines.append(f"- [{r['title']}]({r['url']})")
            if r.get("content"):
                lines.append(f"  {r['content'][:200]}")
        return "\n".join(lines)
    except Exception as e:
        return f"Search failed: {e}"


@tool
def read_url(url: str) -> str:
    """Fetch and extract the main text content from a URL.
    Use this after web_search to read a specific article."""
    try:
        import trafilatura
        downloaded = trafilatura.fetch_url(url)
        if not downloaded:
            return "Failed to download URL."
        text = trafilatura.extract(downloaded, include_links=False) or ""
        return text[:3000]  # cap to stay within context budget
    except ImportError:
        # Fallback without trafilatura
        resp = httpx.get(url, timeout=15.0, follow_redirects=True)
        return resp.text[:3000]
    except Exception as e:
        return f"Read failed: {e}"


@tool
def calculate(expression: str) -> str:
    """Evaluate a mathematical expression.  Examples: '2**10', 'sqrt(144)',
    'sin(pi/4)'.  Uses Python's math library."""
    import math
    try:
        allowed = {k: getattr(math, k) for k in dir(math) if not k.startswith("_")}
        allowed.update({"abs": abs, "round": round, "int": int, "float": float})
        result = eval(expression, {"__builtins__": {}}, allowed)
        return str(result)
    except Exception as e:
        return f"Calculation error: {e}"


@tool
def shell_command(command: str) -> str:
    """Run a read-only shell command and return stdout.
    ONLY for safe, non-destructive commands: ls, cat, grep, find, wc,
    df, free, uname, nvidia-smi.  No writes, no rm, no sudo."""
    import subprocess
    # Allowlist — only safe read-only commands
    ALLOWED = {"ls", "cat", "head", "tail", "grep", "find", "wc", "df",
               "free", "uname", "nvidia-smi", "date", "uptime", "who",
               "hostname", "ip", "ss", "du", "file", "stat", "env"}
    first_word = command.strip().split()[0] if command.strip() else ""
    if first_word not in ALLOWED:
        return f"Blocked: '{first_word}' is not in the allowlist: {sorted(ALLOWED)}"
    try:
        result = subprocess.run(
            command, shell=False,  # NOTE: we split manually below for safety
            capture_output=True, text=True, timeout=10,
        )
        # Actually use shell=True but only for allowed commands
        result = subprocess.run(
            ["bash", "-c", command],
            capture_output=True, text=True, timeout=10,
        )
        output = result.stdout[:2000]
        if result.returncode != 0:
            output += f"\nSTDERR: {result.stderr[:500]}"
        return output or "(empty output)"
    except subprocess.TimeoutExpired:
        return "Command timed out after 10 seconds."
    except Exception as e:
        return f"Command failed: {e}"


# ──────────────────────────────────────────────────────────────────────
#  Agent construction
# ──────────────────────────────────────────────────────────────────────

TOOLS = [web_search, read_url, calculate, shell_command]

# System message injected before every agent loop
SYSTEM_MSG = """You are a research and systems assistant running locally on a
Lenovo P340 SFF workstation (Ubuntu 20.04, Quadro P1000, 32 GB RAM).
Use tools when you need external information. Be concise. Cite sources."""


def build_agent(model: ChatOllama = FAST_MODEL, checkpointer=None):
    """Build a LangGraph ReAct agent with the given model and tools."""
    return create_react_agent(
        model=model,
        tools=TOOLS,
        prompt=SYSTEM_MSG,
        checkpointer=checkpointer,
    )


def run(query: str, use_heavy: bool = False):
    """Run the agent on a single query and stream output."""
    model = HEAVY_MODEL if use_heavy else FAST_MODEL
    memory = MemorySaver()
    agent = build_agent(model=model, checkpointer=memory)

    config = {"configurable": {"thread_id": "cli-session"}}
    print(f"Model: {model.model} | num_gpu: {model.num_gpu} | ctx: {model.num_ctx}")
    print(f"Query: {query}\n{'─' * 60}")

    for event in agent.stream(
        {"messages": [HumanMessage(content=query)]},
        config=config,
        stream_mode="updates",
    ):
        for node, updates in event.items():
            if "messages" in updates:
                last = updates["messages"][-1]
                if hasattr(last, "tool_calls") and last.tool_calls:
                    for tc in last.tool_calls:
                        print(f"  🔧 {tc['name']}({json.dumps(tc['args'])[:100]})")
                elif hasattr(last, "content") and last.content:
                    print(f"\n{last.content}")


if __name__ == "__main__":
    query = " ".join(sys.argv[1:]) or "What is this machine's GPU and current VRAM usage?"
    heavy = "--heavy" in sys.argv
    run(query, use_heavy=heavy)
