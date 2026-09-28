"""Read-only tools the investigator agent may call. They read files under
governance/ and nothing else: no network, no writes, no path escapes."""

from __future__ import annotations

from pathlib import Path

from agent_framework import tool

GOV = Path(__file__).resolve().parents[1] / "governance"
CHANGES = GOV / "changes"


def _safe(name: str) -> Path:
    p = (CHANGES / name).resolve()
    if p.parent != CHANGES.resolve() or p.suffix not in {".yml", ".yaml"}:
        raise ValueError(f"not a change file: {name}")
    return p


@tool
def list_changes() -> list[str]:
    """List the proposed repository change files awaiting review."""
    return sorted(p.name for p in CHANGES.glob("*.y*ml"))


@tool
def read_change(name: str) -> str:
    """Return the YAML of one proposed change file, by file name (for example billing-service.yml)."""
    return _safe(name).read_text()


@tool
def read_policy() -> str:
    """Return the repository policy that every change must satisfy."""
    return (GOV / "policy.md").read_text()


TOOLS = [list_changes, read_change, read_policy]
