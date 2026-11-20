#!/usr/bin/env bash
# The CoolDeploy story from first deploy to rollback, on one machine.
#
#   demo/run-local.sh            run every chapter and exit
#   demo/run-local.sh --pause    wait for Enter between chapters (for a class)
#   demo/run-local.sh --keep     afterwards, keep cool-api up and serve the portal
#
# Needs Go 1.24, git, curl and jq. No Azure, no GitHub, no cluster: cool-api
# writes to a throwaway Git repository and a fake issuer stands in for
# GitHub's OIDC. What Argo CD would install is shown by rendering the chart.
set -euo pipefail

# shellcheck source=demo/lib.sh
. "$(dirname "$0")/lib.sh"

PAUSE=0 KEEP=0
for arg in "$@"; do
	case $arg in
	--pause) PAUSE=1 ;;
	--keep) KEEP=1 ;;
	*) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 2 ;;
	esac
done

WORK=$(mktemp -d "${TMPDIR:-/tmp}/cooldemo.XXXXXX")
GITOPS=$WORK/cool-gitops
APP=$ROOT/examples/checkout
REPO=CoolEngOrg/payments-checkout
SIM_PORT=${COOL_DEMO_SIM_PORT:-9913}
trap stop EXIT

chapter() {
	if [ "$PAUSE" = 1 ]; then
		printf '\n\033[2m(Enter for the next chapter)\033[0m'
		read -r _
	fi
	say "$*"
}

# expect_denied COMMAND... runs a command that cool-api must refuse.
expect_denied() {
	if "$@" 2>"$WORK/denied.txt"; then
		echo "demo: that should have been refused" >&2
		exit 1
	fi
	note "refused: $(tail -1 "$WORK/denied.txt")"
}

show_releases() {
	jq -rM '.releases[] | "   \(.id)  \(.weight)%\(if .standby then "  standby" else "" end)"' \
		"$GITOPS/apps/payments/checkout/$1/values.json"
}

say "Build the tools and start a local control plane"
build
mkdir -p "$GITOPS"
# Start from a platform with teams and clusters but no apps.
(cd "$ROOT/cool-gitops" && tar -cf - --exclude=./apps --exclude=./.git .) | tar -xf - -C "$GITOPS"
mkdir -p "$GITOPS/apps"
export COOL_PORTAL_ORIGIN=http://127.0.0.1:8000
start "$GITOPS"
note "cool-api   $COOL_API_URL   (writes to $GITOPS)"
note "issuer     http://$ISSUER_ADDR   (fake; signs what the demo asks for)"

chapter "1. The app: one manifest, no Dockerfile, no pipeline of its own"
cd "$APP"
run coolctl validate
note "The go-htmx preset supplies the Dockerfile:"
coolctl render dockerfile | sed -n '3,8p' | sed 's/^/   /'

chapter "2. Register the app (IssueOps request cool-app-create, approved)"
as_ops app-create dev coolctl register -app payments/checkout -repo "$REPO" \
	-identity dev=aaaaaaaa-0000-4000-8000-00000000d0e1 \
	-identity tst=aaaaaaaa-0000-4000-8000-0000000075f2 \
	-identity prd=aaaaaaaa-0000-4000-8000-00000000b4d3 \
	-issue "$ISSUES/101"

chapter "3. Push to main: CI builds an image and tells cool-api its digest"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest a)" -sha "$(commit a)"
note "cool-api wrote one file and made one commit:"
git -C "$GITOPS" log -1 --format='   %s%n%w(0,3,3)%b' | sed '/^ *$/d'
show_releases dev

chapter "4. What the trust model refuses"
note "Another repository's pipeline tries to deploy checkout:"
expect_denied as_ci CoolEngOrg/someone-else coolctl deploy -env dev -digest "$(digest f)" -sha "$(commit f)"
note "Checkout's own pipeline tries to skip the queue to production:"
expect_denied as_ci "$REPO" coolctl promote -from dev -to prd
note "An IssueOps job approved for tst tries to act on prd:"
expect_denied as_ops deploy-promote tst coolctl promote -from dev -to prd
note "A workflow that is not the platform's pipeline, with a valid GitHub token:"
expect_denied env COOL_TOKEN="$(token "repository=$REPO" \
	"job_workflow_ref=$REPO/.github/workflows/sneaky.yml@refs/heads/main")" \
	coolctl deploy -env dev -digest "$(digest f)" -sha "$(commit f)"

chapter "5. A second push: the first release becomes the standby"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest b)" -sha "$(commit b)"
show_releases dev

chapter "6. A pull request gets a preview, and loses it when it closes"
as_ci "$REPO" coolctl deploy -env dev -pr 42 -digest "$(digest d)" -sha "$(commit d)"
note "file: apps/payments/checkout/previews/pr-42/values.json"
as_ci "$REPO" coolctl preview down -pr 42
note "The file is gone, and Argo CD would delete the namespace with it."
as_ci "$REPO" coolctl deploy -env dev -pr 42 -digest "$(digest d)" -sha "$(commit d)" >/dev/null

chapter "7. Promote: dev to tst, then tst to prd (cool-deploy-promote)"
as_ops deploy-promote tst coolctl promote -from dev -to tst -issue "$ISSUES/102"
as_ops deploy-promote prd coolctl promote -from tst -to prd -issue "$ISSUES/103"
note "The same image digest now runs in all three environments."

chapter "8. A rolling release in prd (cool-rolling-release)"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest c)" -sha "$(commit c)"
as_ops deploy-promote tst coolctl promote -from dev -to tst -issue "$ISSUES/104"
as_ops rolling-release prd coolctl weights -env prd -from tst -weight 10 -issue "$ISSUES/105"
show_releases prd

note "Between stages the workflow asks Prometheus whether the new release is"
note "inside its error budget. Here appsim stands in for both the release and"
note "Prometheus. First a release that fails three requests in ten:"
PORT=$SIM_PORT SIM_LATENCY_MS=2 SIM_ERROR_RATE=0.3 "$BIN/appsim" >"$WORK/appsim.log" 2>&1 &
PIDS+=($!)
wait_http "http://127.0.0.1:$SIM_PORT/healthz"
for _ in $(seq 1 120); do curl -s -o /dev/null "http://127.0.0.1:$SIM_PORT/"; done
gate() {
	coolctl gate -endpoint "http://127.0.0.1:$SIM_PORT" -namespace payments-checkout \
		-workload checkout-rccccccc -max-error-rate 0.01 -max-p95-ms 500
}
if gate >"$WORK/gate.json" 2>"$WORK/gate.err"; then
	echo "demo: the gate should have failed" >&2
	exit 1
fi
note "gate: $(jq -rM .reason "$WORK/gate.json")"
note "The workflow would stop here and take the release out of traffic."
note "For the demo, fix the release instead:"
run curl -s -X POST "http://127.0.0.1:$SIM_PORT/sim?error_rate=0"
echo
for _ in $(seq 1 220); do curl -s -o /dev/null "http://127.0.0.1:$SIM_PORT/"; done
gate >"$WORK/gate.json"
note "gate: $(jq -rM .reason "$WORK/gate.json")"
as_ops rolling-release prd coolctl weights -env prd -weight 50 -issue "$ISSUES/105"
as_ops rolling-release prd coolctl weights -env prd -weight 100 -issue "$ISSUES/105"
show_releases prd

chapter "9. What Argo CD would install: the chart, rendered from Git"
as_ops rolling-release prd coolctl rollback -env prd -reason "rehearsal" -issue "$ISSUES/106" >/dev/null
as_ops rolling-release prd coolctl rollback -env prd -reason "rehearsal, undone" -issue "$ISSUES/106" >/dev/null
note "(two rollbacks in a row: the second undoes the first)"
RENDER=$WORK/prd.yaml
run coolctl render chart -chart "$ROOT/cool-charts/charts/cool-app" -namespace payments-checkout \
	-f "$GITOPS/teams/payments/team.json" \
	-f "$GITOPS/apps/payments/checkout/prd/values.json" \
	-f "$GITOPS/clusters/prd-eus2-appdev-aks-001/platform-values.json" -o "$RENDER"
grep -E '^kind:' "$RENDER" | sort | uniq -c | sed 's/^/   /'
if command -v kubeconform >/dev/null; then
	kubeconform -kubernetes-version 1.30.0 -strict -summary \
		-schema-location default \
		-schema-location 'https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json' \
		"$RENDER" | sed 's/^/   /'
fi

chapter "10. Settings that belong to an environment (cool-env-var, cool-domain)"
as_ops env-var prd coolctl config -env prd -set LOG_LEVEL=warn -issue "$ISSUES/107"
as_ops domain prd coolctl config -env prd -add-domain pay.coolshop.example -issue "$ISSUES/108"

chapter "11. Roll back prd (cool-deploy-rollback)"
as_ops deploy-rollback prd coolctl rollback -env prd -reason "checkout errors after release" -issue "$ISSUES/109"
show_releases prd
note "Rollback swaps the active release and the standby. It is its own undo."

chapter "12. Where everything stands"
run coolctl status -app payments/checkout

chapter "13. The audit trail is the Git log"
git -C "$GITOPS" log --format='   %h %s' | head -20
note "Each commit says who asked and which workflow run did it:"
git -C "$GITOPS" log -1 --format='%w(0,3,3)%B' | sed '/^ *$/d'

chapter "14. The portal: static pages rendered from the same repository"
SITE=$WORK/site
cp -R "$ROOT/cool-portal/site" "$SITE"
run coolctl render portal -gitops "$GITOPS" -out "$SITE"
note "wrote $(find "$SITE/fragments" -name '*.html' | wc -l | tr -d ' ') fragments and data/apps.json"

say "Done"
note "GitOps repository   $GITOPS"
note "Rendered manifests  $RENDER"
note "Portal              $SITE"
note "Logs                $WORK/*.log"

if [ "$KEEP" = 1 ]; then
	say "Serving the portal at http://127.0.0.1:8000 (Ctrl-C to stop)"
	note "Open app pages for live status from the local cool-api:"
	note "http://127.0.0.1:8000/app.html?team=payments&app=checkout&api=$COOL_API_URL"
	coolctl serve -dir "$SITE" -addr 127.0.0.1:8000
fi
