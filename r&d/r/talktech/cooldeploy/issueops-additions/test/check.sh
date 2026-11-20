#!/usr/bin/env bash
# Check the CoolDeploy additions to IssueOps.
#
#   1. forms, registry and sample issues agree with each other
#   2. every request script does what it says, against a real cool-api
#      running locally (needs ../demo/lib.sh and Go; skipped otherwise)
#
#   test/check.sh
set -euo pipefail

here=$(cd "$(dirname "$0")/.." && pwd)
cd "$here"
fail=0
bad() { echo "FAIL  $*" >&2; fail=1; }
ok() { echo "ok    $*"; }

# --- 1. Static checks ------------------------------------------------------

jq -eM . config/issueops.cool.json config/labels.cool.json config/cool-fields.json >/dev/null

while IFS= read -r id; do
	form=.github/ISSUE_TEMPLATE/$id.yml
	[ -f "$form" ] || { bad "$id has no form"; continue; }
	[ -f ".github/workflows/execute-$id.yml" ] || bad "$id has no execute workflow"
	[ -x "scripts/cool/${id#cool-}.sh" ] || bad "$id has no executable script"
	[ -f "examples/issues/$id.md" ] || bad "$id has no sample issue"
	jq -eM --arg l "issueops:$id" 'any(.[]; .name == $l)' config/labels.cool.json >/dev/null || bad "$id has no label"

	# The ids in the form are the fields the scripts read.
	want=$(jq -rM --arg id "$id" '.[$id] | join(" ")' config/cool-fields.json)
	got=$(awk '$1 == "id:" { printf "%s%s", sep, $2; sep = " " }' "$form")
	[ "$got" = "$want" ] || bad "$form has fields [$got], config/cool-fields.json says [$want]"

	# A field named in a policy condition must exist in the form.
	while IFS= read -r field; do
		case " $want " in *" $field "*) ;; *) bad "$id: policy names field \"$field\", which the form does not have" ;; esac
	done < <(jq -rM --arg id "$id" '.request_types[] | select(.id == $id) | [.. | objects | select(has("field")) | .field] | unique[]' config/issueops.cool.json)
done < <(jq -rM '.request_types[].id' config/issueops.cool.json)
if [ "$fail" = 0 ]; then ok "forms, registry, labels, workflows and scripts line up"; fi

# A heading in an issue is the label of a form field. The scripts read
# fields by id, so each label must reduce to its id.
if command -v coolctl >/dev/null; then
	for body in examples/issues/*.md; do
		id=$(basename "$body" .md)
		want=$(jq -rM --arg id "$id" '.[$id] | sort | join(" ")' config/cool-fields.json)
		got=$(coolctl issue fields -f "$body" | jq -rM 'keys | join(" ")')
		[ "$got" = "$want" ] || bad "$body parses to [$got], want [$want]"
	done
	if [ "$fail" = 0 ]; then ok "sample issues parse into the fields of their forms"; fi
fi

# --- 2. The scripts, against a local cool-api --------------------------------

if [ ! -f ../demo/lib.sh ] || ! command -v go >/dev/null; then
	echo "skip  script tests: they need ../demo/lib.sh and Go"
	[ "$fail" = 0 ] || exit 1
	exit 0
fi

# shellcheck source=../demo/lib.sh
. ../demo/lib.sh
WORK=$(mktemp -d "${TMPDIR:-/tmp}/coolissueops.XXXXXX")
GITOPS=$WORK/cool-gitops
trap stop EXIT
build >/dev/null
mkdir -p "$GITOPS/apps"
cp -R "$ROOT/cool-gitops/teams" "$GITOPS/teams"
start "$GITOPS"
export COOL_WAIT=0 COOL_ISSUE_URL=$ISSUES/900
SIM_PORT=${COOL_DEMO_SIM_PORT:-9913}
REPO=CoolEngOrg/payments-checkout
values() { jq -rM "$2" "$GITOPS/apps/payments/checkout/$1/values.json"; }
releases() { values "$1" '[.releases[] | "\(.id):\(.weight)\(if .standby then "s" else "" end)"] | join(" ")'; }

# request TYPE ENV FIELDS-JSON [VAR=VALUE...] runs a request script the way
# the execute job does: with the fields, and a token for cool-<env>.
request() {
	local type=$1 env=$2 fields=$3
	shift 3
	next_run
	: >"$WORK/report.md"
	env "$@" COOL_FIELDS="$fields" COOL_REPORT="$WORK/report.md" \
		COOL_TOKEN="$(token "repository=CoolEngOrg/issueops" \
			"job_workflow_ref=CoolEngOrg/issueops/.github/workflows/execute-cool-request.yml@refs/heads/main" \
			"environment=cool-$env" "actor=mona" "run_id=$RUN_ID")" \
		"scripts/cool/$type.sh" >"$WORK/out.txt" 2>&1
}
# sample ID prints the fields of a sample issue as JSON.
sample() { coolctl issue fields -f "examples/issues/$1.md"; }
expect() { # expect WHAT GOT WANT
	if [ "$2" = "$3" ]; then ok "$1"; else bad "$1: got \"$2\", want \"$3\""; fi
}
refused() { # refused WHAT REASON COMMAND...: the request must fail, and say why
	local what=$1 reason=$2
	shift 2
	if "$@"; then
		bad "$what: should have been refused"
	elif grep -q "could not be carried out: .*$reason" "$WORK/report.md"; then
		ok "$what: refused"
	else
		bad "$what: refused, but the report does not mention \"$reason\": $(cat "$WORK/report.md")"
	fi
}

cd "$ROOT/examples/checkout"
as_ops app-create dev coolctl register -app payments/checkout -repo "$REPO" >/dev/null 2>&1
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest a)" -sha "$(commit a)" >/dev/null 2>&1
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest b)" -sha "$(commit b)" >/dev/null 2>&1
cd "$here"

request deploy-promote tst '{"app":"payments/checkout","from":"dev","environment":"tst","release":""}'
expect "promote dev to tst" "$(releases tst)" "rbbbbbbb:100"
request deploy-promote prd "$(sample cool-deploy-promote)"
expect "promote tst to prd (the sample issue)" "$(releases prd)" "rbbbbbbb:100"
refused "promote dev straight to prd" "from dev to tst, or from tst to prd" request deploy-promote prd '{"app":"payments/checkout","from":"dev","environment":"prd"}'
refused "promote with a job approved for tst only" "may not promote" request deploy-promote tst "$(sample cool-deploy-promote)"
refused "an app name with shell in it" "not what the form allows" request deploy-promote tst '{"app":"payments/checkout; rm -rf /","from":"dev","environment":"tst"}'

request env-var prd "$(sample cool-env-var)"
expect "set two variables in prd" "$(values prd '.overrides | to_entries | map("\(.key)=\(.value)") | join(" ")')" "CURRENCY=EUR LOG_LEVEL=warn"
request env-var prd '{"app":"payments/checkout","environment":"prd","set":"","unset":"CURRENCY\n"}'
expect "remove one" "$(values prd '.overrides | keys | join(" ")')" "LOG_LEVEL"
refused "a secret in an issue" "looks like a secret" request env-var dev '{"app":"payments/checkout","environment":"dev","set":"DB_PASSWORD=hunter2"}'
refused "a platform variable" "set by the platform" request env-var dev '{"app":"payments/checkout","environment":"dev","set":"PORT=9"}'
refused "a request that changes nothing" "sets nothing" request env-var dev '{"app":"payments/checkout","environment":"dev","set":"","unset":""}'

request domain prd "$(sample cool-domain)"
expect "add a custom domain" "$(values prd '.domains | join(" ")')" "pay.coolshop.example"
refused "a domain in the platform zone" "platform's own zone" request domain prd '{"app":"payments/checkout","environment":"prd","action":"add","domain":"evil.prd.coolapps.example"}'

# A rolling release that goes well: the checks are off, as in the sandbox.
cd "$ROOT/examples/checkout"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest c)" -sha "$(commit c)" >/dev/null 2>&1
cd "$here"
request deploy-promote tst '{"app":"payments/checkout","from":"dev","environment":"tst"}'
request rolling-release prd "$(sample cool-rolling-release | jq -cM '.settle = "0"')" COOL_GATE=off
expect "rolling release to 100%" "$(releases prd)" "rccccccc:100 rbbbbbbb:0s"
refused "stages that do not end at 100" "last stage must be 100" request rolling-release prd '{"app":"payments/checkout","from":"tst","environment":"prd","stages":"10, 50","settle":"0","error_budget":"1"}' COOL_GATE=off

# A rolling release that goes badly: appsim fails every request and also
# answers the gate's questions, so the gate must stop the release.
cd "$ROOT/examples/checkout"
as_ci "$REPO" coolctl deploy -env dev -digest "$(digest d)" -sha "$(commit d)" >/dev/null 2>&1
cd "$here"
request deploy-promote tst '{"app":"payments/checkout","from":"dev","environment":"tst"}'
PORT=$SIM_PORT SIM_LATENCY_MS=1 SIM_ERROR_RATE=1 "$BIN/appsim" >"$WORK/appsim.log" 2>&1 &
PIDS+=($!)
wait_http "http://127.0.0.1:$SIM_PORT/healthz"
for _ in $(seq 1 40); do curl -s -o /dev/null "http://127.0.0.1:$SIM_PORT/"; done
refused "a release outside its error budget" "left its error budget at 10%" request rolling-release prd \
	'{"app":"payments/checkout","from":"tst","environment":"prd","stages":"10, 100","settle":"0","error_budget":"1"}' \
	COOL_PROM_URL="http://127.0.0.1:$SIM_PORT" COOL_PROM_TOKEN=unused
expect "the failed release is out of traffic, on standby" "$(releases prd)" "rddddddd:0s rccccccc:100"
if grep -q 'stopped' "$WORK/report.md"; then
	ok "the report says where it stopped"
else
	bad "the report does not say where the release stopped"
fi

request deploy-rollback prd "$(sample cool-deploy-rollback)"
expect "rollback swaps live and standby" "$(releases prd)" "rddddddd:100 rccccccc:0s"
expect "the reason is in the commit" "$(git -C "$GITOPS" log -1 --format=%B | grep -c 'Checkout errors rose')" "1"
expect "the issue is in the commit" "$(git -C "$GITOPS" log -1 --format=%B | grep -c "Issue: $COOL_ISSUE_URL")" "1"

# Requests that reach GitHub and Azure run dry: the commands are printed,
# and the test checks that the fields were understood.
request app-create dev "$(sample cool-app-create)" DRY_RUN=1
expect "app-create plans a repository from the right template" "$(grep -c 'gh repo create CoolEngOrg/payments-ledger --internal --template CoolEngOrg/cool-template-go-api' "$WORK/out.txt")" "1"
expect "app-create plans three federated identities" "$(grep -c 'federated-credential create' "$WORK/out.txt")" "3"
expect "app-create federates the app's service account" "$(grep -c 'system:serviceaccount:payments-ledger:ledger' "$WORK/out.txt")" "3"
refused "an app name that looks like a preview" "may not end in -pr-" request app-create dev '{"team":"payments","app":"site-pr-7","preset":"static","expose":"internal"}' DRY_RUN=1
request addon dev "$(sample cool-addon)" DRY_RUN=1
expect "addon plans a pull request on cool-infra" "$(grep -c '"addon":"redis"' "$WORK/out.txt")" "1"
request api-publish dev "$(sample cool-api-publish)" DRY_RUN=1
expect "api-publish reads the live commit" "$(grep -c "openapi from $REPO@$(commit d)" "$WORK/out.txt")" "1"

refused "retire without the right confirmation" "does not match the app" request app-retire prd '{"app":"payments/checkout","confirm":"payments/check","reason":"x"}'
request app-retire prd '{"app":"payments/checkout","confirm":"payments/checkout","reason":"end of the test"}'
expect "retire removes every file" "$(find "$GITOPS/apps" -type f 2>/dev/null | wc -l | tr -d ' ')" "0"

[ "$fail" = 0 ] || { echo "check.sh: failures above; last output:" >&2; cat "$WORK/out.txt" >&2; exit 1; }
echo "issueops-additions: all checks passed"
