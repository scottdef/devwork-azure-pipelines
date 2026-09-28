#!/usr/bin/env bash
# entry.sh: start WHAM's run_server.py from the mounted repo snapshot and
# the shim in front of it. If either exits, the container exits.
set -euo pipefail

root=${MODEL_ROOT:-${AZUREML_MODEL_DIR:-/models}}
dir=$(find -L "$root" -maxdepth 5 -name run_server.py -printf '%d %h\n' 2>/dev/null |
  sort -n | head -1 | cut -d' ' -f2-)
if [[ -z "$dir" ]]; then
  echo "entry.sh: run_server.py not found under $root (register the whole microsoft/wham snapshot)" >&2
  exit 1
fi
ckpt="$dir/${WHAM_CHECKPOINT:-models/WHAM_1.6B_v1.ckpt}"
[[ -f "$ckpt" ]] || { echo "entry.sh: checkpoint $ckpt not found" >&2; exit 1; }

python3 /opt/ftm/shim.py --listen 0.0.0.0:8080 --upstream 127.0.0.1:5000 &
shim=$!
cd "$dir"
python3 run_server.py --model "$ckpt" --port 5000 &
server=$!

# Exit as soon as either process dies, so Azure ML restarts the container.
wait -n "$shim" "$server"
status=$?
kill "$shim" "$server" 2>/dev/null || true
exit "$status"
