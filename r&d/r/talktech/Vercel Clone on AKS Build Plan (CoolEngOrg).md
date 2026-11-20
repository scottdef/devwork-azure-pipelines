# Vercel Clone on AKS: Build Plan (CoolEngOrg)

Nov 12, 2026 · @crafty

## 1. Summary

CoolDeploy (working name) gives CoolEngOrg developers the Vercel loop on your own stack: push to build, a preview per pull request, promote without rebuilding, and rollback without rebuilding. GitHub is the control plane, five AKS clusters are the runtime, and every change to a cluster arrives as a Git commit that Argo CD applies.

The design rests on seven decisions:

- **Git is the only write path to a cluster.** Every change lands as a commit in `CoolEngOrg/cool-gitops`. Argo CD on `prd-eus2-coolops-aks-001` pulls and syncs all five clusters. No workflow holds a kubeconfig, and app repos hold no deploy secret: a workflow proves who it is with a GitHub OIDC token, and the cool-api service writes the commit.
- **One Helm chart serves every app.** `cool-app` lives in `CoolEngOrg/cool-charts`, and Argo CD reads it straight from Git at a tag. App teams supply values, never templates.
- **Environments are clusters.** Preview and Development run on `dev-eus2-appdev-aks-001`, Staging on `tst`, Production on `prd`. `sbx-eus2-appsim-aks-001` tests the platform itself with simulated apps.
- **Static sites go to GitHub Pages. Anything with a server goes to AKS.**
- **IssueOps extends the existing `CoolEngOrg/issueops` engine.** New request types reuse `.submit`, `.approve` and `.deny`, and the policy registry in `config/issueops.json`.
- **Terraform owns Azure, except APIM.** APIM v2 is managed only by bash and `az` from YAML configs and OpenAPI specs in `CoolEngOrg/cool-apim`.
- **Go stays on the standard library.** Anything Go reads or writes is JSON. YAML appears only where Helm, Argo CD, kubectl or bash read it.

Out of scope for the first release: a global CDN and WAF, scale to zero, a URL per commit, durable queues and workflows, and Next.js-specific features such as ISR and image optimization. Section 19 says what each would take.

## 2. Feature map

Every Vercel capability in the user guide has a counterpart below, or is marked as a gap. Rows follow the order a developer meets them.

| Vercel capability | CoolDeploy equivalent | Built with |
| --- | --- | --- |
| Project | App repo from a template, a `cool.json` manifest, one namespace per environment | Template repos, IssueOps, `cool-app` chart |
| Git push deploys | Caller workflow → reusable `cool-build` → image in ACR → commit to `cool-gitops` → Argo CD sync | `cool-actions`, Argo CD |
| Static deployments | Reusable `cool-pages` workflow publishes the site | GitHub Pages |
| Preview deployment per PR | Namespace `<team>-<app>-pr-<n>` on the dev cluster with its own host, removed when the PR closes | ApplicationSet, Istio |
| Production and custom environments | The dev, tst and prd clusters | AKS, Terraform |
| Promote without rebuild | Copy the image digest from one environment's values file to the next after `.approve` | IssueOps, `coolctl` |
| Instant Rollback | The previous version stays deployed as a standby track; rollback flips Istio weights with one commit | `cool-app` chart, Istio |
| Rolling Releases | Staged weights (10, 50, 100) with a Prometheus error-rate gate between stages | Istio, managed Prometheus, `coolctl` |
| Functions on Fluid compute | A Deployment with an HPA behind the mesh; no duration limit | `cool-app` chart |
| Cron Jobs | `crons` in `cool.json` become Kubernetes CronJobs that call an app path with a bearer secret | `cool-app` chart |
| Routing Middleware, rewrites, redirects, headers | `routes` in `cool.json` become Istio routing rules; cross-cutting policy sits in APIM | Istio, APIM v2 |
| Environment variables | Plain values in a ConfigMap; secrets in Key Vault, mounted through the CSI driver | Key Vault, workload identity |
| Domains and TLS | A wildcard host per environment, custom domains by request | Terraform (DNS), Istio, APIM |
| Deployment Protection | Previews reachable only on the internal ingress; Entra ID token checks for APIs | Istio, APIM v2 |
| CDN and caching | APIM response cache for APIs, the Pages CDN for static sites | Gap: no global CDN for AKS apps |
| Firewall and rate limits | APIM rate-limit and IP-filter policies, Istio AuthorizationPolicy | Gap: no managed WAF |
| Observability and logs | Mesh and app metrics, a dashboard per app, logs through `coolctl logs` | Managed Prometheus, Managed Grafana |
| Speed Insights, Web Analytics | A raw JavaScript beacon posts Web Vitals to a Go collector | Go, managed Prometheus |
| Alerts | Prometheus rule groups with action groups | Terraform |
| Marketplace storage | Add-on requests provision Postgres, Redis or Blob; credentials land in Key Vault | IssueOps, Terraform |
| Edge Config and flags | `flags` in values become a ConfigMap the app re-reads without a restart | `cool-app` chart |
| AI Gateway and AI SDK | APIM AI gateway policies in front of Foundry model deployments; a small `cool-ai.js` helper | APIM v2, Foundry |
| Dashboard | `cool-portal`, a static HTMX site backed by a read-only status API | GitHub Pages, Go |
| `vercel` CLI | `coolctl` | Go standard library |
| REST API | Reads through `cool-api` behind APIM; CI deploys authenticate with GitHub OIDC, and production changes are issues | Go, APIM v2 |
| Activity log | Issue timeline, `cool-gitops` history, GitHub audit log | GitHub |
| Spend management | ResourceQuota and LimitRange per namespace | Kubernetes |

## 3. Naming conventions

Azure resources follow the pattern your cluster names already set: `<env>-<region>-<workload>-<type>-<nnn>`. Everything the platform ships starts with `cool`.

| Segment | Values |
| --- | --- |
| Environment | `sbx`, `dev`, `tst`, `prd` |
| Region | `eus2` |
| Workload | `coolops` (platform), `appsim` (platform testing), `appdev` (app teams) |

### Azure resources

| Resource | Name | Constraint |
| --- | --- | --- |
| AKS clusters | `prd-eus2-coolops-aks-001`, `sbx-eus2-appsim-aks-001`, `dev-`, `tst-`, `prd-eus2-appdev-aks-001` | As given |
| Resource group | `prd-eus2-coolops-rg-001` |  |
| Virtual network | `prd-eus2-coolops-vnet-001` |  |
| API Management | `dev-`, `tst-`, `prd-eus2-appdev-apim-001`, `sbx-eus2-appsim-apim-001` |  |
| Container registry | `prdeus2coolopsacr001` | Alphanumerics only, so no hyphens |
| Storage account | `prdeus2coolopsst001` | 3 to 24 lowercase letters and digits |
| Key Vault, platform | `prd-eus2-coolops-kv-001` | 3 to 24 characters |
| Key Vault, per team | `prd-eus2-<team>-kv-001` | The 24-character limit caps team slugs at 8 characters |
| Azure Monitor workspace | `prd-eus2-coolops-amw-001` |  |
| Managed Grafana | `prd-eus2-coolops-mg-001` | 23 characters at most; `amg` would make 24 |
| Foundry resource | `<env>-eus2-appdev-aif-001` |  |
| Managed identity | `prd-eus2-appdev-id-<team>-<app>` |  |

Limits are from Microsoft's [resource naming rules](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules) and the [Managed Grafana troubleshooting guide](https://learn.microsoft.com/en-us/azure/managed-grafana/troubleshoot-managed-grafana).

### GitHub

| Thing | Convention | Examples |
| --- | --- | --- |
| Enterprise and organization | Existing | `CoolEngEnt`, `CoolEngOrg` |
| Platform repositories | `cool-<thing>`, kebab case | `cool-actions`, `cool-gitops` |
| Template repositories | `cool-template-<stack>` | `cool-template-go-htmx` |
| GitHub App | Org name plus product | `CoolEngOrg CoolDeploy`, beside the existing `CoolEngOrg IssueOps` |
| Request types | `cool-<noun>-<verb>` | `cool-deploy-promote` |
| Labels | Existing `issueops:<phase>`, plus `cool:<type>` | `cool:rollback` |
| GitHub environments | `cool-<env>` | `cool-dev`, `cool-tst`, `cool-prd` |
| Reusable workflows | `cool-<verb>.yml` | `cool-build.yml` |
| Teams | Existing names, plus `cool-` for new platform roles | `platform-admins`, `cool-release-approvers` |
| Runner label | `cool-linux` |  |

### Kubernetes and DNS

| Thing | Convention | Example |
| --- | --- | --- |
| Team slug | `[a-z][a-z0-9]{1,7}` | `payments` |
| App slug | `[a-z][a-z0-9-]{1,19}` | `checkout` |
| App namespace | `<team>-<app>` | `payments-checkout` |
| Preview namespace | `<team>-<app>-pr-<n>` | `payments-checkout-pr-42` |
| Platform namespaces | `cool-system`, `cool-runners`, `argocd` |  |
| Labels | `cool/team`, `cool/app`, `cool/env`, `cool/release` |  |
| Release ID | `r` plus the 7-character commit SHA | `r3f9c2ab` |
| Image | `<registry>/<team>/<app>@sha256:...` | Deployed by digest, never by tag |
| App host | `<app>.<env>.coolapps.example` | `checkout.prd.coolapps.example` |
| Preview host | `<app>-pr-<n>.preview.dev.coolapps.example` |  |
| API host | `api.<env>.coolapps.example/<team>/<app>` |  |

`coolapps.example` stands in for your DNS zone. The product names are CoolDeploy for the platform, `coolctl` for the CLI, `cool-api` for the service, `cool-app` for the chart and `cool.json` for the manifest.

## 4. Architecture

GitHub decides, the hub cluster writes and syncs, the workload clusters run, and Azure services sit underneath.

&#91;embedded content: platform architecture · 4 layers, 12 components\]

Runners hand `cool-api` a signed token and an image digest. `cool-api` is the only writer to `cool-gitops`, and Argo CD is the only writer to a cluster. Users reach apps through APIM for APIs and through the Istio gateways for web pages.

### The five clusters

| Cluster | Role | Runs |
| --- | --- | --- |
| `prd-eus2-coolops-aks-001` | Platform hub for SRE, DevOps and PlatformOps | Argo CD, self-hosted runners, `cool-api`, platform collectors |
| `sbx-eus2-appsim-aks-001` | Platform test bed | Simulated apps that exercise each chart version, mesh revision and Argo CD upgrade before a team sees it |
| `dev-eus2-appdev-aks-001` | Development and previews | The `main` branch of every app, plus one namespace per open pull request |
| `tst-eus2-appdev-aks-001` | Staging | Promoted releases under production policy |
| `prd-eus2-appdev-aks-001` | Production | Promoted releases only |

`appsim` is a small Go binary with knobs for latency, error rate and CPU. Deployed many times over, it lets the sandbox rehearse a bad canary, a noisy neighbour or a mesh upgrade without touching a real app.

### Network

Each workload class has its own virtual network, peered to the hub. API servers are private. Argo CD reaches the four spoke API servers over the peering, and APIM's integration subnet reaches the internal gateways the same way.

### Kubernetes version

Run the clusters on AKS 1.35 or 1.36. AKS's calendar, updated 10 October 2026, puts end of life for 1.34 in November 2026, for 1.35 in March 2027 and for 1.36 in June 2027.

The project pins kubectl 1.30. Kubernetes supports kubectl within one minor version of the API server, so that client covers 1.29 to 1.31, and AKS long-term support for 1.31 ends in November 2026. Because no workflow talks to a cluster, the pin only touches `coolctl logs` and break-glass access. Raising it is the first decision in section 19.

## 5. Repository layout

Nine repositories in CoolEngOrg hold the platform. An app repo holds only source, a manifest and a twelve-line workflow.

| Repository | Visibility | Holds |
| --- | --- | --- |
| `issueops` (existing) | internal | Issue forms, router workflows, the policy registry and the engine CLI. Gains the CoolDeploy request types in section 6. |
| `cool-actions` | internal | Composite actions in `actions/<name>/action.yml` and reusable workflows in `.github/workflows/` |
| `cool-charts` | internal | Helm charts: `cool-app`, `cool-namespace`, platform charts, and vendored third-party charts |
| `cool-gitops` | private | Desired state for all five clusters: ApplicationSets, AppProjects and per-app values |
| `cool-infra` | private | Terraform modules and stacks |
| `cool-apim` | private | APIM YAML configs, OpenAPI specs, policy XML and the bash scripts that apply them |
| `coolctl` | internal | The Go CLI, the `cool-api` service and their Go templates |
| `cool-portal` | internal | The GitHub Pages portal and platform docs |
| `cool-template-*` | internal, template | Starters: `go-api`, `go-htmx`, `static-js` |

### Central actions and workflows

`cool-actions` is the only place pipeline logic lives. GitHub requires an internal repository to allow access explicitly: set its Actions access policy to repositories in the organization. Reusable workflows must sit directly in `.github/workflows/`. GitHub Enterprise Cloud allows ten levels of nesting and 50 unique reusable workflows per workflow file; this design uses three levels at most.

| Reusable workflow | Purpose |
| --- | --- |
| `cool-pipeline.yml` | The one entry point app repos call. Routes push, pull request and PR-close events. |
| `cool-build.yml` | Test, build the image with `az acr build`, record the digest |
| `cool-deploy.yml` | Ask `cool-api` to deploy a digest, then wait for Argo CD health |
| `cool-preview.yml` | Create, update and remove a PR preview |
| `cool-pages.yml` | Publish a static site to GitHub Pages |
| `cool-terraform.yml` | Plan on PR, apply on merge, per stack |
| `cool-apim-sync.yml` | Validate and apply `cool-apim` configs |

| Composite action | Purpose |
| --- | --- |
| `setup-coolctl` | Install the pinned `coolctl` binary |
| `azure-login` | Sign in to Azure with OIDC; no client secret |
| `acr-build` | Build and push by digest with ACR Tasks |
| `cool-token` | Request the GitHub OIDC token `cool-api` expects |
| `wait-healthy` | Poll deployment status until healthy or timed out |
| `prom-gate` | Run a PromQL check and fail on breach |
| `pr-comment` | Create or update one sticky PR comment |

App repos pin `@v1`, a major tag the platform team moves. One tag move ships a pipeline fix to every app. Third-party actions inside `cool-actions` are pinned by commit SHA.

### What an app repo contains

```
checkout/
├── cool.json                   # manifest: preset, port, routes, crons, env
├── .github/workflows/cool.yml  # caller workflow below
├── Dockerfile                  # optional; the preset supplies one
└── ...                         # source
```

```yaml
# .github/workflows/cool.yml
name: cool
on:
  push: { branches: [main] }
  pull_request: { types: [opened, synchronize, reopened, closed] }
permissions: { contents: read, id-token: write, pull-requests: write }
jobs:
  cool:
    uses: CoolEngOrg/cool-actions/.github/workflows/cool-pipeline.yml@v1
```

The workflow passes no secrets. Section 7 explains why that matters.

### GitOps repository

```
cool-gitops/
├── bootstrap/                        # root app per cluster
├── clusters/
│   ├── prd-eus2-coolops-aks-001/     # Argo CD, runners, cool-api
│   ├── sbx-eus2-appsim-aks-001/
│   ├── dev-eus2-appdev-aks-001/
│   ├── tst-eus2-appdev-aks-001/
│   └── prd-eus2-appdev-aks-001/
├── appsets/                          # apps, previews, namespaces
├── projects/                         # one AppProject per team
├── teams/<team>/team.json            # Entra group, quota tier
└── apps/<team>/<app>/
    ├── app.json                      # source repo, chart version, owners
    ├── dev/values.json
    ├── tst/values.json
    ├── prd/values.json
    └── previews/pr-<n>.json
```

Values files are JSON. JSON is valid YAML, so Helm and Argo CD read them unchanged, and `coolctl` writes them with `encoding/json`.

## 6. IssueOps control plane

CoolDeploy adds nine request types to `CoolEngOrg/issueops`. The engine stays as it is: the CLI decides and actions act, state is rebuilt from the issue timeline, and approvals bind to a content digest.

### Request types

Approval rules below are proposed defaults. They live in `config/issueops.json`, so changing one is a reviewed pull request, not a code change.

| Request type | Key form fields | Default approval | Environment | What the executor does |
| --- | --- | --- | --- | --- |
| `cool-app-create` | Team, app name, template, data classification | Auto-approved for members of the named team within quota | `cool-dev` | Creates the repo from a template, registers the app in `cool-gitops`, sets the repo's OIDC subject template, triggers the first deploy |
| `cool-deploy-promote` | App, target environment, release (default: latest healthy) | tst: one teammate. prd: one teammate plus one `cool-release-approvers` | `cool-tst`, `cool-prd` | Copies the release's image digest into the target values file |
| `cool-deploy-rollback` | App, environment, reason | Auto-approved for the app team when the target is the standby release | `cool-prd` | Flips traffic weights back to the standby release |
| `cool-rolling-release` | App, environment, stages, error budget, soak minutes | Same as promote | `cool-tst`, `cool-prd` | Advances weights stage by stage behind the Prometheus gate |
| `cool-env-var` | App, environment, key, value or secret name | Non-prod: auto. prd: one teammate | per target | Writes plain values to the values file; for a secret, creates the Key Vault entry and tells the requester how to set it |
| `cool-domain` | App, environment, hostname | One `platform-admins` | per target | Adds the DNS record request and the route host |
| `cool-addon` | App, environment, kind (postgres, redis, blob), size | One `platform-admins`; `finops-approvers` above the cost threshold | per target | Writes the add-on to the Terraform inputs and runs the stack |
| `cool-api-publish` | App, environment, OpenAPI path, product, auth mode | Non-prod: auto. prd or a new product: one `platform-admins` | per target | Opens the config change in `cool-apim` and runs the sync |
| `cool-app-retire` | App, typed confirmation | One teammate plus one `platform-admins` | `cool-prd` | Removes routes, then workloads, then add-ons; archives the repo |

Secret values never enter an issue. The `cool-env-var` executor creates an empty Key Vault secret and replies with the `az keyvault secret set` command the requester runs themselves.

### Request lifecycle

1. A developer opens a form. The portal links to forms with fields prefilled from the app page.
2. Intake parses and validates: names against allow-list patterns, team membership, that the app and release exist.
3. The requester comments `.submit`, which records the content digest.
4. Approvers comment `.approve`. Editing the issue after that voids the approvals.
5. The router calls `execute-cool-<type>.yml`. Its gate job re-checks the approvals from the timeline.
6. The execute job runs in the target GitHub environment, requests an OIDC token and calls `cool-api`.
7. `cool-api` checks the token's `environment` claim against the target, then commits to `cool-gitops`.
8. The finish job waits for Argo CD health, comments the release ID, URLs and dashboard link, and closes the issue.

Step 7 is the control that matters. A production change needs a token whose `environment` claim is `cool-prd`, and GitHub only issues that token to a job that passed the environment's required reviewers.

### The Go tools

One repository, `CoolEngOrg/coolctl`, builds two binaries from the standard library alone: the `coolctl` CLI and the `cool-api` service.

| Command | Run by | Does |
| --- | --- | --- |
| `coolctl init` | Developer | Scaffolds `cool.json` and the caller workflow from a preset |
| `coolctl validate` | Developer, CI | Checks `cool.json` against the manifest rules |
| `coolctl render` | CI | Renders the preset Dockerfile, dashboards and portal fragments from Go templates |
| `coolctl deploy` | CI | Sends a digest to `cool-api` and waits for health |
| `coolctl preview up`, `down` | CI | Creates or removes a PR preview |
| `coolctl promote`, `rollback`, `weights` | IssueOps | Production-path changes, each one commit |
| `coolctl gate` | CI, IssueOps | Runs a PromQL check against the Azure Monitor workspace |
| `coolctl status`, `logs`, `open` | Developer | Shows releases, tails logs through kubectl 1.30, opens the app URL |

```
coolctl/
├── cmd/coolctl/         # CLI entry point
├── cmd/cool-api/        # service entry point
├── internal/manifest/   # cool.json types and validation
├── internal/oidc/       # GitHub OIDC: JWKS fetch, RS256 verify, claims
├── internal/ghapp/      # App JWT, installation token, contents API
├── internal/gitops/     # read, change and write values.json
├── internal/argo/       # Application status over the in-cluster API
├── internal/prom/       # PromQL instant queries
├── internal/tmpl/       # embedded text/template and html/template files
└── go.mod               # go 1.24, no require block
```

The existing `issueops` CLI keeps deciding state and policy. `coolctl` only executes, so the two never share logic.

## 7. Build and deploy pipeline

A push to `main` reaches the dev cluster in five steps, and none of them needs a secret stored in the app repo.

1. The caller workflow invokes `cool-pipeline.yml@v1`.
2. `cool-build` validates `cool.json`, runs the preset's tests, renders a Dockerfile if the repo has none, and builds with `az acr build`. The output is an image digest.
3. `cool-deploy` requests a GitHub OIDC token with audience `cool-api` and posts the digest, commit SHA and environment to `cool-api`.
4. `cool-api` verifies the token, writes `apps/<team>/<app>/dev/values.json` in `cool-gitops`, and asks Argo CD to refresh the Application.
5. Argo CD renders the `cool-app` chart with those values and syncs. The workflow polls until the Application is healthy, then reports the URL.

### Why app repos hold no secret

The common shortcut is an organization secret holding a GitHub App key, so any app workflow can commit to the GitOps repo. That breaks tenancy: every workflow in every app repo can read that secret and rewrite another team's values.

CoolDeploy trusts a claim instead of a secret. A job defined by a reusable workflow carries a `job_workflow_ref` claim in its OIDC token, and the caller cannot add steps to that job. Two verifiers check it:

| Verifier | Checks | Grants |
| --- | --- | --- |
| Entra ID federated credential on the build identity | The token subject names `cool-build.yml` in `cool-actions` at tag `v1` | Push to the container registry |
| `cool-api` | Signature, issuer, audience, expiry, `job_workflow_ref`, and that `repository` is the app's registered repo | One commit under that app's path |

`cool-app-create` sets each new repo's subject template to `job_workflow_ref` through GitHub's OIDC customization API. Entra ID matches subjects exactly, so one credential keyed on the workflow covers every app repo. Two cautions: the template changes the subject for every federation that repo uses, and you should read the exact subject string from a test run in sbx before creating the credential.

```go
// fromCI reports whether a platform-owned pipeline file defines the job.
func fromCI(ref string) bool {
	return strings.HasPrefix(ref, "CoolEngOrg/cool-actions/.github/workflows/") &&
		strings.HasSuffix(ref, "@refs/tags/v1")
}

// fromIssueOps reports whether an IssueOps execute workflow defines the job.
func fromIssueOps(ref string) bool {
	return strings.HasPrefix(ref, "CoolEngOrg/issueops/.github/workflows/execute-cool-") &&
		strings.HasSuffix(ref, "@refs/heads/main")
}

// authorize reports whether the token's job may deploy app to env.
// The caller has already verified the signature against GitHub's JWKS.
func authorize(c claims, app App, env string, now time.Time) error {
	if c.Iss != issuer || c.Aud != audience || now.Unix() >= c.Exp {
		return errors.New("bad issuer, audience or expiry")
	}
	switch {
	case fromCI(c.JobWorkflowRef) && c.Repository == app.Repo && env == "dev":
		return nil
	case fromIssueOps(c.JobWorkflowRef) && c.Repository == "CoolEngOrg/issueops" &&
		c.Environment == "cool-"+env:
		return nil
	}
	return fmt.Errorf("%s may not deploy %s to %s", c.Repository, app.Name, env)
}
```

The CoolDeploy App's private key lives in Key Vault and is mounted only into `cool-api`. It never becomes a GitHub secret.

### The manifest

```json
{
  "team": "payments",
  "app": "checkout",
  "preset": "go-htmx",
  "port": 8080,
  "health": "/healthz",
  "scale": { "min": 2, "max": 10, "cpu": 70 },
  "resources": { "cpu": "250m", "memory": "256Mi" },
  "env": { "LOG_LEVEL": "info" },
  "secrets": ["DATABASE_URL"],
  "routes": [
    { "prefix": "/api/", "timeout": "30s" },
    { "redirect": { "from": "/old", "to": "/new", "code": 308 } }
  ],
  "crons": [{ "path": "/internal/cleanup", "schedule": "0 3 * * *" }],
  "api": { "openapi": "api/openapi.yaml", "product": "payments" }
}
```

### Build presets

| Preset | For | Image |
| --- | --- | --- |
| `go-server` | A `net/http` server that listens on `$PORT` | Multi-stage build on Go 1.24, static binary on a distroless base |
| `go-htmx` | The same, with templates and static files embedded through `embed` | Same |
| `static` | Plain HTML and JavaScript with no server | A platform-supplied Go file server plus the site files |
| `dockerfile` | Anything else | The repo's own Dockerfile; must run as non-root and listen on `$PORT` |

Base images are imported into `prdeus2coolopsacr001` and pinned by digest. `az acr build` runs the build inside the registry service, so runners need no Docker daemon.

### Charts and Argo CD

- **Charts come from Git.** Argo CD reads `charts/cool-app` from `cool-charts` at a tag such as `cool-app-1.4.0`. Third-party charts are vendored into the same repo, so every chart on a cluster has passed your review.
- **One hub.** Argo CD runs on `prd-eus2-coolops-aks-001` and registers the other four clusters with workload identity, so it stores no cluster credentials.
- **One AppProject per team.** It limits sources to `cool-charts` and `cool-gitops`, destinations to the team's namespaces, and resource kinds to what `cool-app` renders.
- **One ApplicationSet per environment.** A Git files generator turns each `values.json` into an Application.
- **Platform values win.** A per-cluster file is listed last, so teams cannot override hosts, gateways, the registry allow-list or the mesh revision.

```yaml
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: cool-apps-dev
  namespace: argocd
spec:
  goTemplate: true
  goTemplateOptions: ["missingkey=error"]
  generators:
    - git:
        repoURL: https://github.com/CoolEngOrg/cool-gitops
        revision: main
        files:
          - path: apps/*/*/dev/values.json
  template:
    metadata:
      name: '{{.team}}-{{.app}}-dev'
    spec:
      project: '{{.team}}'
      destination:
        name: dev-eus2-appdev-aks-001
        namespace: '{{.team}}-{{.app}}'
      sources:
        - repoURL: https://github.com/CoolEngOrg/cool-charts
          targetRevision: '{{.chart}}'
          path: charts/cool-app
          helm:
            valueFiles:
              - $values/{{.path.path}}/values.json
              - $values/clusters/dev-eus2-appdev-aks-001/platform-values.json
        - repoURL: https://github.com/CoolEngOrg/cool-gitops
          targetRevision: main
          ref: values
      syncPolicy:
        automated: { prune: true, selfHeal: true }
```

Install Argo CD from its Helm chart once, then let it manage itself from `cool-gitops/bootstrap`. Microsoft's Argo CD extension for AKS entered public preview in March 2026; revisit it when it reaches general availability.

## 8. Previews, promote, rollback and rolling releases

All four are edits to one JSON file, so each is one commit and one Argo CD sync.

### The release list

Each environment's `values.json` carries a `releases` list. The chart renders one Deployment per entry, one DestinationRule subset per release, and a VirtualService that splits traffic by `weight`. A standby release keeps one replica warm.

```json
"releases": [
  { "id": "r3f9c2ab", "digest": "sha256:9b1c...", "weight": 100 },
  { "id": "r81d04e7", "digest": "sha256:47aa...", "weight": 0, "standby": true }
]
```

| Operation | Change to `releases` | Trigger |
| --- | --- | --- |
| Deploy to dev | Add the new release at 100; the old one becomes standby | Push to `main` |
| Promote | Add the source environment's digest at 100 in the target | `cool-deploy-promote` |
| Instant rollback | Swap the weights of the active and standby releases | `cool-deploy-rollback` |
| Rolling release | Add the new release at 10, then raise it stage by stage | `cool-rolling-release` |
| Retire | Drop the standby entry once its retention period ends | Scheduled workflow |

Promotion never rebuilds: the digest that passed in dev is the digest that runs in prd. Each retained release also answers on its own host, `<app>-<release>.<env>.coolapps.example`. That is the closest equivalent to Vercel's URL per deployment.

### Rolling releases

&#91;embedded content: rolling release · 5 steps, 1 decision\]

A breach at any stage sends all traffic back to the standing release. The check is one PromQL query against the mesh's request metrics:

```promql
sum(rate(istio_requests_total{destination_workload="checkout-r3f9c2ab", response_code=~"5.."}[5m]))
/
sum(rate(istio_requests_total{destination_workload="checkout-r3f9c2ab"}[5m]))
```

Stages, soak time and the error budget are form fields with defaults in the policy registry. `coolctl gate` exits non-zero on a breach, and the workflow then runs `coolctl weights` to restore the previous split.

### Previews

1. A pull request opens, and `cool-build` produces a digest for the PR head.
2. `cool-api` writes `apps/<team>/<app>/previews/pr-<n>.json`.
3. The previews ApplicationSet creates namespace `<team>-<app>-pr-<n>` on the dev cluster and installs the app with one replica.
4. The workflow posts one sticky comment with the preview URL and updates it on each push.
5. When the PR closes, the file is deleted and Argo CD prunes the namespace.

Previews bind to the internal gateway only, and they read the dev environment's secrets and add-ons unless the manifest opts out. Set a cap per app and an idle expiry in the platform values; five previews and seven days are reasonable starting points.

## 9. App runtime

Every app is one container that listens on `$PORT`. Three templates make the common shapes a single request away.

| Template | Shape |
| --- | --- |
| `cool-template-go-htmx` | A Go `net/http` server that renders `html/template` pages and returns HTML fragments to HTMX. Assets ship inside the binary through `embed`. |
| `cool-template-go-api` | A JSON API on `http.ServeMux` method patterns with `log/slog`, plus an OpenAPI file that section 10 publishes |
| `cool-template-static-js` | HTML and raw JavaScript modules with no build step. Production goes to GitHub Pages; PR previews run on the dev cluster with the `static` preset. |

Node backends are outside the named tool list. Teams that need one use the `dockerfile` preset; there is no Node template in the first release.

### The runtime contract

- Listen on `$PORT` and answer `GET /healthz` with 200.
- On SIGTERM, stop accepting connections and drain within 25 seconds.
- Log JSON lines to stdout.
- Read configuration from environment variables and secrets from mounted files.
- Keep no state on disk. The root filesystem is read-only and the process is non-root.

The chart injects `PORT`, `COOL_TEAM`, `COOL_APP`, `COOL_ENV`, `COOL_RELEASE` and `COOL_URL`, the counterparts of Vercel's system environment variables.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// An HTMX fragment: <div hx-get="/fragments/time" hx-trigger="every 5s"></div>
	mux.HandleFunc("GET /fragments/time", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "<span>%s</span>", time.Now().Format(time.Kitchen))
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("listening", "port", port, "release", os.Getenv("COOL_RELEASE"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()

	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		log.Error("shutdown", "err", err)
	}
}
```

### Front ends

- **HTMX.** The server returns HTML fragments. The template vendors one pinned `htmx.min.js` file; there is no npm and no bundler.
- **Raw JavaScript.** ES modules are served as static files, with `fetch` for APIs and `EventSource` for streams.
- **Caching.** The Go server fingerprints embedded assets at start-up and serves them as immutable. HTML is sent with `no-cache`.

### Functions, background work and cron

| Vercel concept | Here |
| --- | --- |
| A function per route | One server per app; routes are handlers |
| Max duration | None. Set a per-route timeout in `routes` when you want one. |
| `waitUntil` | A goroutine that the SIGTERM drain waits for |
| Scaling | An HPA on CPU between `scale.min` and `scale.max` |
| Scale to zero | Not in the first release; see section 19 |
| Cron Jobs | A Kubernetes CronJob per `crons` entry, with `concurrencyPolicy: Forbid` |

A cron pod stays outside the mesh and calls the app through the internal gateway with `Authorization: Bearer $CRON_SECRET`. That way the Job finishes cleanly instead of waiting on a sidecar.

## 10. Edge and API layer

Traffic enters by one of two lanes: APIs through APIM v2, web pages straight to the mesh's ingress gateway. Both end at the same Istio routing rules.

| Lane | Host | Path |
| --- | --- | --- |
| API | `api.<env>.coolapps.example` | Client → APIM v2 policies → Istio internal gateway → app |
| Web | `<app>.<env>.coolapps.example` | Client → Istio external gateway → app |
| Preview | `<app>-pr-<n>.preview.dev.coolapps.example` | Corporate network → Istio internal gateway → app |

### APIM v2 instances

| Instance | Tier | Why |
| --- | --- | --- |
| `sbx-eus2-appsim-apim-001` | Basic v2 | Tests the scripts and policies against mock backends |
| `dev-`, `tst-`, `prd-eus2-appdev-apim-001` | Standard v2 | Needs virtual network integration to reach the internal gateway |

Standard v2 reaches private backends through outbound virtual network integration, and its gateway keeps a public inbound address. InfoQ reported its price in September 2026 as starting at $700 a month per unit. The v2 tiers do not offer the free managed TLS certificate, backup and restore, the self-hosted gateway or multi-region deployment, so bring certificates from Key Vault and treat `cool-apim` as the backup.

### Managing APIM with bash and az

Terraform never touches APIM. The `cool-apim` repository is the source of truth, and bash scripts apply it.

```
cool-apim/
├── instances/<instance>.yaml      # tier, units, subnet name, hostnames
├── global/
│   ├── policy.xml                 # all-APIs policy
│   ├── named-values.yaml
│   └── backends.yaml              # mesh-internal, foundry
├── fragments/                     # jwt.xml, rate-limit.xml, cors.xml, ai.xml
├── products/<product>.yaml
├── apis/<team>/<app>/
│   ├── api.yaml                   # path, product, auth, limits
│   └── openapi.yaml               # copied from the app repo at publish
└── scripts/
    ├── lib.sh                     # az wrappers, logging, yaml2json
    ├── apim-instance.sh           # create or update the service
    ├── apim-global.sh             # named values, backends, fragments, global policy
    ├── apim-product.sh
    ├── apim-api.sh                # import spec, attach product, set policy
    ├── apim-diff.sh               # live state against the repo
    └── apim-sync.sh               # runs the others for one instance
```

Three facts about the CLI shape the scripts:

- **`az apim create` does not list the v2 SKUs.** Its documented `--sku-name` values are the classic tiers and Consumption. `apim-instance.sh` therefore creates the service with `az rest` against the ARM API, version 2024-05-01 or later.
- **`az apim` has no policy command.** API, product and global policies also go through `az rest`.
- **Everything else has a command.** `az apim api import`, `az apim product`, `az apim nv` and `az apim backend` cover specs, products, named values and backends.

```yaml
# apis/payments/checkout/api.yaml
id: payments-checkout
displayName: Checkout API
path: payments/checkout
product: payments
backend: mesh-internal
auth:
  mode: entra-jwt            # none | subscription | entra-jwt
  audience: api://cool-payments-checkout
limits:
  callsPerMinute: 600
```

```bash
#!/usr/bin/env bash
# apim-api.sh: publish one API to one instance.
# usage: apim-api.sh <instance> <api-dir>
set -euo pipefail
. "$(dirname "$0")/lib.sh"

instance=$1 dir=$2
cfg=$(yaml2json "$dir/api.yaml")      # one conversion, then jq
id=$(jq -Mr .id <<<"$cfg")
rg=$(instance_rg "$instance")

az apim api import \
  --resource-group "$rg" --service-name "$instance" \
  --api-id "$id" --path "$(jq -Mr .path <<<"$cfg")" \
  --display-name "$(jq -Mr .displayName <<<"$cfg")" \
  --specification-format OpenApi \
  --specification-path "$dir/openapi.yaml" \
  --subscription-required "$(jq -Mr '.auth.mode == "subscription"' <<<"$cfg")"

az apim product api add \
  --resource-group "$rg" --service-name "$instance" \
  --product-id "$(jq -Mr .product <<<"$cfg")" --api-id "$id"

# No az command sets a policy, so put it through ARM.
policy=$(render_policy "$cfg")
az rest --method put \
  --url "$(arm_id "$instance")/apis/$id/policies/policy?api-version=2024-05-01" \
  --body "$(jq -Mn --arg xml "$policy" '{properties:{format:"rawxml",value:$xml}}')"
```

Bash cannot parse YAML, so `yaml2json` converts each file once and `jq -M` does the rest. The converter is the one tool this plan adds: `yq`, a single static binary pinned by checksum. If you prefer to add nothing, `helm template` with `toJson` does the same job with tools already on the list.

The scripts find subnets, vaults and gateways by name with `az`, which the naming convention makes deterministic. They never read Terraform state.

**Change flow.** A pull request to `cool-apim` runs `apim-diff.sh` and comments the plan. Merge applies to dev. The `cool-tst` and `cool-prd` environments gate the other two. A nightly run of `apim-diff.sh` reports drift. The `cool-api-publish` request type copies an app's OpenAPI file into `apis/<team>/<app>/` by pull request, so app teams never need write access here.

### Istio ingress

- **Gateways.** Enable both add-on gateways on each appdev cluster. They run in `aks-istio-ingress`. One shared `Gateway` per lane lives in `cool-system`; each app's VirtualService binds to it.
- **API choice.** Use Istio's own Gateway and VirtualService resources in the first release. The add-on also supports Kubernetes Gateway API ingress from revision asm-1-26 with the managed Gateway API CRDs. Its page does not state whether that support is GA, so confirm before adopting it. The chart keeps routing in one template, which keeps a later switch cheap.
- **Revisions.** As of late October 2026, asm-1-30 and asm-1-31 are the current revisions, both compatible with AKS 1.32 to 1.37. Minor upgrades are canary upgrades: add the revision, relabel namespaces in waves through `cool-gitops`, restart workloads. Rehearse each one on `sbx-eus2-appsim-aks-001`.
- **Mesh policy.** STRICT mutual TLS mesh-wide, and a default-deny AuthorizationPolicy per namespace that admits the gateways and same-namespace callers.

### Domains and TLS

- **Wildcards.** Terraform owns the zone and each environment's records: a web wildcard, the API host and, on dev, a preview wildcard.
- **Custom domains.** `cool-domain` appends a hostname to a JSON list that the DNS stack reads, and adds the host to the app's route.
- **Certificates.** Certificates live in Key Vault. The Secrets Store CSI Driver add-on syncs them into `aks-istio-ingress` for the gateways, and APIM reads them directly. Issuance is an open decision in section 19.

## 11. Terraform

Terraform owns every Azure resource except APIM, and nothing inside a cluster.

| Stack | Environments | Contains |
| --- | --- | --- |
| `00-state` | once | The state storage account `prdeus2coolopsst001` and its locks |
| `10-network` | all | Virtual networks, subnets, hub-to-spoke peering, private DNS zones |
| `20-coolops` | prd | The hub cluster, container registry, platform Key Vault, Azure Monitor workspace, Managed Grafana, identities for Argo CD and `cool-api` |
| `30-appsim` | sbx | The sandbox cluster |
| `40-appdev` | dev, tst, prd | The app cluster, DNS zone and wildcard records, gateway addresses |
| `50-foundry` | dev, tst, prd | The Foundry resource, project and role assignments |
| `60-teams` | dev, tst, prd | Per-team vaults, per-app managed identities and their federated credentials |
| `70-addons` | dev, tst, prd | Postgres, Redis and storage requested through `cool-addon` |

### Rules

- **State.** The azurerm backend holds one key per stack and environment.
- **Identity.** Workflows sign in to Azure with GitHub OIDC. The plan identity is read-only. Each apply identity is bound to one GitHub environment.
- **Flow.** `cool-terraform.yml` plans on pull request and comments the result. Merge applies to dev; `cool-tst` and `cool-prd` approvals gate the rest.
- **JSON in, HCL fixed.** Stacks 60 and 70 read JSON with `jsondecode` and `for_each`. IssueOps appends an entry; nobody generates HCL.
- **No cluster providers.** The stacks do not use the kubernetes or helm providers. A bootstrap workflow installs Argo CD on the hub once with `helm upgrade --install`, and Argo CD installs everything else.
- **The APIM line.** Terraform creates the subnet APIM integrates with, the DNS records and the Key Vault certificates, and stops there. CI fails any plan that contains an `azurerm_api_management` resource.

### The cluster module

All five clusters come from one module, so they differ only in variables.

```hcl
resource "azurerm_kubernetes_cluster" "this" {
  name                = var.name # dev-eus2-appdev-aks-001
  location            = var.location
  resource_group_name = var.resource_group_name
  dns_prefix          = var.name
  kubernetes_version  = var.kubernetes_version

  oidc_issuer_enabled       = true
  workload_identity_enabled = true
  local_account_disabled    = true

  azure_active_directory_role_based_access_control {
    azure_rbac_enabled = true
    tenant_id          = var.tenant_id
  }

  network_profile {
    network_plugin      = "azure"
    network_plugin_mode = "overlay"
    network_data_plane  = "cilium"
  }

  service_mesh_profile {
    mode                             = "Istio"
    revisions                        = var.mesh_revisions # ["asm-1-31"]
    internal_ingress_gateway_enabled = true
    external_ingress_gateway_enabled = var.external_gateway
  }

  monitor_metrics {}

  key_vault_secrets_provider {
    secret_rotation_enabled = true
  }

  default_node_pool {
    name                 = "system"
    vm_size              = var.system_vm_size
    auto_scaling_enabled = true
    min_count            = 3
    max_count            = 5
    zones                = ["1", "2", "3"]
    vnet_subnet_id       = var.node_subnet_id
  }

  identity {
    type = "SystemAssigned"
  }
}
```

Argument names follow the azurerm 4.x provider. Run `terraform validate` against the version you pin before relying on them.

## 12. Observability

The mesh measures every request, so each app gets request rate, error rate and latency without writing code.

| Signal | Source | Seen through |
| --- | --- | --- |
| Request rate, errors, latency | Envoy sidecars, per release | The app's Grafana dashboard and `coolctl gate` |
| App metrics | The app's `/metrics` endpoint, merged by the sidecar | The same dashboard |
| Platform metrics | Kubernetes, Argo CD, `cool-api` | Platform dashboards |
| Deploy events | `cool-api` writes a Grafana annotation per release | Overlays on every dashboard |
| Logs | JSON lines on stdout | `coolctl logs`; retention is an open decision in section 19 |
| Web Vitals | A raw JavaScript beacon posting to a Go collector | The app's dashboard, from phase 6 |
| Alerts | Prometheus rule groups with action groups | Your paging and chat channels |

### Managed Prometheus

- **One workspace.** All five clusters send to `prd-eus2-coolops-amw-001`, separated by the `cluster` label.
- **Scrape config is GitOps.** A platform chart writes the `ama-metrics-settings-configmap` that turns on pod-annotation scraping for team namespaces and the mesh namespace.
- **Keep the volume down.** Microsoft warns that annotation scraping across many namespaces can produce a large volume of metrics. The chart ships an allow-list of the mesh metrics the dashboards and gates use.
- **Mutual TLS needs proof.** Microsoft's integration page lists collecting metrics from an Istio setup with mutual TLS as unsupported. The plan relies on the sidecar's merged-metrics port, which sits outside mutual TLS and carries the app's metrics with the mesh's. Prove that on the sandbox in phase 1.
- **No client library.** The Go templates include a small standard-library package that writes counters and histograms in the Prometheus text format.

### Managed Grafana

- **Workspace.** `prd-eus2-coolops-mg-001` on the Standard tier, linked to the Azure Monitor workspace. The Essential tier is deprecated.
- **Access.** Entra ID groups map to Grafana roles, with one folder per team.
- **Dashboards as code.** `coolctl render` turns a Go template into dashboard JSON per app, and a workflow applies it with the `az grafana` extension. If the workspace runs Grafana 13 or later, Git Sync can read dashboards from a GitHub repository instead.
- **No mesh graph.** Microsoft lists Kiali as unsupported with managed Prometheus, so the Istio dashboards in Grafana are the mesh view.

### Alerts and gates

Put per-app alerts in Azure Monitor Prometheus rule groups, generated by Terraform from the app list. Managed Grafana caps alert rules at 500 per Standard X1 instance, which a growing platform would outrun.

Every app starts with four rules: 5xx ratio, p95 latency, restart count and an HPA pinned at its maximum. The rolling-release gate in section 8 runs the same queries through the workspace's query endpoint with an Entra ID token.

Dynatrace already exists in your estate. This plan does not depend on it, and nothing here prevents its operator from running beside the managed stack.

## 13. Data services

Add-ons are the Marketplace equivalent: a `cool-addon` request provisions an Azure service with Terraform and hands its connection details to the app through Key Vault.

| Vercel offering | Add-on kind | Azure service | From Go, standard library only |
| --- | --- | --- | --- |
| Postgres (Neon, Supabase) | `postgres` | Azure Database for PostgreSQL flexible server | `database/sql` is in the standard library, but every Postgres driver is an external module and needs your approval |
| Redis (Upstash) | `redis` | Azure Managed Redis | The Redis wire protocol is small enough to speak over `crypto/tls`; the template carries a minimal client |
| Blob | `blob` | Azure Blob Storage | REST over `net/http` with an Entra ID token |
| Edge Config | none | A ConfigMap rendered from `flags` | Read a file |

Use Azure Managed Redis, not Azure Cache for Redis. Microsoft is retiring the older service: the Enterprise tiers on 31 March 2027, and Basic, Standard and Premium on 30 September 2028.

### How an add-on arrives

1. A `cool-addon` request names the app, environment, kind and size.
2. After approval, the executor appends one entry to the environment's `addons.json` in `cool-infra`.
3. The `70-addons` stack creates the service with a private endpoint and writes its endpoint or connection string to the team's Key Vault.
4. `cool-api` adds the secret name to the app's values. The CSI driver mounts it, and the next release starts with `DATABASE_URL` or `REDIS_URL` set.

### Practices

- **Identity before passwords.** Where the service accepts Entra ID tokens, grant the app's managed identity a role and store no password. The template includes a standard-library token exchange for workload identity.
- **Sizes are a table.** `small`, `medium` and `large` map to SKUs in one JSON file, and `finops-approvers` gate anything above the cost threshold.
- **Pooling is simpler than on Vercel.** Pods are long-lived, so a normal connection pool works. Keep pool size times maximum replicas under the server's connection limit.
- **Previews share dev data** unless the manifest opts out. A schema per pull request is a later refinement.

## 14. AI layer

The AI Gateway equivalent is one APIM API, `cool-ai`, in front of your Foundry model deployments, with a token budget per app. Microsoft's documentation now calls the service Microsoft Foundry; this section means that product.

| Vercel | Here |
| --- | --- |
| AI Gateway: one endpoint, many models | The `cool-ai` API on each environment's APIM, with Foundry deployments as a load-balanced backend pool |
| Provider keys | None. APIM's managed identity calls Foundry, and apps authenticate to APIM. |
| Spend limits | The `llm-token-limit` policy with one counter per app |
| Usage telemetry | Token metrics that APIM emits to Azure Monitor, tracked per consumer in Application Insights |
| AI SDK streaming helpers | `cool-ai.js`, a raw JavaScript stream reader, and a Go relay handler in the templates |
| Model catalog | The existing `foundry-model-deployment` request type and its approvers |

### Build steps

1. The `50-foundry` stack creates a Foundry resource and project per environment and grants the APIM identity access.
2. Model deployments keep flowing through the existing `foundry-model-deployment` request, approved by `platform-ai` and `ai-governance`.
3. `cool-apim` gains `apis/platform/cool-ai/` and the policy fragment below.
4. An app opts in with an `ai` block in `cool.json`. The platform creates its APIM subscription, stores the key in the team vault and sets `COOL_AI_URL` and `COOL_AI_KEY`.
5. The templates gain the two helpers.

```xml
<fragment>
  <authentication-managed-identity resource="https://cognitiveservices.azure.com" />
  <llm-token-limit counter-key="@(context.Subscription.Id)"
                   tokens-per-minute="20000"
                   estimate-prompt-tokens="true" />
  <llm-emit-token-metric namespace="cool-ai">
    <dimension name="app" value="@(context.Subscription.Name)" />
  </llm-emit-token-metric>
</fragment>
```

The limit shown is a placeholder; budgets come from the size named in the manifest.

### Platform uses

- **Build triage.** When `cool-build` fails, a step sends the tail of the log to `cool-ai` and posts a short diagnosis on the pull request. Log text is passed as data, never as instructions.
- **Agent runs.** The existing `agent-runner` can call models through `cool-ai`, which puts agent tasks under the same budgets.

### Cautions

- **One writer for APIM.** Foundry's own AI Gateway feature, in preview, drives an APIM instance from the Foundry portal. Do not point it at an instance that `cool-apim` manages.
- **Semantic caching needs Redis.** APIM's semantic cache requires an external cache such as Azure Managed Redis. Leave it for later.
- **Application Insights is outside the named tool list.** Per-consumer token tracking uses it, so decide whether to add it; your estate already uses it.
- **A dedicated AI Gateway tier** of APIM entered public preview in August 2026. Watch it; do not build on it yet.

## 15. Palantir AIP and Foundry integrations

Low priority, outline only. Palantir Foundry is a different product from Microsoft Foundry, so this section always names it in full.

Palantir documents a REST API at `https://<hostname>/api/v2/...`. Production clients use OAuth2, either authorization code on behalf of a user or client credentials as a service user, with scopes such as `api:ontologies-read`. Clients are registered in the Developer Console.

| Integration | Outline | Palantir API area |
| --- | --- | --- |
| Egress facade | Publish the Palantir Foundry API as an APIM API, `palantir-foundry`. APIM obtains and caches the client-credentials token, so apps never hold the client secret. | OAuth2 client credentials |
| Ontology access from Go | Read objects, run queries and apply actions with `net/http`. The service user's scopes bound what an app can do. | Ontologies v2 |
| Delivery telemetry out | `cool-api` publishes each deploy, promote and rollback as a record to a Palantir Foundry stream for delivery analytics. | Streams |
| AIP Agents | An app opens an agent session and continues it in blocking or streaming mode through APIM. | AIP Chatbot Studio: agents and sessions |
| AIP models behind `cool-ai` | Palantir's OpenAI-compatible and Anthropic-compatible proxy endpoints register as another backend of `cool-ai`. | LLM APIs |
| Static hosting | Palantir's API reference lists a Deploy Website endpoint, a possible second target for `cool-pages`. | Third-party applications |
| Inbound triggers | A Palantir Foundry action calls APIM, which opens a prefilled IssueOps issue. | Actions |

Four questions to answer before any of this is scheduled:

- What is the enrollment hostname, and is the network path public or private?
- Who owns the Developer Console application and its service user?
- Which markings and classification rules govern data that leaves Palantir Foundry?
- Is AIP enabled on the enrollment?

## 16. Portal and docs

The portal is a static site on GitHub Pages. HTMX swaps in HTML fragments, raw JavaScript handles sign-in, and no page needs a bundler.

| Page | Shows | Data |
| --- | --- | --- |
| Apps | Every app with its team, environments and current release | Static fragment, rebuilt on each `cool-gitops` change |
| App | Releases and weights per environment, previews, links to the dashboard and open requests | Static fragment plus live status from `cool-api` |
| New request | Buttons that open IssueOps forms with fields prefilled | Links with query strings |
| Catalog | Templates, add-on kinds and API products | Static |
| Docs | Quickstart, `cool.json` reference, runbooks | Markdown built by Pages |

### How it works

1. **Static first.** When `cool-gitops` changes, a workflow in `cool-portal` runs `coolctl render` to write fragments such as `fragments/apps.html`, then publishes the site.
2. **Live second.** A later phase adds fragments served by `cool-api` and rendered with `html/template`. APIM applies a CORS policy for the portal's origin and checks an Entra ID token.
3. **Sign-in without a library.** The authorization code flow with PKCE fits in about a hundred lines of raw JavaScript using Web Crypto. One event listener attaches the token to API requests.

```html
<meta name="htmx-config" content='{"selfRequestsOnly":false}'>

<main hx-get="fragments/apps.html" hx-trigger="load"></main>

<section hx-get="https://api.prd.coolapps.example/cool/v1/apps/checkout/status"
         hx-trigger="load, every 15s"></section>

<script type="module">
  import { token } from "./js/auth.js"; // PKCE sign-in in raw JavaScript
  document.body.addEventListener("htmx:configRequest", (e) => {
    if (e.detail.path.startsWith("https://api.")) {
      e.detail.headers["Authorization"] = "Bearer " + token();
    }
  });
</script>
```

HTMX 2 refuses cross-origin requests unless `selfRequestsOnly` is turned off, which the first line does.

### GitHub Pages limits that shape the design

- **Size and time.** A published site may not exceed 1 GB, and a deployment times out after 10 minutes. Keep fragments small and render only apps that changed.
- **Traffic.** Pages has a soft bandwidth limit of 100 GB a month. That suits a portal and internal static sites, not a high-traffic public product.
- **Visibility.** On GitHub Enterprise Cloud a Pages site can be private to people with access to the repository. With Enterprise Managed Users, Pages sites are always private.
- **No server code.** Pages serves files only, which is why live data comes from `cool-api`.

### Static app sites

The `cool-pages.yml` workflow publishes a `static-js` app with GitHub's upload and deploy actions. Pages gives one site per repository and no pull request previews, so previews for static apps run on the dev cluster through the `static` preset.

## 17. Security and tenancy

Teams share the three appdev clusters, so isolation is layered, and the platform owns every layer.

| Layer | Control | Defined in |
| --- | --- | --- |
| Namespace | ResourceQuota, LimitRange, Pod Security `restricted`, default-deny NetworkPolicy | `cool-namespace` chart |
| Mesh | STRICT mutual TLS; AuthorizationPolicy admits the gateways and same-namespace callers | `cool-namespace` chart |
| Kubernetes access | One Entra ID group per team, bound to `view` in its own namespaces. Changes go through Git. | `cool-namespace` chart |
| GitOps | One AppProject per team limits sources, destinations and resource kinds | `cool-gitops/projects` |
| Cloud identity | One managed identity per app, federated to its service account, with roles only on its own vault and add-ons | `60-teams` stack |
| Images | A ValidatingAdmissionPolicy admits images only from `prdeus2coolopsacr001`. Kubernetes has shipped that API as stable since 1.30, so it needs no extra controller. | Platform chart |
| Secrets | Key Vault through the CSI driver. No app secret lives in Git, an issue or a GitHub secret. | `cool-app` chart |
| Pipeline trust | OIDC claims, not shared secrets; required reviewers on `cool-tst` and `cool-prd` | Sections 6 and 7 |
| Platform repos | Rulesets: code-owner review, no force push, and a protected `v1` tag only the release workflow can move | GitHub |

### What a compromise can reach

| If this is compromised | It can | It cannot |
| --- | --- | --- |
| An app repo or its workflow | Deploy that app to dev and open previews within quota | Touch another app, tst or prd |
| A hostile `cool.json` | Fail validation in `coolctl` and again in the chart's values schema | Override hosts, gateways or the registry, because platform values are applied last |
| A developer account | Open requests | Approve its own request; the engine bars requesters from approving |
| `cool-api` | Commit to `cool-gitops` | Reach a cluster API or Azure; every commit is visible and revertible |

A scheduled check flags any commit under a `prd` path that does not link to an approved issue. That closes the gap the last row leaves.

### Audit trail

The issue timeline records who asked and who approved. The `cool-gitops` history records what changed and when. GitHub's audit log, Argo CD's events and the AKS control-plane logs cover the rest. Together they answer the questions Vercel's activity log answers.

### Break glass

`platform-admins` can merge directly to `cool-gitops` under a ruleset that still requires one review. Cluster-admin access comes from a time-bound Entra ID group, never a standing role.

## 18. Phased roadmap

Eight phases take the platform from empty subscriptions to an AI gateway. Week numbers are my sizing for three platform engineers, not commitments; rescale them to your team.

&#91;embedded content: roadmap · 8 phases over 25 weeks\]

Phases 3 to 5 overlap on purpose: APIM and observability have no dependency on the IssueOps work beyond the dev cluster.

| Phase | Delivers | Exit gate |
| --- | --- | --- |
| P0 Foundations | Names approved; the nine repos; state, network, hub cluster, registry, vaults, monitor workspace and Grafana; Argo CD managing itself; runners | Argo CD syncs a hello chart to the hub from `cool-charts` |
| P1 Walking skeleton | sbx and dev clusters; `cool-app` chart; `cool-build` and `cool-deploy`; the `cool-api` deploy endpoint; `go-api` template; `appsim` | A push to `main` is live on dev, with no secret in the app repo |
| P2 Previews and routing | Gateways, wildcard DNS and TLS; the previews ApplicationSet and sticky comment; `go-htmx` and `static-js` templates; `cool-pages` | Opening a pull request yields a URL; closing it removes the namespace |
| P3 IssueOps and production | tst and prd clusters; the create, promote, rollback, env-var and domain requests; the release list with standby | An app travels dev to prd by issue and rolls back in one commit |
| P4 APIM edge | `cool-apim` scripts; the four instances; the publish flow; JWT and rate-limit fragments; the drift check | A merged OpenAPI file is callable through dev APIM, and the nightly diff is clean |
| P5 Observability and rolling releases | Scrape config; per-app dashboards; rule groups; `coolctl gate`; `cool-rolling-release` | The gate halts a bad `appsim` canary in the sandbox |
| P6 Portal and add-ons | `cool-portal`; `cool-addon` for Postgres, Redis and Blob; cron and flags; the Web Vitals beacon | A new team ships an app with a database using only the portal and issues |
| P7 AI gateway | The `50-foundry` stack; `cool-ai`; the two helpers; build triage | An app streams a completion inside its budget, and APIM answers 429 beyond it |

Each gate is a demo on real clusters. A phase that cannot pass its gate does not hand off to the next.

## 19. Risks, open decisions and approvals

Ten decisions need an owner before phase 0 ends. Three of them are approvals the project rules reserve for you.

### Decisions

| # | Decision | Recommendation | Why it is open |
| --- | --- | --- | --- |
| 1 | kubectl version | Raise the pin to within one minor version of the clusters | The 1.30 pin covers API servers 1.29 to 1.31, and AKS support for those ends in November 2026 |
| 2 | Go toolchain | Keep `go 1.24` as the language level and build with a patched toolchain | The Vercel guide in this project notes that upstream support for Go 1.24 ended on 10 February 2026 |
| 3 | YAML in Go tools | Stay on JSON, which needs no approval | Go's standard library has no YAML parser; `cool.yaml` would need an external module |
| 4 | `yq` in the APIM scripts | Approve it, or use the `helm template` fallback | Bash cannot parse YAML |
| 5 | Postgres driver | Approve one driver module for `database/sql` | Every Postgres driver is an external Go module |
| 6 | Production web edge | Decide between the Istio external gateway alone and a WAF and CDN in front of it | Azure Front Door and Application Gateway are outside the named tool list |
| 7 | Certificate issuance | Key Vault with your certificate authority | APIM v2 has no free managed certificate, and ACME automation would add cert-manager |
| 8 | Log retention | Pick a log store | The named tools cover metrics, not logs. Container insights or your existing Dynatrace would fill the gap. |
| 9 | Application Insights | Add it if you want per-app token dashboards | APIM tracks token use per consumer there |
| 10 | APIM instance count | Four as planned | Each Standard v2 instance starts at $700 a month; the sandbox could share dev's |

Decisions 3, 4 and 5 fall under the rule that any Go package or tool beyond the named list needs your approval.

### Deferred scope

| Left out of the first release | What it would take |
| --- | --- |
| Global CDN and WAF | Azure Front Door in front of the gateways: one Terraform module and a DNS change |
| Scale to zero | The AKS KEDA add-on plus its HTTP add-on, and tolerance for cold starts |
| A URL per commit | Retain more releases at zero weight. Each already has its own host, so the cost is one idle replica apiece. |
| Durable queues and workflows | A queue add-on kind and a worker workload type in `cool-app` |
| Next.js features such as ISR and image optimization | Nothing from the platform. A Next.js app runs through the `dockerfile` preset and brings its own cache. |
| A Node template | A fourth template repo and a `node` preset |

### Risks

| Risk | Mitigation |
| --- | --- |
| `cool-api` is the single writer; while it is down nothing deploys | Two replicas, an alert on failed deploy requests, and the break-glass path in section 17. Running apps are unaffected. |
| Microsoft lists metric collection from a mutual-TLS mesh as unsupported with managed Prometheus | Prove sidecar scraping on the sandbox in phase 1. The gate and dashboards depend on it. |
| Mesh revisions turn over fast: asm-1-31 reached AKS in October 2026 and is expected to reach end of life around March 2027 | Budget a canary upgrade every few months and rehearse each on the sandbox |
| From 1 June 2027, AKS upgrades clusters left on unsupported versions, outside maintenance windows | Put version upgrades on the platform calendar; the sandbox goes first |
| APIM v2 has no backup and restore | `cool-apim` is the backup. Rebuild an instance from it in the sandbox once per quarter. |
| Preview features: the Argo CD extension, Foundry's AI Gateway, the APIM AI Gateway tier | None is on the critical path |
| GitHub is the control plane, so a GitHub outage pauses deploys and requests | Accept it. Running apps do not depend on GitHub. |
| A repo's OIDC subject template affects every federation that repo uses | Set it only when `cool-app-create` makes the repo, and document it in the template README |

## 20. Sources

Every version, date, limit and price above comes from one of these pages, opened on the date under the title.

**Microsoft**

- [Supported Kubernetes versions in AKS](https://learn.microsoft.com/en-us/azure/aks/supported-kubernetes-versions)
- [Istio add-on support policy](https://learn.microsoft.com/en-us/azure/aks/istio-support-policy)
- [Istio add-on ingress with the Gateway API](https://learn.microsoft.com/en-us/azure/aks/istio-gateway-api)
- [Istio metrics with managed Prometheus](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/prometheus-istio-integration)
- [Azure Monitor workspace overview](https://learn.microsoft.com/en-us/azure/azure-monitor/metrics/azure-monitor-workspace-overview)
- [Managed Grafana known limitations](https://learn.microsoft.com/en-us/azure/managed-grafana/known-limitations)
- [Managed Grafana troubleshooting](https://learn.microsoft.com/en-us/azure/managed-grafana/troubleshoot-managed-grafana)
- [API Management v2 tiers overview](https://learn.microsoft.com/en-us/azure/api-management/v2-service-tiers-overview)
- [az apim command reference](https://learn.microsoft.com/en-us/cli/azure/apim)
- [AI gateway in API Management](https://learn.microsoft.com/en-us/azure/api-management/genai-gateway-capabilities)
- [Azure resource naming rules](https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/resource-name-rules)
- [Azure Cache for Redis retirement](https://techcommunity.microsoft.com/blog/azure-managed-redis/-/4458721)
- [Argo CD extension for AKS, public preview](https://techcommunity.microsoft.com/t5/azure-arc-blog/announcing-public-preview-of-argo-cd-extension-on-aks-and-azure/ba-p/4504497)

**GitHub**

- [Reusable workflows reference](https://docs.github.com/en/enterprise-cloud@latest/actions/reference/workflows-and-actions/reusing-workflow-configurations)
- [OpenID Connect reference](https://docs.github.com/en/actions/reference/security/oidc)
- [GitHub Pages limits](https://docs.github.com/en/enterprise-cloud@latest/pages/getting-started-with-github-pages/github-pages-limits)

**Others**

- [Argo CD: multiple sources for an Application](https://argo-cd.readthedocs.io/en/latest/user-guide/multiple_sources/)
- [InfoQ: zone redundancy and pricing for APIM Standard v2](https://infoq.com/news/2026/09/apim-standard-v2-zone-redundancy)
- [InfoQ: the APIM AI Gateway tier](https://infoq.com/news/2026/08/azure-apim-ai-gateway-tier)
- [Palantir Foundry API: authentication](https://www.palantir.com/docs/foundry/api/general/overview/authentication/)
- [Palantir Foundry API reference](https://www.palantir.com/docs/foundry/api/v2/third-party-applications-v2-resources/websites/website-basics)

**This project**

- Vercel User Guide (November 2026), the guide this plan mirrors
- IssueOps Self-Service Platform: Implementation Guide (CoolEngOrg), for the engine, commands and team names

Terraform argument names, Istio resource shapes, the HTMX setting and the `llm-emit-token-metric` attributes come from working knowledge rather than a page opened for this plan. Validate them on the sandbox before relying on them.
