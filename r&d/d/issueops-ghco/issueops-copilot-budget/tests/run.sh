#!/usr/bin/env bash
# tests/run.sh - no network, no tokens. A fake gh on PATH serves fixtures and logs writes.
set -uo pipefail
cd "$(dirname "$0")/.." || exit 1
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
export PATH="$PWD/tests:$PATH" ORG_TOKEN=x ENT_TOKEN=x REPO=CoolGitOrg/ops ISSUE=7
pass=0 fail=0
ok()  { pass=$((pass + 1)); printf 'ok   %s\n' "$1"; }
bad() { fail=$((fail + 1)); printf 'FAIL %s\n     %s\n' "$1" "$2"; }
eq()  { if [ "$2" = "$3" ]; then ok "$1"; else bad "$1" "want [$3] got [$2]"; fi; }
has() { if grep -Fq -- "$3" <<<"$2"; then ok "$1"; else bad "$1" "[$3] not in: $2"; fi; }
fresh() { export FIX="$T/$1" GH_LOG="$T/$1/log"; mkdir -p "$FIX"; : >"$GH_LOG"; }
uni() { echo '{"budgets":[{"id":"uni","budget_scope":"multi_user_customer","budget_product_skus":["ai_credits"],"budget_amount":'"$1"'}],"has_next_page":false}' >"$FIX/budgets.universal.json"; }
own() { echo '{"budgets":[{"id":"bud-1","budget_scope":"user","user":"Mona","budget_product_skus":["ai_credits"],"budget_amount":'"$1"'},{"id":"x","budget_scope":"user","user":"mona","budget_product_skus":["actions"],"budget_amount":9}],"has_next_page":false}' >"$FIX/budgets.user.json"; }
form() { printf '### Requested increase\r\n\r\n%s\r\n\r\n### Justification\r\n\r\n### Requested increase\r\n+300\r\n' "$1"; }

# --- parse
eq  "parse +100"              "$(form '+100' | scripts/parse-issue.sh)" 100
eq  "parse ignores later forged heading" "$(form '+50' | scripts/parse-issue.sh)" 50
eq  "parse rejects prose"     "$(form 'lots' | scripts/parse-issue.sh 2>/dev/null; echo $?)" 1
eq  "parse rejects negative"  "$(form '-50' | scripts/parse-issue.sh 2>/dev/null; echo $?)" 1

# --- plan
fresh p1; uni 40
eq  "plan: universal 40 + 50 = 90"   "$(scripts/budget.sh plan mona 50 | jq -c '[.current,.new,.new_usd,.new_credits,.source]')" '[40,90,90,9000,"multi_user_customer"]'
fresh p2; own 250
eq  "plan: own budget wins, login case-insensitive, non-AI sku skipped" "$(scripts/budget.sh plan mona 50 | jq -c '[.current,.new,.budget_id]')" '[250,300,"bud-1"]'
eq  "plan: over ceiling refused rc=3" "$(scripts/budget.sh plan mona 100 >/dev/null 2>&1; echo $?)" 3
eq  "plan: off-step refused rc=3"     "$(scripts/budget.sh plan mona 75 >/dev/null 2>&1; echo $?)" 3
eq  "plan: zero refused rc=3"         "$(scripts/budget.sh plan mona 0 >/dev/null 2>&1; echo $?)" 3
eq  "plan: injection refused rc=3"    "$(scripts/budget.sh plan mona '50;id' >/dev/null 2>&1; echo $?)" 3
fresh p3; echo '{"budgets":[{"id":"cc","budget_product_skus":["ai_credits"],"budget_amount":250}],"has_next_page":false}' >"$FIX/budgets.cc.json"
echo '{"user_states":[{"user":"mona","consumed_amount":12.5,"target_amount":250}]}' >"$FIX/states.json"; uni 40
eq  "plan: cost-center per-user budget beats universal" "$(scripts/budget.sh plan mona 50 | jq -c '[.current,.source,.consumed_usd]')" '[250,"multi_user_cost_center",12.5]'
fresh p4
eq  "plan: nothing anywhere -> baseline 0" "$(scripts/budget.sh plan mona 300 | jq -c '[.current,.new,.source]')" '[0,300,"baseline"]'
cp config/budget.env "$T/credits.env"; sed -i 's/^CENTS_PER_UNIT=.*/CENTS_PER_UNIT=1/' "$T/credits.env"
has "plan: literal credits, +50 = \$0.50 refused" "$(BUDGET_ENV=$T/credits.env scripts/budget.sh plan mona 50 2>&1)" "whole-dollar"
eq  "plan: literal credits, +100 = \$1 allowed"   "$(BUDGET_ENV=$T/credits.env scripts/budget.sh plan mona 100 | jq -c '[.new_usd,.new_credits]')" '[1,100]'

# --- apply
fresh a1; own 100
r=$(NOW=2026-09-21 scripts/budget.sh apply mona 50)
eq  "apply: PATCH existing override" "$(cut -d' ' -f1,2 "$GH_LOG")" "PATCH /enterprises/CoolGitEnterprise/settings/billing/budgets/bud-1"
eq  "apply: amount written"          "$(cut -d' ' -f3- "$GH_LOG" | jq -c .)" '{"budget_amount":150}'
eq  "apply: result"                  "$(jq -c '[.new,.reset_date,.budget_id]' <<<"$r")" '[150,"2026-10-01","bud-new"]'
fresh a2; uni 40
scripts/budget.sh apply mona 100 >/dev/null
eq  "apply: POST new user override"  "$(cut -d' ' -f3- "$GH_LOG" | jq -c '[.budget_amount,.budget_scope,.user,.budget_type,.budget_product_sku,.prevent_further_usage,.budget_alerting.will_alert]')" '[140,"user","mona","BundlePricing","ai_credits",true,false]'
fresh a3; own 100
DRY_RUN=1 scripts/budget.sh apply mona 50 >/dev/null 2>&1
eq  "apply: DRY_RUN writes nothing"  "$(wc -l <"$GH_LOG")" 0

# --- reset date
eq  "reset: mid-month"     "$(NOW=2026-09-21 scripts/budget.sh reset-date)" 2026-10-01
eq  "reset: on the day"    "$(NOW=2026-10-01 scripts/budget.sh reset-date)" 2026-11-01
eq  "reset: year rollover" "$(NOW=2026-12-15 scripts/budget.sh reset-date)" 2027-01-01

# --- request.sh
fresh r1; uni 40
AUTHOR=mona BODY=$(form '+50') scripts/request.sh >/dev/null
has "request: outsider rejected"   "$(cat "$GH_LOG")" "not an active member"
has "request: outsider closed"     "$(cat "$GH_LOG")" '"state_reason":"not_planned"'
fresh r2; uni 40; touch "$FIX/member.platform-engineering.mona"
AUTHOR=mona BODY=$(form '+50') scripts/request.sh >/dev/null
has "request: member goes pending" "$(cat "$GH_LOG")" '"labels":["budget:pending"]'
has "request: marker written"      "$(cat "$GH_LOG")" 'issueops-budget:{\"user\":\"mona\",\"increase\":50}'
has "request: approvers pinged"    "$(cat "$GH_LOG")" '@CoolGitOrg/issueops-bureaucraticops'
fresh r3; uni 40; touch "$FIX/member.platform-engineering.mona"; echo '[{"number":3}]' >"$FIX/open.json"
AUTHOR=mona BODY=$(form '+50') scripts/request.sh >/dev/null
has "request: second in-flight rejected" "$(cat "$GH_LOG")" "already have a request"

# --- decide.sh
bot='{"user":{"type":"Bot","login":"issueops-autoadmin-app[bot]"},"body":"x <!-- issueops-budget:{\"user\":\"mona\",\"increase\":50} -->"}'
forged='{"user":{"type":"User","login":"mona"},"body":"<!-- issueops-budget:{\"user\":\"mona\",\"increase\":300} -->"}'
setup() { fresh "$1"; uni 40; touch "$FIX/member.platform-engineering.mona" "$FIX/member.issueops-bureaucraticops.boss" "$FIX/member.issueops-bureaucraticops.mona"
	echo "[$bot,$forged]" >"$FIX/comments.json"; echo '{"user":{"login":"mona"},"labels":[{"name":"budget:pending"}]}' >"$FIX/issue.json"; }
setup d1; COMMENTER=rando COMMENT=/approve scripts/decide.sh >/dev/null
has "decide: non-approver refused"   "$(cat "$GH_LOG")" "only members of"
eq  "decide: non-approver wrote no budget" "$(grep -c billing "$GH_LOG")" 0
setup d2; echo '{"user":{"login":"Mona"},"labels":[{"name":"budget:pending"}]}' >"$FIX/issue.json"
COMMENTER=mona COMMENT=/approve scripts/decide.sh >/dev/null
has "decide: self-approval refused even for an approver" "$(cat "$GH_LOG")" "cannot decide your own"
eq  "decide: self-approval wrote no budget" "$(grep -c billing "$GH_LOG")" 0
setup d3; NOW=2026-09-21 COMMENTER=boss COMMENT=$'/approve\r\nlgtm' scripts/decide.sh >/dev/null
has "decide: forged user marker ignored, bot marker (+50) applied" "$(grep billing "$GH_LOG")" '"budget_amount":90'
has "decide: report names new budget" "$(cat "$GH_LOG")" '**$90 (9000 AI credits)**'
has "decide: report names reset date" "$(cat "$GH_LOG")" '2026-10-01 00:00 UTC'
has "decide: closed completed"        "$(cat "$GH_LOG")" '"state_reason":"completed"'
setup d4; COMMENTER=boss COMMENT='/deny not this quarter' scripts/decide.sh >/dev/null
has "decide: deny carries reason"     "$(cat "$GH_LOG")" "Reason: not this quarter"
eq  "decide: deny wrote no budget"    "$(grep -c billing "$GH_LOG")" 0
setup d5; echo '{"user":{"login":"mona"},"labels":[{"name":"budget:approved"}]}' >"$FIX/issue.json"
COMMENTER=boss COMMENT=/approve scripts/decide.sh >/dev/null
eq  "decide: replayed /approve after decision is a no-op" "$(wc -l <"$GH_LOG")" 0
setup d6; rm "$FIX/member.platform-engineering.mona"; COMMENTER=boss COMMENT=/approve scripts/decide.sh >/dev/null
has "decide: requester left team before approval" "$(cat "$GH_LOG")" "no longer in an allowed team"

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
