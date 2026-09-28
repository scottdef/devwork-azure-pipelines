"""Drive shared/bin/ftm with a stub az on PATH: the whole deploy sequence in
dry-run mode, and the plan output, without Azure."""

import os
import stat
import subprocess

from conftest import ROOT, SHA

STUB = r"""#!/usr/bin/env bash
# Records every call; answers the few reads ftm makes. Nothing exists yet.
printf '%s\n' "$*" >> "$AZ_LOG"
case "$*" in
  "account show"*) echo 00000000-0000-0000-0000-000000000000 ;;
  *"online-endpoint show"*"--query traffic"*) echo '{}' ;;
  *"online-deployment list"*) : ;;
  *" show "*|*" show") exit 1 ;;
esac
"""


def run_ftm(tmp_path, *args):
    stub = tmp_path / "az"
    stub.write_text(STUB)
    stub.chmod(stub.stat().st_mode | stat.S_IEXEC)
    env = {**os.environ, "PATH": f"{tmp_path}:{os.environ['PATH']}", "AZ_LOG": str(tmp_path / "az.log"),
           "FTM_SET_HF_REVISION": SHA, "FTM_DRY_RUN": "1"}
    env.pop("GITHUB_OUTPUT", None)
    return subprocess.run([ROOT / "shared/bin/ftm", *args], env=env, capture_output=True, text=True)


def test_plan_is_a_table(tmp_path):
    r = run_ftm(tmp_path, "plan", "gemma4")
    assert r.returncode == 0, r.stderr
    assert "| Clients send model | `gemma4-coolgit` |" in r.stdout


def test_all_in_dry_run_prints_the_full_sequence(tmp_path):
    r = run_ftm(tmp_path, "all", "qwen38")
    assert r.returncode == 0, r.stderr
    printed = [line for line in r.stderr.splitlines() if line.startswith("+ az ")]
    order = ["ml environment create", "ml job create", "ml model create", "ml job create", "ml model create",
             "ml online-endpoint create", "ml online-deployment create", "ml online-endpoint update"]
    it = iter(printed)
    for want in order:
        assert any(want in line for line in it), f"{want} missing or out of order in:\n" + "\n".join(printed)
    assert any("--all-traffic" in line for line in printed)          # first deployment takes traffic
    assert any("--traffic v" in line for line in printed)
    assert "would PUT ModelGateway connection ftm-qwen38" in r.stderr
    # Dry run: the only az calls actually made are reads.
    made = (tmp_path / "az.log").read_text().splitlines()
    assert not any(" create" in c or " update" in c or " delete" in c for c in made), made


def test_http_examples_get_no_foundry_connection(tmp_path):
    r = run_ftm(tmp_path, "connect", "wham")
    assert r.returncode == 0 and "no Foundry connection" in r.stderr


def test_unknown_command_fails(tmp_path):
    assert run_ftm(tmp_path, "explode", "gemma4").returncode != 0
