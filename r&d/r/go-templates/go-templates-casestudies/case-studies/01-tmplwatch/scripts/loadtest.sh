#!/usr/bin/env bash
# loadtest: drive a running tmplwatch and check docs/LOADTEST.md targets.
#
#   scripts/loadtest.sh [base-url]        default http://127.0.0.1:9180
#
# Env: RATE (50), DURATION (5m), METRICS_RATE (10), TEMPLATE (Caddyfile),
#      P99_MS (50), REPORT_DIR (dist/loadtest).
# Uses vegeta if installed, else this repo's tmplload. Writes JSON reports
# to $REPORT_DIR and exits non-zero if a target is missed.
set -euo pipefail

cd "$(dirname "$0")/.."
BASE=${1:-http://127.0.0.1:9180}
RATE=${RATE:-50}
DURATION=${DURATION:-5m}
METRICS_RATE=${METRICS_RATE:-10}
TEMPLATE=${TEMPLATE:-Caddyfile}
P99_MS=${P99_MS:-50}
REPORT_DIR=${REPORT_DIR:-dist/loadtest}
mkdir -p "$REPORT_DIR"

if ! curl -fsS "$BASE/healthz" >/dev/null 2>&1; then
	echo "loadtest: $BASE/healthz is not 200; start tmplwatch first" >&2
	exit 2
fi

TMPLLOAD=${TMPLLOAD:-}
if [[ -z "$TMPLLOAD" ]]; then
	TMPLLOAD="$REPORT_DIR/tmplload"
	GOFLAGS=${GOFLAGS:--mod=mod} GOPROXY=${GOPROXY:-off} go build -o "$TMPLLOAD" ./cmd/tmplload
fi

# attack NAME URL RATE: writes $REPORT_DIR/NAME.json in tmplload's format.
attack() {
	local name=$1 url=$2 rate=$3 out="$REPORT_DIR/$1.json"
	echo "loadtest: $name: GET $url at $rate/s for $DURATION" >&2
	if command -v vegeta >/dev/null 2>&1; then
		echo "GET $url" | vegeta attack -rate="$rate" -duration="$DURATION" -timeout=5s |
			vegeta report -type=json >"$REPORT_DIR/$name.vegeta.json"
		# Normalise vegeta's report (nanoseconds) to tmplload's shape.
		jq --arg url "$url" --argjson rate "$rate" --arg dur "$DURATION" '{
			url: $url, rate: $rate, duration: $dur, concurrency: 0,
			requests: .requests, errors: (.requests - (.status_codes["200"] // 0)), dropped: 0,
			throughput_rps: .throughput,
			p50_ms: (.latencies["50th"] / 1e6), p95_ms: (.latencies["95th"] / 1e6),
			p99_ms: (.latencies["99th"] / 1e6), max_ms: (.latencies.max / 1e6),
			mean_ms: (.latencies.mean / 1e6), status: .status_codes,
			first_error: ((.errors // [])[0] // "")
		}' "$REPORT_DIR/$name.vegeta.json" >"$out"
	else
		"$TMPLLOAD" -url "$url" -rate "$rate" -duration "$DURATION" -c 8 -json >"$out" || true
	fi
	jq -c '{requests, errors, p50_ms, p95_ms, p99_ms}' "$out" >&2
}

# Run both attacks concurrently: /metrics scrapes contend with renders in
# production too.
attack render "$BASE/render?template=$TEMPLATE" "$RATE" &
r=$!
attack metrics "$BASE/metrics" "$METRICS_RATE" &
m=$!
wait "$r" "$m"

fail=0
check() {
	local name=$1 p99_max=$2 f="$REPORT_DIR/$1.json"
	local errors p99
	errors=$(jq '.errors + .dropped' "$f")
	p99=$(jq '.p99_ms' "$f")
	if [[ "$errors" != 0 ]]; then
		echo "FAIL $name: $errors errors/drops" >&2; fail=1
	fi
	if ! awk -v a="$p99" -v b="$p99_max" 'BEGIN { exit !(a < b) }'; then
		echo "FAIL $name: p99 ${p99}ms >= ${p99_max}ms" >&2; fail=1
	fi
}
check render "$P99_MS"
check metrics 100
jq -s '{render: .[0], metrics: .[1], targets: {render_p99_ms: '"$P99_MS"', metrics_p99_ms: 100}, pass: '"$([[ $fail == 0 ]] && echo true || echo false)"'}' \
	"$REPORT_DIR/render.json" "$REPORT_DIR/metrics.json" >"$REPORT_DIR/report.json"
echo "loadtest: report in $REPORT_DIR/report.json" >&2
exit "$fail"
