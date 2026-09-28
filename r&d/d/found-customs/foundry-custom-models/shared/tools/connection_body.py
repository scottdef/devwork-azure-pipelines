#!/usr/bin/env python3
"""connection_body.py: the ARM body for a Foundry ModelGateway connection
that points Foundry Agent Service at an Azure ML online endpoint.

Shape follows microsoft-foundry/foundry-samples (01-connections, ModelGateway):
category ModelGateway, authType ApiKey, and string-valued metadata. Foundry
appends /chat/completions to the target, so the target ends in /v1.

    SCORE_KEY=... connection_body.py --target https://ep.eastus2.inference.ml.azure.com/v1 \
        --model gemma4-coolgit [--model other] > body.json
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys


def body(target: str, models: list[str], key: str, shared: bool = False, headers: dict | None = None) -> dict:
    if not re.fullmatch(r"https://[a-z0-9-]+\.[a-z0-9]+\.inference\.ml\.azure\.com/v1", target):
        raise ValueError(f"target {target!r} is not an Azure ML online endpoint /v1 URL")
    if not models:
        raise ValueError("at least one model")
    static = [{"name": m, "properties": {"model": {"name": m, "version": "1", "format": "OpenAI"}}} for m in models]
    return {
        "properties": {
            "category": "ModelGateway",
            "target": target,
            "authType": "ApiKey",
            "isSharedToAll": shared,
            "credentials": {"key": key},
            "metadata": {
                # Every metadata value is a string; objects are JSON-encoded.
                "models": json.dumps(static, separators=(",", ":")),
                "deploymentInPath": "false",
                "authHeaderName": "Authorization",
                "authHeaderFormat": "Bearer {api_key}",
                "customHeaders": json.dumps(headers or {}, separators=(",", ":")),
            },
        }
    }


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--target", required=True)
    p.add_argument("--model", action="append", required=True)
    p.add_argument("--key-env", default="SCORE_KEY")
    p.add_argument("--shared", action="store_true", help="visible to every project on the account")
    a = p.parse_args(argv)
    key = os.environ.get(a.key_env, "")
    if not key:
        print(f"connection_body.py: ${a.key_env} is empty", file=sys.stderr)
        return 2
    try:
        json.dump(body(a.target, a.model, key, a.shared), sys.stdout)
    except ValueError as e:
        print(f"connection_body.py: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
