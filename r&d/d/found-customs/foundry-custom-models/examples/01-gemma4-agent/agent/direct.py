#!/usr/bin/env python3
"""direct.py: call the endpoint without Foundry, as any OpenAI client would.

Useful to separate "the model is wrong" from "the connection is wrong".

    FTM_BASE_URL=https://ftm-gemma4-coolgit.eastus2.inference.ml.azure.com \
    FTM_KEY=$(az ml online-endpoint get-credentials -n ftm-gemma4-coolgit -g RG -w WS --query primaryKey -o tsv) \
    direct.py "How do I revoke a leaked token?"
"""

from __future__ import annotations

import argparse
import os
import sys

from openai import OpenAI


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("question")
    p.add_argument("--model", default=os.environ.get("FTM_MODEL", "gemma4-coolgit"))
    p.add_argument("--deployment", default="", help="pin to one deployment (blue/green testing)")
    a = p.parse_args(argv)
    base, key = os.environ.get("FTM_BASE_URL"), os.environ.get("FTM_KEY")
    if not (base and key):
        sys.exit("direct.py: set FTM_BASE_URL and FTM_KEY")
    headers = {"azureml-model-deployment": a.deployment} if a.deployment else None
    client = OpenAI(base_url=base.rstrip("/") + "/v1", api_key=key, default_headers=headers)
    r = client.chat.completions.create(
        model=a.model,
        messages=[{"role": "user", "content": a.question}],
        temperature=0.3,
        max_tokens=1024,
    )
    print(r.choices[0].message.content)
    return 0


if __name__ == "__main__":
    sys.exit(main())
