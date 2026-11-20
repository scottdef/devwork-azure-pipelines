# coolctl

The Go tools of CoolDeploy. One module, standard library only, five binaries.

| Binary | What it is | Where it runs |
| --- | --- | --- |
| `coolctl` | the command-line tool: scaffold, validate, render, deploy, promote, roll back | laptops, workflow jobs, cron pods |
| `cool-api` | the control-plane API; the single writer to `cool-gitops` | hub cluster `prd-eus2-coolops-aks-001` |
| `cool-static` | file server behind the `static` preset | app pods |
| `appsim` | a simulated app with latency, error and CPU knobs | sandbox cluster `sbx-eus2-appsim-aks-001` |
| `cool-oidc-fake` | a stand-in for GitHub's OIDC issuer; loopback only | the local demo, never a cluster |

```sh
make check build        # gofmt, vet, tests, an empty module graph, then bin/
```

Go 1.24. The module has no `require` lines and `make check` fails if one
appears. A new dependency needs the approval the platform's rules ask for.

## How a deploy works

```
app repo ──push──▶ cool-pipeline.yml (cool-actions@v1)
                     │ az acr build ─▶ image digest
                     │ coolctl deploy ─▶ cool-api ─▶ commit to cool-gitops
                     ▼                                   │
                   coolctl wait ◀── Argo CD syncs ◀──────┘
```

Nothing in an app repository can write to a cluster or to `cool-gitops`.
A workflow job asks GitHub for an OIDC token and sends it to `cool-api`,
which checks two claims GitHub signs and the job cannot choose:

| The job is defined by | It may |
| --- | --- |
| `CoolEngOrg/cool-actions/.github/workflows/*@refs/tags/v1` | deploy its own repository's app to `dev`, and manage its previews |
| `CoolEngOrg/issueops/.github/workflows/execute-cool-*@refs/heads/main`, running in the GitHub environment `cool-<env>` | promote, roll back, set weights, change config, register, retire in `<env>` |

`internal/oidc` holds that policy, in about forty lines, with tests.

## The manifest: cool.json

```json
{
  "team": "payments",
  "app": "checkout",
  "preset": "go-htmx",
  "expose": "public",
  "port": 8080,
  "health": "/healthz",
  "scale": {"min": 2, "max": 6, "cpu": 70},
  "resources": {"cpu": "250m", "memory": "256Mi"},
  "env": {"LOG_LEVEL": "info"},
  "secrets": ["CRON_SECRET"],
  "routes": [
    {"prefix": "/api/", "timeout": "10s"},
    {"redirect": {"from": "/cart", "to": "/", "code": 308}}
  ],
  "crons": [{"path": "/internal/cron/expire-orders", "schedule": "*/15 * * * *"}],
  "flags": {"express-checkout": false},
  "api": {"openapi": "openapi.yaml", "product": "payments", "auth": "entra-jwt"},
  "environments": {"prd": {"scale": {"min": 3, "max": 10, "cpu": 60}}}
}
```

| Key | Rule |
| --- | --- |
| `team` | 2 to 8 characters, `[a-z][a-z0-9]*` |
| `app` | 2 to 20 characters, lower case, digits and `-`; may not end in `-pr-<n>` |
| `preset` | `go-server`, `go-htmx`, `static` or `dockerfile` |
| `expose` | `internal` (default) or `public` |
| `port` | 1024 to 65535, default 8080; the app must listen on `$PORT` |
| `health` | path that answers 200 when the app can serve, default `/healthz` |
| `scale` | replicas between `min` and `max`, scaled on `cpu` percent |
| `env` | plain settings; `PORT` and `COOL_*` are reserved |
| `secrets` | names only; values live in the team's Key Vault as `<app>-<NAME>` with `_` written `-` |
| `routes` | a `prefix` with a `timeout` or response `headers`, or a same-host `redirect` |
| `crons` | a path called on a schedule with `Authorization: Bearer $CRON_SECRET`; needs `CRON_SECRET` in `secrets` |
| `environments` | per-environment `scale`, `resources` and `env` |

Unknown keys are refused, so a typo is an error and not a silent default.
`coolctl validate` checks all of it; `cool-api` checks it again.

## Commands

```
coolctl init      -team payments -app checkout -preset go-htmx
coolctl validate
coolctl render    dockerfile | workflow | dashboard | portal | chart
coolctl deploy    -env dev -digest sha256:... [-pr 42] [-wait 10m]
coolctl preview   down -pr 42
coolctl promote   -from dev -to tst [-release r1a2b3c4]
coolctl weights   -env prd -from tst -weight 10      # start a rolling release
coolctl weights   -env prd -weight 50                # next stage
coolctl rollback  -env prd -reason "error budget"
coolctl config    -env prd -set LOG_LEVEL=warn -add-domain pay.coolshop.example
coolctl register  -app payments/checkout -repo CoolEngOrg/payments-checkout
coolctl retire    -app payments/checkout -confirm payments/checkout
coolctl status    [-json]
coolctl wait      -env dev -release r1a2b3c4
coolctl gate      -namespace payments-checkout -workload checkout-r1a2b3c4
coolctl logs      -app payments/checkout -env dev -f
coolctl issue     fields -f body.md
```

Commands that change something call `cool-api` at `$COOL_API_URL`. They send
`$COOL_TOKEN` if it is set, and otherwise ask GitHub Actions for a token with
audience `cool-api`. In a workflow, results also land in `$GITHUB_OUTPUT` as
`release`, `url` and `application`.

`coolctl gate` asks Prometheus for a release's error rate and p95 latency and
exits 1 when either is over budget. A release with no traffic fails the gate
unless `-allow-no-data` is given: no data is not good news.

## cool-api

| Route | Caller | Effect |
| --- | --- | --- |
| `GET /healthz`, `GET /metrics` | probes, Prometheus | |
| `GET /v1/apps` | anyone the gateway lets in | list of registered apps |
| `GET /v1/apps/{team}/{app}` | same | releases per environment, with Argo CD health |
| `GET /v1/apps/{team}/{app}/status` | the portal | the same as an HTML fragment |
| `POST /v1/apps` | IssueOps | register an app |
| `POST /v1/apps/{team}/{app}/deployments` | CI | new release in `dev`, or a preview |
| `DELETE /v1/apps/{team}/{app}/previews/{pr}` | CI | remove a preview |
| `POST /v1/apps/{team}/{app}/promote` | IssueOps | copy a release to the next environment |
| `POST /v1/apps/{team}/{app}/weights` | IssueOps | start or advance a rolling release |
| `POST /v1/apps/{team}/{app}/rollback` | IssueOps | swap active and standby |
| `POST /v1/apps/{team}/{app}/config` | IssueOps | overrides, custom domains, identity |
| `DELETE /v1/apps/{team}/{app}` | IssueOps | retire the app everywhere |

Reads carry no token: APIM in front of `cool-api` requires an Entra ID sign-in
for them. Writes carry a GitHub OIDC token, which `cool-api` verifies itself.

Every write is one commit to `cool-gitops`, with trailers that say who asked:

```
promote: payments/checkout r1a2b3c4 from tst to prd
Actor: octocat
Workflow: CoolEngOrg/issueops/.github/workflows/execute-cool-deploy-promote.yml@refs/heads/main
Run: 9876543210
Issue: https://github.com/CoolEngOrg/issueops/issues/103
```

Configuration is environment variables; `cmd/cool-api/main.go` lists them.
`COOL_STORE=dir:/path` writes to a local checkout, which is how the demo and
the tests run it. `COOL_STORE=github:CoolEngOrg/cool-gitops@main` writes
through the GitHub contents API as the CoolDeploy GitHub App.

## Releases

An app's `values.json` in an environment holds a list of releases, newest
first. Every operation is an edit of that list:

| Operation | Edit |
| --- | --- |
| deploy, promote | new release at 100; the previous active one becomes the standby; older ones go |
| rolling release | new release at a small weight; the active one keeps the rest; `weights` moves the split |
| rollback | active and standby swap; rolling back twice returns to where you were |
| retire | the files go, and Argo CD removes what they described |

A rollback restores the image, not the configuration: `port`, `routes`, `env`
and the rest belong to the environment, not to a release. `internal/release`
has the rules and the tests.

## Layout

```
cmd/coolctl  cmd/cool-api  cmd/cool-static  cmd/appsim  cmd/cool-oidc-fake
internal/manifest   cool.json: parse, default, validate
internal/release    the release list and its edits
internal/gitops     the layout of cool-gitops, and a store over a directory
internal/ghapp      the same store over the GitHub contents API, as a GitHub App
internal/oidc       verify GitHub OIDC tokens; the authorization policy
internal/api        cool-api's handlers
internal/client     the HTTP client coolctl uses
internal/argo       read Argo CD Application health from inside the hub cluster
internal/prom       the two PromQL queries behind `coolctl gate`
internal/chart      render the platform's charts without Helm
internal/tmpl       Dockerfile, workflow, dashboard and portal templates
internal/issueform  read the fields of a GitHub issue-form body
internal/metrics    a Prometheus text-format registry in 150 lines
internal/oidctest   a fake OIDC issuer for tests and the demo
```

## What coolctl render chart is, and is not

`coolctl render chart` runs the platform's Helm charts through `text/template`
with the two dozen Helm functions those charts use. It exists so a developer
can see what a values file becomes without installing Helm, and so the charts
can be tested with the standard library. It reads values as JSON only, has no
`toYaml`, `tpl`, `lookup` or subcharts, and is not what installs anything:
clusters get real Helm, through Argo CD. `cool-charts/test/render.sh` compares
the two renderers when Helm is on the PATH.
