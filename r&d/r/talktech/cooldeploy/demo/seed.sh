#!/usr/bin/env bash
# Regenerate the example apps in cool-gitops/apps.
#
# The files there are not written by hand. This script plays the requests
# that would have produced them against a local cool-api, then copies the
# result in. Run it after changing the shape of values.json, so that the
# examples never drift from the code.
#
#   demo/seed.sh
#
# The state it leaves:
#
#   payments/checkout   dev   rccccccc live, rbbbbbbb standby
#                       tst   rccccccc live, rbbbbbbb standby
#                       prd   a rolling release in flight: rccccccc 10%, rbbbbbbb 90%
#                       pr-42 a preview of rddddddd
#   sim/appsim-steady   sbx   a healthy simulator
#   sim/appsim-flaky    sbx   one that fails 5% of requests, with a canary at 20%
#   sim/appsim-noisy    sbx   one that burns CPU
set -euo pipefail

# shellcheck source=demo/lib.sh
. "$(dirname "$0")/lib.sh"

WORK=$(mktemp -d "${TMPDIR:-/tmp}/coolseed.XXXXXX")
GITOPS=$WORK/cool-gitops
trap stop EXIT

build
mkdir -p "$GITOPS"
(cd "$ROOT/cool-gitops" && tar -cf - --exclude=./apps --exclude=./.git .) | tar -xf - -C "$GITOPS"
mkdir -p "$GITOPS/apps"
start "$GITOPS"

REPO=CoolEngOrg/payments-checkout
cd "$ROOT/examples/checkout"
as_ops app-create dev coolctl register -app payments/checkout -repo "$REPO" \
	-identity dev=aaaaaaaa-0000-4000-8000-00000000d0e1 \
	-identity tst=aaaaaaaa-0000-4000-8000-0000000075f2 \
	-identity prd=aaaaaaaa-0000-4000-8000-00000000b4d3 \
	-issue "$ISSUES/101"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest a)" -sha "$(commit a)"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest b)" -sha "$(commit b)"
as_ops deploy-promote tst coolctl promote -from dev -to tst -issue "$ISSUES/102"
as_ops deploy-promote prd coolctl promote -from tst -to prd -issue "$ISSUES/103"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest c)" -sha "$(commit c)"
as_ci "$REPO" coolctl deploy -env dev -pr 42 -digest "$(digest d)" -sha "$(commit d)"
as_ops deploy-promote tst coolctl promote -from dev -to tst -issue "$ISSUES/104"
as_ops env-var prd coolctl config -env prd -set LOG_LEVEL=warn -issue "$ISSUES/107"
as_ops domain prd coolctl config -env prd -add-domain pay.coolshop.example -issue "$ISSUES/108"
as_ops rolling-release prd coolctl weights -env prd -from tst -weight 10 -issue "$ISSUES/105"

# The simulators run on the sandbox cluster. The platform team deploys them
# with the execute-cool-sandbox workflow, in the GitHub environment cool-sbx.
SIM=CoolEngOrg/coolctl
for name in steady flaky noisy; do
	as_ops sandbox sbx coolctl register -env sbx -app "sim/appsim-$name" -repo "$SIM"
	as_ops sandbox sbx coolctl deploy -env sbx -f "$ROOT/demo/appsim/$name/cool.json" \
		-digest "$(digest e)" -sha "$(commit e)"
done
# A canary on the flaky one, for the gate lab.
as_ops sandbox sbx coolctl deploy -env sbx -f "$ROOT/demo/appsim/flaky/cool.json" \
	-digest "$(digest f)" -sha "$(commit f)"
as_ops sandbox sbx coolctl weights -app sim/appsim-flaky -env sbx -weight 20

rm -rf "$ROOT/cool-gitops/apps"
cp -R "$GITOPS/apps" "$ROOT/cool-gitops/apps"
echo
echo "seeded $(find "$ROOT/cool-gitops/apps" -name '*.json' | wc -l | tr -d ' ') files into cool-gitops/apps"
