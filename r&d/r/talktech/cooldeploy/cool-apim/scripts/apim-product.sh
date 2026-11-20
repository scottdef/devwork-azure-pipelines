#!/usr/bin/env bash
# Create or update one product from a file in products/.
#
#   scripts/apim-product.sh --env dev --file products/payments.yaml [--dry-run]
#
# A product groups APIs and says whether callers need a subscription key and
# whether a person must approve each subscription.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"

env="" file=""
while [ $# -gt 0 ]; do
	case $1 in
	--env) env=${2:-}; shift 2 ;;
	--file) file=${2:-}; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	*) die "usage: apim-product.sh --env ENV --file FILE [--dry-run]" ;;
	esac
done
load_instance "$env"

product=$(yaml2json "$file")
id=$(jq -rM .name <<<"$product")
[[ $id =~ ^[a-z][a-z0-9-]{1,30}$ ]] || die "$file: product name \"$id\" must be lower case letters, digits and hyphens"

args=(--service-name "$APIM_NAME" --resource-group "$APIM_RG" --product-id "$id"
	--product-name "$(jq -rM '.displayName // .name' <<<"$product")"
	--description "$(jq -rM '.description // ""' <<<"$product")"
	--subscription-required "$(jq -rM '.subscriptionRequired // false' <<<"$product")"
	--state published)
if [ "$(jq -rM '.subscriptionRequired // false' <<<"$product")" = true ]; then
	args+=(--approval-required "$(jq -rM '.approvalRequired // true' <<<"$product")"
		--subscriptions-limit "$(jq -rM '.subscriptionsLimit // 1' <<<"$product")")
fi

if exists az apim product show --service-name "$APIM_NAME" --resource-group "$APIM_RG" --product-id "$id"; then
	change "update product $id" az apim product update "${args[@]}"
else
	change "create product $id" az apim product create "${args[@]}"
fi
