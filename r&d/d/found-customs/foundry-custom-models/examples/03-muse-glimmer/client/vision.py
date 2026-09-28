#!/usr/bin/env python3
"""vision.py: send an image and a question to Muse Glimmer (image + text in,
text out) through the OpenAI chat API that vLLM serves.

    FTM_BASE_URL=https://ftm-muse-coolgit.eastus2.inference.ml.azure.com FTM_KEY=... \
    vision.py architecture.png "List every component and arrow in this diagram."

The image travels inline as a data URL, so nothing is fetched by the server.
"""

from __future__ import annotations

import argparse
import base64
import mimetypes
import os
import sys
from pathlib import Path

from openai import OpenAI


def data_url(path: Path) -> str:
    mime = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
    if not mime.startswith("image/"):
        raise SystemExit(f"vision.py: {path} is not an image ({mime})")
    return f"data:{mime};base64,{base64.b64encode(path.read_bytes()).decode()}"


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("image", type=Path)
    p.add_argument("question")
    p.add_argument("--model", default=os.environ.get("FTM_MODEL", "muse-glimmer"))
    p.add_argument("--show-reasoning", action="store_true")
    a = p.parse_args(argv)
    base, key = os.environ.get("FTM_BASE_URL"), os.environ.get("FTM_KEY")
    if not (base and key):
        sys.exit("vision.py: set FTM_BASE_URL and FTM_KEY")
    client = OpenAI(base_url=base.rstrip("/") + "/v1", api_key=key)
    r = client.chat.completions.create(
        model=a.model,
        messages=[{"role": "user", "content": [
            {"type": "image_url", "image_url": {"url": data_url(a.image)}},
            {"type": "text", "text": a.question},
        ]}],
        # Muse Glimmer is a reasoning model; its card warns against greedy decoding.
        temperature=0.6,
        max_tokens=4096,
    )
    msg = r.choices[0].message
    if a.show_reasoning and (thought := getattr(msg, "reasoning", None) or (msg.model_extra or {}).get("reasoning")):
        print(f"--- reasoning ---\n{thought}\n--- answer ---")
    print(msg.content)
    return 0


if __name__ == "__main__":
    sys.exit(main())
