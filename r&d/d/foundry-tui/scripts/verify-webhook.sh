#!/usr/bin/env bash
# verify-webhook.sh: check the HMAC on a foundry-deploy webhook.
#
#   verify-webhook.sh BODY_FILE SIGNATURE_HEADER
#
# SIGNATURE_HEADER is the value of X-Foundry-Signature-256 ("sha256=<hex>").
# The shared secret is read from $FOUNDRY_WEBHOOK_SECRET, never from argv.
set -euo pipefail

[[ $# -eq 2 ]] || { echo "usage: $0 BODY_FILE SIGNATURE_HEADER" >&2; exit 2; }
: "${FOUNDRY_WEBHOOK_SECRET:?set FOUNDRY_WEBHOOK_SECRET}"

body=$1
given=${2#sha256=}
want=$(openssl dgst -sha256 -hmac "$FOUNDRY_WEBHOOK_SECRET" < "$body" | awk '{print $NF}')

# Compare digests of the two values so timing says nothing about $want.
a=$(printf '%s' "$given" | sha256sum)
b=$(printf '%s' "$want" | sha256sum)
if [[ "$a" == "$b" ]]; then echo "signature ok"; else echo "signature MISMATCH" >&2; exit 1; fi
