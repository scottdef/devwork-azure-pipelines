# cool-infra

Terraform for the Azure side of CoolDeploy: networks, the registry, the five
AKS clusters, metrics, identities, Key Vaults, Microsoft Foundry and app
add-ons.

Not here: API Management. `cool-apim` manages it with scripts, and
`scripts/check.sh` fails if a stack mentions it.

> **Checked only as far as this could be done without Azure or the provider
> registry.** The files are formatted with `terraform fmt`, which proves they
> parse, and the configuration is checked against the naming rules. No stack
> has been through `terraform validate` or `plan`. Run `make lock check` on a
> machine that can reach the registry before anything else, and expect to
> fix an attribute or two: provider schemas move. The add-ons stack is the
> most likely place, since Azure Managed Redis is declared through the ARM
> API with `azapi`.

## Stacks

Applied in order; each reads what the ones before it built, by name.

| Stack | Builds |
| --- | --- |
| `00-state` | the storage account for Terraform state. Applied once, by hand, with local state. |
| `10-network` | a virtual network per cluster, peered with the hub; a delegated subnet per environment for API Management's outbound integration |
| `20-registry` | the platform registry `prdeus2coolopsacr001` |
| `30-clusters` | the five clusters, from `config/clusters.json` |
| `35-dns` | private DNS: app hosts and preview hosts to each environment's internal gateway; `cool-api` for API Management |
| `40-observability` | one Azure Monitor workspace (managed Prometheus) for all clusters, and Azure Managed Grafana `prd-eus2-coolops-mg-001` |
| `50-identity` | the managed identities automation runs as, their federated credentials and roles; the resource groups API Management lives in |
| `60-vaults` | a platform Key Vault per environment, a Key Vault per team and environment, and who may read them |
| `65-ai` | a Foundry account per environment with model deployments; the Entra ID application whose app roles are AI token budgets |
| `70-addons` | Redis, PostgreSQL, storage and AI budgets for apps, from `addons.json` |

## Clusters

`config/clusters.json` is the whole definition:

| Cluster | For | Mesh |
| --- | --- | --- |
| `prd-eus2-coolops-aks-001` | the hub: Argo CD, runners, `cool-api` | no |
| `sbx-eus2-appsim-aks-001` | the sandbox: simulators, rehearsals | yes |
| `dev-eus2-appdev-aks-001`, `tst-…`, `prd-…` | apps, one namespace per app | yes |

Each is Kubernetes 1.30 with Entra ID sign-in and Azure RBAC, no local
accounts, workload identity, Azure CNI overlay with Cilium (which enforces
the NetworkPolicies the charts create), the Key Vault secrets provider, and
managed Prometheus. App clusters also get the Istio add-on with its internal
and external ingress gateways.

Two things the provider cannot set and you may want:

- **Istio CNI.** Without it, a meshed namespace cannot enforce the
  `restricted` pod security level. It is a preview feature of the add-on:
  `az aks mesh enable --proxy-redirection-mechanism CNIChaining`. If you turn
  it on, set `platform.podSecurity` to `restricted` in that cluster's
  `platform-values.json` in `cool-gitops`.
- **The Istio revision.** `istio_revision` in `config/platform.json` must be
  one the region offers: `az aks mesh get-revisions --location eastus2`. The
  same revision goes in `platform.mesh.revision` in `cool-gitops`.

## Names

`<env>-<region>-<workload>-<type>-<nnn>`: `prd-eus2-coolops-aks-001`. Where
Azure forbids hyphens, the same without them: `prdeus2coolopsacr001`.

Two limits shape the rest. Managed Grafana names may be 23 characters, and
`prd-eus2-coolops-mg-001` is exactly 23. Key Vault names may be 24, and
`<env>-eus2-<team>-kv-001` with an eight-character team is exactly 24, which
is why a team slug may not be longer.

## Identities

Nothing signs in with a stored secret. `50-identity` has the full table; the
one worth understanding is `coolbuild`, the identity app pipelines build
images as. Its federated credential does not name a repository:

```
job_workflow_ref:CoolEngOrg/cool-actions/.github/workflows/cool-build.yml@refs/tags/v1
```

Any app repository's pipeline may use it, because the job that does is
defined by the platform's workflow and runs the platform's steps. This needs
the repository's OIDC subject to include `job_workflow_ref`, which the
`cool-app-create` request sets.

An app's own identities, one per environment, are not in Terraform. The
`cool-app-create` request creates them, so that a new app does not wait for a
pull request here. `70-addons` looks them up by name.

## First apply

```sh
az login                                       # as an Owner of the subscription
$EDITOR config/platform.json                   # tenant, subscription, admin group
terraform -chdir=stacks/00-state init && terraform -chdir=stacks/00-state apply
# uncomment the backend block in stacks/00-state/main.tf, then:
terraform -chdir=stacks/00-state init -migrate-state
make lock                                      # commit the lock files
for s in 10-network 20-registry 30-clusters 35-dns 40-observability 50-identity 60-vaults 65-ai; do
  terraform -chdir=stacks/$s init && terraform -chdir=stacks/$s apply
done
scripts/import-base-images.sh
make values                                    # what goes into GitHub, cool-gitops and cool-apim
```

`50-identity` creates the identity that CI applies with, so the first apply
of it is by a person. After that, pull requests plan and merges apply,
through `cool-actions`' `cool-terraform` workflow and the `cool-infra`
environment's reviewers.

Three values exist only after something else has run, and go into
`config/platform.json` when they do:

| Value | Known after | Apply again |
| --- | --- | --- |
| `internal_gateway_ips` | a cluster's Istio add-on has created its internal gateway | `35-dns` |
| `cool_api_ip` | Argo CD has installed `cool-api` on the hub | `35-dns` |
| `apim_principals` | `cool-apim` has created the instances | `60-vaults`, `65-ai` |

Certificates are not issued here. Import each environment's wildcard
certificate into its platform vault as `wildcard-<env>-coolapps`; the
`cool-platform` chart syncs it to the gateways.

After `cool-apim` has created the API Management instances, put their
principal IDs in `apim_principals` in `config/platform.json` and apply
`60-vaults` and `65-ai` again: that is what lets API Management read its
Key Vault references and call Foundry.

## Add-ons

`stacks/70-addons/addons.json` is a list of
`{team, app, environment, addon, size}`. An approved `cool-addon` request
opens the pull request that adds a line. The two entries in the file are
examples for `payments/checkout` in `dev`; remove them before a first apply,
or create the app first.

| Add-on | What the app gets |
| --- | --- |
| `redis` | Azure Managed Redis over a private endpoint; signs in with its identity; URL in the team vault as `<app>-REDIS-URL` |
| `postgres` | a database on the team's server for the environment, over a private endpoint; URL in the team vault as `<app>-DATABASE-URL` |
| `storage` | a blob container its identity can read and write; URL as `<app>-STORAGE-URL` |
| `ai` | an app role on its identity, which API Management reads as its token budget |

The PostgreSQL URL holds a password, because granting a managed identity a
database role takes SQL that Terraform cannot run. It is generated, kept in
state and in the team vault, and never shown. The other three involve no
secret at all.

## Check

```sh
make check            # TF=tofu make check, for OpenTofu
```
