#!/usr/bin/env bash
# cool-addon: ask for a managed backing service for an app.
#
# Add-ons are infrastructure, and infrastructure is Terraform's. This script
# does not create anything in Azure. It adds one line to the list that the
# cool-infra stack 70-addons builds from, and opens a pull request. Merging
# it runs the stack, behind that repository's own review and approval.
#
# Needs gh and git (GH_TOKEN: an App token that may push branches and open
# pull requests on cool-infra).
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

ORG=${COOL_ORG:-CoolEngOrg}
INFRA=${COOL_INFRA_REPO:-$ORG/cool-infra}
LIST=stacks/70-addons/addons.json

app=$(need app "$RE_APP")
env=$(need environment 'dev|tst|prd')
addon=$(need addon 'redis|postgres|storage|ai')
size=$(need size 'small|medium|large')
team=${app%%/*} name=${app##*/}
branch=addon/$team-$name-$env-$addon
issue=${COOL_ISSUE_URL:-}

entry=$(jq -nM --arg team "$team" --arg app "$name" --arg env "$env" --arg addon "$addon" --arg size "$size" \
	'{team: $team, app: $app, environment: $env, addon: $addon, size: $size}')

if [ "$DRY_RUN" = 1 ]; then
	echo "+ add $(jq -cM . <<<"$entry") to $INFRA/$LIST on branch $branch and open a pull request" >&2
	url=https://github.com/$INFRA/pull/0
else
	work=$(mktemp -d)
	gh repo clone "$INFRA" "$work/infra" -- --quiet --depth 1
	git -C "$work/infra" switch --quiet -c "$branch"
	# One add-on of a kind per app and environment: a second request resizes it.
	jq -M --argjson e "$entry" '
		[.[] | select((.team == $e.team and .app == $e.app and .environment == $e.environment and .addon == $e.addon) | not)]
		+ [$e] | sort_by(.team, .app, .environment, .addon)' "$work/infra/$LIST" >"$work/addons.json"
	mv "$work/addons.json" "$work/infra/$LIST"
	git -C "$work/infra" add "$LIST"
	git -C "$work/infra" -c user.name="cooldeploy[bot]" -c user.email="cooldeploy[bot]@users.noreply.github.com" \
		commit --quiet -m "Add-on $addon ($size) for $app in $env" -m "Requested in $issue"
	git -C "$work/infra" push --quiet --force origin "$branch"
	body=$(
		cat <<EOF
Adds a \`$size\` \`$addon\` add-on for \`$app\` in \`$env\`.

Requested and approved in $issue. Merging runs \`stacks/70-addons\`.
EOF
	)
	url=$(gh pr create --repo "$INFRA" --head "$branch" --title "Add-on $addon ($size) for $app in $env" --body "$body")
fi

report "Opened a pull request for a \`$size\` \`$addon\` add-on for \`$app\` in \`$env\`: $url"
report ""
case $addon in
redis) report "When it is merged and applied, the connection string is in the team Key Vault as \`$name-REDIS-URL\`. Add \`REDIS_URL\` to \`secrets\` in \`cool.json\`." ;;
postgres) report "When it is merged and applied, the connection string is in the team Key Vault as \`$name-DATABASE-URL\`. Add \`DATABASE_URL\` to \`secrets\` in \`cool.json\`." ;;
storage) report "When it is merged and applied, the app's identity can read and write the container \`$name\`; its URL is in the team Key Vault as \`$name-STORAGE-URL\`. No key is involved." ;;
ai) report "When it is merged and applied, the app's identity may call the \`cool-ai\` API in API Management with a \`$size\` token budget. Add \`\"ai\": {\"budget\": \"$size\"}\` to \`cool.json\`. No key is involved." ;;
esac
