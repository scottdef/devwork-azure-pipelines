#!/usr/bin/env python3
"""
research_graph.py — Multi-node research pipeline using LangGraph.

Architecture:
  plan → search → filter → read → synthesize → write

VRAM strategy:
  - planner/synth/writer: qwen2.5:7b partial offload (16 layers on GPU)
  - filter:               qwen2.5:3b full GPU (fast yes/no classifier)

Models swap via OLLAMA_KEEP_ALIVE — only one is loaded at a time.
The 15-minute keep-alive means the 3B stays warm while the 7B runs,
and Ollama auto-evicts the 7B when the 3B is needed again.

Usage:
    python research_graph.py "State of local LLM inference in 2026"
"""
from __future__ import annotations

import asyncio
import json
import os
import sys
from typing import TypedDict, List, Optional

import httpx
from langchain_ollama import ChatOllama
from langgraph.graph import StateGraph, START, END

OLLAMA_BASE = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")
SEARXNG     = os.getenv("SEARXNG_URL", "http://127.0.0.1:8888")


# ──────────────────────────────────────────────────────────────────────
#  Models — VRAM-aware configuration
# ──────────────────────────────────────────────────────────────────────

# Heavy (7B hybrid): planner, synthesizer, writer
heavy = ChatOllama(
    base_url=OLLAMA_BASE,
    model="p1000-heavy",
    temperature=0.3,
    num_ctx=8192,             # needs more context for synthesis
    num_predict=2048,
    num_thread=10,
    num_gpu=16,               # 16/33 layers on GPU → ~2.6 GB VRAM
)

# Fast (3B full GPU): relevance filter
fast = ChatOllama(
    base_url=OLLAMA_BASE,
    model="p1000-agent",
    temperature=0.0,
    num_ctx=2048,             # short prompts for yes/no classification
    num_predict=10,           # just "relevant" or "irrelevant"
    num_thread=10,
    num_gpu=99,               # all layers on GPU
)


# ──────────────────────────────────────────────────────────────────────
#  State
# ──────────────────────────────────────────────────────────────────────

class ResearchState(TypedDict):
    topic: str
    subqueries: List[str]
    raw_hits: List[dict]
    relevant_hits: List[dict]
    documents: List[dict]
    synthesis: str
    report: str


# ──────────────────────────────────────────────────────────────────────
#  Nodes
# ──────────────────────────────────────────────────────────────────────

def plan(state: ResearchState) -> dict:
    """Break the research topic into 4-6 search subqueries."""
    prompt = (
        f"Break this research topic into 4-6 specific web-search queries.\n"
        f"Return ONLY a JSON array of strings, nothing else.\n"
        f"Topic: {state['topic']}"
    )
    raw = heavy.invoke(prompt).content
    # Extract JSON array from response
    start = raw.find("[")
    end = raw.rfind("]") + 1
    if start >= 0 and end > start:
        queries = json.loads(raw[start:end])
    else:
        queries = [state["topic"]]
    return {"subqueries": queries[:6]}


async def _search_one(query: str) -> List[dict]:
    """Search SearXNG for a single query."""
    async with httpx.AsyncClient(timeout=20.0) as client:
        resp = await client.get(
            f"{SEARXNG}/search",
            params={
                "q": query,
                "format": "json",
                "categories": "general,it,science",
                "language": "en",
                "pageno": 1,
            },
            headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"},
        )
        resp.raise_for_status()
        return [
            {
                "url": h["url"],
                "title": h["title"],
                "snippet": h.get("content", ""),
                "engine": h.get("engine", ""),
            }
            for h in resp.json().get("results", [])[:8]
        ]


def search(state: ResearchState) -> dict:
    """Execute all subqueries in parallel against SearXNG."""
    batches = asyncio.run(
        asyncio.gather(*[_search_one(q) for q in state["subqueries"]])
    )
    seen, unique = set(), []
    for hit in [h for batch in batches for h in batch]:
        if hit["url"] not in seen:
            seen.add(hit["url"])
            unique.append(hit)
    return {"raw_hits": unique}


def filter_hits(state: ResearchState) -> dict:
    """Use the fast 3B model to classify each hit as relevant or not."""
    relevant = []
    for hit in state["raw_hits"][:20]:  # cap at 20 to control time
        prompt = (
            f"Topic: {state['topic']}\n"
            f"Title: {hit['title']}\n"
            f"Snippet: {hit['snippet'][:200]}\n"
            f"Is this search result relevant to the topic? "
            f"Reply with exactly one word: relevant or irrelevant"
        )
        answer = fast.invoke(prompt).content.strip().lower()
        if "relevant" in answer and "irrelevant" not in answer:
            relevant.append(hit)
    return {"relevant_hits": relevant[:10]}


def read_pages(state: ResearchState) -> dict:
    """Fetch and extract text from the top relevant URLs."""
    docs = []
    try:
        import trafilatura
    except ImportError:
        trafilatura = None

    for hit in state["relevant_hits"][:6]:
        try:
            if trafilatura:
                html = trafilatura.fetch_url(hit["url"])
                text = trafilatura.extract(html, include_links=False) if html else ""
            else:
                resp = httpx.get(hit["url"], timeout=15.0, follow_redirects=True)
                text = resp.text[:5000]
            if text:
                docs.append({
                    "url": hit["url"],
                    "title": hit["title"],
                    "content": text[:3000],  # cap per doc
                })
        except Exception:
            continue
    return {"documents": docs}


def synthesize(state: ResearchState) -> dict:
    """Combine all document extracts into structured research notes."""
    doc_text = "\n\n---\n\n".join(
        f"Source: {d['title']} ({d['url']})\n{d['content']}"
        for d in state["documents"]
    )
    prompt = (
        f"You are a research analyst. Synthesize these sources into "
        f"detailed, structured notes about: {state['topic']}\n\n"
        f"Sources:\n{doc_text[:12000]}\n\n"
        f"Write organized notes with key findings, contradictions, "
        f"and gaps. Cite sources by title."
    )
    result = heavy.invoke(prompt).content
    return {"synthesis": result}


def write_report(state: ResearchState) -> dict:
    """Write a polished Markdown report from the synthesis notes."""
    prompt = (
        f"Write a concise research report in Markdown format.\n"
        f"Topic: {state['topic']}\n\n"
        f"Research notes:\n{state['synthesis']}\n\n"
        f"Format with ## headings, bullet points for key findings, "
        f"and a Sources section at the end. Be direct and factual."
    )
    result = heavy.invoke(prompt).content
    return {"report": result}


# ──────────────────────────────────────────────────────────────────────
#  Graph assembly
# ──────────────────────────────────────────────────────────────────────

def build_graph():
    g = StateGraph(ResearchState)
    g.add_node("plan", plan)
    g.add_node("search", search)
    g.add_node("filter", filter_hits)
    g.add_node("read", read_pages)
    g.add_node("synthesize", synthesize)
    g.add_node("write", write_report)

    g.add_edge(START, "plan")
    g.add_edge("plan", "search")
    g.add_edge("search", "filter")
    g.add_edge("filter", "read")
    g.add_edge("read", "synthesize")
    g.add_edge("synthesize", "write")
    g.add_edge("write", END)

    return g.compile()


if __name__ == "__main__":
    topic = " ".join(sys.argv[1:]) or "State of local LLM inference in 2026"
    print(f"Researching: {topic}\n{'─' * 60}")

    graph = build_graph()
    result = graph.invoke({"topic": topic})

    print("\n" + "═" * 60)
    print(result["report"])
    print("═" * 60)

    # Save report
    outfile = "research_report.md"
    with open(outfile, "w") as f:
        f.write(result["report"])
    print(f"\nSaved to {outfile}")
