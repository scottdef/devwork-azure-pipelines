# CoolDeploy

The code for the plan "Vercel Clone on AKS: Build Plan (CoolEngOrg)": a
deployment platform on AKS where an app is one manifest, a push deploys it,
a pull request gets a preview, and everything after dev is a request someone
approves.

Each directory here is one repository of the `CoolEngOrg` organization.

```sh
make demo     # the whole story on this machine: no Azure, no GitHub, no cluster
make test     # every check that needs no cloud
```

Both need Go 1.24, git, curl and jq.

## The repositories

| Directory | Repository | Holds |
| --- | --- | --- |
| `coolctl/` | `CoolEngOrg/coolctl` | the Go tools: `coolctl`, `cool-api`, `cool-static`, `appsim`. Standard library only. |
| `cool-charts/` | `CoolEngOrg/cool-charts` | the Helm charts: `cool-app`, `cool-namespace`, `cool-platform`, `cool-api` |
| `cool-gitops/` | `CoolEngOrg/cool-gitops` | what runs where; Argo CD configuration; the example deployment |
| `cool-actions/` | `CoolEngOrg/cool-actions` | composite actions and reusable workflows |
| `cool-apim/` | `CoolEngOrg/cool-apim` | API Management v2 from YAML and OpenAPI, with bash and the Azure CLI. No Terraform. |
| `cool-infra/` | `CoolEngOrg/cool-infra` | Terraform for everything else in Azure |
| `cool-portal/` | `CoolEngOrg/cool-portal` | the portal: HTML, HTMX and plain JavaScript on GitHub Pages |
| `issueops-additions/` | added to `CoolEngOrg/issueops` | nine request types: forms, policy, workflows, scripts |
| `cool-template-go-api/`, `-go-htmx/`, `-static-js/` | template repositories | what a new app starts from |
| `examples/checkout/` | `CoolEngOrg/payments-checkout` | the sample app: a shop in Go and HTMX, with sample data |
| `demo/` | | the local demo, the seed script, five labs |

Each has its own README. Start with `demo/README.md` to see it work, and
`coolctl/README.md` to see how.

## How it fits together

```
 app repository            issueops                     cool-gitops
 ──────────────            ────────                     ───────────
 push / pull request       request ─ approve            apps/<team>/<app>/<env>/values.json
        │                        │                               ▲            │
        ▼                        ▼                               │            ▼
 cool-pipeline.yml@v1      execute-cool-*.yml             ┌──────────┐    Argo CD (hub)
 build in ACR              in environment cool-<env>      │ cool-api │        │
        │                        │                        └──────────┘        ▼
        └──── GitHub OIDC token ─┴──── verified by ─────────────▲       cool-app chart
              (no secret)                                        │       on dev, tst, prd, sbx
                                                         the only writer
```

Three ideas carry most of it.

**No deploy secrets.** A job proves what it is with the OIDC token GitHub
signs for it. `cool-api` lets the platform's pipeline deploy an app's own
repository to dev, and lets an approved request's job, in the GitHub
environment `cool-<env>`, do the rest. Nothing an app team can edit holds a
credential. The policy is forty lines in `coolctl/internal/oidc`.

**Git is the only way in.** `cool-api` is the single writer to
`cool-gitops`, one commit per request, each naming who asked. Argo CD pulls.
No pipeline has cluster credentials.

**A release is a line in a list.** An environment's `values.json` holds its
releases, newest first, each an image digest with a traffic weight. Deploy,
promote, rolling release and rollback are edits of that list, and the chart
turns the list into Deployments and Istio weights.

## What has been checked, and what has not

`make test` runs all of this, and all of it passes:

| Checked | How |
| --- | --- |
| The Go tools, the sample app, two templates | `gofmt`, `go vet`, unit tests; each module's dependency graph is empty |
| The charts | rendered for a rolling release, a preview, a standby, a cluster with no mesh; 81 objects validated against Kubernetes 1.30 and CRD schemas with `kubeconform` |
| The GitOps repository | every app renders with the charts; paths and contents agree |
| The request scripts | each one run against a real `cool-api` on localhost, including a rolling release stopped by its error-budget gate, and the requests that must be refused |
| The API Management scripts | run against a stand-in `az` that records calls: order, idempotence, dry run |
| Workflows and shell | `actionlint` on 31 workflows, `shellcheck` on every script |
| The whole flow | `make demo`: register, deploy, preview, promote, rolling release, rollback, portal |

Not checked, because this was built without Azure, GitHub Actions, a cluster
or the Terraform registry:

| Not checked | What to do first |
| --- | --- |
| **The charts under real Helm.** They were rendered by `coolctl render chart`, a renderer written for them. | `make -C cool-charts check` on a machine with `helm`: it lints, renders with Helm and compares. |
| **Terraform beyond formatting.** No `validate`, no `plan`. | `make -C cool-infra lock check`. Expect to correct an attribute or two; the add-ons stack first. |
| **Anything against Azure.** The `az` calls are right as far as the CLI's documentation goes. | Sync the sandbox API Management instance first; it is what compiles the policies. |
| **Workflows running.** `actionlint` checks syntax and expressions, not behaviour. | Run the pipeline on a template app in the sandbox. |
| **The IssueOps engine.** The additions assume its `gate` and `finish` actions and its registry keys. | `issueops-additions/README.md` lists the four assumptions. |
| **Metrics under strict mutual TLS.** Managed Prometheus must scrape the sidecar's merged-metrics port. | Lab 5, step 3, on the sandbox, and again after every mesh upgrade. |

Every ID that is all zeros, and every `*.example` name, is a placeholder.
The image digests in `cool-gitops/apps` are made up.

## Decisions taken where the plan left a choice

| Question | Taken | Because |
| --- | --- | --- |
| YAML or JSON for what Go reads | JSON: `cool.json`, `values.json`, and values files written in JSON syntax | Go's standard library has no YAML, and the Go tools use the standard library only. JSON is valid YAML, so Helm and Argo CD read the same files. |
| Reading YAML in the API Management scripts | `yq`, with Helm as the fallback | The scripts are bash; either tool is on a platform runner. |
| A PostgreSQL driver in the sample app | none; orders live in memory | A driver is a dependency outside the standard library. The app's storage sits behind one type, to be replaced with a database. |

Adding a Go dependency anywhere needs the approval the project rules ask for.
`go-echarts` is allowed and unused: the dashboards are Grafana's.

## Where the code departs from the plan's text

- A preview's values are at `apps/<team>/<app>/previews/pr-<n>/values.json`,
  a directory per preview, so that the same path pattern serves apps and
  previews.
- The namespace is not a separate Argo CD Application. `cool-namespace` and
  `cool-app` are two sources of one Application, so deleting it removes the
  namespace and everything in it. That is all a preview teardown or a
  retirement has to do.
- An app's managed identities are recorded when it is registered and copied
  into each environment's values on first deploy. Otherwise a first deploy
  of an app with secrets could not render.
- A preview gets no secrets and runs no crons.
- An AI token budget is an app role on the app's identity, read by API
  Management from the caller's token. There is no table of budgets to keep.
- Simulators reach the sandbox through a hand-started workflow,
  `execute-cool-sandbox`, in the environment `cool-sbx`.

## Bringing it up for real

1. `cool-infra`: state, network, registry, clusters, metrics, identities,
   vaults, Foundry. Its README has the order.
2. `coolctl`: `make images` builds `cool-api`, `coolctl`, `cool-static` and
   `appsim` in the registry.
3. `cool-actions`: push, tag `v1.0.0`; protect the `v1` tag.
4. `cool-charts`: push, tag `cool-app-1.0.0`, `cool-platform-1.0.0` and
   `cool-api-1.0.0`. Charts are installed by Git tag. An app's namespace
   chart is read at the same tag as its app chart, so `cool-app-1.0.0`
   covers both.
5. `cool-gitops`: empty `apps/`, fill the placeholders from
   `make -C cool-infra values`, then `bootstrap/install.sh all`.
6. `cool-apim`: `scripts/apim-instance.sh` per environment, then the sync.
7. `issueops-additions`: `scripts/merge-registry.sh`, the four adaptations,
   the GitHub environments `cool-dev`, `cool-tst`, `cool-prd`, `cool-sbx`.
8. The three templates as template repositories; `cool-portal` on Pages.
9. A `cool-app-create` request. The app deploys itself to dev.

## Versions

Go 1.24. Kubernetes and `kubectl` 1.30. Azure API Management v2 tiers.
Istio through the AKS add-on. HTMX 2.0.11, vendored. Third-party GitHub
Actions pinned to commit SHAs.
