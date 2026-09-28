# Setup guide

This guide sets up the platform in the `CoolEngOrg` organization of the `CoolEngEnt` enterprise. Allow about half a day for GitHub, plus Azure and AKS time.

**Prerequisites:** organization owner on CoolEngOrg; enterprise owner or billing manager (only for enterprise cost-center budgets); Owner or User Access Administrator on the Azure subscriptions and resource groups involved; `az` CLI 2.60 or later; `kubectl` 1.30; Helm 3; Go 1.21.

## 1. Repository

1. Create `CoolEngOrg/issueops` with **internal** visibility, so every CoolEngEnt member can open requests. Push this code to `main`.
2. Add a ruleset on `main`:
   - require a pull request with **Code owners** review ([CODEOWNERS](../.github/CODEOWNERS))
   - for `config/issueops.json`, add a **required reviewers** rule for each of `platform-admins`, `ai-governance` and `finops-approvers`. CODEOWNERS only *requests* all three; one code owner's approval satisfies CODEOWNERS on its own
   - require status checks `Go build, vet, test`, `Custom validators (node --test)` and `actionlint`
   - block force pushes
3. Settings → Actions → General:
   - **Workflow permissions:** read repository contents only. Each workflow elevates per job.
   - **Allow actions:** only the actions and reusable workflows listed in [reference.md](reference.md#pinned-actions), pinned by SHA.
   - Fork pull-request workflows: require approval for all outside collaborators.
4. Keep write access to this repository to `platform-admins`. Anyone with write access can read repository-level secrets, which is one reason sensitive secrets are environment-scoped.

## 2. Teams and roles

Create the teams (or map the names in `config/issueops.json` to existing ones):

| Team | Purpose |
|---|---|
| `copilot-admins` | Approve Copilot budgets and per-user report detail |
| `finops-approvers` | Approve budgets over $1,000/month and PTU deployments |
| `engineering-managers` | May request Copilot usage reports |
| `platform-admins` | Own the platform and may request reports |
| `platform-ai` | Approve Foundry deployments and AKS agent runs |
| `ai-governance` | Approve production Foundry deployments and confidential agent tasks |

The `security_manager` organization role also satisfies the AI-governance rule. Assign it under Organization settings → Roles. To use another built-in or custom role, change `org_roles` in the registry.

## 3. GitHub App

Create an organization-owned GitHub App, **CoolEngOrg IssueOps**. Turn off the webhook: workflows drive everything. Grant:

| Scope | Permission | Used for |
|---|---|---|
| Repository | Issues: **Read & write** | Comments, labels, closing (IssueOps repo) |
| Repository | Contents: Read | Search, commit counts and code statistics for reports |
| Repository | Pull requests: Read | Merged-PR search for reports |
| Repository | Actions: **Read & write** | `workflow_dispatch` of catalog agentic workflows in target repos |
| Repository | Metadata: Read | Mandatory; repository and collaborator permission lookups |
| Organization | Members: Read | Team and org membership (approval gating) |
| Organization | Custom organization roles: Read | `org_roles` selectors |
| Organization | Administration: **Read & write** | Organization billing budgets API |
| Organization | Copilot metrics: Read | Copilot usage-metrics reports |
| Organization | GitHub Copilot Business: Read | Seat count for seat cost |

Install the App on **All repositories** of CoolEngOrg. Agentic tasks and reports read permissions and data on target repositories. Workflows narrow each token to what the job needs.

Store these on the `issueops` repository:

| Name | Kind | Value |
|---|---|---|
| `ISSUEOPS_APP_CLIENT_ID` | Variable | The App's client ID. Not sensitive; used by `actions/create-github-app-token` v3 |
| `ISSUEOPS_APP_PRIVATE_KEY` | Secret | A generated private key. Rotate every 90 days ([operations.md](operations.md#rotate-the-github-app-key)) |

Put the App's bot login (`<app-slug>[bot]`) in the Copilot and metrics runbooks. Workflows derive it automatically.

## 4. Organization and enterprise settings for Copilot

- Enterprise → Policies → Copilot: set **Copilot usage metrics** to *Enabled everywhere*. The usage-metrics report endpoints require it.
- Budgets are managed over the REST API (`/organizations/{org}/settings/billing/budgets`). For **cost-center** budgets, set `enterprise_scopes_enabled: true` in the registry. Then store a classic PAT of an enterprise billing manager as the environment secret `ENTERPRISE_BILLING_PAT` on `copilot-budgets-enterprise`. The enterprise budgets endpoints do not accept App or fine-grained tokens. Use a dedicated machine account.

## 5. Environments

Create these environments. Set a deployment branch policy of `main` only, required reviewers where shown, and prevent self-review.

| Environment | Required reviewers (suggested) | Variables | Secrets |
|---|---|---|---|
| `copilot-budgets` | none (comment approval is enough) | | |
| `copilot-budgets-high` | `finops-approvers` | | |
| `copilot-budgets-enterprise` | enterprise billing owner | | `ENTERPRISE_BILLING_PAT` |
| `copilot-reports` | none | | |
| `foundry-dev` | none | `AZURE_CLIENT_ID` | |
| `foundry-uat` | none | `AZURE_CLIENT_ID` | |
| `foundry-prod` | `platform-ai` | `AZURE_CLIENT_ID` | |
| `agentic-copilot` | none | | `COPILOT_AGENT_TOKEN` |
| `agentic-catalog` | none | | |
| `agentic-aks` | `platform-ai` | `AZURE_CLIENT_ID`, `AKS_RESOURCE_GROUP`, `AKS_CLUSTER_NAME` | |
| `acr-push` | none | `AZURE_CLIENT_ID`, `ACR_NAME` | |

Repository variables: `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID`, `PUSHGATEWAY_URL`, `ISSUEOPS_RUNS_ON` and `ISSUEOPS_AKS_RUNS_ON` (see step 9).

Repository secret: `PUSHGATEWAY_AUTH` (`user:password` for the Pushgateway ingress). Both the report workflow and the hourly ops-metrics workflow use it; ops-metrics runs outside any environment.

`COPILOT_AGENT_TOKEN` is a fine-grained PAT of a **Copilot-licensed machine user**. Give it Issues, Pull requests and Contents read and write on the target repositories. Assigning Copilot requires a user token; installation tokens are rejected.

## 6. Labels

Run **Actions → IssueOps · sync labels → Run workflow**. It creates the type, state and escalation labels from [config/labels.json](../config/labels.json).

## 7. Azure

```bash
export SUBSCRIPTION_ID=<subscription>
# Optional overrides: FOUNDRY_RG_DEV|UAT|PROD, PROD_ACCOUNT, AKS_RG, AKS_NAME, ACR_NAME, ACR_RG, MI_RG,
# OIDC_ISSUER (for a customized enterprise OIDC issuer).
deploy/azure/setup-oidc.sh
```

The script creates:

- One Entra app per GitHub environment. Each has a federated credential for `repo:CoolEngOrg/issueops:environment:<env>` and roles scoped to one resource group (or namespace or registry).
- A custom **IssueOps Foundry Quota Reader** role for `az cognitiveservices usage list`.
- The `id-issueops-agent-runner` managed identity. It is federated to `system:serviceaccount:issueops-agents:issueops-agent-runner` and holds **Cognitive Services OpenAI User** on the production Foundry account.

Copy the printed client IDs into the environments from step 5.

The Foundry accounts themselves (network isolation, private endpoints, CMK, diagnostics) remain landing-zone infrastructure. Self-service creates **deployments** on them only. Allow-list the accounts, models, versions, SKUs and capacity limits in `config/issueops.json` after confirming each with `az cognitiveservices account list-models`.

## 8. AKS (agent runner)

```bash
az aks update -g rg-cooleng-aks-prod -n aks-cooleng-prod-eus2 --enable-oidc-issuer --enable-workload-identity
# Set the managed identity client ID (script output) in deploy/aks/serviceaccount.yaml,
# and the deployer service principal object ID in deploy/aks/rbac.yaml. Then:
kubectl apply -k deploy/aks
kubectl get ns issueops-agents --show-labels   # pod-security.kubernetes.io/enforce=restricted
```

Build and push the runner image with **Actions → IssueOps · build agent runner** (version `1.0.0`). Then set `settings.backends.aks-foundry-agent.image` in the registry, preferably by digest. Set `foundry_endpoint` and `allowed_deployments` to the production Foundry account and its agent deployments. Create those deployments with a foundry-model-deployment request.

## 9. Runners

GitHub-hosted `ubuntu-latest` works for intake, commands, budgets and Foundry deployments. Use **self-hosted Ubuntu runners in the AKS virtual network** for:

- the AKS agent job, which needs the API server (private clusters)
- pushing metrics to the internal Pushgateway (reports and ops metrics)

Register them in a runner group restricted to `CoolEngOrg/issueops`, labeled `issueops`. The runner must be 2.327.1 or later, because the pinned actions use Node 24. Install Go 1.21 and the `az` CLI, or rely on `setup-go`. Then set:

```text
ISSUEOPS_RUNS_ON      = ["ubuntu-latest"]                       # or ["self-hosted","linux","issueops"]
ISSUEOPS_AKS_RUNS_ON  = ["self-hosted","linux","issueops","aks"]
```

## 10. Metrics and Grafana OSS 12

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm upgrade --install pushgateway prometheus-community/prometheus-pushgateway \
  -n monitoring --create-namespace -f deploy/pushgateway/values.yaml --version 3.9.0

helm repo add grafana-community https://grafana-community.github.io/helm-charts
helm repo update
kubectl -n grafana create configmap issueops-dashboards --from-file=deploy/grafana/dashboards/ \
  --dry-run=client -o yaml | kubectl apply -f -
helm upgrade --install grafana grafana-community/grafana -n grafana \
  -f gold-graf-values.yaml -f deploy/grafana/values-issueops.yaml --version 13.2.6
```

`gold-graf-values.yaml` stands for your existing base values (admin credentials, persistence, ingress, plugins). Omit it for a fresh install. The classic `grafana/grafana` chart is deprecated; the chart continues as `grafana-community/grafana`. The overlay pins the image to `grafana/grafana-oss:12.4.11`. It adds the `IssueOps Prometheus` data source and provisions two dashboards into an **IssueOps** folder: *Copilot ROI* and *Platform operations*. Set `PUSHGATEWAY_URL` to the internal ingress URL.

## 11. Tailor the registry

Edit [config/issueops.json](../config/issueops.json). See [approval-gating.md](approval-gating.md) for the policy language. Then:

```bash
scripts/forms-to-json.sh   # after any issue-form change
make test check-forms      # unit + end-to-end scenarios, form/registry drift
```

CODEOWNERS requests review from platform-admins, AI governance and FinOps. The ruleset from step 1 requires an approval from each.

## 12. Smoke tests (in dev)

1. **Budget:** open a repository budget request for a sandbox repository at $50. Check the summary, then `.submit` and approve as a copilot-admin. The budget should appear under Billing → Budgets.
2. **Report:** request a 7-day organization report without per-user detail. It auto-approves on `.submit`. Download the artifact and open `copilot-roi.html`.
3. **Foundry:** request `gpt-4.1-mini` GlobalStandard with capacity 1 on the dev account. It auto-approves. Check the endpoint in the completion comment.
4. **Agentic:** in a sandbox repository, run the catalog `docs-refresh` flow, then the AKS runner with a `design-doc` deliverable on a dev deployment.
5. **Negative tests:**
   - approve your own request (rejected)
   - edit the body after approval (gate refuses; `.submit` again)
   - `.approve` from someone outside the rules (rejected with the eligible approvers listed)

## Go-live checklist

- [ ] Ruleset on `main` with CODEOWNERS review and required checks
- [ ] Default `GITHUB_TOKEN` is read-only; the allowed-actions list is restricted
- [ ] App installed with the permissions above; private key stored as a secret; rotation reminder scheduled
- [ ] Environments created with reviewers and `main`-only deployment branches
- [ ] Federated credentials verified: each environment logs in and nothing else can
- [ ] Registry reviewed by FinOps (amounts, thresholds) and AI governance (models, classifications, backends)
- [ ] Labels synced; issue forms visible in *New issue*
- [ ] The Pushgateway is not publicly reachable; Grafana dashboards load
- [ ] Runbook owners named in [operations.md](operations.md)
- [ ] Announcement posted with the link to this repository's *New issue* page
