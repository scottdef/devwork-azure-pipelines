#!/usr/bin/env python3
"""smoke_test.py: prove a deployment answers before it gets traffic.

Standard library only, so it runs on any runner without an install step.
The key is read from an environment variable, never from argv.

    SCORE_KEY=... smoke_test.py --base-url https://ep.eastus2.inference.ml.azure.com \
        --deployment v1a2b3c4d5e --model gemma4-coolgit --kind tools

kinds:
  chat   GET /v1/models lists --model; a chat completion returns the marker
  tools  chat, plus a completion that must call the offered tool
  http   GET --path answers with a status below 500 (non-OpenAI servers)
Exit status 0 only when every check passed. One JSON line per check on stdout.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

MARKER = "FTM_OK"
TOOL = {
    "type": "function",
    "function": {
        "name": "get_utc_time",
        "description": "Return the current UTC time as ISO 8601.",
        "parameters": {"type": "object", "properties": {}, "required": []},
    },
}


class Client:
    def __init__(self, base: str, key: str, deployment: str, timeout: float):
        self.base, self.timeout = base.rstrip("/"), timeout
        self.headers = {"Authorization": f"Bearer {key}", "Content-Type": "application/json"}
        if deployment:
            # Pins the request to one deployment regardless of traffic split.
            self.headers["azureml-model-deployment"] = deployment

    def call(self, method: str, path: str, body: dict | None = None) -> tuple[int, bytes]:
        data = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(self.base + path, data=data, method=method, headers=self.headers)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as r:
                return r.status, r.read()
        except urllib.error.HTTPError as e:
            return e.code, e.read()

    def retry(self, method: str, path: str, body: dict | None, attempts: int) -> tuple[int, bytes]:
        delay = 5.0
        for i in range(attempts):
            try:
                status, raw = self.call(method, path, body)
            except (urllib.error.URLError, TimeoutError, ConnectionError) as e:
                status, raw = 0, str(e).encode()
            if status and status < 500 and status != 429:
                return status, raw
            if i < attempts - 1:
                time.sleep(delay)
                delay = min(delay * 2, 60)
        return status, raw


def chat_body(model: str, messages: list, **extra) -> dict:
    body = {"model": model, "messages": messages, "max_tokens": 1024, "temperature": 0.6}
    # Reasoning models (Qwen3.x) think by default; a smoke test only needs the answer.
    # Templates that do not know the variable ignore it.
    body["chat_template_kwargs"] = {"enable_thinking": False}
    body.update(extra)
    return body


def check_chat(c: Client, model: str, attempts: int) -> list[dict]:
    out = []
    st, raw = c.retry("GET", "/v1/models", None, attempts)
    ids = [m.get("id") for m in _json(raw).get("data", [])] if st == 200 else []
    out.append({"check": "models", "ok": st == 200 and model in ids, "status": st, "models": ids})
    t0 = time.monotonic()
    msg = [{"role": "user", "content": f"Reply with exactly this word and nothing else: {MARKER}"}]
    st, raw = c.retry("POST", "/v1/chat/completions", chat_body(model, msg), attempts)
    body = _json(raw)
    text = _content(body)
    out.append({"check": "chat", "ok": st == 200 and MARKER in text, "status": st,
                "latency_ms": int((time.monotonic() - t0) * 1000),
                "usage": body.get("usage"), "reply": text[:120]})
    return out


def check_tools(c: Client, model: str, attempts: int) -> dict:
    msg = [{"role": "user", "content": "What is the current UTC time? Use the tool; do not guess."}]
    st, raw = c.retry("POST", "/v1/chat/completions",
                      chat_body(model, msg, tools=[TOOL], tool_choice="auto"), attempts)
    calls = ((_json(raw).get("choices") or [{}])[0].get("message") or {}).get("tool_calls") or []
    names = [(x.get("function") or {}).get("name") for x in calls]
    return {"check": "tools", "ok": st == 200 and "get_utc_time" in names, "status": st, "tool_calls": names}


def _json(raw: bytes) -> dict:
    try:
        v = json.loads(raw)
        return v if isinstance(v, dict) else {}
    except ValueError:
        return {}


def _content(body: dict) -> str:
    try:
        return body["choices"][0]["message"].get("content") or ""
    except (KeyError, IndexError, TypeError, AttributeError):
        return ""


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--base-url", required=True, help="endpoint root, without /v1")
    p.add_argument("--key-env", default="SCORE_KEY", help="environment variable holding the key")
    p.add_argument("--deployment", default="", help="pin requests to this deployment")
    p.add_argument("--model", default="")
    p.add_argument("--kind", choices=("chat", "tools", "http"), default="chat")
    p.add_argument("--path", default="/")
    p.add_argument("--timeout", type=float, default=175.0)
    p.add_argument("--attempts", type=int, default=6)
    a = p.parse_args(argv)
    key = os.environ.get(a.key_env, "")
    if not key:
        print(f"smoke_test.py: ${a.key_env} is empty", file=sys.stderr)
        return 2
    local = a.base_url.startswith(("http://127.0.0.1", "http://localhost"))  # tests only
    if not (a.base_url.startswith("https://") or local):
        print("smoke_test.py: --base-url must be https", file=sys.stderr)
        return 2
    c = Client(a.base_url, key, a.deployment, a.timeout)
    if a.kind == "http":
        st, _ = c.retry("GET", a.path, None, a.attempts)
        results = [{"check": "http", "ok": 0 < st < 500, "status": st, "path": a.path}]
    else:
        results = check_chat(c, a.model, a.attempts)
        if a.kind == "tools":
            results.append(check_tools(c, a.model, a.attempts))
    for r in results:
        print(json.dumps(r))
    return 0 if all(r["ok"] for r in results) else 1


if __name__ == "__main__":
    sys.exit(main())
