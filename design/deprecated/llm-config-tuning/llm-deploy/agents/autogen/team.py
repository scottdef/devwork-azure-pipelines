#!/usr/bin/env python3
"""
team.py — AutoGen 0.4+ multi-agent team with local Ollama.

AutoGen 0.4 uses an actor-model architecture (agentchat + core + ext).
OllamaChatCompletionClient is the native Ollama connector; alternatively
use OpenAIChatCompletionClient pointed at Ollama's /v1/ endpoints.

Hardware: Quadro P1000 4 GB · i9-10900 · 32 GB RAM

Usage:
    python team.py "Build a Python script that monitors GPU temperature"
"""
from __future__ import annotations

import asyncio
import os
import sys

from autogen_agentchat.agents import AssistantAgent
from autogen_agentchat.conditions import TextMentionTermination, MaxMessageTermination
from autogen_agentchat.teams import RoundRobinGroupChat
from autogen_agentchat.ui import Console
from autogen_ext.models.ollama import OllamaChatCompletionClient


# ──────────────────────────────────────────────────────────────────────
#  Model Configuration — VRAM-constrained
# ──────────────────────────────────────────────────────────────────────

OLLAMA_HOST = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")

# 3B full GPU — fast agent for tool calls and coding
fast_client = OllamaChatCompletionClient(
    model="p1000-agent",
    host=OLLAMA_HOST,
    model_info={
        "vision": False,
        "function_calling": True,
        "json_output": True,
        "family": "qwen2.5",
        "context_window": 4096,
    },
    # Ollama-specific options
    options={
        "num_gpu": 99,            # all layers on GPU
        "num_ctx": 4096,          # 4K context
        "num_predict": 1024,
        "num_thread": 10,         # physical cores
        "temperature": 0.0,       # deterministic
        "top_k": 1,
    },
)

# 7B partial offload — smarter reasoning, slower
heavy_client = OllamaChatCompletionClient(
    model="p1000-heavy",
    host=OLLAMA_HOST,
    model_info={
        "vision": False,
        "function_calling": True,
        "json_output": True,
        "family": "qwen2.5",
        "context_window": 8192,
    },
    options={
        "num_gpu": 16,            # 16/33 layers on GPU
        "num_ctx": 8192,
        "num_predict": 2048,
        "num_thread": 10,
        "temperature": 0.3,
    },
)

# Coder-specialized
code_client = OllamaChatCompletionClient(
    model="p1000-coder",
    host=OLLAMA_HOST,
    model_info={
        "vision": False,
        "function_calling": True,
        "json_output": True,
        "family": "qwen2.5",
        "context_window": 8192,
    },
    options={
        "num_gpu": 99,
        "num_ctx": 8192,
        "num_predict": 4096,
        "num_thread": 10,
        "temperature": 0.1,
    },
)


# ──────────────────────────────────────────────────────────────────────
#  Agents
# ──────────────────────────────────────────────────────────────────────

planner = AssistantAgent(
    name="Planner",
    model_client=fast_client,
    system_message=(
        "You are a project planner. Break the user's request into clear, "
        "numbered steps. Assign each step to either 'Coder' or 'Reviewer'. "
        "When all steps are complete, say TERMINATE."
    ),
)

coder = AssistantAgent(
    name="Coder",
    model_client=code_client,
    system_message=(
        "You are a senior Python/Go/Bash developer on Ubuntu 20.04. "
        "Write complete, runnable code with error handling, type hints, "
        "and docstrings. Target the i9-10900 (10 cores) and 32 GB RAM. "
        "When done, hand off to Reviewer for review."
    ),
)

reviewer = AssistantAgent(
    name="Reviewer",
    model_client=fast_client,
    system_message=(
        "You are a code reviewer. Check for bugs, security issues, "
        "missing error handling, and style.  Provide specific line-level "
        "feedback. If the code is acceptable, approve it and say TERMINATE."
    ),
)


# ──────────────────────────────────────────────────────────────────────
#  Team assembly
# ──────────────────────────────────────────────────────────────────────

async def run_team(task: str):
    """Run a round-robin group chat until TERMINATE or 12 messages."""
    termination = TextMentionTermination("TERMINATE") | MaxMessageTermination(12)

    team = RoundRobinGroupChat(
        participants=[planner, coder, reviewer],
        termination_condition=termination,
    )

    stream = team.run_stream(task=task)
    await Console(stream)


if __name__ == "__main__":
    task = " ".join(sys.argv[1:]) or (
        "Build a Python script that monitors Quadro P1000 GPU temperature "
        "and VRAM usage every 5 seconds, logging to CSV with timestamps."
    )
    print(f"Task: {task}\n{'─' * 60}")
    asyncio.run(run_team(task))
