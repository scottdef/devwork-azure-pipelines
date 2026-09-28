#!/usr/bin/env python3
"""run.py: an Agent Framework harness over the custom Qwen3.8 endpoint.

Two models live on one vLLM server: the base (`qwen38-27b`) and the LoRA
adapter (`coolgit-ops`). The harness uses both.

    run.py review  FILE     one agent: the adapter reviews a change file
    run.py workflow FILE    two agents: base model investigates with tools,
                            adapter decides; an Agent Framework workflow
    run.py eval             review every case in governance/cases.json and
                            exit 1 below the accuracy bar

Environment: FTM_BASE_URL (endpoint root), FTM_KEY, and optionally
FTM_BASE_MODEL (default qwen38-27b), FTM_ADAPTER_MODEL (default coolgit-ops),
FTM_DEPLOYMENT (pin one Azure ML deployment).
"""

from __future__ import annotations

import argparse
import asyncio
import json
import os
import re
import sys
from pathlib import Path

from agent_framework import Agent, WorkflowBuilder
from agent_framework.openai import OpenAIChatCompletionClient

sys.path.insert(0, str(Path(__file__).parent))
from tools import CHANGES, GOV, TOOLS  # noqa: E402

# Same system prompt the adapter was trained with (data/build.py).
REVIEWER = ("You are coolgit-ops, the CoolGitOrg governance reviewer. Review the proposed YAML change "
            "and reply with one JSON object and nothing else: {\"decision\": \"approve\"|\"changes\"|\"reject\", "
            "\"risks\": [strings], \"checks\": [strings]}. Base the decision on the policy: repositories are "
            "private or internal unless public is approved; admin is only for platform-admins; every repo "
            "has CODEOWNERS and a default-branch ruleset; outside collaborators need an expiry.")

INVESTIGATOR = ("You prepare governance reviews. Use the tools: read the policy, read the named change file. "
                "Then write a short brief: the YAML verbatim in a ```yaml block, followed by each policy rule "
                "it may touch. Do not decide; a reviewer decides after you.")

NO_THINK = {"extra_body": {"chat_template_kwargs": {"enable_thinking": False}}}
DECISIONS = {"approve", "changes", "reject"}


def client(model: str) -> OpenAIChatCompletionClient:
    base, key = os.environ.get("FTM_BASE_URL", ""), os.environ.get("FTM_KEY", "")
    local = base.startswith(("http://127.0.0.1", "http://localhost"))  # tests only
    if not ((base.startswith("https://") or local) and key):
        sys.exit("run.py: set FTM_BASE_URL (https://...) and FTM_KEY")
    headers = {"azureml-model-deployment": d} if (d := os.environ.get("FTM_DEPLOYMENT")) else None
    return OpenAIChatCompletionClient(model=model, base_url=base.rstrip("/") + "/v1", api_key=key,
                                      default_headers=headers)


def reviewer() -> Agent:
    model = os.environ.get("FTM_ADAPTER_MODEL", "coolgit-ops")
    return Agent(client(model), instructions=REVIEWER, name="reviewer",
                 default_options={"temperature": 0.2, "max_tokens": 1024, **NO_THINK})


def investigator() -> Agent:
    model = os.environ.get("FTM_BASE_MODEL", "qwen38-27b")
    # Thinking stays on here: tool planning benefits from it.
    return Agent(client(model), instructions=INVESTIGATOR, name="investigator", tools=TOOLS,
                 default_options={"temperature": 0.6, "max_tokens": 4096})


def decision(text: str) -> dict:
    """First JSON object in the reply with a valid decision; {'decision': 'invalid'} otherwise.
    Scans every '{' so braces in surrounding prose do not hide the object."""
    dec = json.JSONDecoder()
    for m in re.finditer(r"\{", text):
        try:
            v, _ = dec.raw_decode(text, m.start())
        except ValueError:
            continue
        if isinstance(v, dict) and v.get("decision") in DECISIONS:
            return v
    return {"decision": "invalid", "raw": text[:300]}


def _text(o) -> str:
    for attr in ("text",):
        if isinstance(getattr(o, attr, None), str):
            return getattr(o, attr)
    if (r := getattr(o, "agent_response", None)) is not None:
        return r.text
    return str(o)


async def review(name: str) -> dict:
    yml = (CHANGES / Path(name).name).read_text()
    resp = await reviewer().run(f"Review this change:\n```yaml\n{yml}\n```")
    return {"file": Path(name).name, **decision(resp.text)}


async def workflow(name: str) -> dict:
    inv, rev = investigator(), reviewer()
    wf = WorkflowBuilder(start_executor=inv, output_from=[rev], name="governance-review").add_edge(inv, rev).build()
    result = await wf.run(f"Prepare the review brief for change file {Path(name).name}.")
    outputs = result.get_outputs()
    if not outputs:
        return {"file": Path(name).name, "decision": "invalid", "raw": "workflow produced no output"}
    return {"file": Path(name).name, **decision(_text(outputs[-1]))}


async def evaluate() -> int:
    spec = json.loads((GOV / "cases.json").read_text())
    right = 0
    for case in spec["cases"]:
        got = await review(case["file"])
        ok = got["decision"] == case["expected"]
        right += ok
        print(json.dumps({"file": case["file"], "expected": case["expected"], "got": got["decision"], "ok": ok}))
    acc = right / len(spec["cases"])
    print(json.dumps({"accuracy": round(acc, 3), "min_accuracy": spec["min_accuracy"]}))
    return 0 if acc >= spec["min_accuracy"] else 1


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    sub.add_parser("review").add_argument("file")
    sub.add_parser("workflow").add_argument("file")
    sub.add_parser("eval")
    a = p.parse_args(argv)
    if a.cmd == "eval":
        return asyncio.run(evaluate())
    out = asyncio.run(review(a.file) if a.cmd == "review" else workflow(a.file))
    print(json.dumps(out, indent=2))
    return 0 if out["decision"] in DECISIONS else 1


if __name__ == "__main__":
    sys.exit(main())
