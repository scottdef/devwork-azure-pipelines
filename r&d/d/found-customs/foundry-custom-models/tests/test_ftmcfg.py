import json
import shutil

import pytest
import yaml

import ftmcfg
from conftest import ROOT, SHA

ENV = {"FTM_SET_HF_REVISION": SHA}


@pytest.mark.parametrize("ex", list(ftmcfg.EXAMPLES))
def test_every_example_loads_renders_and_parses(ex, tmp_path):
    files = ftmcfg.render(ex, tmp_path, ENV)
    for f in files:
        text = f.read_text()
        assert "@{" not in text
        if f.suffix == ".yml":
            assert isinstance(yaml.safe_load(text), dict), f.name
    cfg = json.loads((tmp_path / "config.json").read_text())
    assert cfg["RESOLVED"] == "true"
    assert cfg["DEPLOYMENT_NAME"].startswith("v") and len(cfg["DEPLOYMENT_NAME"]) == 11


def test_versions_are_deterministic_and_content_addressed(tmp_path):
    a = ftmcfg.load("qwen38", ENV)
    assert a == ftmcfg.load("qwen38", ENV)
    # Change the training data in a copy of the tree: the served version must move.
    root = tmp_path / "repo"
    shutil.copytree(ROOT, root, ignore=shutil.ignore_patterns(".git", ".ftm", "records"))
    data = root / "examples/02-qwen38-harness/data/train.jsonl"
    data.write_text(data.read_text() + data.read_text().splitlines()[0] + "\n")
    b = ftmcfg.load("qwen38", ENV, root=root)
    assert b["SERVE_MODEL_VERSION"] != a["SERVE_MODEL_VERSION"]
    assert b["BASE_MODEL_VERSION"] == a["BASE_MODEL_VERSION"]  # base weights unchanged


def test_unpinned_revision_is_marked_unresolved():
    c = ftmcfg.load("gemma4", {})
    assert c["RESOLVED"] == "false" and c["SERVE_MODEL_VERSION"] == "unresolved"


def test_adapter_example_serves_the_adapter_name():
    c = ftmcfg.load("qwen38", ENV)
    assert c["TRAIN_MODE"] == "adapter" and c["CLIENT_MODEL"] == "coolgit-ops"
    assert ftmcfg.load("gemma4", ENV)["CLIENT_MODEL"] == "gemma4-coolgit"


@pytest.mark.parametrize("key,value", [
    ("FTM_SET_INSTANCE_TYPE", "Standard_D2s_v3"),
    ("FTM_SET_HF_REPO", "evil/repo; rm -rf /"),
    ("FTM_SET_HF_REVISION", "deadbeef"),
    ("FTM_SET_INSTANCE_COUNT", "9"),
    ("FTM_SET_TRAIN_ENABLED", "maybe"),
])
def test_overrides_are_validated(key, value):
    with pytest.raises(ftmcfg.ConfigError):
        ftmcfg.load("gemma4", {**ENV, key: value})


def test_only_listed_keys_are_overridable():
    c = ftmcfg.load("gemma4", {**ENV, "FTM_SET_ENDPOINT_NAME": "someone-else"})
    assert c["ENDPOINT_NAME"] == "ftm-gemma4-coolgit"


def test_fill_rejects_values_that_could_break_yaml():
    with pytest.raises(ftmcfg.ConfigError):
        ftmcfg.fill("a: '@{X}'", {"X": "it's"})
    with pytest.raises(ftmcfg.ConfigError):
        ftmcfg.fill("a: @{X}", {"X": "1\nb: 2"})
    with pytest.raises(ftmcfg.ConfigError):
        ftmcfg.fill("a: @{MISSING}", {})


def test_fetch_globs_are_quoted_for_the_job_shell(tmp_path):
    ftmcfg.render("wham", tmp_path, ENV)
    cmd = yaml.safe_load((tmp_path / "fetch-job.yml").read_text())["command"]
    assert '--include "*.py"' in cmd and "--include *.py" not in cmd


def test_training_job_points_at_the_example_data(tmp_path):
    ftmcfg.render("gemma4", tmp_path, ENV)
    job = yaml.safe_load((tmp_path / "train-job.yml").read_text())
    assert (tmp_path / job["inputs"]["data"]["path"]).resolve() == ROOT / "examples/01-gemma4-agent/data/train.jsonl"
    assert (tmp_path / job["code"]).resolve() == ROOT / "shared/train"
    assert job["inputs"]["base"]["path"].startswith("azureml:gemma4-e4b-coolgit-base:")


def test_parse_env_rejects_garbage():
    with pytest.raises(ftmcfg.ConfigError):
        ftmcfg.parse_env("export FOO=bar")
    assert ftmcfg.parse_env("# c\nA='x y'\n\nB=2") == {"A": "x y", "B": "2"}


def test_issue_form_and_config_agree_on_skus():
    form = yaml.safe_load((ROOT / ".github/ISSUE_TEMPLATE/model-request.yml").read_text())
    it = next(f for f in form["body"] if f.get("id") == "instance_type")
    assert tuple(it["attributes"]["options"][1:]) == ftmcfg.SKUS
    for wf in ("ex1-gemma4.yml", "ex2-qwen38.yml", "ex3-muse.yml"):
        on = yaml.safe_load((ROOT / ".github/workflows" / wf).read_text())[True]
        opts = on["workflow_dispatch"]["inputs"]["instance_type"]["options"]
        assert tuple(opts[1:]) == ftmcfg.SKUS, wf
