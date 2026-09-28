#!/usr/bin/env bash
# copilot-byok.sh: run GitHub Copilot CLI against the custom Muse Glimmer
# endpoint (bring your own key). No GitHub model routing is used.
#
#   copilot-byok.sh probe            one-word round trip; exit 0 on success
#   copilot-byok.sh ask "PROMPT"     one non-interactive task, in a scratch dir
#   copilot-byok.sh shell            interactive session with the same settings
#
# Environment: FTM_BASE_URL (endpoint root), FTM_KEY, FTM_MODEL (default muse-glimmer).
# COPILOT_PROVIDER_TYPE defaults to openai (vLLM /v1/chat/completions). vLLM also
# serves the Anthropic Messages API, so COPILOT_PROVIDER_TYPE=anthropic works as a
# fallback; the base URL then drops the /v1 suffix.
set -euo pipefail

: "${FTM_BASE_URL:?set FTM_BASE_URL to the endpoint root}"
: "${FTM_KEY:?set FTM_KEY to the endpoint key}"
command -v copilot >/dev/null || { echo "copilot not found: npm install -g @github/copilot" >&2; exit 1; }

base=${FTM_BASE_URL%/}
export COPILOT_PROVIDER_TYPE=${COPILOT_PROVIDER_TYPE:-openai}
if [[ "$COPILOT_PROVIDER_TYPE" == anthropic ]]; then
  export COPILOT_PROVIDER_BASE_URL=$base
else
  export COPILOT_PROVIDER_BASE_URL=$base/v1
  export COPILOT_PROVIDER_WIRE_API=${COPILOT_PROVIDER_WIRE_API:-completions}
fi
export COPILOT_PROVIDER_API_KEY=$FTM_KEY
export COPILOT_MODEL=${FTM_MODEL:-muse-glimmer}
export COPILOT_OFFLINE=true   # the CLI talks to the provider only
export NO_COLOR=1

scratch() {
  local d
  d=$(mktemp -d)
  trap 'rm -rf "$d"' EXIT
  cd "$d"
}

case "${1:-probe}" in
  probe)
    scratch
    marker=FTM_OK
    prompt="Do not use any tools. Reply with exactly this word and nothing else: $marker"
    out=$(copilot -p "$prompt" -s --no-ask-user 2>&1 || true)
    if ! grep -q "$marker" <<<"$out"; then
      # Some CLI builds print -p output only in JSON mode.
      out=$(copilot -p "$prompt" -s --no-ask-user --output-format json 2>&1 || true)
    fi
    if grep -q "$marker" <<<"$out"; then
      echo "copilot-byok: ok ($COPILOT_PROVIDER_TYPE, model $COPILOT_MODEL)"
    else
      echo "copilot-byok: no marker in output:" >&2
      printf '%s\n' "$out" | tail -20 >&2
      exit 1
    fi
    ;;
  ask)
    [[ $# -ge 2 ]] || { echo "usage: $0 ask PROMPT" >&2; exit 2; }
    scratch
    copilot -p "$2" -s --no-ask-user
    ;;
  shell)
    exec copilot
    ;;
  *)
    echo "usage: $0 probe | ask PROMPT | shell" >&2
    exit 2
    ;;
esac
