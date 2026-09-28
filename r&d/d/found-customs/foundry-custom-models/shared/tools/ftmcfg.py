#!/usr/bin/env python3
"""ftmcfg: load, validate, version and render one example's configuration.

Everything here is offline and deterministic: the same commit, config and
Hugging Face revision always produce the same model version, deployment
name and YAML. That is what makes `ftm plan` a real plan.

    ftmcfg.py env    EXAMPLE            KEY=VALUE lines for bash
    ftmcfg.py json   EXAMPLE            the same, as JSON
    ftmcfg.py render EXAMPLE OUTDIR     write Azure ML YAML into OUTDIR
    ftmcfg.py examples                  list example keys

Precedence (last wins): shared/config/site.env, the example's model.env,
site keys from the process environment, FTM_SET_<KEY> for OVERRIDABLE keys.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

EXAMPLES = {
    "gemma4": "examples/01-gemma4-agent/model.env",
    "qwen38": "examples/02-qwen38-harness/model.env",
    "muse": "examples/03-muse-glimmer/model.env",
    "wham": "examples/03-muse-glimmer/wham/model.env",
}

SKUS = (
    "Standard_NC24ads_A100_v4", "Standard_NC48ads_A100_v4", "Standard_NC96ads_A100_v4",
    "Standard_ND96amsr_A100_v4", "Standard_NC40ads_H100_v5", "Standard_NC80adis_H100_v5",
    "Standard_ND96isr_H100_v5",
)

NAME = r"[a-z][a-z0-9-]{1,30}[a-z0-9]"
HFREPO = r"[A-Za-z0-9][A-Za-z0-9._-]{0,95}/[A-Za-z0-9][A-Za-z0-9._-]{0,95}"
SHA = r"[0-9a-f]{40}"
ANY_SAFE = r"[^\n'\"\\`$]*"  # no quotes, newlines or shell/template metacharacters

FIELDS: dict[str, str] = {
    # site
    "AZURE_RESOURCE_GROUP": r"[A-Za-z0-9._()-]{1,90}",
    "AZURE_LOCATION": r"[a-z0-9]{3,30}",
    "AML_WORKSPACE": r"[A-Za-z0-9][A-Za-z0-9_-]{2,32}",
    "FOUNDRY_ACCOUNT": r"[A-Za-z0-9][A-Za-z0-9-]{1,63}",
    "FOUNDRY_PROJECT": r"[A-Za-z0-9][A-Za-z0-9_-]{1,63}",
    "FETCH_COMPUTE": r"[a-z][a-z0-9-]{1,23}",
    "KEY_VAULT_URL": r"(https://[a-z0-9-]{3,24}\.vault\.azure\.net/?)?",
    # example
    "EXAMPLE_TITLE": r"[A-Za-z0-9 .,:()/+-]{3,80}",
    "HF_REPO": HFREPO,
    "HF_REVISION": rf"({SHA}|main)",
    "HF_TOKEN_SECRET": r"([a-z0-9-]{1,127})?",
    "FETCH_INCLUDE": r"([A-Za-z0-9_.*/-]+(,[A-Za-z0-9_.*/-]+)*)?",
    "AML_MODEL_NAME": NAME,
    "SERVED_NAME": NAME,
    "ENDPOINT_NAME": NAME,
    "CONNECTION_NAME": NAME,
    "INSTANCE_TYPE": "(" + "|".join(SKUS) + ")",
    "INSTANCE_COUNT": r"[1-4]",
    "MAX_CONCURRENT": r"([1-9]|[1-5][0-9]|6[0-4])",
    "VLLM_ARGS": r"[A-Za-z0-9_.,:={}\[\]\"/ -]{0,400}",
    "SERVE_ENV": r"(ftm-serve|ftm-wham)",
    "SCORE_PORT": r"[0-9]{2,5}",
    "SMOKE_KIND": r"(chat|tools|http)",
    "SMOKE_PATH": r"(/[A-Za-z0-9_./?=-]*)?",
    "TRAIN_ENABLED": r"(true|false)",
    "TRAIN_MODE": r"(merge|adapter)",
    "TRAIN_DATA": r"([A-Za-z0-9_./-]+\.jsonl)?",
    "TRAIN_COMPUTE": r"([a-z][a-z0-9-]{1,23})?",
    "ADAPTER_NAME": r"([a-z][a-z0-9-]{1,30})?",
    "EPOCHS": r"[0-9]{1,2}(\.[0-9])?",
    "LORA_R": r"(8|16|32|64)",
    "MAX_SEQ_LEN": r"[0-9]{3,6}",
    "LICENSE_NOTE": ANY_SAFE,
}

SITE = ("AZURE_RESOURCE_GROUP", "AZURE_LOCATION", "AML_WORKSPACE", "FOUNDRY_ACCOUNT",
        "FOUNDRY_PROJECT", "FETCH_COMPUTE", "KEY_VAULT_URL")
OVERRIDABLE = ("HF_REPO", "HF_REVISION", "INSTANCE_TYPE", "INSTANCE_COUNT", "TRAIN_ENABLED", "EPOCHS")
OPTIONAL = {"HF_TOKEN_SECRET", "FETCH_INCLUDE", "TRAIN_DATA", "TRAIN_COMPUTE", "ADAPTER_NAME",
            "SMOKE_PATH", "KEY_VAULT_URL", "LICENSE_NOTE", "VLLM_ARGS"}
DEFAULTS = {"SERVE_ENV": "ftm-serve", "SCORE_PORT": "8000", "SMOKE_KIND": "chat", "MAX_CONCURRENT": "16",
            "TRAIN_ENABLED": "false", "TRAIN_MODE": "merge", "EPOCHS": "2", "LORA_R": "16",
            "MAX_SEQ_LEN": "2048", "INSTANCE_COUNT": "1"}

CONTEXTS = {  # Azure ML environment name -> build context
    "ftm-serve": "shared/containers/serve",
    "ftm-train": "shared/containers/train",
    "ftm-wham": "shared/containers/wham",
}
TRAINER_FILES = ("shared/train/sft_lora.py", "shared/train/sftdata.py", "shared/train/ftmlib.py")


class ConfigError(ValueError):
    pass


def parse_env(text: str, where: str = "") -> dict[str, str]:
    """KEY=VALUE, '#' comments, optional single quotes. No expansion, no export."""
    out: dict[str, str] = {}
    for n, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        m = re.fullmatch(r"([A-Z][A-Z0-9_]*)=(.*)", line)
        if not m:
            raise ConfigError(f"{where}:{n}: expected KEY=VALUE")
        k, v = m.groups()
        if len(v) >= 2 and v[0] == v[-1] == "'":
            v = v[1:-1]
        out[k] = v
    return out


def load(example: str, environ: dict[str, str] | None = None, root: Path = ROOT) -> dict[str, str]:
    environ = dict(os.environ if environ is None else environ)
    if example not in EXAMPLES:
        raise ConfigError(f"unknown example {example!r}; one of {', '.join(EXAMPLES)}")
    path = root / EXAMPLES[example]
    cfg = dict(DEFAULTS)
    cfg.update(parse_env((root / "shared/config/site.env").read_text(), "site.env"))
    cfg.update(parse_env(path.read_text(), str(path.relative_to(root))))
    for k in SITE:
        if environ.get(k):
            cfg[k] = environ[k]
    for k in OVERRIDABLE:
        v = environ.get(f"FTM_SET_{k}", "")
        if v:
            cfg[k] = v
    unknown = set(cfg) - set(FIELDS)
    if unknown:
        raise ConfigError(f"{example}: unknown keys {sorted(unknown)}")
    for k, pat in FIELDS.items():
        v = cfg.get(k, "")
        if v == "" and k in OPTIONAL:
            cfg[k] = ""
            continue
        if not re.fullmatch(pat, v):
            raise ConfigError(f"{example}: {k}={v!r} is not valid")
    if cfg["TRAIN_ENABLED"] == "true":
        for k in ("TRAIN_DATA", "TRAIN_COMPUTE"):
            if not cfg[k]:
                raise ConfigError(f"{example}: {k} is required when TRAIN_ENABLED=true")
        if cfg["TRAIN_MODE"] == "adapter" and not cfg["ADAPTER_NAME"]:
            raise ConfigError(f"{example}: ADAPTER_NAME is required for TRAIN_MODE=adapter")
        if not (path.parent / cfg["TRAIN_DATA"]).is_file():
            raise ConfigError(f"{example}: TRAIN_DATA {cfg['TRAIN_DATA']} not found")
    cfg["EXAMPLE"] = example
    cfg["EXAMPLE_DIR"] = str(path.parent.relative_to(root))
    derive(cfg, root)
    return cfg


def tree_hash(root: Path, rels: list[str]) -> str:
    h = hashlib.sha256()
    for rel in rels:
        p = root / rel
        files = sorted(x for x in p.rglob("*") if x.is_file()) if p.is_dir() else [p]
        for f in files:
            h.update(f.relative_to(root).as_posix().encode() + b"\0")
            h.update(f.read_bytes() + b"\0")
    return h.hexdigest()


def derive(cfg: dict[str, str], root: Path) -> None:
    """Names and versions that follow from the inputs. Nothing random."""
    resolved = cfg["HF_REVISION"] != "main"
    cfg["RESOLVED"] = "true" if resolved else "false"
    rev = cfg["HF_REVISION"][:12] if resolved else "unresolved"
    cfg["BASE_MODEL_NAME"] = f"{cfg['AML_MODEL_NAME']}-base"
    cfg["BASE_MODEL_VERSION"] = rev
    # Environment versions are content hashes of their build context, so a
    # changed Dockerfile (including its vLLM base tag) is a new version.
    for env, ctx in CONTEXTS.items():
        key = env.upper().replace("-", "_").replace("FTM_", "") + "_ENV_VERSION"
        cfg[key] = tree_hash(root, [ctx])[:12]
    if cfg["TRAIN_ENABLED"] == "true":
        data = root / cfg["EXAMPLE_DIR"] / cfg["TRAIN_DATA"]
        h = hashlib.sha256()
        for part in (rev, cfg["HF_REPO"], cfg["TRAIN_MODE"], cfg["ADAPTER_NAME"], cfg["EPOCHS"], cfg["LORA_R"],
                     cfg["MAX_SEQ_LEN"], cfg["TRAIN_ENV_VERSION"], tree_hash(root, list(TRAINER_FILES)),
                     hashlib.sha256(data.read_bytes()).hexdigest()):
            h.update(part.encode() + b"\0")
        cfg["SERVE_MODEL_NAME"] = cfg["AML_MODEL_NAME"]
        cfg["SERVE_MODEL_VERSION"] = h.hexdigest()[:12] if resolved else "unresolved"
        cfg["TRAIN_DATA_PATH"] = f"{cfg['EXAMPLE_DIR']}/{cfg['TRAIN_DATA']}"
    else:
        cfg["SERVE_MODEL_NAME"] = cfg["BASE_MODEL_NAME"]
        cfg["SERVE_MODEL_VERSION"] = rev
        cfg["TRAIN_DATA_PATH"] = ""
    # What clients send as "model": the adapter when one is served, else the base.
    adapter = cfg["TRAIN_ENABLED"] == "true" and cfg["TRAIN_MODE"] == "adapter"
    cfg["CLIENT_MODEL"] = cfg["ADAPTER_NAME"] if adapter else cfg["SERVED_NAME"]
    serve_env_key = cfg["SERVE_ENV"].upper().replace("-", "_").replace("FTM_", "") + "_ENV_VERSION"
    cfg["SERVE_ENV_VERSION_USED"] = cfg[serve_env_key]
    # Deployment names: letter first, <=32 chars. One per served model version.
    cfg["DEPLOYMENT_NAME"] = "v" + (cfg["SERVE_MODEL_VERSION"][:10] if resolved else "unresolved")
    # Quoted: the job's shell would otherwise glob-expand the patterns.
    cfg["FETCH_INCLUDE_ARGS"] = " ".join(f'--include "{g}"' for g in cfg["FETCH_INCLUDE"].split(",") if g)


def render(example: str, outdir: Path, environ: dict[str, str] | None = None, root: Path = ROOT) -> list[Path]:
    cfg = load(example, environ, root)
    outdir = outdir.resolve()
    outdir.mkdir(parents=True, exist_ok=True)
    rel = lambda p: os.path.relpath(root / p, outdir)  # noqa: E731
    ctx = dict(cfg)
    ctx.update(
        CODE_DIR=rel("shared/train"),
        SERVE_CONTEXT=rel(CONTEXTS["ftm-serve"]),
        TRAIN_CONTEXT=rel(CONTEXTS["ftm-train"]),
        WHAM_CONTEXT=rel(CONTEXTS["ftm-wham"]),
        TRAIN_DATA_REL=rel(cfg["TRAIN_DATA_PATH"]) if cfg["TRAIN_DATA_PATH"] else "",
        GIT_SHA=environ_get(environ, "GITHUB_SHA", "local"),
        HF_TOKEN_ENV=_token_env(cfg),
    )
    names = ["endpoint", "deployment", "fetch-job", "environment-train", f"environment-{cfg['SERVE_ENV'][4:]}"]
    if cfg["TRAIN_ENABLED"] == "true":
        names.append("train-job")
    written = []
    for name in names:
        src = root / "shared/templates" / f"{name}.yml.tmpl"
        dst = outdir / f"{name}.yml"
        dst.write_text(fill(src.read_text(), ctx, str(src.name)))
        written.append(dst)
    (outdir / "config.json").write_text(json.dumps(cfg, indent=2, sort_keys=True) + "\n")
    written.append(outdir / "config.json")
    return written


def environ_get(environ, k, default):
    return (os.environ if environ is None else environ).get(k, default)


def _token_env(cfg: dict[str, str]) -> str:
    if not (cfg["HF_TOKEN_SECRET"] and cfg["KEY_VAULT_URL"]):
        return "{}"
    return json.dumps({"FTM_KEY_VAULT_URL": cfg["KEY_VAULT_URL"], "FTM_HF_TOKEN_SECRET": cfg["HF_TOKEN_SECRET"]})


def fill(text: str, ctx: dict[str, str], where: str = "") -> str:
    """Replace @{KEY}. Unknown keys and values that could break YAML fail."""
    def sub(m: re.Match) -> str:
        k = m.group(1)
        if k not in ctx:
            raise ConfigError(f"{where}: no value for @{{{k}}}")
        v = str(ctx[k])
        if "\n" in v or "'" in v:
            raise ConfigError(f"{where}: value for {k} contains a newline or single quote")
        return v
    out = re.sub(r"@\{([A-Z0-9_]+)\}", sub, text)
    if "@{" in out:
        raise ConfigError(f"{where}: malformed placeholder")
    return out


def main(argv: list[str]) -> int:
    if not argv or argv[0] in {"-h", "--help"}:
        print(__doc__)
        return 0
    cmd, rest = argv[0], argv[1:]
    try:
        if cmd == "examples":
            print("\n".join(EXAMPLES))
        elif cmd == "env" and len(rest) == 1:
            for k, v in sorted(load(rest[0]).items()):
                print(f"{k}={v}")
        elif cmd == "json" and len(rest) == 1:
            print(json.dumps(load(rest[0]), indent=2, sort_keys=True))
        elif cmd == "render" and len(rest) == 2:
            for p in render(rest[0], Path(rest[1])):
                print(p)
        else:
            print(__doc__, file=sys.stderr)
            return 2
    except ConfigError as e:
        print(f"ftmcfg: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
