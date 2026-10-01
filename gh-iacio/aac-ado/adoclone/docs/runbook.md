# adoclone runbook

Access, the phased plan, the operator steps, validation and rollback.

## Permissions and PAT scopes

Use a dedicated migration identity that is a Project Collection Administrator
(or Project Administrator in both projects plus "Create new projects" at
organization level), and grant it in the **target** project:

- "Bypass rules on work item updates" (for `-bypass-rules`, the default)
- "Suppress notifications for work item updates"

PAT scopes (Basic auth, empty username):

| Scope | Why it's needed |
|---|---|
| Project and Team (read, write, manage) | project and team creation |
| Work Items (read, write, manage) | work items, queries, classification nodes, delivery plans |
| Code (full) | repos, imports, branch policies, wiki repos |
| Build (read and execute), Release (read, write, execute, manage) | pipelines |
| Variable Groups (read, create, manage) | variable groups |
| Service Connections (read, query, manage) | sharing service connections |
| Agent Pools (read, manage) | queues, deployment groups |
| Environment (read, manage) | environments |
| Pipeline Resources (use and manage) | checks and pipeline permissions |
| Secure Files (read, create, manage) | secure files |
| Test Management (read, write) | test plans |
| Wiki (read, write) | wikis |
| Graph and Identity (read, manage), Security (manage) | groups, memberships, ACLs, role assignments |
| Packaging (read, write, manage) | feeds |
| Dashboards | dashboards |
| Service Hooks | service hooks |

## Required tasks checklist

- [ ] PAT stored as `ADO_PAT` (GitHub secret, Azure Pipelines variable group, or Key Vault secret `ado-pat` for the AKS Job)
- [ ] Migration identity holds Bypass rules and Suppress notifications in the target project
- [ ] Source change freeze announced (work item edits, pushes)
- [ ] `adoclone plan` reviewed; a full dry run's `todo-dryrun.json` reviewed (it lists secrets and manual items up front)
- [ ] Secret values ready: `SECRET_<GROUP>_<VAR>` environment variables, or Key Vault-linked variable groups
- [ ] Secure files collected into one directory for `-securefiles-dir`
- [ ] AKS: the Kubernetes service connections used by environments are in the source project (they're shared with the target)
- [ ] Each phase's exit gate passed and `state.json` archived
- [ ] Source project set read-only after cutover (deny Contribute to Contributors)

## Implementation plan

Estimates assume about 10k work items, 30 repos and 50 pipelines.

| Phase | Components | Depends on | Estimate | Exit gate |
|---|---|---|---|---|
| 0 Prerequisites | preflight, `plan`, full dry run | — | 1–2 d | plan and dry-run todo.json reviewed |
| 1 Project shell | `project`, `groups` | 0 | 0.5 h | target exists, groups mapped |
| 2 Boards structure | `nodes`, `teams`, `boards` | 1 | 0.5 d | node and team counts equal |
| 3 Repos | `repos` | 1 | 0.5–1 d | ref and HEAD SHA parity |
| 4 Pipeline resources | `endpoints`, `vargroups`, `securefiles`, `queues`, `deploymentgroups`, `envs` | 1 | 0.5 d | resources mapped; secrets todo list known |
| 5 Pipelines | `taskgroups`, `pipelines`, `releases`, `checks`, `permissions`, `policies`, `settings` | 3, 4 | 1 d | one queued build per pipeline succeeds |
| 6 Work items | `workitems` | 2, 3 | 1–3 d runtime | verify sample matches |
| 7 Test plans | `testplans` | 6 | 0.5 d | suite and test case counts |
| 8 Queries, dashboards, wiki | `queries`, `dashboards`, `plans`, `wiki`, `feeds` | 2, 5, 6 | 0.5 d | queries run without errors |
| 9 Security | `security`, `hooks` | 1–8 | 0.5 d | spot-check repo and area permissions |
| 10 Verify and cutover | `verify`, `report` | all | 0.5 d | sign-off |

## Operator runbook

1. Run `scripts/preflight.sh`, then `adoclone plan`.
2. Dry-run everything: `adoclone clone ... -dry-run=true`. Read the logged writes and `todo-dryrun.json`.
3. Announce the freeze and remove Contribute on the source.
4. Run with `-dry-run=false`, either all at once or phase by phase with `-components`. Exit `1` means a component stopped: fix the cause and rerun the same command; it resumes. Exit `3` means it finished but some items failed: they're in `todo.json` with the error.
5. Work through `todo.json`: secret values, secure files, VM agents, packages to re-publish, hooks to re-enable.
6. Queue one build per pipeline in the target; approve resource authorizations if prompted.
7. `adoclone verify` (exit `4` lists missing objects and work item differences) and `adoclone report`. Do UAT and sign off. Keep the source read-only as the archive.

## Running on AKS

One-time setup (Key Vault secret, Workload ID, CSI driver):

```
RG=<resource-group> AKS=<cluster> KV=<key-vault>
az aks update -g $RG -n $AKS --enable-oidc-issuer --enable-workload-identity
az aks enable-addons -g $RG -n $AKS --addons azure-keyvault-secrets-provider
az identity create -g $RG -n adoclone-id
CLIENT_ID=$(az identity show -g $RG -n adoclone-id --query clientId -o tsv)
PRINCIPAL_ID=$(az identity show -g $RG -n adoclone-id --query principalId -o tsv)
ISSUER=$(az aks show -g $RG -n $AKS --query oidcIssuerProfile.issuerUrl -o tsv)
az identity federated-credential create -g $RG --identity-name adoclone-id -n adoclone \
  --issuer "$ISSUER" --subject system:serviceaccount:adoclone:adoclone --audiences api://AzureADTokenExchange
az keyvault secret set --vault-name $KV -n ado-pat --value "$ADO_PAT"
az role assignment create --assignee-object-id $PRINCIPAL_ID --assignee-principal-type ServicePrincipal \
  --role "Key Vault Secrets User" --scope $(az keyvault show -n $KV --query id -o tsv)
az account show --query tenantId -o tsv   # tenant ID for the SecretProviderClass
```

Fill in `<workload-identity-client-id>`, `<key-vault-name>`, `<tenant-id>` and
`<acr>` in `deploy/k8s`, build and push the image, then:

```
kubectl apply -k deploy/k8s
kubectl -n adoclone logs -f job/adoclone-run
kubectl -n adoclone wait --for=condition=complete job/adoclone-run --timeout=72h
```

The Job template is immutable. To change settings (for example `DRY_RUN:
"false"`), edit the ConfigMap, then
`kubectl -n adoclone delete job adoclone-run && kubectl apply -k deploy/k8s`.
The checkpoint on the PVC carries over, so the new Job resumes.

To read `todo.json` and `state.json` after the Job finishes:

```
kubectl -n adoclone run inspect --rm -it --image=alpine:3.20 --restart=Never --overrides='
{"spec":{"securityContext":{"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}},
 "containers":[{"name":"inspect","image":"alpine:3.20","command":["cat","/state/todo.json"],"stdin":true,"tty":true,
  "securityContext":{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]}},
  "volumeMounts":[{"name":"state","mountPath":"/state"}]}],
 "volumes":[{"name":"state","persistentVolumeClaim":{"claimName":"adoclone-state"}}]}}'
```

Without Key Vault: `kubectl -n adoclone create secret generic ado-pat --from-literal=ADO_PAT=...`,
then in `job.yaml` replace the `ADO_PAT_FILE` env entry with
`envFrom: [{secretRef: {name: ado-pat}}]` and remove the `kv` volume and mount.

Monitoring: apply `deploy/k8s/podmonitor.yaml` (Azure Monitor managed
Prometheus; for kube-prometheus-stack change the apiVersion as noted in the
file) and import `deploy/grafana/adoclone-dashboard.json` into Grafana 12.
`-metrics-hold=2m` keeps the endpoint up after the run so the final values are
scraped.

## Validation guide

- **Counts:** `verify` compares 18 components. A target with more than the source is fine (new projects come with a default repo and queues); fewer is a problem.
- **Work items:** `verify` samples `-sample` percent of copied items and compares type, title, state, area path, iteration path, work item link count and attachment count.
- **Git:** `git ls-remote` ref and SHA sets match on both sides (excluding `refs/pull/*`).
- **Pipelines:** a test build on the default branch succeeds, and branch policies block direct pushes.
- **Security:** spot-check repo, area and pipeline permissions for the copied groups. ACL entries are merged into the target's, so the target's own entries (its build service, its default groups) stay; source-only identities aren't granted anything and appear in `todo.json`.

## Rollback and cleanup

- **Partial failure:** fix the cause and rerun; every component skips what's already mapped.
- **Full rollback:** `CONFIRM=<target> adoclone cleanup -source <src> -target <target> -dry-run=false`. It removes the target from service connections (and, in share mode, variable groups) that adoclone shared, deletes the target project (restorable from Organization settings > Projects during the retention period), and renames `state.json` and `todo.json` with a `.deleted-<timestamp>` suffix. It refuses if the target is the source, doesn't match the checkpoint, or the checkpoint doesn't know it (add `-adopt-existing` only if you mean it).
- **The source isn't changed** apart from reads, service connection and variable group sharing, and the read-only ACL you set at cutover.
