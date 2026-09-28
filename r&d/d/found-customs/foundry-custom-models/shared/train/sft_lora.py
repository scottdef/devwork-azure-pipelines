"""sft_lora.py: LoRA supervised fine-tuning for Gemma 4, Qwen3.8 and
Muse Glimmer, then either merge into full weights or keep a servable
base + adapter bundle.

All three families load through AutoModelForImageTextToText and keep their
text decoder under `language_model`, so one target-module pattern adapts the
language model and leaves the vision (and audio) towers untouched.

    python sft_lora.py --base BASE_DIR --data train.jsonl --out OUT --mode merge
    python sft_lora.py ... --mode adapter --adapter-name coolgit-ops

Output layouts (both understood by shared/containers/serve/serve.sh):
    merge    OUT/config.json, OUT/*.safetensors, tokenizer + processor files
    adapter  OUT/base/<copy of base>, OUT/adapters/<name>/adapter_*.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import shutil
import sys
import time
from pathlib import Path

from ftmlib import is_weight
from sftdata import collate, encode_all, read_jsonl

TARGET = r".*language_model.*\.(q_proj|k_proj|v_proj|o_proj|gate_proj|up_proj|down_proj)$"


def args(argv: list[str] | None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--base", required=True, help="directory with config.json (the mounted base model)")
    p.add_argument("--data", required=True, help="chat JSONL: {'messages': [...]}")
    p.add_argument("--out", required=True)
    p.add_argument("--mode", choices=("merge", "adapter"), default="merge")
    p.add_argument("--adapter-name", default="custom")
    p.add_argument("--epochs", type=float, default=2.0)
    p.add_argument("--lr", type=float, default=1e-4)
    p.add_argument("--lora-r", type=int, default=16, choices=(8, 16, 32, 64))
    p.add_argument("--lora-alpha", type=int, default=0, help="default 2*r")
    p.add_argument("--max-seq-len", type=int, default=2048)
    p.add_argument("--batch", type=int, default=1)
    p.add_argument("--grad-accum", type=int, default=8)
    p.add_argument("--seed", type=int, default=17)
    return p.parse_args(argv)


def find_base(root: Path) -> Path:
    """The mount may add a directory level; take the shallowest config.json."""
    hits = sorted(root.rglob("config.json"), key=lambda p: len(p.parts))
    if not hits:
        raise SystemExit(f"sft_lora.py: no config.json under {root}")
    return hits[0].parent


def copy_non_weights(src: Path, dst: Path) -> None:
    """Tokenizer, chat template, processor and generation config travel with
    the weights; the vLLM server reads them from the same directory."""
    for f in src.iterdir():
        if f.is_file() and not is_weight(f.name) and f.name not in {"SHA256SUMS", "ftm-fetch.json"}:
            shutil.copy2(f, dst / f.name)


def main(argv: list[str] | None = None) -> int:
    a = args(argv)
    import torch
    from peft import LoraConfig, get_peft_model
    from transformers import AutoModelForImageTextToText, AutoTokenizer, Trainer, TrainingArguments

    torch.manual_seed(a.seed)
    base = find_base(Path(a.base))
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    t0 = time.time()

    tok = AutoTokenizer.from_pretrained(base)
    if tok.pad_token_id is None:
        tok.pad_token = tok.eos_token

    rows = read_jsonl(a.data)
    train, dropped = encode_all(tok, rows, a.max_seq_len)
    if not train:
        raise SystemExit("sft_lora.py: every example was longer than --max-seq-len")
    print(f"sft_lora.py: {len(train)} examples, {dropped} dropped (prompt longer than {a.max_seq_len})")

    model = AutoModelForImageTextToText.from_pretrained(base, dtype=torch.bfloat16, device_map="auto")
    model.config.use_cache = False
    model.gradient_checkpointing_enable(gradient_checkpointing_kwargs={"use_reentrant": False})
    model.enable_input_require_grads()
    lora = LoraConfig(
        r=a.lora_r,
        lora_alpha=a.lora_alpha or 2 * a.lora_r,
        lora_dropout=0.05,
        target_modules=TARGET,
        task_type="CAUSAL_LM",
    )
    model = get_peft_model(model, lora)
    model.print_trainable_parameters()

    def collator(batch):
        c = collate(batch, tok.pad_token_id)
        return {k: torch.tensor(v) for k, v in c.items()}

    trainer = Trainer(
        model=model,
        args=TrainingArguments(
            output_dir=str(out / "_checkpoints"),
            num_train_epochs=a.epochs,
            learning_rate=a.lr,
            per_device_train_batch_size=a.batch,
            gradient_accumulation_steps=a.grad_accum,
            lr_scheduler_type="cosine",
            warmup_ratio=0.05,
            bf16=True,
            logging_steps=5,
            save_strategy="no",
            report_to=[],
            remove_unused_columns=False,
            seed=a.seed,
        ),
        train_dataset=train,
        data_collator=collator,
    )
    result = trainer.train()
    shutil.rmtree(out / "_checkpoints", ignore_errors=True)

    if a.mode == "merge":
        merged = model.merge_and_unload()
        merged.config.use_cache = True
        copy_non_weights(base, out)
        merged.save_pretrained(out, safe_serialization=True, max_shard_size="5GB")
    else:
        shutil.copytree(base, out / "base", dirs_exist_ok=True)
        adir = out / "adapters" / a.adapter_name
        model.save_pretrained(adir)
        tok.save_pretrained(adir)

    manifest = {
        "mode": a.mode,
        "adapter_name": a.adapter_name if a.mode == "adapter" else None,
        "base": str(base),
        "base_fetch": _read_json(base / "ftm-fetch.json"),
        "data_sha256": hashlib.sha256(Path(a.data).read_bytes()).hexdigest(),
        "examples": len(train),
        "dropped": dropped,
        "hyperparameters": {k: getattr(a, k) for k in
                            ("epochs", "lr", "lora_r", "lora_alpha", "max_seq_len", "batch", "grad_accum", "seed")},
        "target_modules": TARGET,
        "train_loss": round(float(result.training_loss), 4),
        "git_sha": os.environ.get("GIT_SHA", ""),
        "seconds": round(time.time() - t0, 1),
    }
    (out / "ftm-train.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps(manifest))
    return 0


def _read_json(p: Path):
    try:
        return json.loads(p.read_text())
    except (OSError, ValueError):
        return None


if __name__ == "__main__":
    sys.exit(main())
