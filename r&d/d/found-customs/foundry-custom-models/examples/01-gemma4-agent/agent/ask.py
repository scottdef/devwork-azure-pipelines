#!/usr/bin/env python3
"""ask.py: ask the Foundry agent a question through the Responses API.

    ask.py "How do I archive an inactive repository?"
    ask.py --expect Summary --expect Rollback "Runbook: rotate the app key."

With --expect, exit 1 unless every expected string appears in the answer:
the workflow uses that as an end-to-end test of connection, agent and model.
"""

from __future__ import annotations

import argparse
import sys

from azure.ai.projects import AIProjectClient
from azure.identity import DefaultAzureCredential

from create_agent import project_endpoint


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("question")
    p.add_argument("--name", default="coolgit-runbooks")
    p.add_argument("--expect", action="append", default=[])
    a = p.parse_args(argv)

    with AIProjectClient(endpoint=project_endpoint(), credential=DefaultAzureCredential()) as project:
        client = project.get_openai_client()
        resp = client.responses.create(
            input=a.question,
            extra_body={"agent_reference": {"name": a.name, "type": "agent_reference"}},
        )
    text = resp.output_text or ""
    print(text)
    missing = [e for e in a.expect if e not in text]
    if missing:
        print(f"ask.py: answer lacks {missing}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
