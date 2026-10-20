#!/usr/bin/env bash
# beta-run.sh: end-to-end beta run against the real repositories.
#
#   scripts/beta-run.sh [-n 10] [-i 10] [-t caddyfile] [-timeout 900]
#
# Opens N request issues on the intake repo, one every I seconds (labels:
# issueops, request, beta), then polls each item's Pages URL until it
# answers 200 and reports issue->Pages latency (min/p50/p90/max).
#
# Requires: gh (authenticated, issues:write on the intake repo), curl, jq.
#
# API budget (why the defaults are what they are):
#   - Each request costs ~6 REST writes in the intake run (comment, labels,
#     dispatch) and ~4 in the library run (push, comment, label, close).
#     An App installation token gets at least 5,000 requests/hour, so ten
#     requests are ~100 calls: nowhere near the limit.
#   - GitHub's secondary limits ask for >= 1 s between writes and penalise
#     bursts of content creation; one issue every 10 s keeps this script
#     well inside them. bin/dispatch also waits >= 1 s between writes.
#   - Pages deploys are serialized (concurrency group library-pages) and each
#     takes ~1-3 min, but one deploy publishes everything committed before
#     it starts, so latency grows with the deploy time, not with N.
set -euo pipefail

n=10 gap=10 tmpl=caddyfile timeout=900
owner="${OWNER:-CoolGitOrg}"
intake="${INTAKE_REPO:-issueops-intake}"
base="${BASE_URL:-https://coolgitorg.github.io/issueops-library/}"
while [[ $# -gt 0 ]]; do
	case "$1" in
	-n) n="$2"; shift 2 ;;
	-i) gap="$2"; shift 2 ;;
	-t) tmpl="$2"; shift 2 ;;
	-timeout) timeout="$2"; shift 2 ;;
	*) echo "usage: beta-run.sh [-n N] [-i seconds] [-t template] [-timeout seconds]" >&2; exit 2 ;;
	esac
done
for c in gh curl jq; do command -v "$c" >/dev/null || { echo "beta-run: $c not found" >&2; exit 2; }; done
gh auth status >/dev/null 2>&1 || { echo "beta-run: gh is not authenticated (gh auth login)" >&2; exit 2; }
(( gap >= 1 )) || { echo "beta-run: -i must be >= 1 second" >&2; exit 2; }

run="$(date -u +%m%d%H%M)"
work="$(mktemp -d)"
trap 'wait 2>/dev/null; rm -rf "$work"' EXIT
gh label create beta --repo "$owner/$intake" --color ededed --description "Beta load run" --force >/dev/null

body() { # $1 source-id
	cat <<EOB
### Source ID

$1

### Template

$tmpl

### Phase

1

### Payload

\`\`\`json
{"name": "$1", "domain": "$1.beta.coolgit.internal", "port": 8080, "routes": ["/a", "/b"]}
\`\`\`

### Notes

beta-run $run
EOB
}

poll() { # $1 issue, $2 url, $3 start epoch
	local deadline=$(($3 + timeout))
	while (($(date +%s) < deadline)); do
		if curl -fsS -o /dev/null --max-time 10 "$2?beta=$RANDOM"; then
			echo "$1 $(($(date +%s) - $3)) ok" >>"$work/results"
			return
		fi
		sleep 5
	done
	echo "$1 $timeout timeout" >>"$work/results"
}

echo "beta-run $run: $n issue(s) on $owner/$intake, one every ${gap}s, template $tmpl"
for ((i = 1; i <= n; i++)); do
	src="beta-$run-$i"
	body "$src" >"$work/body-$i.md"
	start=$(date +%s)
	url="$(gh issue create --repo "$owner/$intake" --title "[request] $src $tmpl" \
		--label issueops --label request --label beta --body-file "$work/body-$i.md")"
	num="${url##*/}"
	item="${base%/}/items/$src/$tmpl/$num/"
	printf '  #%s -> %s\n' "$num" "$item"
	poll "$num" "$item" "$start" &
	((i < n)) && sleep "$gap"
done
echo "beta-run: waiting for Pages (timeout ${timeout}s per item)..."
wait

sort -k2 -n "$work/results" | awk '
	$3 == "ok" { lat[++k] = $2 } $3 != "ok" { bad++ }
	END {
		printf "results: %d live, %d timed out\n", k, bad + 0
		if (k == 0) exit
		printf "issue->Pages latency (s): min %d  p50 %d  p90 %d  max %d\n",
			lat[1], lat[int((k + 1) / 2)], lat[int(k * 0.9 + 0.999)], lat[k]
	}'
echo "clean up: gh issue list --repo $owner/$intake --label beta --state all"
