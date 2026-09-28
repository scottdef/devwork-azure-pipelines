import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
for p in ("shared/tools", "shared/train"):
    sys.path.insert(0, str(ROOT / p))

SHA = "0123456789abcdef0123456789abcdef01234567"
