#!/usr/bin/env bash
# cool-api-publish: publish an app's OpenAPI description through APIM.
#
# API Management is configured from files in cool-apim, by scripts. This
# script writes two of those files, apis/<team>/<app>/api.yaml and
# openapi.yaml, and opens a pull request. Merging it runs the sync.
#
# The OpenAPI file is taken from the app's repository at the commit of the
# release that is live in the environment, so what is published is what runs.
#
# Needs gh, git and coolctl (GH_TOKEN: an App token that may read the app's
# repository and open pull requests on cool-apim).
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

ORG=${COOL_ORG:-CoolEngOrg}
APIM=${COOL_APIM_REPO:-$ORG/cool-apim}
DOMAIN=${COOL_DOMAIN:-coolapps.example}

app=$(need app "$RE_APP")
env=$(need environment 'dev|tst|prd')
product=$(need product '[a-z][a-z0-9-]{1,30}')
path=$(optional path '[a-z0-9][a-z0-9/-]{0,60}[a-z0-9]')
auth=$(need auth 'entra-jwt|subscription|none')
team=${app%%/*} name=${app##*/}
path=${path:-$team/$name}
branch=api/$team-$name
issue=${COOL_ISSUE_URL:-}

# What is live, and where its source is.
status=$(coolctl status -app "$app" -json)
repo=$(jq -rM .repo <<<"$status")
sha=$(jq -rM --arg env "$env" '.environments[] | select(.environment == $env) | .releases | max_by(.weight) | .sha // ""' <<<"$status")
[ -n "$sha" ] || die "$app has no release in $env to publish an API for"

# envs LIST adds $env to a comma-separated list, in promotion order.
envs() {
	local have=",$1," out=() e
	for e in dev tst prd; do
		if [ "$e" = "$env" ] || [[ $have == *",$e,"* ]]; then
			out+=("$e")
		fi
	done
	local IFS=,
	printf '%s' "${out[*]}"
}

if [ "$DRY_RUN" = 1 ]; then
	echo "+ copy openapi from $repo@$sha, write apis/$team/$name/api.yaml in $APIM, open a pull request" >&2
	url=https://github.com/$APIM/pull/0
else
	work=$(mktemp -d)
	spec=$(gh api "repos/$repo/contents/cool.json?ref=$sha" --jq .content | base64 -d | jq -rM '.api.openapi // ""')
	[ -n "$spec" ] || die "cool.json at $sha names no OpenAPI file; add \"api\": {\"openapi\": \"openapi.yaml\"}"
	[[ $spec =~ ^[A-Za-z0-9._/-]+$ && $spec != *..* ]] || die "the OpenAPI path \"$spec\" in cool.json is not a plain relative path"
	gh api "repos/$repo/contents/$spec?ref=$sha" --jq .content | base64 -d >"$work/openapi.yaml"
	[ -s "$work/openapi.yaml" ] || die "$spec at $sha is empty"

	gh repo clone "$APIM" "$work/apim" -- --quiet --depth 1
	git -C "$work/apim" switch --quiet -c "$branch"
	dir=$work/apim/apis/$team/$name
	mkdir -p "$dir"
	have=""
	if [ -f "$dir/api.yaml" ]; then
		have=$(sed -n 's/^environments: \[\(.*\)\]$/\1/p' "$dir/api.yaml" | tr -d ' ')
	fi
	cp "$work/openapi.yaml" "$dir/openapi.yaml"
	# This file is written by cool-api-publish requests. Its shape is fixed
	# so that the next request can read it back with sed.
	cat >"$dir/api.yaml" <<EOF
# Written by a cool-api-publish request. Change it with another request.
name: $team-$name
displayName: $team / $name
path: $path
product: $product
auth: $auth
team: $team
app: $name
backendPath: /api
source: $repo@$sha
environments: [$(envs "$have" | sed 's/,/, /g')]
EOF
	git -C "$work/apim" add "apis/$team/$name"
	git -C "$work/apim" -c user.name="cooldeploy[bot]" -c user.email="cooldeploy[bot]@users.noreply.github.com" \
		commit --quiet -m "Publish the API of $app in $env" -m "Requested in $issue"
	git -C "$work/apim" push --quiet --force origin "$branch"
	body=$(
		cat <<EOF
Publishes the API of \`$app\` in \`$env\`, at \`/$path\`, in product \`$product\`, with \`$auth\`.

The OpenAPI file is \`$spec\` from \`$repo\` at \`$sha\`, the release live in \`$env\`.
Requested and approved in $issue. Merging runs the API Management sync.
EOF
	)
	url=$(gh pr create --repo "$APIM" --head "$branch" --title "Publish the API of $app in $env" --body "$body")
fi

report "Opened a pull request to publish the API of \`$app\` in \`$env\`: $url"
report ""
report "| | |"
report "| --- | --- |"
report "| Path | \`/$path\` |"
report "| Product | \`$product\` |"
report "| Callers prove themselves with | \`$auth\` |"
report "| Backend | \`https://$name.$env.$DOMAIN\` |"
report "| OpenAPI from | \`$repo@${sha:0:12}\` |"
