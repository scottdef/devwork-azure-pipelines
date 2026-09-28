"""The Agent Framework harness end to end against the fake endpoint.
Skipped when agent-framework is not installed (pip install -r
examples/02-qwen38-harness/harness/requirements.txt)."""

import json
import os
import subprocess
import sys

import pytest

import fakes
from conftest import ROOT

pytest.importorskip("agent_framework")
RUN = ROOT / "examples/02-qwen38-harness/harness/run.py"


@pytest.fixture(scope="module")
def env():
    srv, url = fakes.serve()
    yield {**os.environ, "FTM_BASE_URL": url, "FTM_KEY": fakes.KEY}
    srv.shutdown()


def test_eval_meets_the_bar(env):
    r = subprocess.run([sys.executable, RUN, "eval"], env=env, capture_output=True, text=True, timeout=120)
    assert r.returncode == 0, r.stdout + r.stderr
    summary = json.loads(r.stdout.splitlines()[-1])
    assert summary["accuracy"] >= summary["min_accuracy"]


def test_workflow_runs_tools_then_decides(env):
    fakes.Handler.calls.clear()
    r = subprocess.run([sys.executable, RUN, "workflow", "research-scratch.yml"], env=env,
                       capture_output=True, text=True, timeout=120)
    assert r.returncode == 0, r.stdout + r.stderr
    assert json.loads(r.stdout)["decision"] == "reject"
    models = [c["body"]["model"] for c in fakes.Handler.calls]
    assert "qwen38-27b" in models and "coolgit-ops" in models  # base investigated, adapter decided
