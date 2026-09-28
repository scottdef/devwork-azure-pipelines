"""smoke_test.py, connection_body.py and the WHAM shim against local servers."""

import json
import os
import subprocess
import sys
import threading
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest

import connection_body
import fakes
from conftest import ROOT

TOOLS = ROOT / "shared/tools"


@pytest.fixture(scope="module")
def fake():
    srv, url = fakes.serve()
    yield url
    srv.shutdown()


def smoke(url, *args, key=fakes.KEY):
    env = {**os.environ, "SCORE_KEY": key}
    return subprocess.run([sys.executable, TOOLS / "smoke_test.py", "--base-url", url, "--attempts", "1", *args],
                          env=env, capture_output=True, text=True)


def test_smoke_tools_passes_and_pins_the_deployment(fake):
    fakes.Handler.calls.clear()
    r = smoke(fake, "--model", "coolgit-ops", "--kind", "tools", "--deployment", "vabc")
    assert r.returncode == 0, r.stdout + r.stderr
    checks = [json.loads(line) for line in r.stdout.splitlines()]
    assert [c["check"] for c in checks] == ["models", "chat", "tools"]
    assert all(c["deployment"] == "vabc" for c in fakes.Handler.calls)


def test_smoke_fails_for_an_unknown_model(fake):
    assert smoke(fake, "--model", "nope").returncode == 1


def test_smoke_fails_with_a_bad_key(fake):
    assert smoke(fake, "--model", "coolgit-ops", key="wrong").returncode == 1


def test_smoke_refuses_plain_http_off_localhost():
    assert smoke("http://example.com", "--model", "x").returncode == 2


def test_connection_body_shape():
    b = connection_body.body("https://ftm-x.eastus2.inference.ml.azure.com/v1", ["coolgit-ops", "qwen38-27b"], "k")
    p = b["properties"]
    assert p["category"] == "ModelGateway" and p["authType"] == "ApiKey" and p["credentials"] == {"key": "k"}
    assert all(isinstance(v, str) for v in p["metadata"].values())  # the contract: strings only
    models = json.loads(p["metadata"]["models"])
    assert [m["name"] for m in models] == ["coolgit-ops", "qwen38-27b"]
    assert p["metadata"]["authHeaderFormat"] == "Bearer {api_key}"


@pytest.mark.parametrize("target", [
    "https://ftm-x.eastus2.inference.ml.azure.com",          # missing /v1
    "https://evil.example.com/v1",
    "http://ftm-x.eastus2.inference.ml.azure.com/v1",
])
def test_connection_body_rejects_other_targets(target):
    with pytest.raises(ValueError):
        connection_body.body(target, ["m"], "k")


def test_wham_shim_health_and_forwarding():
    sys.path.insert(0, str(ROOT / "shared/containers/wham"))
    import shim

    class Up(BaseHTTPRequestHandler):
        def log_message(self, *a):
            pass

        def do_POST(self):
            n = int(self.headers["Content-Length"])
            data = self.rfile.read(n)
            self.send_response(200)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

    up = ThreadingHTTPServer(("127.0.0.1", 0), Up)
    threading.Thread(target=up.serve_forever, daemon=True).start()
    port = up.server_address[1]
    front = ThreadingHTTPServer(("127.0.0.1", 0), shim.make_handler("127.0.0.1", port, 5))
    threading.Thread(target=front.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{front.server_address[1]}"

    assert urllib.request.urlopen(base + "/health").status == 200
    req = urllib.request.Request(base + "/new_job", data=b"payload", method="POST")
    assert urllib.request.urlopen(req).read() == b"payload"
    up.shutdown()
    up.server_close()
    with pytest.raises(urllib.error.HTTPError) as e:
        urllib.request.urlopen(base + "/health")
    assert e.value.code == 503
    front.shutdown()
