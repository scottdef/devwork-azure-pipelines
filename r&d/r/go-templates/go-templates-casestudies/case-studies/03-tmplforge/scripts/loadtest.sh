#!/usr/bin/env bash
# loadtest.sh: POST /render at a constant rate and check the SLO
# (p99 < 150 ms, including the sandbox process spawn; docs/LOADTEST.md).
#
#   scripts/loadtest.sh                    # 20 req/s for 5m against a local server it starts
#   RATE=50 DURATION=30s scripts/loadtest.sh
#   TARGET=http://host:8088 TOKEN=... scripts/loadtest.sh
#
# Uses vegeta if installed, else hey, else cmd/forgeload (stdlib only).
# Writes the JSON report to ${REPORT:-loadtest.json}.
set -euo pipefail

cd "$(dirname "$0")/.."
RATE=${RATE:-20}
DURATION=${DURATION:-5m}
P99_MS=${P99_MS:-150}
REPORT=${REPORT:-loadtest.json}
TEMPLATE=${TEMPLATE:-catalog/motd.txt.tmpl}
FIXTURE=${FIXTURE:-catalog/fixtures/motd.txt.json}
TOKEN=${TOKEN:-}
export GOFLAGS=-mod=mod GOPROXY=off

mkdir -p bin
go build -o bin/tmplforge ./cmd/tmplforge
go build -o bin/forgeload ./cmd/forgeload

server_pid=
cleanup() { [ -n "$server_pid" ] && kill -INT "$server_pid" 2>/dev/null && wait "$server_pid" 2>/dev/null || true; }
trap cleanup EXIT

if [ -z "${TARGET:-}" ]; then
	addr=127.0.0.1:${PORT:-18088}
	TARGET=http://$addr
	# rate limiting off: we are measuring render latency, not the limiter
	bin/tmplforge serve -a "$addr" -rate 0 -q &
	server_pid=$!
	for _ in $(seq 50); do
		curl -fs "$TARGET/healthz" >/dev/null 2>&1 && break
		sleep 0.1
	done
	curl -fs "$TARGET/healthz" >/dev/null
fi

echo "loadtest: $RATE req/s for $DURATION against $TARGET/render (SLO p99 < ${P99_MS}ms)"

if command -v vegeta >/dev/null 2>&1; then
	body=$(mktemp)
	printf 'template=%s&fixture=%s&format=txt' \
		"$(jq -sRr @uri < "$TEMPLATE")" "$(jq -sRr @uri < "$FIXTURE")" > "$body"
	hdr=(-header "Content-Type: application/x-www-form-urlencoded" -header "HX-Request: true")
	[ -n "$TOKEN" ] && hdr+=(-header "Authorization: Bearer $TOKEN")
	echo "POST $TARGET/render" | vegeta attack -rate="$RATE" -duration="$DURATION" -body="$body" "${hdr[@]}" \
		| tee results.bin | vegeta report
	vegeta report -type=json results.bin > "$REPORT"
	p99=$(jq '.latencies["99th"] / 1e6' "$REPORT")
	ok=$(jq '.success' "$REPORT")
	rm -f "$body"
	awk -v p="$p99" -v s="$ok" -v slo="$P99_MS" 'BEGIN { exit !(p < slo && s == 1) }' \
		&& echo "SLO met: p99 ${p99}ms" || { echo "SLO MISSED: p99 ${p99}ms success $ok" >&2; exit 1; }
elif command -v hey >/dev/null 2>&1 && [ "${USE_HEY:-0}" = 1 ]; then
	# hey is closed-loop (-q is per worker); kept for parity, not the default
	hey -z "$DURATION" -q 1 -c "$RATE" -m POST -T application/x-www-form-urlencoded \
		-H "HX-Request: true" ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
		-d "template=$(jq -sRr @uri < "$TEMPLATE")&fixture=$(jq -sRr @uri < "$FIXTURE")" "$TARGET/render"
else
	bin/forgeload -u "$TARGET/render" -r "$RATE" -d "$DURATION" -t "$TEMPLATE" -f "$FIXTURE" \
		${TOKEN:+-token "$TOKEN"} -p99 "$P99_MS" -json > "$REPORT" || status=$?
	bin/tmplforge render -t scripts/loadtest-report.md.tmpl -d "$REPORT" -f md
	exit "${status:-0}"
fi
