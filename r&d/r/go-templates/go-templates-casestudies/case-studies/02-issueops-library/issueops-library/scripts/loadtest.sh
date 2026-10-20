#!/usr/bin/env bash
# loadtest.sh: constant-rate GET load on the static library site.
#
#   BASE=https://coolgitorg.github.io/issueops-library/ RATE=50 DURATION=2m scripts/loadtest.sh
#   scripts/loadtest.sh -local      # serve site-sample/ on localhost and test that
#
# Uses vegeta if installed, else hey, else this repo's cmd/siteload. All
# three get the same three paths: catalog, one item page, ledger fragment.
# See docs/LOADTEST.md for targets and how to read the output.
set -euo pipefail
cd "$(dirname "$0")/.."

BASE="${BASE:-https://coolgitorg.github.io/issueops-library/}"
RATE="${RATE:-50}"
DURATION="${DURATION:-2m}"
ITEM="${ITEM:-items/payments-api/caddyfile/42/}"
PATHS="${PATHS:- $ITEM ledger/open.html}" # leading space: "" is the catalog
MAXP99="${MAXP99:-1500ms}"
srv=""
# Build once: killing "go run" would leave its compiled child running.
mkdir -p bin
go build -trimpath -o bin/siteload ./cmd/siteload

if [[ "${1:-}" == "-local" ]]; then
	[[ -d site-sample ]] || scripts/site-sample.sh
	port="${PORT:-8765}"
	# A tiny Go file server, so the local run needs nothing but the toolchain.
	bin/siteload -serve site-sample -addr "127.0.0.1:$port" &
	srv=$!
	trap 'kill "$srv" 2>/dev/null || true' EXIT
	BASE="http://127.0.0.1:$port/"
	for _ in $(seq 1 50); do curl -fsS --noproxy '*' "$BASE" >/dev/null 2>&1 && break; sleep 0.2; done
fi

read -r -a paths <<<"$PATHS"
[[ "$PATHS" == " "* ]] && paths=("" "${paths[@]}")

if command -v vegeta >/dev/null 2>&1; then
	echo "loadtest: vegeta, $RATE/s for $DURATION against $BASE"
	for p in "${paths[@]}"; do echo "GET ${BASE}${p}"; done |
		vegeta attack -rate="$RATE/s" -duration="$DURATION" | vegeta report -type=text
elif command -v hey >/dev/null 2>&1; then
	echo "loadtest: hey, $RATE/s per path split across ${#paths[@]} paths for $DURATION"
	per=$(((RATE + ${#paths[@]} - 1) / ${#paths[@]}))
	for p in "${paths[@]}"; do
		echo "== /$p"
		hey -z "$DURATION" -q 1 -c "$per" "${BASE}${p}" | sed -n '/Summary/,/Latency distribution/p;/99%/p;/Status code/,$p'
	done
else
	echo "loadtest: neither vegeta nor hey found; using cmd/siteload"
	args=()
	for p in "${paths[@]}"; do args+=(-p "$p"); done
	[[ ${#paths[@]} -gt 0 ]] || args=(-p "")
	bin/siteload -base "$BASE" -rate "$RATE" -d "$DURATION" -max-p99 "$MAXP99" "${args[@]}"
fi
