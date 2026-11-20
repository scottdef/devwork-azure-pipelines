# shellcheck shell=bash
# shellcheck disable=SC2034  # the variables set here are used by the scripts that source this file
# Shared by the API Management scripts. Source it.
#
# The scripts turn the YAML and OpenAPI files of this repository into Azure
# CLI calls. They are idempotent: every call creates or replaces, so running
# a script twice leaves API Management as the files describe it.
#
# `az apim` covers named values, products and API import. It has no commands
# for the v2 tiers' instance creation, for policies or for policy fragments,
# so those go through `az rest` against the management API.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
API_VERSION=${APIM_API_VERSION:-2024-05-01}
DRY_RUN=${DRY_RUN:-0}

die() { echo "cool-apim: $*" >&2; exit 1; }
log() { echo "$*" >&2; }

for tool in az jq; do
	command -v "$tool" >/dev/null || die "$tool not found on PATH"
done

# yaml2json FILE prints a YAML file as JSON. It uses whichever converter the
# machine has: mikefarah's yq, the Python yq, or Helm.
yaml2json() {
	[ -f "$1" ] || die "$1 not found"
	if command -v yq >/dev/null; then
		if yq --version 2>&1 | grep -qi mikefarah; then
			yq -o=json '.' "$1"
		else
			yq '.' "$1"
		fi
	elif command -v helm >/dev/null; then
		# A one-template chart that prints its values as JSON.
		helm template yaml2json "$ROOT/scripts/yaml2json" --values "$1" | grep -v -e '^---' -e '^#'
	else
		die "need yq or helm to read YAML"
	fi
}

# load_instance ENV reads instances/ENV.yaml into the variables every script
# uses, and checks that the instance is the one for that environment.
load_instance() {
	COOL_ENV=$1
	case $COOL_ENV in sbx | dev | tst | prd) ;; *) die "environment \"$COOL_ENV\": want sbx, dev, tst or prd" ;; esac
	INSTANCE=$(yaml2json "$ROOT/instances/$COOL_ENV.yaml")
	APIM_NAME=$(jq -rM .name <<<"$INSTANCE")
	APIM_RG=$(jq -rM .resourceGroup <<<"$INSTANCE")
	DOMAIN=$(jq -rM .backendDomain <<<"$INSTANCE")
	case $APIM_NAME in "$COOL_ENV"-*) ;; *) die "instances/$COOL_ENV.yaml names $APIM_NAME, which is not a $COOL_ENV instance" ;; esac
	SUBSCRIPTION=${AZURE_SUBSCRIPTION_ID:-}
	if [ -z "$SUBSCRIPTION" ]; then
		SUBSCRIPTION=$(az account show --query id --output tsv)
	fi
	SERVICE=/subscriptions/$SUBSCRIPTION/resourceGroups/$APIM_RG/providers/Microsoft.ApiManagement/service/$APIM_NAME
}

# change DESCRIPTION COMMAND... runs a command that changes API Management.
# In a dry run it prints the description and runs nothing.
change() {
	local what=$1
	shift
	if [ "$DRY_RUN" = 1 ]; then
		echo "would $what"
		return 0
	fi
	echo "$what"
	"$@" >/dev/null
}

# put DESCRIPTION PATH BODY-JSON sends a PUT to the management API, for the
# resources `az apim` has no command for. PATH is relative to the service.
put() {
	local what=$1 path=$2 body=$3 file
	file=$(mktemp)
	printf '%s' "$body" >"$file"
	change "$what" az rest --method put \
		--url "https://management.azure.com$SERVICE/$path?api-version=$API_VERSION" \
		--headers "Content-Type=application/json" \
		--body "@$file"
	rm -f "$file"
}

# policy_body FILE prints the request body that sets a policy from an XML
# file. The files are well-formed XML, so the format is "xml".
policy_body() {
	[ -f "$1" ] || die "$1 not found"
	jq -nM --rawfile xml "$1" '{properties: {format: "xml", value: $xml}}'
}

# exists COMMAND... reports whether a read succeeds: `exists az apim product show ...`.
exists() { "$@" >/dev/null 2>&1; }
