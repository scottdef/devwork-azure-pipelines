#!/usr/bin/env bash
# preflight.sh: check that this machine can host the stack, and say what is
# missing. Read-only: it installs nothing and starts nothing.
set -uo pipefail

pass=0 warn=0 fail=0
ok()   { printf '  ok    %s\n' "$1"; pass=$((pass+1)); }
note() { printf '  warn  %s\n' "$1"; warn=$((warn+1)); }
bad()  { printf '  FAIL  %s\n' "$1"; fail=$((fail+1)); }
have() { command -v "$1" >/dev/null 2>&1; }

echo "== GPU"
if have nvidia-smi; then
  IFS=, read -r name drv vram < <(nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader,nounits | head -n1)
  name=${name# }; drv=${drv# }; vram=${vram# }
  ok "card: $name, driver $drv, ${vram} MiB"
  [ "${drv%%.*}" -ge 580 ] 2>/dev/null && ok "driver 580 or newer" || note "driver ${drv}: Strata's CUDA 13 engines need 580; its CUDA 12 engine accepts 525 on Linux"
  if   [ "$vram" -ge 12000 ]; then ok "12 GB or more: Strata's recommended size"
  elif [ "$vram" -ge 8000 ];  then note "8 GB class: Strata reports community runs, slower"
  else note "under 8 GB: below every card size Strata documents; run 'make plan' and keep the llama-server lane"
  fi
else
  bad "nvidia-smi not found: install the driver first"
fi

echo "== Memory and disk"
mem=$(awk '/MemTotal/ {printf "%d", $2/1048576 + 0.5}' /proc/meminfo)
if   [ "$mem" -ge 46 ]; then ok "RAM ${mem} GB: Qwen3.6 fits; Flash-Next Q2_0 and IQ2_XS may fit"
elif [ "$mem" -ge 30 ]; then ok "RAM ${mem} GB: Qwen3.6-35B-A3B UD-Q4_K_XL only"
else bad "RAM ${mem} GB: too little for any Strata model"
fi
free=$(df -BG --output=avail "$HOME" | tail -n1 | tr -dc 0-9)
[ "$free" -ge 60 ] && ok "disk: ${free} GB free under \$HOME" || note "disk: ${free} GB free; the model is 23 GB and Studio needs working room"
rot=$(lsblk -dno ROTA "$(df --output=source "$HOME" | tail -n1 | sed 's/[0-9]*$//;s/p$//')" 2>/dev/null | head -n1)
[ "${rot:-0}" = "0" ] && ok "home is on an SSD" || note "home is on a spinning disk: model loads will be slow"

echo "== Toolchain"
glibc=$(ldd --version 2>/dev/null | head -n1 | grep -o '[0-9]*\.[0-9]*$')
ok "glibc ${glibc:-unknown}"
case "${glibc:-0}" in 2.2*|2.30|2.31|2.32|2.33|2.34) note "glibc ${glibc}: prebuilt engines built on newer systems may not start; Docker or Ubuntu 24.04 avoids this";; esac
for t in git curl jq cmake uv; do have "$t" && ok "$t: $($t --version 2>/dev/null | head -n1 | cut -c1-60)" || note "$t not found"; done
have go && ok "$(go version)" || note "go not found: needed to build gate, council, agent and p340sim"
have nvcc && ok "nvcc: $(nvcc --version | grep -o 'release [0-9.]*')" || note "nvcc not found: needed only to build llama-server for the P1000"
have strata  && ok "strata: $(strata --version 2>/dev/null | head -n1)" || note "strata not installed"
have unsloth && ok "unsloth found" || note "unsloth not installed"

echo "== Ports"
for p in 4000 8080 8081 8083 8888 8889; do
  if ss -ltn 2>/dev/null | awk '{print $4}' | grep -q ":$p\$"; then note "port $p is in use"; else ok "port $p is free"; fi
done

echo "== Lanes (only if started)"
for u in "gate http://127.0.0.1:4000/healthz" "strata http://127.0.0.1:8080/v1/models" "llama http://127.0.0.1:8081/health" "studio http://127.0.0.1:8888/"; do
  set -- $u
  code=$(curl -s -o /dev/null -m 3 -w '%{http_code}' "$2" || true)
  [ "$code" != "000" ] && ok "$1 answers ($code)" || note "$1 is not running"
done

printf '\n%d ok, %d warnings, %d failures\n' "$pass" "$warn" "$fail"
[ "$fail" -eq 0 ]
