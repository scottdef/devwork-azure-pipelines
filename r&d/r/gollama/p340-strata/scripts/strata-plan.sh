#!/usr/bin/env bash
# strata-plan.sh: ask Strata what it would do on this machine, and keep the
# answer. --plan downloads nothing and changes nothing.
# The third probe is the Flash-Next Coder, which Strata documents for 32 GB of
# RAM with a 12 to 16 GB card; its catalog name was not confirmed, and Strata
# answers an unknown name with the closest match.
set -euo pipefail

command -v strata >/dev/null || { echo "strata is not installed; see the guide, section 4" >&2; exit 1; }
out="sim/strata-plan-$(date +%Y%m%d-%H%M%S).txt"
mkdir -p sim
{
  echo "# $(strata --version 2>/dev/null | head -n1)   $(date -Is)"
  nvidia-smi --query-gpu=name,driver_version,memory.total,memory.used --format=csv,noheader
  free -g | sed -n '1,2p'
  for args in \
    "--family qwen36 --model UD-Q4_K_XL --context 8192 --parallel 1 --mtp off --vision off" \
    "--family qwen36 --model UD-Q4_K_XL --context 8192 --parallel 1 --mtp on  --vision off" \
    "--family qwen   --model coder      --context 8192 --parallel 1 --mtp off --vision off"
  do
    echo; echo "## strata launch --plan $args"
    # shellcheck disable=SC2086
    strata launch --plan $args 2>&1 || echo "(exit $?)"
  done
} | tee "$out"
echo "saved to $out"
