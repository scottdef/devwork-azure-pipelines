"""fetch.py: copy a Hugging Face snapshot into an Azure ML job output.

Runs inside Azure ML, not on the GitHub runner: a 30B checkpoint is 60 GB and
a hosted runner has a fraction of that. Writes SHA256SUMS and ftm-fetch.json
next to the weights so the registered model carries its own provenance.

    python fetch.py --repo google/gemma-4-E4B-it --revision <sha> --out DIR
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from pathlib import Path

from huggingface_hub import HfApi, snapshot_download

from ftmlib import hf_token, sha256_tree


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--repo", required=True)
    p.add_argument("--revision", required=True, help="40-hex commit; 'main' is refused")
    p.add_argument("--out", required=True)
    p.add_argument("--include", action="append", default=[], help="glob, repeatable")
    p.add_argument("--no-hash", action="store_true", help="skip SHA256SUMS (faster)")
    a = p.parse_args(argv)

    if len(a.revision) != 40 or any(c not in "0123456789abcdef" for c in a.revision):
        print("fetch.py: --revision must be a full commit sha; resolve it first", file=sys.stderr)
        return 2

    token = hf_token()
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    t0 = time.time()
    snapshot_download(
        repo_id=a.repo,
        revision=a.revision,
        local_dir=out,
        token=token,
        allow_patterns=a.include or None,
    )
    # Drop the hub's local cache metadata; it is not part of the model.
    cache = out / ".cache"
    if cache.exists():
        for f in sorted(cache.rglob("*"), reverse=True):
            f.unlink() if f.is_file() else f.rmdir()
        cache.rmdir()

    info = HfApi(token=token).model_info(a.repo, revision=a.revision)
    manifest = {
        "repo": a.repo,
        "revision": a.revision,
        "resolved_sha": info.sha,
        "license": (info.card_data or {}).get("license") if info.card_data else None,
        "gated": getattr(info, "gated", None),
        "seconds": round(time.time() - t0, 1),
    }
    if not a.no_hash:
        sums = sha256_tree(out, exclude={"SHA256SUMS", "ftm-fetch.json"})
        (out / "SHA256SUMS").write_text("".join(f"{h}  {n}\n" for n, h in sums))
        manifest["sha256sums_sha256"] = hashlib.sha256((out / "SHA256SUMS").read_bytes()).hexdigest()
    (out / "ftm-fetch.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest))
    return 0


if __name__ == "__main__":
    os.environ.setdefault("HF_XET_HIGH_PERFORMANCE", "1")
    sys.exit(main())
