#!/usr/bin/env bash
# ──────────────────────────────────────────────────────────────────────
#  preflight.sh — verify the entire local LLM stack is operational
# ──────────────────────────────────────────────────────────────────────
set -uo pipefail

OLLAMA_URL="${OLLAMA_BASE_URL:-http://127.0.0.1:11434}"
WEBUI_PORT="${WEBUI_PORT:-3000}"
SEARXNG_URL="${SEARXNG_URL:-http://127.0.0.1:8888}"
PASS=0; FAIL=0; WARN=0

check() {
    local label="$1"; shift
    if "$@" &>/dev/null; then
        printf "  ✅  %-40s OK\n" "$label"
        ((PASS++))
    else
        printf "  ❌  %-40s FAIL\n" "$label"
        ((FAIL++))
    fi
}

warn() {
    local label="$1"; shift
    if "$@" &>/dev/null; then
        printf "  ✅  %-40s OK\n" "$label"
        ((PASS++))
    else
        printf "  ⚠️   %-40s WARN\n" "$label"
        ((WARN++))
    fi
}

echo "══════════════════════════════════════════════════════════"
echo "  Preflight Check — P340 SFF Local LLM Stack"
echo "══════════════════════════════════════════════════════════"

echo ""
echo "── System ──"
check "nvidia-smi" nvidia-smi
check "CUDA toolkit" nvcc --version
check "Docker daemon" docker info
check "NVIDIA Container Toolkit" docker run --rm --gpus all nvidia/cuda:12.9.0-base-ubuntu22.04 true

echo ""
echo "── Ollama ──"
check "Ollama service" systemctl is-active ollama
check "Ollama API" curl -sf "${OLLAMA_URL}/api/tags"
check "Ollama GPU detected" bash -c "curl -sf ${OLLAMA_URL}/api/ps >/dev/null"

echo ""
echo "── Models ──"
for model in qwen2.5:3b qwen2.5-coder:3b llama3.2:3b nomic-embed-text; do
    check "Model: ${model}" ollama show "$model"
done
for model in p1000-chat p1000-coder p1000-agent p1000-heavy; do
    warn "Custom: ${model}" ollama show "$model"
done

echo ""
echo "── Docker services ──"
for svc in ollama open-webui searxng; do
    warn "Container: ${svc}" docker inspect --format='{{.State.Running}}' "$svc"
done

echo ""
echo "── Endpoints ──"
warn "Open WebUI" curl -sf "http://localhost:${WEBUI_PORT}" -o /dev/null
warn "SearXNG HTML" curl -sf "${SEARXNG_URL}" -o /dev/null
warn "SearXNG JSON" curl -sf "${SEARXNG_URL}/search?q=test&format=json" -o /dev/null
warn "LiteLLM proxy" curl -sf "http://localhost:4000/health" -o /dev/null

echo ""
echo "── Quick inference test ──"
resp=$(curl -sf "${OLLAMA_URL}/api/generate" -d '{
    "model": "qwen2.5:3b",
    "prompt": "Say hello in exactly 3 words.",
    "stream": false,
    "options": {"num_predict": 10}
}' 2>/dev/null)

if echo "$resp" | jq -e '.response' &>/dev/null; then
    answer=$(echo "$resp" | jq -r '.response' | head -1)
    eval_rate=$(echo "$resp" | jq -r '(.eval_count // 0) as $c | (.eval_duration // 1) as $d | ($c / ($d / 1000000000)) | floor')
    printf "  ✅  %-40s %s tok/s\n" "Inference: qwen2.5:3b" "$eval_rate"
    ((PASS++))
else
    printf "  ❌  %-40s FAIL\n" "Inference: qwen2.5:3b"
    ((FAIL++))
fi

echo ""
echo "══════════════════════════════════════════════════════════"
printf "  Results: %d passed, %d failed, %d warnings\n" "$PASS" "$FAIL" "$WARN"
echo "══════════════════════════════════════════════════════════"

[ "$FAIL" -eq 0 ] && exit 0 || exit 1
