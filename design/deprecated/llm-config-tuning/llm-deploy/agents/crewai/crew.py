#!/usr/bin/env python3
"""
crew.py — CrewAI hierarchical crew with local Ollama backends.

CrewAI uses LiteLLM internally, so model names use the ollama/ prefix.
The manager runs on the 7B partial-offload model; workers run on 3B.

Hardware: Quadro P1000 4 GB · i9-10900 · 32 GB RAM

Usage:
    python crew.py "Research the current state of RISC-V adoption"
"""
from __future__ import annotations

import os
import sys

from crewai import LLM, Agent, Task, Crew, Process


# ──────────────────────────────────────────────────────────────────────
#  Model Configuration — VRAM-constrained
# ──────────────────────────────────────────────────────────────────────
OLLAMA_BASE = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")

# Manager LLM — 7B partial offload (slower but smarter delegation)
manager_llm = LLM(
    model="ollama/p1000-heavy",
    base_url=OLLAMA_BASE,
    temperature=0.2,
    max_tokens=2048,
    timeout=300,          # 7B is slow at ~6-8 tok/s
    # CrewAI passes these through LiteLLM → Ollama options:
    num_ctx=8192,
)

# Worker LLM — 3B full GPU (fast tool-calling agent)
worker_llm = LLM(
    model="ollama/p1000-agent",
    base_url=OLLAMA_BASE,
    temperature=0.0,
    max_tokens=1024,
    timeout=120,
    num_ctx=4096,
)

# Coder LLM — specialized for code tasks
coder_llm = LLM(
    model="ollama/p1000-coder",
    base_url=OLLAMA_BASE,
    temperature=0.1,
    max_tokens=4096,
    timeout=180,
    num_ctx=8192,
)


# ──────────────────────────────────────────────────────────────────────
#  Agents
# ──────────────────────────────────────────────────────────────────────

researcher = Agent(
    role="Senior Research Analyst",
    goal="Find accurate, current information on the assigned topic",
    backstory=(
        "You are a meticulous researcher who cross-references multiple "
        "sources and flags contradictions.  You cite every claim."
    ),
    llm=worker_llm,
    verbose=True,
    max_iter=6,           # step budget — prevents runaway loops
    max_rpm=10,           # rate limit against Ollama
    allow_delegation=False,
)

writer = Agent(
    role="Technical Writer",
    goal="Produce clear, well-structured Markdown reports",
    backstory=(
        "You are a senior technical writer who converts research notes "
        "into polished reports.  You prefer concise prose over bullet "
        "lists and always include a Sources section."
    ),
    llm=worker_llm,
    verbose=True,
    max_iter=4,
    allow_delegation=False,
)

coder = Agent(
    role="Software Engineer",
    goal="Write production-quality code with error handling",
    backstory=(
        "You are a senior engineer specializing in Python, Go, and Bash "
        "on Ubuntu 20.04.  You write runnable code, never pseudocode."
    ),
    llm=coder_llm,
    verbose=True,
    max_iter=6,
    allow_delegation=False,
)


# ──────────────────────────────────────────────────────────────────────
#  Tasks
# ──────────────────────────────────────────────────────────────────────

def build_research_task(topic: str) -> Task:
    return Task(
        description=(
            f"Research the following topic thoroughly: {topic}\n"
            f"Find at least 3 authoritative sources.  For each source, "
            f"note the key claims, data points, and any contradictions."
        ),
        expected_output="Structured research notes with cited sources.",
        agent=researcher,
    )


def build_report_task() -> Task:
    return Task(
        description=(
            "Take the research notes and write a polished Markdown report. "
            "Include ## headings, a brief executive summary, key findings, "
            "analysis, and a Sources section.  Keep it under 1500 words."
        ),
        expected_output="A complete Markdown research report.",
        agent=writer,
    )


# ──────────────────────────────────────────────────────────────────────
#  Crew assembly
# ──────────────────────────────────────────────────────────────────────

def build_crew(topic: str) -> Crew:
    """Build a hierarchical crew where the manager delegates tasks."""
    research_task = build_research_task(topic)
    report_task = build_report_task()

    return Crew(
        agents=[researcher, writer],
        tasks=[research_task, report_task],
        process=Process.hierarchical,
        manager_llm=manager_llm,
        verbose=True,
        max_rpm=10,               # global rate limit
        memory=False,             # disable for constrained hardware
        planning=False,           # disable LLM-based planning to save tokens
    )


def build_sequential_crew(topic: str) -> Crew:
    """Simpler sequential crew — no manager model needed."""
    research_task = build_research_task(topic)
    report_task = build_report_task()

    return Crew(
        agents=[researcher, writer],
        tasks=[research_task, report_task],
        process=Process.sequential,  # tasks run in order, no manager
        verbose=True,
        max_rpm=10,
        memory=False,
    )


if __name__ == "__main__":
    topic = " ".join(sys.argv[1:]) or "Local LLM deployment on constrained hardware"
    mode = "sequential"  # safer on 4 GB VRAM — no manager model needed

    if "--hierarchical" in sys.argv:
        mode = "hierarchical"

    print(f"Mode: {mode} | Topic: {topic}\n{'─' * 60}")

    if mode == "hierarchical":
        crew = build_crew(topic)
    else:
        crew = build_sequential_crew(topic)

    result = crew.kickoff()
    print("\n" + "═" * 60)
    print(result)
