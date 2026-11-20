#!/usr/bin/env bash
# Render every chart with the test values and check the result.
#
# Uses coolctl to render, so it needs no Helm. With helm on the PATH it also
# runs `helm lint` and `helm template` and compares object lists, which is
# the check that the two renderers agree. With kubeconform on the PATH it
# validates every object against the Kubernetes 1.30 and CRD schemas.
#
#   test/render.sh            render and check
#   OUT=dir test/render.sh    also keep the rendered YAML in dir
set -euo pipefail

cd "$(dirname "$0")/.."
COOLCTL=${COOLCTL:-coolctl}
OUT=${OUT:-$(mktemp -d)}
mkdir -p "$OUT"
V=test/values
fail=0

command -v "$COOLCTL" >/dev/null || { echo "render.sh: coolctl not found; set COOLCTL" >&2; exit 2; }

# kinds prints "Kind/name" for each object of a YAML stream, sorted.
kinds() {
	awk '
		/^---/ { if (kind != "") print kind "/" name; kind = ""; name = ""; meta = 0; next }
		/^kind:/ { kind = $2 }
		/^metadata:/ { meta = 1; next }
		/^[a-zA-Z]/ { meta = 0 }
		meta && /^  name:/ && name == "" { name = $2; gsub(/"/, "", name) }
		END { if (kind != "") print kind "/" name }
	' "$1" | sort
}

# render NAME CHART NAMESPACE VALUES...
render() {
	local name=$1 chart=$2 ns=$3 args=() f
	shift 3
	for f in "$@"; do args+=(-f "$V/$f"); done
	if ! "$COOLCTL" render chart -chart "charts/$chart" -release "$chart" -namespace "$ns" "${args[@]}" -o "$OUT/$name.yaml"; then
		echo "FAIL  $name: coolctl could not render it" >&2
		fail=1
		return
	fi
	echo "ok    $name: $(kinds "$OUT/$name.yaml" | wc -l | tr -d ' ') objects"
	if command -v helm >/dev/null; then
		helm lint "charts/$chart" "${args[@]}" >/dev/null || { echo "FAIL  $name: helm lint" >&2; fail=1; }
		helm template "$chart" "charts/$chart" --namespace "$ns" "${args[@]}" >"$OUT/$name.helm.yaml"
		if ! diff <(kinds "$OUT/$name.yaml") <(kinds "$OUT/$name.helm.yaml") >/dev/null; then
			echo "FAIL  $name: coolctl and helm render different objects" >&2
			fail=1
		fi
	fi
}

# refuse NAME CHART VALUES... expects the render to fail.
refuse() {
	local name=$1 chart=$2 args=() f
	shift 2
	for f in "$@"; do args+=(-f "$V/$f"); done
	if "$COOLCTL" render chart -chart "charts/$chart" "${args[@]}" -o /dev/null 2>/dev/null; then
		echo "FAIL  $name: rendered, but should have been refused" >&2
		fail=1
	else
		echo "ok    $name: refused"
	fi
}

# expect NAME PATTERN COUNT checks how often a pattern occurs in a render.
expect() {
	local n
	n=$(grep -c -- "$2" "$OUT/$1.yaml" || true)
	if [ "$n" != "$3" ]; then
		echo "FAIL  $1: want $3 lines matching '$2', got $n" >&2
		fail=1
	fi
}

render app-rolling     cool-app payments-checkout       app-rolling.json platform.json
render app-preview     cool-app payments-checkout-pr-42 app-preview.json platform.json
render app-standby     cool-app sim-appsim-steady       app-standby.json platform.json
render app-no-mesh     cool-app sim-appsim-steady       app-standby.json platform.json no-mesh.json
render ns-app          cool-namespace payments-checkout       app-rolling.json platform.json
render ns-preview      cool-namespace payments-checkout-pr-42 app-preview.json platform.json
render ns-no-mesh      cool-namespace sim-appsim-steady       app-standby.json platform.json no-mesh.json
render platform        cool-platform cool-system platform.json
render platform-nomesh cool-platform cool-system platform.json no-mesh.json
render cool-api        cool-api cool-system cool-api.json

# Secrets from Key Vault need an identity; the chart must say so.
refuse app-no-identity cool-app app-missing-identity.json platform.json
# cool-api without its GitHub App is not a deployment anyone wants.
refuse cool-api-empty  cool-api

# A rolling release: two weighted Deployments, the third release is gone.
expect app-rolling '^kind: Deployment$' 2
expect app-rolling '"weight": 10}' 3
expect app-rolling '^kind: HorizontalPodAutoscaler$' 2
expect app-rolling 'pay.coolshop.example' 1
# A preview: one replica, no autoscaler, no cron, no external gateway.
expect app-preview '^kind: HorizontalPodAutoscaler$' 0
expect app-preview '^kind: CronJob$' 0
expect app-preview 'cool-external' 0
expect app-preview 'checkout-pr-42.preview.dev.coolapps.example' 2
# A standby keeps a Deployment but no autoscaler.
expect app-standby '^kind: Deployment$' 2
expect app-standby '^kind: HorizontalPodAutoscaler$' 1
# Without the mesh there is no Istio object and the Service picks the release.
expect app-no-mesh 'istio.io' 0
expect app-no-mesh 'cool/release: "rbbbbbbb"' 1
expect ns-no-mesh 'istio.io' 0
expect ns-preview '"cool/quota": "preview"' 1
expect ns-app '"cool/quota": "medium"' 1

if command -v kubeconform >/dev/null; then
	kubeconform -kubernetes-version 1.30.0 -strict -summary \
		-schema-location default \
		-schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
		"$OUT"/*.yaml || fail=1
else
	echo "skip  kubeconform not found: objects were not checked against schemas"
fi

if [ "$fail" != 0 ]; then
	echo "render.sh: failures above" >&2
	exit 1
fi
echo "all charts rendered as expected"
