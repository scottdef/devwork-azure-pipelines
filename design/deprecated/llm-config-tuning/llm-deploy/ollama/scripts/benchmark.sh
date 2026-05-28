#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  benchmark.sh — measure tokens/sec for each configured model
#  Requires: ollama running, jq installed, models already pulled
# ──────────────────────────────────────────────────────────────────────
set -euo pipefail

OLLAMA_URL="${OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
PROMPT="Write a detailed paragraph explaining how memory-mapped I/O works in Unix operating systems, covering both mmap and Plan 9's segment-based approach."

# Models to benchmark — add/remove as needed
MODELS=(
    "qwen2.5:3b"
    "qwen2.5-coder:3b"
    "llama3.2:3b"
    "p1000-chat"
    "p1000-coder"
    "p1000-agent"
    "p1000-heavy"
)

header_printed=false

for model in "${MODELS[@]}"; do
    # Check if model exists
    if ! ollama show "$model" &>/dev/null 2>&1; then
        echo "⚠  Skipping $model (not found)"
        continue
    fi

    if [ "$header_printed" = false ]; then
        printf "\n%-22s %10s %10s %10s %10s %10s\n" \
            "MODEL" "PROMPT" "EVAL" "TOT_DUR" "P_TOK/S" "E_TOK/S"
        printf '%.0s─' {1..76}; echo
        header_printed=true
    fi

    # Run inference with stream=false to get timing metrics
    response=$(curl -s "${OLLAMA_URL}/api/generate" \
        -d "{
            \"model\": \"${model}\",
            \"prompt\": \"${PROMPT}\",
            \"stream\": false,
            \"options\": {
                \"num_predict\": 256
            }
        }" 2>/dev/null)

    if [ -z "$response" ] || ! echo "$response" | jq -e '.eval_count' &>/dev/null; then
        echo "⚠  $model: inference failed"
        continue
    fi

    # Extract timing (nanoseconds → human-readable)
    prompt_eval_count=$(echo "$response" | jq -r '.prompt_eval_count // 0')
    eval_count=$(echo "$response" | jq -r '.eval_count // 0')
    total_duration=$(echo "$response" | jq -r '.total_duration // 0')
    prompt_eval_duration=$(echo "$response" | jq -r '.prompt_eval_duration // 1')
    eval_duration=$(echo "$response" | jq -r '.eval_duration // 1')

    # Calculate tok/s
    prompt_tps=$(echo "scale=1; $prompt_eval_count / ($prompt_eval_duration / 1000000000)" | bc 2>/dev/null || echo "n/a")
    eval_tps=$(echo "scale=1; $eval_count / ($eval_duration / 1000000000)" | bc 2>/dev/null || echo "n/a")
    total_sec=$(echo "scale=2; $total_duration / 1000000000" | bc 2>/dev/null || echo "n/a")

    printf "%-22s %8s t %8s t %8s s %8s/s %8s/s\n" \
        "$model" \
        "$prompt_eval_count" \
        "$eval_count" \
        "$total_sec" \
        "$prompt_tps" \
        "$eval_tps"
done

echo ""
echo "── VRAM snapshot ──"
nvidia-smi --query-gpu=name,memory.used,memory.total,utilization.gpu \
    --format=csv,noheader,nounits 2>/dev/null || echo "(nvidia-smi unavailable)"
echo ""
echo "── Loaded models ──"
curl -s "${OLLAMA_URL}/api/ps" | jq -r '.models[] | "\(.name)\t\(.size / 1048576 | floor) MB\tGPU layers: \(.details.quantization_level // "n/a")"' 2>/dev/null || echo "(none loaded)"
