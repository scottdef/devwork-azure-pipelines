# demo

CoolDeploy on one machine, for demos, testing and training. No Azure, no
GitHub, no cluster.

```sh
demo/run-local.sh            # the whole story, start to finish, in about ten seconds
demo/run-local.sh --pause    # the same, one chapter at a time
demo/run-local.sh --keep     # afterwards, keep it running and serve the portal
```

Needs Go 1.24, git, curl and jq.

## What is real and what is not

| Real | Stand-in |
| --- | --- |
| `cool-api`, with its checks and its policy | GitHub's OIDC issuer: `cool-oidc-fake` signs whatever claims the demo asks for |
| `coolctl`, as a pipeline or a request job would run it | GitHub: `cool-api` writes to a local Git repository instead |
| the GitOps repository layout, the commits and their trailers | Argo CD: what it would install is shown by rendering the chart |
| the charts, rendered by `coolctl render chart` | Prometheus: `appsim` answers the gate's two queries from its own requests |
| the request scripts in `issueops-additions` | the IssueOps engine: its approvals are assumed to have happened |
| the sample app, the portal | Azure |

The fake issuer is why one script can be an app's pipeline in one line and
an approved production request in the next. It is also why it refuses to
listen on anything but loopback.

## The story `run-local.sh` tells

1. An app is one manifest; the preset supplies the Dockerfile.
2. Registering it (a `cool-app-create` request).
3. A push to `main`: one commit to the GitOps repository.
4. Four things the trust model refuses.
5. A second push: the first release becomes the standby.
6. A pull-request preview, up and down.
7. Promotion to tst and prd.
8. A rolling release, with a gate that fails and then passes.
9. The chart, rendered from Git, and checked against Kubernetes 1.30 schemas when `kubeconform` is installed.
10. Environment settings and a custom domain.
11. A rollback.
12. Status everywhere.
13. The Git log as the audit trail.
14. The portal, rendered from the same repository.

## Files

```
run-local.sh    the story
seed.sh         regenerates cool-gitops/apps from the same requests
lib.sh          starts the two local processes; mints tokens
appsim/         simulator manifests: steady, flaky, noisy
labs/           five hands-on labs
```

Sample issue bodies, one per request type, are in
`issueops-additions/examples/issues/`.

## Labs

| Lab | Needs | Teaches |
| --- | --- | --- |
| [1. From push to production](labs/lab-1-first-deploy.md) | this machine | releases, promotion, what Git ends up holding |
| [2. Who may do what](labs/lab-2-trust.md) | this machine | the trust model, by trying to break it |
| [3. A rolling release that goes wrong](labs/lab-3-rolling-release.md) | this machine | weights, the error-budget gate, rollback |
| [4. Requests](labs/lab-4-requests.md) | this machine | issue forms, fields, the scripts behind a request |
| [5. The sandbox cluster](labs/lab-5-sandbox.md) | the sandbox cluster | the same on real infrastructure: mesh, metrics, chart upgrades |

## Ports

`9911` issuer, `9912` cool-api, `9913` appsim, `8000` portal. Change the
first three with `COOL_DEMO_ISSUER_PORT`, `COOL_DEMO_API_PORT` and
`COOL_DEMO_SIM_PORT`.
