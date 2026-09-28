#!/usr/bin/env python3
"""wham_client.py: ask the Microsoft Muse (WHAM) research server to continue
a gameplay sequence. Standard library only.

WHAM is not a chat model. A job is up to 10 context steps, each a 300x180
frame plus that step's controller action, and it returns predicted frames.
The request shape follows run_server.py in the microsoft/wham repository:
POST /new_job with a multipart form whose "json" field lists the steps and
whose file parts are named after each step's image_name; then GET
/get_job_results?job_ids=... returns a zip of PNGs and predictions.json.

    FTM_BASE_URL=https://ftm-wham-coolgit.eastus2.inference.ml.azure.com FTM_KEY=... \
    wham_client.py --steps steps.json --frames frames/ --predict 5 --out result.zip

steps.json: [{"image_name": "f0.png", "action": [...], "tokens": [...]}, ...]
using the action encoding documented in the WHAM model card. Research use only;
keep the output watermark intact.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path


def multipart(fields: dict[str, str], files: dict[str, Path]) -> tuple[bytes, str]:
    boundary = uuid.uuid4().hex
    out = bytearray()
    for k, v in fields.items():
        out += f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode()
    for k, path in files.items():
        out += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"; filename=\"{path.name}\"\r\n"
                "Content-Type: image/png\r\n\r\n").encode() + path.read_bytes() + b"\r\n"
    out += f"--{boundary}--\r\n".encode()
    return bytes(out), f"multipart/form-data; boundary={boundary}"


def call(req: urllib.request.Request, timeout: float = 170) -> tuple[int, bytes, str]:
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, r.read(), r.headers.get("Content-Type", "")
    except urllib.error.HTTPError as e:
        return e.code, e.read(), e.headers.get("Content-Type", "")


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--steps", type=Path, required=True)
    p.add_argument("--frames", type=Path, required=True)
    p.add_argument("--predict", type=int, default=5, help="num_steps_to_predict (server caps it)")
    p.add_argument("--out", type=Path, default=Path("wham-result.zip"))
    p.add_argument("--wait", type=float, default=600, help="seconds to poll for results")
    a = p.parse_args(argv)
    base, key = (os.environ.get("FTM_BASE_URL") or "").rstrip("/"), os.environ.get("FTM_KEY")
    if not (base.startswith("https://") and key):
        sys.exit("wham_client.py: set FTM_BASE_URL (https) and FTM_KEY")
    auth = {"Authorization": f"Bearer {key}"}

    steps = json.loads(a.steps.read_text())
    if not 1 <= len(steps) <= 10:
        sys.exit("wham_client.py: 1 to 10 context steps")
    files = {s["image_name"]: a.frames / s["image_name"] for s in steps}
    body, ctype = multipart({"json": json.dumps({"num_steps_to_predict": a.predict, "steps": steps})}, files)
    st, raw, _ = call(urllib.request.Request(f"{base}/new_job", data=body, method="POST",
                                             headers={**auth, "Content-Type": ctype}))
    if st != 200:
        sys.exit(f"wham_client.py: new_job {st}: {raw[:300]!r}")
    job = json.loads(raw).get("job_id") or raw.decode().strip()
    print(f"job {job}", file=sys.stderr)

    deadline = time.monotonic() + a.wait
    while time.monotonic() < deadline:
        st, raw, ct = call(urllib.request.Request(f"{base}/get_job_results?job_ids={job}", headers=auth))
        if st == 200 and "zip" in ct:
            a.out.write_bytes(raw)
            print(a.out)
            return 0
        time.sleep(5)
    sys.exit(f"wham_client.py: no results after {a.wait:.0f}s (last status {st})")


if __name__ == "__main__":
    sys.exit(main())
