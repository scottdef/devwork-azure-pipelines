#!/usr/bin/env bash
# Bootstrap Argo CD on the hub cluster and hand the rest to Git.
#
#   bootstrap/install.sh argocd            install or upgrade Argo CD
#   bootstrap/install.sh repo-creds        let Argo CD read the CoolEngOrg repositories
#   bootstrap/install.sh cluster NAME RG   register an app cluster
#   bootstrap/install.sh root              apply the root Application
#   bootstrap/install.sh all               all of the above, for the five clusters
#
# Run it with kubectl pointed at the hub:
#
#   az aks get-credentials -g prd-eus2-coolops-rg-001 -n prd-eus2-coolops-aks-001
#
# Needs az, kubectl 1.30, helm and jq. Reads these from the environment:
#
#   ARGOCD_CHART_VERSION  version of the argo/argo-cd chart to install
#   ARGOCD_CLIENT_ID      client ID of Argo CD's managed identity (cool-infra output)
#   TENANT_ID             Entra ID tenant; default: the signed-in one
#   VAULT                 Key Vault with the GitHub App secrets; default prd-eus2-coolops-kv-001
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
VAULT=${VAULT:-prd-eus2-coolops-kv-001}
NS=argocd

die() { echo "install.sh: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null || die "$1 not found on PATH"; }

usage() {
	sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
}

tenant() {
	if [ -z "${TENANT_ID:-}" ]; then
		TENANT_ID=$(az account show --query tenantId -o tsv)
	fi
	echo "$TENANT_ID"
}

cmd_argocd() {
	need helm
	[ -n "${ARGOCD_CLIENT_ID:-}" ] || die "set ARGOCD_CLIENT_ID to the client ID of Argo CD's managed identity"
	helm repo add argo https://argoproj.github.io/argo-helm --force-update >/dev/null
	if [ -z "${ARGOCD_CHART_VERSION:-}" ]; then
		echo "Set ARGOCD_CHART_VERSION to one of:" >&2
		helm search repo argo/argo-cd --versions | head -6 >&2
		exit 1
	fi
	# The three service accounts that reach other clusters carry the identity.
	local key='serviceAccount.annotations.azure\.workload\.identity/client-id'
	helm upgrade --install argocd argo/argo-cd \
		--namespace "$NS" --create-namespace \
		--version "$ARGOCD_CHART_VERSION" \
		--values "$here/argocd-values.yaml" \
		--set-string "controller.$key=$ARGOCD_CLIENT_ID" \
		--set-string "server.$key=$ARGOCD_CLIENT_ID" \
		--set-string "applicationSet.$key=$ARGOCD_CLIENT_ID" \
		--wait --timeout 10m
	kubectl -n "$NS" get pods
}

# Argo CD reads cool-gitops and cool-charts as a GitHub App with read access
# to both. Its three secrets come from Key Vault and stay out of Git.
cmd_repo_creds() {
	local id installation key
	id=$(az keyvault secret show --vault-name "$VAULT" --name cool-argocd-app-id --query value -o tsv)
	installation=$(az keyvault secret show --vault-name "$VAULT" --name cool-argocd-app-installation-id --query value -o tsv)
	key=$(mktemp)
	trap 'rm -f "$key"' RETURN
	az keyvault secret show --vault-name "$VAULT" --name cool-argocd-app-private-key --query value -o tsv >"$key"
	kubectl create secret generic cool-repo-creds --namespace "$NS" --dry-run=client -o yaml \
		--from-literal=type=git \
		--from-literal=url=https://github.com/CoolEngOrg \
		--from-literal=githubAppID="$id" \
		--from-literal=githubAppInstallationID="$installation" \
		--from-file=githubAppPrivateKey="$key" |
		kubectl label --local -f - argocd.argoproj.io/secret-type=repo-creds -o yaml |
		kubectl apply -f -
}

# An app cluster is a Secret that tells Argo CD where the API server is and
# how to sign in: with its own workload identity, through argocd-k8s-auth.
# No token is stored. The identity needs "Azure Kubernetes Service RBAC
# Cluster Admin" on the cluster, which cool-infra assigns.
cmd_cluster() {
	local name=${1:-} rg=${2:-} fqdn ca config
	if [ -z "$name" ] || [ -z "$rg" ]; then
		die "usage: install.sh cluster NAME RESOURCE-GROUP"
	fi
	[ -n "${ARGOCD_CLIENT_ID:-}" ] || die "set ARGOCD_CLIENT_ID"
	fqdn=$(az aks show -g "$rg" -n "$name" --query 'privateFqdn || fqdn' -o tsv)
	ca=$(az aks get-credentials -g "$rg" -n "$name" --file - |
		awk '$1 == "certificate-authority-data:" { print $2; exit }')
	if [ -z "$fqdn" ] || [ -z "$ca" ]; then
		die "could not read the address or CA of $name"
	fi
	config=$(jq -nM --arg ca "$ca" --arg client "$ARGOCD_CLIENT_ID" --arg tenant "$(tenant)" '{
		execProviderConfig: {
			command: "argocd-k8s-auth",
			args: ["azure"],
			apiVersion: "client.authentication.k8s.io/v1beta1",
			env: {
				AAD_ENVIRONMENT_NAME: "AzurePublicCloud",
				AAD_LOGIN_METHOD: "workloadidentity",
				AZURE_CLIENT_ID: $client,
				AZURE_TENANT_ID: $tenant,
				AZURE_AUTHORITY_HOST: "https://login.microsoftonline.com/",
				AZURE_FEDERATED_TOKEN_FILE: "/var/run/secrets/azure/tokens/azure-identity-token"
			}
		},
		tlsClientConfig: {insecure: false, caData: $ca}
	}')
	kubectl create secret generic "cluster-$name" --namespace "$NS" --dry-run=client -o yaml \
		--from-literal=name="$name" \
		--from-literal=server="https://$fqdn:443" \
		--from-literal=config="$config" |
		kubectl label --local -f - argocd.argoproj.io/secret-type=cluster "cool/cluster=$name" -o yaml |
		kubectl apply -f -
	echo "registered $name at https://$fqdn:443"
}

cmd_root() {
	kubectl apply -f "$here/root.yaml"
	echo "Argo CD now follows argocd/ in cool-gitops. Watch it with:"
	echo "  kubectl -n $NS get applications,applicationsets"
}

cmd_all() {
	cmd_argocd
	cmd_repo_creds
	cmd_cluster sbx-eus2-appsim-aks-001 sbx-eus2-appsim-rg-001
	cmd_cluster dev-eus2-appdev-aks-001 dev-eus2-appdev-rg-001
	cmd_cluster tst-eus2-appdev-aks-001 tst-eus2-appdev-rg-001
	cmd_cluster prd-eus2-appdev-aks-001 prd-eus2-appdev-rg-001
	cmd_root
}

need az
need kubectl
need jq

case ${1:-} in
argocd) cmd_argocd ;;
repo-creds) cmd_repo_creds ;;
cluster) shift; cmd_cluster "$@" ;;
root) cmd_root ;;
all) cmd_all ;;
*) usage ;;
esac
