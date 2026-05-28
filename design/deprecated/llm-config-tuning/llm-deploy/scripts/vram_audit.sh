#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  vram_audit.sh — live VRAM usage report for the Quadro P1000
#  Shows: driver, VRAM breakdown, loaded Ollama models, budget math
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

TOTAL_VRAM_MB="${TOTAL_VRAM_MB:-4096}"
OVERHEAD_MB="${VRAM_OVERHEAD_MB:-512}"
USABLE_MB=$(( TOTAL_VRAM_MB - OVERHEAD_MB ))
OLLAMA_URL="${OLLAMA_BASE_URL:-http://127.0.0.1:11434}"

echo "╔══════════════════════════════════════════════════════════════╗"
echo "║              VRAM Audit — Quadro P1000 (4 GB)               ║"
echo "╠══════════════════════════════════════════════════════════════╣"

# GPU info from nvidia-smi
if command -v nvidia-smi &>/dev/null; then
    gpu_info=$(nvidia-smi --query-gpu=name,driver_version,memory.used,memory.total,memory.free,temperature.gpu,utilization.gpu \
        --format=csv,noheader,nounits 2>/dev/null || echo "error")

    if [ "$gpu_info" != "error" ]; then
        IFS=',' read -r gpu_name driver_ver mem_used mem_total mem_free temp util <<< "$gpu_info"
        gpu_name=$(echo "$gpu_name" | xargs)
        mem_used=$(echo "$mem_used" | xargs)
        mem_total=$(echo "$mem_total" | xargs)
        mem_free=$(echo "$mem_free" | xargs)
        temp=$(echo "$temp" | xargs)
        util=$(echo "$util" | xargs)

        printf "║  GPU:          %-44s ║\n" "$gpu_name"
        printf "║  Driver:       %-44s ║\n" "$driver_ver"
        printf "║  Temperature:  %-44s ║\n" "${temp}°C"
        printf "║  Utilization:  %-44s ║\n" "${util}%"
        echo "╠══════════════════════════════════════════════════════════════╣"
        printf "║  VRAM Total:   %6s MB                                   ║\n" "$mem_total"
        printf "║  VRAM Used:    %6s MB                                   ║\n" "$mem_used"
        printf "║  VRAM Free:    %6s MB                                   ║\n" "$mem_free"
        printf "║  Reserved:     %6s MB  (desktop/display overhead)       ║\n" "$OVERHEAD_MB"
        printf "║  Usable:       %6s MB  (for models + KV cache)          ║\n" "$USABLE_MB"

        remaining=$(( mem_free - OVERHEAD_MB ))
        if [ "$remaining" -lt 0 ]; then remaining=0; fi
        printf "║  Available:    %6s MB  (free minus overhead)            ║\n" "$remaining"
    else
        echo "║  nvidia-smi query failed                                    ║"
    fi
else
    echo "║  nvidia-smi not found — GPU drivers may not be installed     ║"
fi

echo "╠══════════════════════════════════════════════════════════════╣"

# Ollama loaded models
if curl -sf "${OLLAMA_URL}/api/ps" &>/dev/null; then
    echo "║  Loaded Ollama Models:                                       ║"
    models=$(curl -sf "${OLLAMA_URL}/api/ps" | \
        jq -r '.models[]? | "    \(.name)  \(.size / 1048576 | floor) MB"' 2>/dev/null)
    if [ -n "$models" ]; then
        while IFS= read -r line; do
            printf "║  %-58s ║\n" "$line"
        done <<< "$models"
    else
        echo "║    (none loaded)                                             ║"
    fi
else
    echo "║  Ollama not reachable at ${OLLAMA_URL}             ║"
fi

echo "╠══════════════════════════════════════════════════════════════╣"

# Budget guidance
echo "║  Model Budget Guide (with q8_0 KV cache + 512 MB reserve):  ║"
echo "║    3B Q4_K_M  → ~2.4 GB → fits ✓  (full GPU)               ║"
echo "║    4B Q4_K_M  → ~3.0 GB → fits ✓  (tight, reduce num_ctx)  ║"
echo "║    7B Q4_K_M  → ~4.5 GB → partial (16/33 layers on GPU)    ║"
echo "║    13B Q4_K_M → ~7.5 GB → CPU-only or 5 layers on GPU      ║"
echo "║    embed 137M → ~0.27 GB → co-resides with any chat model  ║"
echo "╚══════════════════════════════════════════════════════════════╝"

# Process-level GPU memory (shows which PIDs are using VRAM)
if command -v nvidia-smi &>/dev/null; then
    echo ""
    echo "── Per-process VRAM usage ──"
    nvidia-smi --query-compute-apps=pid,name,used_gpu_memory \
        --format=csv,noheader 2>/dev/null || echo "(no compute processes)"
fi
