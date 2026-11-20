# cool-charts

The Helm charts of CoolDeploy. Argo CD installs them straight from this
repository by Git tag; there is no chart registry to run.

| Chart | Installed | By |
| --- | --- | --- |
| `cool-app` | once per app per environment, and once per preview | the `cool-apps` and `cool-previews` ApplicationSets |
| `cool-namespace` | alongside every `cool-app`, in the same Argo CD Application | the same ApplicationSets |
| `cool-platform` | once per app cluster | the `cool-platform` ApplicationSet |
| `cool-api` | once, on the hub cluster | `cool-gitops/bootstrap` |

`cool-app` and `cool-namespace` share one Application on purpose. Deleting
the Application deletes the namespace with everything in it, which is all
that tearing down a preview or retiring an app has to do.

## Versions

A release is a Git tag named `<chart>-<version>`, for example
`cool-app-1.0.0`. An app's `app.json` in `cool-gitops` names the tag it is
on, so charts roll out app by app and not all at once:

```sh
make check                      # render, compare with helm, validate
git tag cool-app-1.1.0 && git push origin cool-app-1.1.0
```

Then move one sandbox app to the new tag, watch it, and move the rest.

An app's Application reads both of its charts, `cool-app` and
`cool-namespace`, at the one tag in `chart`. A change to `cool-namespace`
therefore ships under a new `cool-app-<version>` tag too.

## Values are JSON

Every values file on the platform is YAML written in JSON syntax. Helm and
Argo CD read it as the YAML it is. The Go tools read it with `encoding/json`,
because Go's standard library has no YAML parser and the platform's Go code
uses the standard library only.

The templates follow from that: structured values are emitted with `toJson`,
never `toYaml`. Flow-style JSON inside block-style YAML looks unusual and
cannot be mis-indented.

## Test

```sh
make check
```

runs `test/render.sh`, which renders every chart against `test/values/` with
`coolctl render chart` and checks object counts for the cases that matter:
a rolling release, a preview, a standby, a cluster with no mesh, and two
renders that must be refused. With `helm` on the PATH it also lints, renders
with Helm and compares the object lists. With `kubeconform` on the PATH it
validates every object against the Kubernetes 1.30 and CRD schemas.

## Things to know before changing a chart

- Numbers reach a template as floats. Use `int` before comparing or printing.
- Large numbers in values, such as a GitHub App installation ID, must be
  strings. `toString` of a float prints `1.2345678e+07`.
- Without Istio CNI, a namespace with sidecar injection cannot enforce the
  `baseline` or `restricted` pod security levels: the sidecar's init container
  needs `NET_ADMIN` and `NET_RAW`. `platform.podSecurity` is therefore
  `privileged` by default, with `warn` and `audit` at `restricted`. On a
  cluster where the Istio add-on runs with CNI chaining, set it to
  `restricted`. The app containers already satisfy `restricted` either way.
- `ama-metrics-settings-configmap` is read by the Azure Monitor metrics
  add-on by name. Microsoft adds keys to it over time; compare with the
  template in `Azure/prometheus-collector` before changing more than the
  namespace regular expression, or set `platform.metrics.manageSettings` to
  `false` and manage it yourself.
- With mutual TLS set to `STRICT`, managed Prometheus cannot scrape an app
  port directly. It scrapes the sidecar's merged-metrics port instead, which
  Istio keeps outside mutual TLS. Prove this on the sandbox cluster after
  every mesh upgrade.
