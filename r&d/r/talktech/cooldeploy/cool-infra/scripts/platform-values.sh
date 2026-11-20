#!/usr/bin/env bash
# Print the values that other repositories need from this one.
#
#   scripts/platform-values.sh
#
# It says where each value goes: a GitHub variable, a file in cool-gitops, a
# file in cool-apim. Placeholders there are all zeros until you copy these in.
#
# Everything comes from `terraform output`, so the stacks must be applied and
# you must be signed in with read access to the state.
set -euo pipefail

cd "$(dirname "$0")/.."
TF=${TF:-terraform}
out() { "$TF" -chdir="stacks/$1" output -json "$2"; }

platform=$(jq -cM . config/platform.json)
tenant=$(jq -rM .tenant_id <<<"$platform")
identities=$(out 50-identity client_ids)
tls=$(out 60-vaults tls_client_ids)

echo "GitHub organization variables"
echo "  COOL_TENANT_ID              $tenant"
echo "  COOL_SUBSCRIPTION_ID        $(jq -rM .subscription_id <<<"$platform")"
echo "  COOL_REGISTRY               $(jq -rM .registry <<<"$platform")"
echo "  COOL_BUILD_CLIENT_ID        $(jq -rM .coolbuild <<<"$identities")"
echo "  COOL_PROVISION_CLIENT_ID    $(jq -rM .coolprov <<<"$identities")"
echo "  COOL_MONITOR_CLIENT_ID      $(jq -rM .coolmon <<<"$identities")"
echo "  COOL_INFRA_PLAN_CLIENT_ID   $(jq -rM .coolplan <<<"$identities")"
echo "  COOL_INFRA_APPLY_CLIENT_ID  $(jq -rM .coolapply <<<"$identities")"
echo "  COOL_APIM_READ_CLIENT_ID    $(jq -rM .coolapimr <<<"$identities")"
echo "  COOL_APIM_WRITE_CLIENT_ID   $(jq -rM .coolapimw <<<"$identities")"
echo "  COOL_PROM_URL               $(out 40-observability prometheus_query_endpoint | jq -rM .)"
echo
echo "cool-gitops/bootstrap"
echo "  ARGOCD_CLIENT_ID            $(jq -rM .coolargo <<<"$identities")"
echo
echo "cool-gitops/clusters/prd-eus2-coolops-aks-001/cool-api-values.json"
echo "  identity                    $(jq -rM .coolapi <<<"$identities")"
echo "  tenantId                    $tenant"
echo
echo "cool-gitops/clusters/<cluster>/platform-values.json"
echo "  platform.tenantId           $tenant"
jq -rM 'to_entries[] | "  platform.tls.identity (\(.key))  \(.value)"' <<<"$tls"
echo
echo "cool-apim/instances/<env>.yaml"
out 10-network apim_outbound_subnets | jq -rM 'to_entries[] | "  vnet.outboundSubnetId (\(.key))  \(.value)"'
out 65-ai audiences | jq -rM 'to_entries[] | "  namedValues.cool-api-audience (\(.key))  \(.value)"'
out 65-ai foundry_endpoints | jq -rM 'to_entries[] | "  backends.cool-foundry (\(.key))  \(.value)"'
