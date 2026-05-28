#!/usr/bin/env python3
"""
structured_agent.py — PydanticAI agent with typed, schema-constrained outputs.

PydanticAI's killer feature for small models: output_type forces the LLM
to produce valid JSON matching a Pydantic schema.  This eliminates the
#1 failure mode of 3B-8B tool calling: malformed JSON.

Hardware: Quadro P1000 4 GB · i9-10900 · 32 GB RAM

Usage:
    python structured_agent.py
"""
from __future__ import annotations

import os
import asyncio
from dataclasses import dataclass
from typing import Optional

from pydantic import BaseModel, Field
from pydantic_ai import Agent, RunContext
from pydantic_ai.models.openai import OpenAIModel
from pydantic_ai.providers.openai import OpenAIProvider


# ──────────────────────────────────────────────────────────────────────
#  Model Configuration — VRAM-constrained
# ──────────────────────────────────────────────────────────────────────

OLLAMA_BASE = os.getenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")

# PydanticAI connects to Ollama via the OpenAI-compatible /v1/ endpoint
ollama_model = OpenAIModel(
    model_name="p1000-agent",
    provider=OpenAIProvider(
        base_url=f"{OLLAMA_BASE}/v1",
        api_key="not-needed",
    ),
)

ollama_heavy = OpenAIModel(
    model_name="p1000-heavy",
    provider=OpenAIProvider(
        base_url=f"{OLLAMA_BASE}/v1",
        api_key="not-needed",
    ),
)


# ──────────────────────────────────────────────────────────────────────
#  Output schemas — the model MUST produce valid JSON matching these
# ──────────────────────────────────────────────────────────────────────

class TaskBreakdown(BaseModel):
    """A structured task plan."""
    goal: str = Field(description="The overall goal in one sentence")
    steps: list[str] = Field(description="Ordered list of concrete steps")
    estimated_minutes: int = Field(description="Rough time estimate")
    tools_needed: list[str] = Field(description="Tools or resources required")


class CodeReview(BaseModel):
    """Structured code review output."""
    summary: str = Field(description="One-line assessment")
    issues: list[Issue] = Field(default_factory=list)
    suggestions: list[str] = Field(default_factory=list)
    approved: bool = Field(description="Whether the code is ready to merge")
    confidence: float = Field(ge=0.0, le=1.0, description="Confidence in the review")


class Issue(BaseModel):
    """A single code issue."""
    severity: str = Field(description="critical, warning, or info")
    line: Optional[int] = Field(default=None, description="Line number if applicable")
    description: str


class SearchResult(BaseModel):
    """Structured search result summary."""
    query: str
    findings: list[Finding]
    answer: str = Field(description="Direct answer to the query")
    confidence: float = Field(ge=0.0, le=1.0)


class Finding(BaseModel):
    """A single finding from search."""
    source: str
    claim: str
    relevance: float = Field(ge=0.0, le=1.0)


class HardwareReport(BaseModel):
    """Structured system hardware report."""
    gpu_name: str
    vram_total_mb: int
    vram_used_mb: int
    vram_free_mb: int
    gpu_temp_c: Optional[int] = None
    gpu_utilization_pct: Optional[int] = None
    ram_total_mb: int
    ram_used_mb: int
    cpu_model: str
    cpu_cores: int
    recommendations: list[str] = Field(
        description="Model recommendations based on available resources"
    )


# ──────────────────────────────────────────────────────────────────────
#  Dependencies (injected context)
# ──────────────────────────────────────────────────────────────────────

@dataclass
class SystemContext:
    """Runtime context available to all tools."""
    gpu_vram_mb: int = 4096
    gpu_vram_overhead_mb: int = 512
    ram_mb: int = 32768
    cpu_cores: int = 10
    ollama_url: str = "http://127.0.0.1:11434"
    searxng_url: str = "http://127.0.0.1:8888"


# ──────────────────────────────────────────────────────────────────────
#  Agents — each bound to a specific output schema
# ──────────────────────────────────────────────────────────────────────

# Task planner — produces structured TaskBreakdown
planner_agent = Agent(
    model=ollama_model,
    output_type=TaskBreakdown,
    deps_type=SystemContext,
    system_prompt=(
        "You are a task planner. Given a goal, break it into concrete "
        "steps with time estimates and required tools. Consider that "
        "you are running on a Quadro P1000 with 4 GB VRAM and 32 GB RAM."
    ),
    retries=2,
)

# Hardware auditor — produces structured HardwareReport
hardware_agent = Agent(
    model=ollama_model,
    output_type=HardwareReport,
    deps_type=SystemContext,
    system_prompt=(
        "You are a hardware analyst. Given system information, produce "
        "a structured report with model recommendations for local LLM "
        "deployment based on available VRAM and RAM."
    ),
    retries=2,
)


# ──────────────────────────────────────────────────────────────────────
#  Tools (registered to specific agents)
# ──────────────────────────────────────────────────────────────────────

@hardware_agent.tool
async def get_gpu_info(ctx: RunContext[SystemContext]) -> str:
    """Get current GPU information from nvidia-smi."""
    import subprocess
    try:
        result = subprocess.run(
            ["nvidia-smi",
             "--query-gpu=name,memory.used,memory.total,temperature.gpu,utilization.gpu",
             "--format=csv,noheader,nounits"],
            capture_output=True, text=True, timeout=5,
        )
        return result.stdout.strip()
    except Exception as e:
        return f"nvidia-smi failed: {e}"


@hardware_agent.tool
async def get_system_info(ctx: RunContext[SystemContext]) -> str:
    """Get CPU and RAM information."""
    import subprocess
    parts = []
    try:
        cpu = subprocess.run(
            ["lscpu"], capture_output=True, text=True, timeout=5
        )
        parts.append(cpu.stdout[:500])
    except Exception:
        pass
    try:
        mem = subprocess.run(
            ["free", "-m"], capture_output=True, text=True, timeout=5
        )
        parts.append(mem.stdout)
    except Exception:
        pass
    return "\n".join(parts) or "System info unavailable"


@planner_agent.tool
async def estimate_model_vram(
    ctx: RunContext[SystemContext],
    model_params_billions: float,
    quantization_bits: float,
) -> str:
    """Estimate VRAM needed for a model given parameter count and quantization.

    Args:
        model_params_billions: Number of parameters in billions (e.g. 7.0)
        quantization_bits: Bits per weight (e.g. 4.0 for Q4, 8.0 for Q8)
    """
    # Rough formula: params * bits / 8 + 20% overhead for KV cache
    weight_gb = (model_params_billions * 1e9 * quantization_bits / 8) / 1e9
    kv_overhead = weight_gb * 0.2
    total_gb = weight_gb + kv_overhead
    usable_vram = (ctx.deps.gpu_vram_mb - ctx.deps.gpu_vram_overhead_mb) / 1024

    fits = "YES" if total_gb <= usable_vram else "NO"
    return (
        f"Model: {model_params_billions}B at Q{quantization_bits}\n"
        f"Estimated VRAM: {total_gb:.1f} GB\n"
        f"Usable VRAM: {usable_vram:.1f} GB\n"
        f"Fits on GPU: {fits}"
    )


# ──────────────────────────────────────────────────────────────────────
#  Example: using raw Ollama structured output (no PydanticAI)
# ──────────────────────────────────────────────────────────────────────

def ollama_structured_output():
    """
    Direct Ollama SDK structured output — the simplest way to get
    validated JSON from a small model.  No agent framework needed.
    """
    from ollama import chat

    class ModelFit(BaseModel):
        model_name: str
        params_billions: float
        quantization: str
        estimated_vram_gb: float
        fits_on_p1000: bool
        recommended_num_gpu: int = Field(description="Layers to offload to GPU")

    class ModelAssessment(BaseModel):
        models: list[ModelFit]
        best_pick: str = Field(description="Recommended model for this hardware")

    response = chat(
        model="p1000-agent",
        messages=[{
            "role": "user",
            "content": (
                "Assess these models for a Quadro P1000 (4 GB VRAM): "
                "qwen2.5:3b Q4, llama3.2:3b Q4, mistral:7b Q4, "
                "qwen2.5:14b Q4, gemma2:27b Q4. "
                "For each, estimate VRAM usage and whether it fits."
            ),
        }],
        format=ModelAssessment.model_json_schema(),
    )

    assessment = ModelAssessment.model_validate_json(response.message.content)
    return assessment


# ──────────────────────────────────────────────────────────────────────
#  Main
# ──────────────────────────────────────────────────────────────────────

async def main():
    ctx = SystemContext()

    print("── Task Planner ──")
    plan_result = await planner_agent.run(
        "Set up a multi-model LLM system on a P340 SFF with 4 GB VRAM",
        deps=ctx,
    )
    print(f"Goal: {plan_result.output.goal}")
    for i, step in enumerate(plan_result.output.steps, 1):
        print(f"  {i}. {step}")
    print(f"  Est. time: {plan_result.output.estimated_minutes} min")
    print(f"  Tools: {', '.join(plan_result.output.tools_needed)}")

    print("\n── Hardware Audit ──")
    hw_result = await hardware_agent.run(
        "Analyze this system's hardware and recommend LLM models",
        deps=ctx,
    )
    hw = hw_result.output
    print(f"  GPU: {hw.gpu_name} ({hw.vram_free_mb} MB free / {hw.vram_total_mb} MB)")
    print(f"  RAM: {hw.ram_used_mb} MB used / {hw.ram_total_mb} MB")
    for rec in hw.recommendations:
        print(f"  → {rec}")

    print("\n── Direct Ollama Structured Output ──")
    try:
        assessment = ollama_structured_output()
        print(f"  Best pick: {assessment.best_pick}")
        for m in assessment.models:
            fit = "✓" if m.fits_on_p1000 else "✗"
            print(f"  {fit} {m.model_name} ({m.params_billions}B {m.quantization}) "
                  f"→ {m.estimated_vram_gb:.1f} GB, num_gpu={m.recommended_num_gpu}")
    except Exception as e:
        print(f"  (skipped — {e})")


if __name__ == "__main__":
    asyncio.run(main())
