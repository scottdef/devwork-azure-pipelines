#!/usr/bin/env bash
# serve.sh: find the model under the Azure ML mount and exec vllm serve.
#
# Two layouts are accepted, whatever depth the mount adds above them:
#   <dir>/config.json                          full (merged) weights
#   <dir>/base/config.json + <dir>/adapters/*/  base weights plus LoRA adapters
#
# Environment:
#   SERVED_MODEL_NAME  required; the name clients put in "model"
#   MODEL_ROOT         mount point (default /models; falls back to AZUREML_MODEL_DIR)
#   VLLM_ARGS          extra flags, split on whitespace (values must not contain spaces)
#   VLLM_PORT          default 8000
set -euo pipefail

root=${MODEL_ROOT:-${AZUREML_MODEL_DIR:-/models}}
served=${SERVED_MODEL_NAME:?SERVED_MODEL_NAME is required}

# Shallowest config.json wins; adapter_config.json never matches.
dir=$(find -L "$root" -maxdepth 5 -name config.json -printf '%d %h\n' 2>/dev/null |
  sort -n | head -1 | cut -d' ' -f2-)
if [[ -z "$dir" ]]; then
  echo "serve.sh: no config.json under $root" >&2
  exit 1
fi

args=(serve "$dir" --served-model-name "$served" --host 0.0.0.0 --port "${VLLM_PORT:-8000}")

if [[ "$(basename "$dir")" == base ]]; then
  bundle=$(dirname "$dir")
  mods=()
  rank=8
  for cfg in "$bundle"/adapters/*/adapter_config.json; do
    [[ -e "$cfg" ]] || continue
    a=$(dirname "$cfg")
    mods+=("$(basename "$a")=$a")
    r=$(python3 -c 'import json,sys; print(int(json.load(open(sys.argv[1])).get("r", 8)))' "$cfg")
    if ((r > rank)); then rank=$r; fi
  done
  if ((${#mods[@]} > 0)); then
    args+=(--enable-lora --max-lora-rank "$rank" --lora-modules "${mods[@]}")
  fi
fi

extra=()
if [[ -n "${VLLM_ARGS:-}" ]]; then
  read -r -a extra <<<"$VLLM_ARGS"
fi

echo "serve.sh: vllm ${args[*]} ${extra[*]}" >&2
exec vllm "${args[@]}" "${extra[@]}"
