#!/usr/bin/env python3
"""create_agent.py: publish a Foundry prompt agent that runs on the
fine-tuned Gemma 4 served from Azure ML.

Foundry reaches the model through the ModelGateway connection that
`ftm connect gemma4` created; the agent names it "<connection>/<model>".
Each run creates a new agent version, so the agent's history in Foundry
matches the model's history in Azure ML.

    FOUNDRY_PROJECT_ENDPOINT=https://ais-coolgit-copilot-prod.services.ai.azure.com/api/projects/proj-coolgit-agents \
    create_agent.py --model ftm-gemma4/gemma4-coolgit --model-version 1a2b3c4d5e6f
"""

from __future__ import annotations

import argparse
import json
import os
import sys

from azure.ai.projects import AIProjectClient
from azure.ai.projects.models import PromptAgentDefinition
from azure.identity import DefaultAzureCredential

# The fine-tuning taught the format; the instructions restate it so the base
# model behaves, too, and so a reader of the agent sees the contract.
INSTRUCTIONS = """You are the CoolGitOrg platform runbook assistant.
Answer with three sections, Summary, Steps and Rollback, in that order.
Steps are numbered shell commands with one comment each. Never invent flags.
If a change needs approval, name the IssueOps label (approval:*).
If you do not know the procedure, say so in Summary and stop."""


def project_endpoint() -> str:
    if ep := os.environ.get("FOUNDRY_PROJECT_ENDPOINT"):
        return ep
    acct, proj = os.environ.get("FOUNDRY_ACCOUNT"), os.environ.get("FOUNDRY_PROJECT")
    if not (acct and proj):
        sys.exit("create_agent.py: set FOUNDRY_PROJECT_ENDPOINT, or FOUNDRY_ACCOUNT and FOUNDRY_PROJECT")
    return f"https://{acct}.services.ai.azure.com/api/projects/{proj}"


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--name", default="coolgit-runbooks", help="agent name")
    p.add_argument("--model", required=True, help="<connection>/<model>, e.g. ftm-gemma4/gemma4-coolgit")
    p.add_argument("--model-version", default="", help="recorded in agent metadata")
    p.add_argument("--temperature", type=float, default=0.3)
    a = p.parse_args(argv)
    if "/" not in a.model:
        p.error("--model must be <connection>/<model> for a BYOM model")

    with AIProjectClient(endpoint=project_endpoint(), credential=DefaultAzureCredential()) as project:
        agent = project.agents.create_version(
            agent_name=a.name,
            definition=PromptAgentDefinition(model=a.model, instructions=INSTRUCTIONS, temperature=a.temperature),
            description="Runbook assistant on a fine-tuned Gemma 4 served from Azure ML",
            metadata={
                "model": a.model,
                "model_version": a.model_version,
                "source": os.environ.get("GITHUB_REPOSITORY", "local"),
                "git_sha": os.environ.get("GITHUB_SHA", "")[:12],
            },
        )
    print(json.dumps({"name": agent.name, "version": agent.version, "model": a.model}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
