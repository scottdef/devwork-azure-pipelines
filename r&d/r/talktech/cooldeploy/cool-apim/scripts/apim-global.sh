#!/usr/bin/env bash
# Apply what every API of an instance shares: named values, backends, policy
# fragments and the global policy.
#
#   scripts/apim-global.sh --env dev [--dry-run]
#
# Order matters. Policies name fragments, fragments name named values, and
# API Management refuses a policy that names something it does not have.
set -euo pipefail
# shellcheck source=scripts/lib.sh
. "$(dirname "$0")/lib.sh"

env=""
while [ $# -gt 0 ]; do
	case $1 in
	--env) env=${2:-}; shift 2 ;;
	--dry-run) DRY_RUN=1; shift ;;
	*) die "usage: apim-global.sh --env ENV [--dry-run]" ;;
	esac
done
load_instance "$env"

# Named values. global/named-values.yaml holds the ones every instance has;
# instances/<env>.yaml holds this instance's values for them. A named value
# with keyVaultSecret is a reference: API Management reads the secret with
# its own identity, and the secret never passes through this repository.
names=$(yaml2json "$ROOT/global/named-values.yaml")
while IFS= read -r nv; do
	id=$(jq -rM .name <<<"$nv")
	if [ "$(jq -rM '.enabled == false' <<<"$nv")" = true ]; then
		log "skip named value $id: switched off"
		continue
	fi
	secret=$(jq -rM '.keyVaultSecret // ""' <<<"$nv")
	if [ -n "$secret" ]; then
		vault=$(jq -rM '.keyVault // ""' <<<"$INSTANCE")
		[ -n "$vault" ] || die "named value $id comes from Key Vault, but instances/$COOL_ENV.yaml names no keyVault"
		put "set named value $id from Key Vault secret $secret" "namedValues/$id" "$(jq -nM \
			--arg id "$id" --arg url "https://$vault.vault.azure.net/secrets/$secret" \
			'{properties: {displayName: $id, secret: true, keyVault: {secretIdentifier: $url}}}')"
		continue
	fi
	value=$(jq -rM --arg id "$id" '.namedValues[$id] // ""' <<<"$INSTANCE")
	[ -n "$value" ] || die "instances/$COOL_ENV.yaml has no value for named value $id"
	if exists az apim nv show --service-name "$APIM_NAME" --resource-group "$APIM_RG" --named-value-id "$id"; then
		change "update named value $id" az apim nv update --service-name "$APIM_NAME" --resource-group "$APIM_RG" \
			--named-value-id "$id" --value "$value"
	else
		change "create named value $id" az apim nv create --service-name "$APIM_NAME" --resource-group "$APIM_RG" \
			--named-value-id "$id" --display-name "$id" --value "$value"
	fi
done < <(jq -cM '.namedValues[]' <<<"$names")

# Backends: the things behind API Management that are not one app.
while IFS= read -r backend; do
	id=$(jq -rM .name <<<"$backend")
	url=$(jq -rM --arg id "$id" '.backends[$id] // ""' <<<"$INSTANCE")
	[ -n "$url" ] || die "instances/$COOL_ENV.yaml has no URL for backend $id"
	put "set backend $id to $url" "backends/$id" "$(jq -M --arg url "$url" \
		'{properties: {url: $url, protocol: "http", description: (.description // ""),
		  tls: {validateCertificateChain: true, validateCertificateName: true}}}' <<<"$backend")"
done < <(yaml2json "$ROOT/global/backends.yaml" | jq -cM '.backends[]')

# Policy fragments, by file name.
for file in "$ROOT"/fragments/*.xml; do
	id=$(basename "$file" .xml)
	put "set policy fragment $id" "policyFragments/$id" "$(jq -nM --rawfile xml "$file" --arg id "$id" \
		'{properties: {format: "xml", value: $xml, description: ("cool-apim fragments/" + $id + ".xml")}}')"
done

# The global policy wraps every API.
put "set the global policy" "policies/policy" "$(policy_body "$ROOT/global/policy.xml")"
