#!/usr/bin/env bash
# Check that this repository is something Argo CD can act on.
#
#   - every JSON file parses, and says what its path says
#   - every team with apps has a team.json and an Argo CD project
#   - every app renders with the chart tag it names, when CHARTS points at a
#     checkout of cool-charts and coolctl is on the PATH
#
#   test/check.sh
#   CHARTS=../cool-charts test/check.sh
set -euo pipefail

cd "$(dirname "$0")/.."
COOLCTL=${COOLCTL:-coolctl}
CHARTS=${CHARTS:-}
fail=0
bad() { echo "FAIL  $*" >&2; fail=1; }

command -v jq >/dev/null || { echo "check.sh: jq not found" >&2; exit 2; }

# 1. JSON parses.
while IFS= read -r f; do
	jq -eM . "$f" >/dev/null 2>&1 || bad "$f is not JSON"
done < <(find apps teams clusters -name '*.json' | sort)

# 2. Paths and contents agree.
while IFS= read -r f; do
	IFS=/ read -r _ team app env _ <<<"$f"
	got=$(jq -rM '"\(.team)/\(.app)/\(.environment)"' "$f")
	[ "$got" = "$team/$app/$env" ] || bad "$f says it is $got"
	jq -eM '(.releases | length) >= 1 and (.releases | length) <= 3 and ([.releases[].weight] | add) == 100' "$f" >/dev/null ||
		bad "$f: releases must number 1 to 3 and their weights add up to 100"
	[ -f "apps/$team/$app/app.json" ] || bad "$f has no apps/$team/$app/app.json"
done < <(find apps -mindepth 4 -maxdepth 4 -name values.json | sort)

while IFS= read -r f; do
	IFS=/ read -r _ team app _ pr _ <<<"$f"
	got=$(jq -rM '"\(.team)/\(.app)/pr-\(.preview)"' "$f")
	[ "$got" = "$team/$app/$pr" ] || bad "$f says it is $got"
	[ "$(jq -rM .environment "$f")" = dev ] || bad "$f: previews run in dev"
done < <(find apps -path '*/previews/*/values.json' | sort)

# 3. Teams are complete.
for dir in apps/*/; do
	[ -d "$dir" ] || continue
	team=$(basename "$dir")
	[ -f "teams/$team/team.json" ] || bad "team $team has apps but no teams/$team/team.json"
	[ -f "argocd/projects/$team.yaml" ] || bad "team $team has apps but no argocd/projects/$team.yaml"
done
for f in teams/*/team.json; do
	[ -f "$f" ] || continue
	[ "$(jq -rM .team "$f")" = "$(basename "$(dirname "$f")")" ] || bad "$f names another team"
	jq -eM '.quota | IN("small", "medium", "large")' "$f" >/dev/null || bad "$f: quota must be small, medium or large"
done

# 4. Every app renders.
cluster_of() {
	case $1 in
	sbx) echo sbx-eus2-appsim-aks-001 ;;
	*) echo "$1-eus2-appdev-aks-001" ;;
	esac
}
if [ -n "$CHARTS" ] && command -v "$COOLCTL" >/dev/null; then
	n=0
	while IFS= read -r f; do
		team=$(jq -rM .team "$f") app=$(jq -rM .app "$f") env=$(jq -rM .environment "$f")
		platform=clusters/$(cluster_of "$env")/platform-values.json
		for chart in cool-namespace cool-app; do
			"$COOLCTL" render chart -chart "$CHARTS/charts/$chart" -namespace "$team-$app" \
				-f "teams/$team/team.json" -f "$f" -f "$platform" -o /dev/null ||
				bad "$f does not render with $chart"
		done
		n=$((n + 1))
	done < <(find apps -name values.json | sort)
	for c in clusters/*/platform-values.json; do
		"$COOLCTL" render chart -chart "$CHARTS/charts/cool-platform" -namespace cool-system -f "$c" -o /dev/null ||
			bad "$c does not render with cool-platform"
	done
	echo "ok    rendered $n app installs and $(find clusters -name platform-values.json | wc -l | tr -d ' ') platform installs"
else
	echo "skip  rendering: set CHARTS to a checkout of cool-charts and put coolctl on the PATH"
fi

[ "$fail" = 0 ] || exit 1
echo "cool-gitops is consistent"
