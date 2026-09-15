#!/usr/bin/env bash
# validate-endpoint.sh — prove the deployment does what Copilot BYOK needs:
#   1. plain chat completion (auth + generation)
#   2. streaming
#   3. a tool-call turn (function calling)  ← the thing a ping never tests
# Tries both URL shapes; reports which one answered.  Keyless (Entra) by default,
# key-based if FOUNDRY_KEY is set.
#
# Env: ENDPOINT DEPLOYMENT [FOUNDRY_KEY] [API_VERSION=2024-10-21] [EXPECT_TOOLS=true]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need curl jq

: "${ENDPOINT:?}" "${DEPLOYMENT:?}"
API_VERSION="${API_VERSION:-2024-10-21}"
EXPECT_TOOLS="${EXPECT_TOOLS:-true}"
ENDPOINT="${ENDPOINT%/}"

if [[ -n "${FOUNDRY_KEY:-}" ]]; then
  auth=(-H "api-key: $FOUNDRY_KEY" -H "Authorization: Bearer $FOUNDRY_KEY")
else
  need az
  tok="$(az account get-access-token --resource https://cognitiveservices.azure.com --query accessToken -o tsv)"
  auth=(-H "Authorization: Bearer $tok")
fi

urlA="$ENDPOINT/openai/deployments/$DEPLOYMENT/chat/completions?api-version=$API_VERSION"
urlB="$ENDPOINT/models/chat/completions?api-version=2024-05-01-preview"

req() { # req <url> <json-body> → response body; sets $code
  local url="$1" body="$2" tmp; tmp="$(mktemp)"
  code="$(curl -sS -o "$tmp" -w '%{http_code}' "$url" -H 'Content-Type: application/json' "${auth[@]}" -d "$body" || echo 000)"
  cat "$tmp"; rm -f "$tmp"
}

ping='{"messages":[{"role":"user","content":"Reply with exactly: OK"}],"max_tokens":8}'
pingB="$(jq --arg m "$DEPLOYMENT" '. + {model:$m}' <<<"$ping")"

shape=""; resp=""
resp="$(req "$urlA" "$ping")"; [[ "$code" == 200 ]] && shape=A
if [[ -z "$shape" ]]; then resp="$(req "$urlB" "$pingB")"; [[ "$code" == 200 ]] && shape=B; fi
[[ -n "$shape" ]] || die "no URL shape returned 200 (last code=$code): $resp"
url="$([[ $shape == A ]] && echo "$urlA" || echo "$urlB")"
log "chat OK via shape $shape: $(jq -r '.choices[0].message.content' <<<"$resp" | head -c 80)"
out url_shape "$shape"
out base_url "$url"

# streaming
sbody="$(jq '. + {stream:true}' <<<"$([[ $shape == A ]] && echo "$ping" || echo "$pingB")")"
chunks="$(curl -sS -N "$url" -H 'Content-Type: application/json' "${auth[@]}" -d "$sbody" | grep -c '^data:' || true)"
(( chunks > 1 )) || die "streaming returned $chunks SSE chunks"
log "streaming OK ($chunks chunks)"
out streaming true

# tool calling
tbody="$(jq '. + {
  messages:[{"role":"user","content":"What is the weather in Boston? Use the tool."}],
  tools:[{"type":"function","function":{"name":"get_weather","description":"Get weather",
    "parameters":{"type":"object","properties":{"city":{"type":"string"}},"required":["city"]}}}],
  tool_choice:"auto", max_tokens:128 }' <<<"$([[ $shape == A ]] && echo "$ping" || echo "$pingB")")"
tresp="$(req "$url" "$tbody")"
if [[ "$code" == 200 ]] && jq -e '.choices[0].message.tool_calls[0].function.name=="get_weather"' <<<"$tresp" >/dev/null; then
  log "tool calling OK"; out tool_calling true
else
  out tool_calling false
  [[ "$EXPECT_TOOLS" == "true" ]] && die "tool call not returned (code=$code): $(head -c 300 <<<"$tresp")"
  warn "tool calling not supported — model is not Copilot-BYOK eligible"
fi
out validated true
