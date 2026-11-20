#!/usr/bin/env bash
# Create or update one API Management instance from instances/<env>.yaml.
#
#   scripts/apim-instance.sh --env dev [--dry-run]
#
# `az apim create` does not offer the v2 tiers, so the instance is a PUT to
# the management API. Creating one takes several minutes; the script returns
# when Azure has accepted the request and prints how to watch it.
#
# This runs rarely, by hand or from a workflow run started on purpose. The
# regular sync does not call it.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"

env=""
while [ $# -gt 0 ]; do
	case $1 in
	--env) env=${2:-}; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	*) die "usage: apim-instance.sh --env ENV [--dry-run]" ;;
	esac
done
load_instance "$env"

sku=$(jq -rM .sku <<<"$INSTANCE")
case $sku in BasicV2 | StandardV2 | PremiumV2) ;; *) die "sku \"$sku\": this platform runs the v2 tiers" ;; esac

# The instance signs in to backends and Key Vault with its own identity.
# Outbound VNet integration lets it reach the clusters' internal gateways.
body=$(jq -cM '{
	location,
	sku: {name: .sku, capacity: (.capacity // 1)},
	identity: {type: "SystemAssigned"},
	tags: {"cool/env": .environment, "cool/managed-by": "cool-apim"},
	properties: ({
		publisherName,
		publisherEmail,
		publicNetworkAccess: (.publicNetworkAccess // "Enabled")
	} + (if (.vnet.outboundSubnetId // "") != "" then {
		virtualNetworkType: "External",
		virtualNetworkConfiguration: {subnetResourceId: .vnet.outboundSubnetId}
	} else {} end))
}' <<<"$INSTANCE")

file=$(mktemp)
printf '%s' "$body" >"$file"
change "create or update the instance $APIM_NAME ($sku) in $APIM_RG" az rest --method put \
	--url "https://management.azure.com$SERVICE?api-version=$API_VERSION" \
	--headers "Content-Type=application/json" \
	--body "@$file"
rm -f "$file"

if [ "$DRY_RUN" != 1 ]; then
	log "Azure is working on it. Watch with:"
	log "  az apim show --name $APIM_NAME --resource-group $APIM_RG --query provisioningState --output tsv"
fi
