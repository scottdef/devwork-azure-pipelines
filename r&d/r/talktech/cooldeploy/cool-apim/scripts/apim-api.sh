#!/usr/bin/env bash
# Import one API from a directory under apis/ and set its policy.
#
#   scripts/apim-api.sh --env dev --dir apis/payments/checkout [--dry-run]
#
# The directory holds api.yaml and openapi.yaml, and may hold policy.xml.
# api.yaml lists the environments the API is published in; the script does
# nothing for an environment that is not on the list.
#
# Without a policy.xml the policy is built from api.yaml's auth:
#
#   entra-jwt      callers send an Entra ID token; no subscription key
#   subscription   callers send a subscription key of the product
#   none           open to whoever can reach the gateway
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"

env="" dir=""
while [ $# -gt 0 ]; do
	case $1 in
	--env) env=${2:-}; shift 2 ;;
	--dir) dir=${2:-}; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	*) die "usage: apim-api.sh --env ENV --dir DIR [--dry-run]" ;;
	esac
done
load_instance "$env"
dir=${dir%/}
[ -f "$dir/api.yaml" ] || die "$dir/api.yaml not found"
[ -f "$dir/openapi.yaml" ] || die "$dir/openapi.yaml not found"

api=$(yaml2json "$dir/api.yaml")
id=$(jq -rM .name <<<"$api")
if ! jq -eM --arg env "$COOL_ENV" '(.environments // []) | index($env)' <<<"$api" >/dev/null; then
	log "skip $id: not published in $COOL_ENV"
	exit 0
fi
[[ $id =~ ^[a-z][a-z0-9-]{1,60}$ ]] || die "$dir/api.yaml: name \"$id\" must be lower case letters, digits and hyphens"

path=$(jq -rM .path <<<"$api")
product=$(jq -rM .product <<<"$api")
auth=$(jq -rM '.auth // "entra-jwt"' <<<"$api")
case $auth in entra-jwt | subscription | none) ;; *) die "$dir/api.yaml: auth \"$auth\": want entra-jwt, subscription or none" ;; esac

# Where requests go: a named backend, or the app's own host in this
# environment, which only exists inside the network.
backend=$(jq -rM '.backend // ""' <<<"$api")
if [ -n "$backend" ]; then
	service_url=$(jq -rM --arg id "$backend" '.backends[$id] // ""' <<<"$INSTANCE")
	[ -n "$service_url" ] || die "instances/$COOL_ENV.yaml has no URL for backend $backend"
else
	app=$(jq -rM '.app // ""' <<<"$api")
	[ -n "$app" ] || die "$dir/api.yaml needs either app or backend"
	service_url=https://$app.$DOMAIN$(jq -rM '.backendPath // ""' <<<"$api")
fi

subscription=false
if [ "$auth" = subscription ]; then
	subscription=true
fi

change "import API $id at /$path from $dir/openapi.yaml, backend $service_url" \
	az apim api import --service-name "$APIM_NAME" --resource-group "$APIM_RG" \
	--api-id "$id" --path "$path" \
	--display-name "$(jq -rM '.displayName // .name' <<<"$api")" \
	--specification-format OpenApi --specification-path "$dir/openapi.yaml" \
	--service-url "$service_url" --protocols https \
	--subscription-required "$subscription"

change "put API $id in product $product" \
	az apim product api add --service-name "$APIM_NAME" --resource-group "$APIM_RG" \
	--product-id "$product" --api-id "$id"

if [ -f "$dir/policy.xml" ]; then
	body=$(policy_body "$dir/policy.xml")
else
	fragments='<include-fragment fragment-id="cool-rate-limit" />'
	if [ "$auth" = entra-jwt ]; then
		fragments='<include-fragment fragment-id="cool-jwt" />'$'\n    '$fragments
	fi
	xml=$(
		cat <<EOF
<policies>
  <inbound>
    <base />
    $fragments
  </inbound>
  <backend>
    <base />
  </backend>
  <outbound>
    <base />
  </outbound>
  <on-error>
    <base />
  </on-error>
</policies>
EOF
	)
	body=$(jq -nM --arg xml "$xml" '{properties: {format: "xml", value: $xml}}')
fi
put "set the policy of API $id ($auth)" "apis/$id/policies/policy" "$body"
