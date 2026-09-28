import pytest
import yaml

import issueform
from conftest import ROOT, SHA

BODY = """### Example

qwen38 — Qwen3.8-27B + coolgit-ops adapter, Agent Framework harness

### Action

deploy

### Hugging Face repository override

_No response_

### Hugging Face revision

{rev}

### Fine-tune

yes

### Instance type

Standard_NC40ads_H100_v5

### Instance count

1

### Reason

{reason}
"""


def test_valid_request():
    vals, errors = issueform.validate(issueform.parse(BODY.format(rev=SHA, reason="new adapter data for Q4")))
    assert not errors
    assert vals["example"] == "qwen38" and vals["train"] == "yes" and vals["hf_repo"] == ""


@pytest.mark.parametrize("rev,reason,bad", [
    ("main; curl evil", "a fine reason", "hf_revision"),
    (SHA, "short", "reason"),
    (SHA, "has a `backtick` in it", "reason"),
    (SHA, "has a $(command) in it", "reason"),
])
def test_invalid_values_are_reported(rev, reason, bad):
    _, errors = issueform.validate(issueform.parse(BODY.format(rev=rev, reason=reason)))
    assert any(e.startswith(bad) for e in errors), errors


def test_missing_example_is_an_error():
    _, errors = issueform.validate(issueform.parse("### Action\n\ndeploy\n"))
    assert "example is required" in errors


@pytest.mark.parametrize("comment,cmd", [
    ("/approve", "approve"), ("  /APPROVE \nlooks good", "approve"), ("/plan", "plan"),
    ("/cancel", "cancel"), ("please /approve", "none"), ("/deploy", "none"), ("", "none"),
])
def test_commands(comment, cmd):
    assert issueform.command(comment) == cmd


def test_form_labels_match_parser():
    form = yaml.safe_load((ROOT / ".github/ISSUE_TEMPLATE/model-request.yml").read_text())
    labels = {f["attributes"]["label"] for f in form["body"] if f["type"] != "markdown"}
    assert labels == set(issueform.LABELS)
