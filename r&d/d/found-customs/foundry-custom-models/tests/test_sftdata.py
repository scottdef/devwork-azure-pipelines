import json

import pytest

import sftdata
from conftest import ROOT


class Tok:
    """Chat template: <r>content</r> per turn; generation prompt '<assistant>'."""

    def apply_chat_template(self, msgs, tokenize, add_generation_prompt):
        s = "".join(f"<{m['role']}>{m['content']}</{m['role']}>" for m in msgs)
        if add_generation_prompt:
            s += "<assistant>"
        return [ord(c) for c in s]


MSGS = [{"role": "system", "content": "s"}, {"role": "user", "content": "q"}, {"role": "assistant", "content": "answer"}]


def test_loss_is_on_the_answer_only():
    e = sftdata.encode(Tok(), MSGS, 1000)
    prompt_len = len(Tok().apply_chat_template(MSGS[:-1], True, True))
    assert all(x == sftdata.IGNORE for x in e["labels"][:prompt_len])
    tail = "".join(chr(x) for x in e["labels"][prompt_len:])
    assert tail == "answer</assistant>"


def test_prompt_longer_than_limit_is_dropped():
    assert sftdata.encode(Tok(), MSGS, 5) is None


def test_template_without_prefix_property_trains_on_everything():
    class Rewriting(Tok):
        def apply_chat_template(self, msgs, tokenize, add_generation_prompt):
            ids = super().apply_chat_template(msgs, tokenize, add_generation_prompt)
            return ids if add_generation_prompt else [0] + ids

    e = sftdata.encode(Rewriting(), MSGS, 1000)
    assert sftdata.IGNORE not in e["labels"]


def test_collate_pads_right():
    b = sftdata.collate([{"input_ids": [1, 2, 3], "attention_mask": [1, 1, 1], "labels": [-100, 2, 3]},
                         {"input_ids": [4], "attention_mask": [1], "labels": [4]}], pad_id=0)
    assert b["input_ids"][1] == [4, 0, 0] and b["attention_mask"][1] == [1, 0, 0] and b["labels"][1] == [4, -100, -100]


@pytest.mark.parametrize("row", [
    {"messages": [{"role": "user", "content": "q"}]},
    {"messages": [{"role": "user", "content": "q"}, {"role": "user", "content": "q2"}]},
    {"messages": [{"role": "robot", "content": "q"}, {"role": "assistant", "content": "a"}]},
])
def test_bad_rows_are_rejected(row, tmp_path):
    p = tmp_path / "d.jsonl"
    p.write_text(json.dumps(row) + "\n")
    with pytest.raises(ValueError):
        sftdata.read_jsonl(p)


@pytest.mark.parametrize("path", ["examples/01-gemma4-agent/data/train.jsonl", "examples/02-qwen38-harness/data/train.jsonl"])
def test_shipped_datasets_are_valid_and_current(path, tmp_path):
    rows = sftdata.read_jsonl(ROOT / path)
    assert len(rows) >= 20
    # The committed file is exactly what build.py produces.
    import runpy
    import shutil
    d = tmp_path / "data"
    shutil.copytree((ROOT / path).parent, d)
    (d / "train.jsonl").unlink()
    runpy.run_path(str(d / "build.py"), run_name="__main__")
    assert (d / "train.jsonl").read_text() == (ROOT / path).read_text()
