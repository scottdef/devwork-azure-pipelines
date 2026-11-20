# Lab 5: The sandbox cluster

**You need:** the platform installed, access to `sbx-eus2-appsim-aks-001` as
a platform admin, `kubectl` 1.30, and the right to run the
`execute-cool-sandbox` workflow. **Time:** an hour.

**You will:** do on real infrastructure what labs 1 to 4 did on a laptop,
and check the things only a cluster can show: the mesh, the metrics, a
chart upgrade.

The sandbox runs `appsim`, an app whose only job is to misbehave on demand.
Nothing there matters to anyone, which is the point.

```sh
az aks get-credentials -g sbx-eus2-appsim-rg-001 -n sbx-eus2-appsim-aks-001
kubectl get ns -l cool/team=sim
```

## 1. Deploy a scenario

In `CoolEngOrg/issueops`, run the workflow **execute-cool-sandbox** with
scenario `steady`. Then:

```sh
kubectl -n argocd get applications -l cool/team=sim        # on the hub cluster
kubectl -n sim-appsim-steady get deploy,hpa,pdb,svc,virtualservice,destinationrule
kubectl -n sim-appsim-steady get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[*].name}{"\n"}{end}'
```

**Check:** every pod has two containers. The second is the mesh sidecar,
which nobody asked for in `cool.json`; the namespace label did that.

## 2. The mesh

```sh
kubectl -n sim-appsim-steady run probe --rm -it --restart=Never \
  --image=prdeus2coolopsacr001.azurecr.io/platform/coolctl:1 \
  --labels=sidecar.istio.io/inject=false -- call -method GET -url http://appsim-steady/healthz
```

**Predict first.** The pod is in the same namespace but outside the mesh.
Does the call succeed? What does that tell you about `PeerAuthentication`?
Now try an image from Docker Hub in the same command. What stops it?

## 3. Metrics

Send traffic through the internal gateway for a few minutes, then in
Grafana (`prd-eus2-coolops-mg-001`) open Explore and run:

```promql
sum by (destination_workload) (rate(istio_requests_total{destination_workload_namespace="sim-appsim-steady"}[5m]))
```

**This is the check that matters most.** Managed Prometheus scrapes pods
that are under strict mutual TLS, through the sidecar's merged-metrics
port. If this query returns nothing, the rolling-release gate has nothing
to judge and every rolling release will stop at its first stage. Repeat
this step after every upgrade of the mesh add-on.

Then the gate itself, from your machine:

```sh
export COOL_PROM_URL=<the workspace's query endpoint>
export COOL_PROM_TOKEN=$(az account get-access-token --resource https://prometheus.monitor.azure.com --query accessToken -o tsv)
coolctl gate -namespace sim-appsim-steady -workload appsim-steady-<release>
```

## 4. A canary that fails

Run the workflow with scenario `flaky`, then again with a newer image tag
and `weight` 20. The flaky simulator fails one request in twenty.

```sh
kubectl -n sim-appsim-flaky get virtualservice appsim-flaky -o jsonpath='{.spec.http[-1].route}' | jq -M .
coolctl gate -namespace sim-appsim-flaky -workload appsim-flaky-<new release> -max-error-rate 0.01
```

Make it worse without deploying, and watch the gate's answer follow:

```sh
kubectl -n sim-appsim-flaky port-forward deploy/appsim-flaky-<new release> 8080 &
curl -X POST 'localhost:8080/sim?error_rate=0.5'
```

## 5. A noisy neighbour

Run scenario `noisy`. It burns 40 ms of CPU per request and scales on 50%
CPU.

```sh
kubectl -n sim-appsim-noisy get hpa -w
kubectl -n sim-appsim-noisy describe resourcequota cool-quota
```

**Find:** the point where the autoscaler wants more replicas than the
namespace's quota allows. What does the app's team see? What does the
`steady` simulator, on the same nodes, see?

## 6. A chart upgrade

This is what the sandbox is for.

1. In `cool-charts`, make a small change to `cool-app`, bump its version,
   run `make check`, and tag it `cool-app-1.0.1`.
2. In `cool-gitops`, open a pull request that sets `"chart": "cool-app-1.0.1"`
   in `apps/sim/appsim-steady/app.json` and `sbx/values.json`. This is the
   one place people edit files under `apps/`, and CI allows it only for
   `sim/`.
3. Merge, and watch Argo CD roll it out. Compare what changed:

```sh
kubectl -n argocd get application sim-appsim-steady-sbx -o jsonpath='{.status.sync.revisions}'
```

Only when the sandbox is healthy does the tag go to real apps, a team at a
time.

## Check yourself

- The sandbox is where the platform's own changes are tried: charts, the
  mesh, the release machinery.
- Step 3 is not optional knowledge. A gate with no data fails closed, so a
  broken scrape path stops every rolling release on the platform.
- Quotas contain a noisy app inside its namespace; they do not make its
  neighbours on the same nodes immune. That is what node pools are for.
