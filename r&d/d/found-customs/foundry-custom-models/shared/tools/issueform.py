#!/usr/bin/env python3
"""issueform.py: parse and validate a model-request issue.

Reads the issue body (GitHub issue-form markdown) on stdin and prints one
JSON object. Every value is checked against the same rules ftmcfg applies,
so nothing a requester types reaches a shell, a YAML file or Azure
unvalidated. Exit status 1 with {"ok": false, "errors": [...]} otherwise.

    issueform.py < body.md
    issueform.py --command "$COMMENT_BODY"     prints the slash command or "none"
"""

from __future__ import annotations

import json
import re
import sys

sys.path.insert(0, __file__.rsplit("/", 1)[0])
from ftmcfg import EXAMPLES, SKUS  # noqa: E402

LABELS = {  # issue-form label -> key (keep in step with .github/ISSUE_TEMPLATE/model-request.yml)
    "Example": "example",
    "Action": "action",
    "Hugging Face repository override": "hf_repo",
    "Hugging Face revision": "hf_revision",
    "Fine-tune": "train",
    "Instance type": "instance_type",
    "Instance count": "instance_count",
    "Reason": "reason",
}
RULES = {
    "example": "(" + "|".join(EXAMPLES) + ")",
    "action": "(deploy|destroy)",
    "hf_repo": r"([A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95})?",
    "hf_revision": r"([0-9a-f]{40}|main)",
    "train": "(default|yes|no)",
    "instance_type": "(default|" + "|".join(SKUS) + ")",
    "instance_count": "[1-4]",
    "reason": r"[^\x00-\x1f`$\\]{8,200}",
}
DEFAULTS = {"hf_repo": "", "hf_revision": "main", "train": "default", "instance_type": "default", "instance_count": "1"}
COMMANDS = ("approve", "plan", "cancel")


def parse(body: str) -> dict[str, str]:
    out: dict[str, str] = {}
    parts = re.split(r"^### +(.+?)\s*$", body.replace("\r\n", "\n"), flags=re.M)
    for label, value in zip(parts[1::2], parts[2::2]):
        key = LABELS.get(label.strip())
        if not key:
            continue
        v = value.strip()
        if v == "_No response_":
            v = ""
        out[key] = v.split(" ")[0] if key in {"example", "action", "train", "instance_type"} else v
    return out


def validate(fields: dict[str, str]) -> tuple[dict[str, str], list[str]]:
    vals = dict(DEFAULTS)
    vals.update({k: v for k, v in fields.items() if v != ""})
    errors = []
    for k, pat in RULES.items():
        if k not in vals:
            errors.append(f"{k} is required")
        elif not re.fullmatch(pat, vals[k]):
            errors.append(f"{k}={vals[k][:60]!r} is not valid")
    return vals, errors


def command(comment: str) -> str:
    first = (comment.strip().splitlines() or [""])[0].strip().lower()
    m = re.fullmatch(r"/(\w+)", first)
    return m.group(1) if m and m.group(1) in COMMANDS else "none"


def main(argv: list[str]) -> int:
    if argv[:1] == ["--command"]:
        print(command(argv[1] if len(argv) > 1 else ""))
        return 0
    vals, errors = validate(parse(sys.stdin.read()))
    vals["ok"] = not errors
    vals["errors"] = errors
    print(json.dumps(vals, sort_keys=True))
    return 0 if not errors else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
