# cool-gitops

What runs where. Argo CD on `prd-eus2-coolops-aks-001` reads this repository
and makes the clusters match it. Nothing else changes a cluster.

```
apps/<team>/<app>/app.json                       the registration: repo, chart tag, identities
apps/<team>/<app>/<env>/values.json              what runs in an environment
apps/<team>/<app>/previews/pr-<n>/values.json    a pull-request preview
teams/<team>/team.json                           the team's Entra ID group and quota
clusters/<cluster>/platform-values.json          what the platform knows about a cluster
argocd/projects/   argocd/appsets/   argocd/apps/    Argo CD's own configuration
bootstrap/                                       the one script and one object applied by hand
```

## Who writes what

| Path | Writer | How |
| --- | --- | --- |
| `apps/` | `cool-api`, and only `cool-api` | commits as the CoolDeploy GitHub App, one commit per request |
| everything else | the platform team | pull requests, reviewed by `CODEOWNERS` |

Branch protection on `main` requires a pull request, with one bypass: the
CoolDeploy GitHub App. CI refuses a pull request that touches `apps/`, so a
person cannot edit a values file by hand even with a review. To change what
runs, open a request in `CoolEngOrg/issueops`.

Every commit under `apps/` names the actor, the workflow and the run that
asked for it, and the IssueOps issue when there is one. `git log apps/` is the
deployment history, and `git revert` is not how to roll back: rollback is a
request too, and it writes a new commit.

## From a file to a running app

```
apps/payments/checkout/prd/values.json
        │  the cool-apps ApplicationSet finds the file
        ▼
Application payments-checkout-prd, project payments
        │  charts cool-namespace and cool-app at the tag in "chart",
        │  values: team.json, then values.json, then platform-values.json
        ▼
namespace payments-checkout on prd-eus2-appdev-aks-001
```

The platform's values come last, so nothing an app sets can override the
registry, the domain or the mesh settings.

Delete the file and the Application goes, and the namespace with it. That is
how a preview ends and how an app is retired.

## The examples in apps/

They are generated, by `demo/seed.sh`, through the same `cool-api` that
writes the real ones:

| App | State |
| --- | --- |
| `payments/checkout` in `dev`, `tst` | `rccccccc` live, `rbbbbbbb` on standby |
| `payments/checkout` in `prd` | a rolling release in flight: `rccccccc` at 10%, `rbbbbbbb` at 90%, one override, one custom domain |
| `payments/checkout` preview `pr-42` | `rddddddd`, one replica |
| `sim/appsim-steady`, `sim/appsim-noisy` in `sbx` | simulators for the sandbox cluster |
| `sim/appsim-flaky` in `sbx` | a canary at 20% that fails one request in twenty |

The digests are made up (`sha256:aaaa…`). Replace `apps/` with an empty
directory before pointing a real Argo CD at a copy of this repository.

## Bootstrap

Once, after `cool-infra` has built the clusters:

```sh
az aks get-credentials -g prd-eus2-coolops-rg-001 -n prd-eus2-coolops-aks-001
export ARGOCD_CLIENT_ID=$(terraform -chdir=../cool-infra/stacks/60-identity output -raw argocd_client_id)
export ARGOCD_CHART_VERSION=<pick one: helm search repo argo/argo-cd --versions>
bootstrap/install.sh all
```

It installs Argo CD with Helm, gives it read access to the two repositories
as a GitHub App, registers the four app clusters, and applies
`bootstrap/root.yaml`. From then on Argo CD manages its own projects and
ApplicationSets from `argocd/`.

Argo CD signs in to the app clusters with a workload identity, through
`argocd-k8s-auth azure`. No cluster token is stored anywhere.

Before the first sync, replace the placeholder IDs in
`clusters/*/platform-values.json` and
`clusters/prd-eus2-coolops-aks-001/cool-api-values.json`. Every value that is
all zeros is a placeholder; `cool-infra/scripts/platform-values.sh` prints the
real ones from Terraform outputs.

## Onboarding a team

A pull request with two files:

```
teams/<team>/team.json         {"team": "...", "group": "<Entra ID group object ID>", "quota": "small"}
argocd/projects/<team>.yaml    copy payments.yaml, change the name and the namespace patterns
```

A team slug is 2 to 8 characters, lower case, no hyphen. No hyphen is what
makes `<team>-*` an exact namespace pattern: `pay-*` cannot match a namespace
of team `payments`.

## Check

```sh
CHARTS=../cool-charts test/check.sh
```

parses every JSON file, checks that each file says what its path says, that
every team with apps has a `team.json` and a project, and renders every app
with the charts.

## Moving to a new chart version

1. Tag the chart in `cool-charts`, for example `cool-app-1.1.0`.
2. Change `"chart"` in one sandbox app's `app.json` and `values.json` by pull
   request to this repository (the one case where the platform team edits
   under `apps/`; CI allows it for members of `platform-admins` on `sim/`).
3. When it is healthy, set `COOL_CHART` for `cool-api` so that new apps start
   on the new tag, and move existing apps with the same edit, a team at a time.
