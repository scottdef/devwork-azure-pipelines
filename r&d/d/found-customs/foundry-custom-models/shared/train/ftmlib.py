"""Helpers shared by fetch.py and sft_lora.py. Standard library plus the
Azure SDK, imported lazily so tests can run without either."""

from __future__ import annotations

import hashlib
import os
from pathlib import Path

WEIGHT_SUFFIXES = (".safetensors", ".bin", ".pt", ".pth", ".ckpt", ".gguf")


def hf_token() -> str | None:
    """HF_TOKEN from the environment, else from Key Vault through the
    compute's managed identity, else None (public, ungated repositories)."""
    if tok := os.environ.get("HF_TOKEN"):
        return tok
    vault, name = os.environ.get("FTM_KEY_VAULT_URL"), os.environ.get("FTM_HF_TOKEN_SECRET")
    if not (vault and name):
        return None
    from azure.identity import DefaultAzureCredential
    from azure.keyvault.secrets import SecretClient

    cred = DefaultAzureCredential(managed_identity_client_id=os.environ.get("DEFAULT_IDENTITY_CLIENT_ID"))
    return SecretClient(vault_url=vault, credential=cred).get_secret(name).value


def sha256_file(path: Path, bufsize: int = 1 << 24) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        while chunk := f.read(bufsize):
            h.update(chunk)
    return h.hexdigest()


def sha256_tree(root: Path, exclude: set[str] | None = None) -> list[tuple[str, str]]:
    """[(relative path, sha256)] for every file under root, sorted."""
    exclude = exclude or set()
    out = []
    for f in sorted(p for p in root.rglob("*") if p.is_file()):
        rel = f.relative_to(root).as_posix()
        if rel in exclude or rel.startswith(".cache/"):
            continue
        out.append((rel, sha256_file(f)))
    return out


def is_weight(name: str) -> bool:
    return name.endswith(WEIGHT_SUFFIXES) or name.endswith(".index.json")
