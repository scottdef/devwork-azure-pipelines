#!/usr/bin/env bash
# Check the configuration, and the scripts against a stand-in Azure CLI.
#
#   test/check.sh
#
# Part 1 needs jq and a YAML reader (yq or helm). It checks that every file
# parses and that the files agree with each other: an API names a product
# that exists, a policy names fragments and named values that exist, every
# instance gives a value for every named value.
#
# Part 2 runs the scripts with test/az first on the PATH and reads back what
# they would have asked Azure to do.
set -euo pipefail

cd "$(dirname "$0")/.."
fail=0
bad() { echo "FAIL  $*" >&2; fail=1; }
ok() { echo "ok    $*"; }

REAL_PATH=$PATH
export AZ_LOG
AZ_LOG=$(mktemp)
PATH=$PWD/test:$PATH
# shellcheck source=scripts/lib.sh
. scripts/lib.sh

# --- 1. The files ------------------------------------------------------------

named_values=$(yaml2json global/named-values.yaml | jq -rM '[.namedValues[] | select(.enabled != false) | .name] | join(" ")')
plain_values=$(yaml2json global/named-values.yaml | jq -rM '[.namedValues[] | select(.enabled != false and (.keyVaultSecret // "") == "") | .name] | join(" ")')
backends=$(yaml2json global/backends.yaml | jq -rM '[.backends[].name] | join(" ")')
products=$(for f in products/*.yaml; do yaml2json "$f" | jq -rM .name; done | tr '\n' ' ')
fragments=$(for f in fragments/*.xml; do basename "$f" .xml; done | tr '\n' ' ')

for env in sbx dev tst prd; do
	inst=$(yaml2json "instances/$env.yaml")
	[ "$(jq -rM .environment <<<"$inst")" = "$env" ] || bad "instances/$env.yaml is for another environment"
	[[ $(jq -rM .name <<<"$inst") =~ ^$env-[a-z0-9]+-[a-z0-9]+-apim-[0-9]{3}$ ]] || bad "instances/$env.yaml: the name does not follow <env>-<region>-<workload>-apim-<nnn>"
	for nv in $plain_values; do
		jq -eM --arg k "$nv" '.namedValues[$k] // "" | length > 0' <<<"$inst" >/dev/null || bad "instances/$env.yaml has no value for named value $nv"
	done
	for b in $backends; do
		jq -eM --arg k "$b" '.backends[$k] // "" | test("^https?://")' <<<"$inst" >/dev/null || bad "instances/$env.yaml has no URL for backend $b"
	done
done

# check_policy FILE: well-formed, and names only things that exist.
check_policy() {
	local file=$1 name
	if command -v python3 >/dev/null; then
		python3 -c 'import sys, xml.dom.minidom; xml.dom.minidom.parse(sys.argv[1])' "$file" 2>/dev/null || bad "$file is not well-formed XML"
	fi
	while IFS= read -r name; do
		case " $fragments " in *" $name "*) ;; *) bad "$file includes fragment $name, which fragments/ does not have" ;; esac
	done < <(grep -o 'fragment-id="[^"]*"' "$file" | cut -d'"' -f2)
	while IFS= read -r name; do
		case " $named_values " in *" $name "*) ;; *) bad "$file uses named value $name, which is not declared or is switched off" ;; esac
	done < <(grep -o '{{[^}]*}}' "$file" | tr -d '{}' | sort -u)
}
for file in global/policy.xml fragments/*.xml; do check_policy "$file"; done

while IFS= read -r file; do
	dir=$(dirname "$file")
	api=$(yaml2json "$file")
	name=$(jq -rM .name <<<"$api")
	[ -f "$dir/openapi.yaml" ] || bad "$dir has no openapi.yaml"
	yaml2json "$dir/openapi.yaml" | jq -eM '.openapi and .info.title and (.paths | length > 0)' >/dev/null || bad "$dir/openapi.yaml is not an OpenAPI description with paths"
	case " $products " in *" $(jq -rM .product <<<"$api") "*) ;; *) bad "$file names product $(jq -rM .product <<<"$api"), which products/ does not have" ;; esac
	jq -eM '.auth | IN("entra-jwt", "subscription", "none")' <<<"$api" >/dev/null || bad "$file: auth must be entra-jwt, subscription or none"
	jq -eM '(.environments // []) | all(IN("sbx", "dev", "tst", "prd"))' <<<"$api" >/dev/null || bad "$file lists an environment the platform does not have"
	if [ "$(jq -rM '.environments | length' <<<"$api")" -gt 0 ]; then
		b=$(jq -rM '.backend // ""' <<<"$api")
		if [ -n "$b" ]; then
			case " $backends " in *" $b "*) ;; *) bad "$file names backend $b, which global/backends.yaml does not have" ;; esac
		fi
		if [ -f "$dir/policy.xml" ]; then check_policy "$dir/policy.xml"; fi
	fi
	[ "$(grep -l "^name: $name\$" apis/*/*/api.yaml | wc -l)" = 1 ] || bad "more than one API is named $name"
done < <(find apis -name api.yaml | sort)
if [ "$fail" = 0 ]; then ok "configuration files parse and agree"; fi

# --- 2. The scripts ----------------------------------------------------------

calls() { grep -c -- "$1" "$AZ_LOG" || true; }
expect() { # expect WHAT PATTERN COUNT
	local n
	n=$(calls "$2")
	if [ "$n" = "$3" ]; then ok "$1"; else bad "$1: $n calls match '$2', want $3"; fi
}

# A dry run asks Azure nothing that changes anything.
: >"$AZ_LOG"
scripts/apim-sync.sh --env dev --dry-run >"$AZ_LOG.out" 2>/dev/null
expect "dry run: no create, update, import or PUT" ' create \| update \| import \| add \|--method put' 0
if [ "$(grep -c '^would ' "$AZ_LOG.out")" -ge 12 ]; then
	ok "dry run: lists what it would apply ($(grep -c '^would ' "$AZ_LOG.out") steps)"
else
	bad "dry run printed too little"
fi

# A first sync of dev: nothing exists yet.
: >"$AZ_LOG"
AZ_EXISTING="" AZ_APIS="payments-checkout cool-ai legacy-orders" scripts/apim-sync.sh --env dev >"$AZ_LOG.out" 2>/dev/null
expect "named values are created" 'apim nv create .*--named-value-id cool-' 6
expect "the switched-off named value is left alone" 'cool-palantir-token' 0
expect "backends are PUT" 'rest --method put .*/backends/cool-' 2
expect "fragments are PUT" 'rest --method put .*/policyFragments/cool-' 4
expect "the global policy is PUT" 'rest --method put .*/service/dev-eus2-coolapi-apim-001/policies/policy' 1
expect "products are created" 'apim product create ' 4
expect "checkout is imported with its app as the backend" 'apim api import .*--api-id payments-checkout --path payments/checkout .*--service-url https://checkout.dev.coolapps.example/api' 1
expect "checkout goes into its product" 'apim product api add .*--product-id payments --api-id payments-checkout' 1
expect "checkout's policy asks for a token" 'apis/payments-checkout/policies/policy.*cool-jwt' 1
expect "the AI gateway is imported with Foundry as the backend" 'apim api import .*--api-id cool-ai .*--service-url https://dev-eus2-coolai-fdry-001' 1
expect "the AI gateway's own policy is used" 'apis/cool-ai/policies/policy.*cool-ai-budget' 1
expect "cool-api is not published in dev" 'api-id cool-api ' 0
expect "the Palantir outline is not published" 'palantir' 0
if grep -q 'note: API legacy-orders is in dev-eus2-coolapi-apim-001 but not in this repository' "$AZ_LOG.out"; then
	ok "an API unknown to the repository is reported, not deleted"
else
	bad "the unmanaged API was not reported"
fi

# Order: a policy must not be sent before what it names.
first() { grep -n -m1 -- "$1" "$AZ_LOG" | cut -d: -f1; }
if [ "$(first 'apim nv create')" -lt "$(first 'policyFragments/')" ] &&
	[ "$(first 'policyFragments/')" -lt "$(first '/policies/policy')" ] &&
	[ "$(first 'apim product create')" -lt "$(first 'apim api import')" ]; then
	ok "named values, then fragments, then policies; products before APIs"
else
	bad "the sync applies things in the wrong order"
fi

# A second sync: things exist, so they are updated, not created.
: >"$AZ_LOG"
AZ_EXISTING="cool-tenant-id cool-api-audience cool-portal-origin cool-ai-tpm-small cool-ai-tpm-medium cool-ai-tpm-large payments platform ai partners" \
	scripts/apim-sync.sh --env dev >/dev/null 2>&1
expect "second sync: named values are updated" 'apim nv update ' 6
expect "second sync: nothing is created twice" 'apim nv create \|apim product create ' 0

# prd publishes cool-api and not checkout.
: >"$AZ_LOG"
scripts/apim-sync.sh --env prd --only apis >/dev/null 2>&1
expect "prd: cool-api goes to the control plane backend" 'apim api import .*--api-id cool-api .*--service-url http://cool-api.prd.coolops.example' 1
expect "prd: checkout is not published there yet" 'api-id payments-checkout' 0

# The instance: a v2 SKU, by PUT, with its own identity.
: >"$AZ_LOG"
scripts/apim-instance.sh --env sbx >/dev/null 2>&1
expect "sbx instance is Basic v2 with a system identity" 'rest --method put .*/service/sbx-eus2-coolapi-apim-001?api-version.*"name":"BasicV2".*"SystemAssigned"' 1
: >"$AZ_LOG"
scripts/apim-instance.sh --env prd >/dev/null 2>&1
expect "prd instance is Standard v2 with outbound VNet integration" '"name":"StandardV2","capacity":2.*subnets/apim-outbound' 1

PATH=$REAL_PATH
rm -f "$AZ_LOG" "$AZ_LOG.out"
[ "$fail" = 0 ] || exit 1
echo "cool-apim: all checks passed"
