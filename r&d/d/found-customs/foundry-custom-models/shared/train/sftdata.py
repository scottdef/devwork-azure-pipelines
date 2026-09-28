"""Turn chat-format JSONL into (input_ids, labels) with loss on the final
assistant turn only. Pure Python over any tokenizer that has
apply_chat_template, so it is unit-tested without torch or weights."""

from __future__ import annotations

import json
from collections.abc import Iterable
from pathlib import Path
from typing import Any

IGNORE = -100


def read_jsonl(path: str | Path) -> list[dict[str, Any]]:
    rows = []
    for n, line in enumerate(Path(path).read_text(encoding="utf-8").splitlines(), 1):
        if not line.strip():
            continue
        row = json.loads(line)
        validate(row, n)
        rows.append(row)
    if not rows:
        raise ValueError(f"{path}: no examples")
    return rows


def validate(row: dict[str, Any], n: int = 0) -> None:
    msgs = row.get("messages")
    if not isinstance(msgs, list) or len(msgs) < 2:
        raise ValueError(f"line {n}: 'messages' must be a list of at least two turns")
    for m in msgs:
        if m.get("role") not in {"system", "user", "assistant"} or not isinstance(m.get("content"), str):
            raise ValueError(f"line {n}: each turn needs role system|user|assistant and string content")
    if msgs[-1]["role"] != "assistant":
        raise ValueError(f"line {n}: the last turn must be the assistant's answer")


def encode(tokenizer: Any, messages: list[dict[str, str]], max_len: int) -> dict[str, list[int]] | None:
    """Tokenise one conversation. The prompt (everything before the last
    assistant turn, plus the generation prompt) is masked out of the loss.
    Returns None when the prompt alone does not fit."""
    prompt = tokenizer.apply_chat_template(messages[:-1], tokenize=True, add_generation_prompt=True)
    full = tokenizer.apply_chat_template(messages, tokenize=True, add_generation_prompt=False)
    prompt, full = _ids(prompt), _ids(full)
    if full[: len(prompt)] != prompt:
        # Templates that rewrite earlier turns (e.g. stripping reasoning) break the
        # prefix property; fall back to training on the whole sequence.
        labels = list(full)
    else:
        labels = [IGNORE] * len(prompt) + full[len(prompt):]
    if len(prompt) >= max_len:
        return None
    full, labels = full[:max_len], labels[:max_len]
    return {"input_ids": full, "attention_mask": [1] * len(full), "labels": labels}


def encode_all(tokenizer: Any, rows: Iterable[dict[str, Any]], max_len: int) -> tuple[list[dict], int]:
    out, dropped = [], 0
    for r in rows:
        e = encode(tokenizer, r["messages"], max_len)
        if e is None:
            dropped += 1
        else:
            out.append(e)
    return out, dropped


def collate(batch: list[dict[str, list[int]]], pad_id: int) -> dict[str, list[list[int]]]:
    """Right-pad to the longest row. Returns lists; the caller tensorises."""
    width = max(len(b["input_ids"]) for b in batch)
    pad = lambda xs, v: xs + [v] * (width - len(xs))  # noqa: E731
    return {
        "input_ids": [pad(b["input_ids"], pad_id) for b in batch],
        "attention_mask": [pad(b["attention_mask"], 0) for b in batch],
        "labels": [pad(b["labels"], IGNORE) for b in batch],
    }


def _ids(x: Any) -> list[int]:
    # transformers 5 may return a BatchEncoding; older versions a list.
    if isinstance(x, dict) or hasattr(x, "input_ids"):
        x = x["input_ids"]
    return list(x)
