"""An OpenAI-compatible fake that behaves like the vLLM endpoint closely
enough for smoke_test.py and the Agent Framework harness: /v1/models,
chat completions with tool calls, and the coolgit-ops reviewer contract."""

from __future__ import annotations

import json
import re
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

KEY = "test-key"


def review(yml: str) -> dict:
    risks = []
    if "visibility: public" in yml:
        risks.append("public")
    if re.search(r"^\s+(?!platform-admins)[\w-]+: admin", yml, re.M):
        return {"decision": "reject", "risks": ["admin"], "checks": []}
    if "rulesets: []" in yml or "codeowners: []" in yml:
        return {"decision": "reject", "risks": ["ruleset"], "checks": []}
    if "outside_collaborators" in yml and "expires" not in yml:
        risks.append("expiry")
    return {"decision": "changes" if risks else "approve", "risks": risks, "checks": []}


class Handler(BaseHTTPRequestHandler):
    models = ("qwen38-27b", "coolgit-ops")
    calls: list = []

    def log_message(self, *a):
        pass

    def _send(self, code, obj):
        raw = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def _auth(self):
        if self.headers.get("Authorization") != f"Bearer {KEY}":
            self._send(401, {"error": "unauthorized"})
            return False
        return True

    def do_GET(self):
        if not self._auth():
            return
        if self.path == "/v1/models":
            self._send(200, {"object": "list", "data": [{"id": m, "object": "model"} for m in self.models]})
        else:
            self._send(404, {"error": "not found"})

    def do_POST(self):
        if not self._auth():
            return
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        Handler.calls.append({"path": self.path, "body": body, "deployment": self.headers.get("azureml-model-deployment")})
        if self.path != "/v1/chat/completions":
            return self._send(404, {"error": "not found"})
        if body.get("model") not in self.models:
            return self._send(404, {"error": f"model {body.get('model')} does not exist"})
        msgs = body["messages"]
        text = " ".join(m["content"] if isinstance(m.get("content"), str) else "" for m in msgs)
        tools = body.get("tools") or []
        used = {m.get("name") or m.get("tool_call_id") for m in msgs if m.get("role") == "tool"}
        msg = {"role": "assistant", "content": ""}
        if tools and len(used) < len(tools):
            # Call the next tool the model has not called yet.
            names = [t["function"]["name"] for t in tools]
            called = [c["function"]["name"] for m in msgs if m.get("role") == "assistant" for c in (m.get("tool_calls") or [])]
            nxt = next((n for n in names if n not in called), None)
            if nxt:
                args = {"name": "research-scratch.yml"} if nxt == "read_change" else {}
                msg["tool_calls"] = [{"id": f"call_{len(called)}", "type": "function",
                                      "function": {"name": nxt, "arguments": json.dumps(args)}}]
        if "tool_calls" not in msg:
            if "Reply with exactly" in text:
                msg["content"] = "FTM_OK"
            elif "coolgit-ops" in text:
                m = re.findall(r"```yaml\n(.*?)```", text, re.S)
                msg["content"] = json.dumps(review(m[-1] if m else text))
            else:
                blocks = [m["content"] for m in msgs if m.get("role") == "tool" and "repo:" in str(m.get("content"))]
                msg["content"] = "Brief:\n```yaml\n" + (blocks[-1] if blocks else "") + "\n```"
        self._send(200, {"id": "x", "object": "chat.completion", "created": int(time.time()), "model": body["model"],
                         "choices": [{"index": 0, "message": msg, "finish_reason": "stop"}],
                         "usage": {"prompt_tokens": 5, "completion_tokens": 1, "total_tokens": 6}})


def serve():
    srv = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    return srv, f"http://127.0.0.1:{srv.server_address[1]}"
