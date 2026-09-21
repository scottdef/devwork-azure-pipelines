#!/usr/bin/env bash
# budget.sh - the only code that talks to the billing budgets API.
#
#   budget.sh get   USER        effective AI-credit budget for USER, as JSON
#   budget.sh plan  USER INC    what an increase of INC units would do; exit 3 if policy refuses
#   budget.sh apply USER INC    do it, read it back, print the result as JSON
#   budget.sh reset-date        next usage reset, YYYY-MM-DD
#
# DRY_RUN=1 prints the write instead of sending it.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq date
refuse() { printf 'refused: %s\n' "$*" >&2; exit 3; } # a policy "no", not a fault
B="/enterprises/$ENTERPRISE/settings/billing/budgets"

# All AI-credit budgets of one scope, as a JSON array.
# The API pages by has_next_page in the body, not by Link header, so no --paginate.
budgets() {
	local page=1 out='[]' r
	while :; do
		r=$(ent "$B?per_page=100&page=$page&scope=$1${2:+&user=$2}")
		out=$(jq -c --argjson a "$out" '$a + (.budgets // [])' <<<"$r")
		[ "$(jq -r '.has_next_page // false' <<<"$r")" = true ] || break
		page=$((page + 1))
	done
	# List responses carry budget_product_skus[]; single responses budget_product_sku.
	jq -c '[.[] | select([.budget_product_skus[]?, .budget_product_sku] | index("ai_credits"))]' <<<"$out"
}

# Precedence is GitHub's: individual override > cost-center per-user > universal.
cmd_get() {
	local user=$1 u j id amt scope
	u=$(lc "$user")
	j=$(budgets user "$user" | jq -c --arg u "$u" \
		'[.[] | select(((.user // .budget_entity_name // "") | ascii_downcase) == $u)][0] // empty')
	if [ -n "$j" ]; then
		jq -c '{id, usd: .budget_amount, source: "user"}' <<<"$j"
		return
	fi
	for scope in multi_user_cost_center multi_user_customer; do
		while read -r id amt; do
			[ -n "$id" ] || continue
			j=$(ent "$B/$id/user-states?user=$user" --jq '.user_states[0] // empty' 2>/dev/null || true)
			# A cost-center budget covers the user only if it holds a state row for them.
			# The universal budget covers everyone, row or no row.
			if [ -n "$j" ]; then
				jq -c --arg s "$scope" \
					'{id: null, usd: .target_amount, consumed: .consumed_amount, source: $s}' <<<"$j"
				return
			elif [ "$scope" = multi_user_customer ]; then
				jq -nc --argjson a "$amt" '{id: null, usd: $a, source: "multi_user_customer"}'
				return
			fi
		done < <(budgets "$scope" | jq -r '.[] | "\(.id) \(.budget_amount)"')
	done
	jq -nc --argjson a "$(usd "$BASELINE")" '{id: null, usd: $a, source: "baseline"}'
}

cmd_plan() {
	local user=$1 inc=$2 j cur cur_u new
	[[ "$inc" =~ ^[0-9]+$ ]] || refuse "increase must be a positive integer"
	[ "$inc" -ge "$STEP" ] && [ "$inc" -le "$MAX" ] || refuse "increase must be between $STEP and $MAX"
	[ $((inc % STEP)) -eq 0 ] || refuse "increase must be a multiple of $STEP"
	j=$(cmd_get "$user")
	cur=$(jq -r '.usd | floor' <<<"$j")
	cur_u=$((cur * 100 / CENTS_PER_UNIT))
	new=$((cur_u + inc))
	[ "$new" -le "$MAX" ] || refuse "current budget $cur_u + $inc = $new exceeds the ceiling of $MAX"
	[ $((new * CENTS_PER_UNIT % 100)) -eq 0 ] ||
		refuse "$new units is not a whole-dollar amount; the budgets API accepts whole dollars only"
	jq -c --arg user "$user" --argjson inc "$inc" --argjson cur "$cur_u" --argjson new "$new" \
		--argjson usd "$(usd "$new")" --argjson cr "$(credits "$new")" \
		'{user: $user, increase: $inc, current: $cur, new: $new, new_usd: $usd, new_credits: $cr,
		  budget_id: .id, source, consumed_usd: (.consumed // null)}' <<<"$j"
}

cmd_apply() {
	local user=$1 inc=$2 p id usd body got
	p=$(cmd_plan "$user" "$inc")
	id=$(jq -r '.budget_id // empty' <<<"$p")
	usd=$(jq -r .new_usd <<<"$p")
	if [ -n "$id" ]; then
		body=$(jq -nc --argjson a "$usd" --arg e "$OVERRIDE_EXPIRES" \
			'{budget_amount: $a} + (if $e != "" then {expires_at: $e} else {} end)')
		set -- -X PATCH "$B/$id"
	else
		# "user" is required for user scope although the published schema omits it.
		# For this scope alerting must be off and prevent_further_usage must be true.
		body=$(jq -nc --argjson a "$usd" --arg u "$user" --arg e "$OVERRIDE_EXPIRES" \
			'{budget_amount: $a, prevent_further_usage: true, budget_scope: "user",
			  budget_entity_name: "", budget_type: "BundlePricing", budget_product_sku: "ai_credits",
			  user: $u, budget_alerting: {will_alert: false, alert_recipients: []}}
			 + (if $e != "" then {expires_at: $e} else {} end)')
		set -- -X POST "$B"
	fi
	if [ "${DRY_RUN:-0}" = 1 ]; then
		echo "dry-run: gh api $* <<< $body" >&2
	else
		id=$(ent "$@" --input - <<<"$body" | jq -r '.budget.id // empty')
		[ -n "$id" ] || die "budget write returned no budget id"
		got=$(ent "$B/$id" --jq .budget_amount) # trust, but read it back
		[ "$got" = "$usd" ] || die "verification failed: wrote $usd, read back $got (budget $id)"
	fi
	jq -c --arg id "$id" --arg r "$(cmd_reset_date)" --arg e "$OVERRIDE_EXPIRES" \
		'. + {budget_id: $id, reset_date: $r, expires_at: (if $e == "" then null else $e end)}' <<<"$p"
}

cmd_reset_date() {
	local y m d
	[ "$CYCLE_DAY" -ge 1 ] && [ "$CYCLE_DAY" -le 28 ] || die "CYCLE_DAY must be 1..28"
	read -r y m d < <(date -u -d "${NOW:-now}" '+%Y %-m %-d')
	if [ "$d" -ge "$CYCLE_DAY" ]; then
		m=$((m + 1))
		if [ "$m" -gt 12 ]; then m=1; y=$((y + 1)); fi
	fi
	printf '%04d-%02d-%02d\n' "$y" "$m" "$CYCLE_DAY"
}

case "${1:-}" in
get)        [ $# -eq 2 ] || die "usage: budget.sh get USER";       cmd_get "$2" ;;
plan)       [ $# -eq 3 ] || die "usage: budget.sh plan USER INC";  cmd_plan "$2" "$3" ;;
apply)      [ $# -eq 3 ] || die "usage: budget.sh apply USER INC"; cmd_apply "$2" "$3" ;;
reset-date) cmd_reset_date ;;
*)          die "usage: budget.sh get|plan|apply|reset-date" ;;
esac
