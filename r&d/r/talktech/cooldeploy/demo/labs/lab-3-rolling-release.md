# Lab 3: A rolling release that goes wrong

**You need:** the set-up of lab 1, with `payments/checkout` promoted to prd.
**Time:** 30 minutes.

**You will:** roll a release out in stages by hand, watch a gate stop a bad
one, and then let the request script do the same thing unattended.

## 1. A new release, at ten percent

```sh
cd examples/checkout
as_ci CoolEngOrg/payments-checkout coolctl deploy -env dev -digest "$(digest d)" -sha "$(commit d)"
as_ops deploy-promote tst coolctl promote -from dev -to tst
as_ops rolling-release prd coolctl weights -env prd -from tst -weight 10
coolctl status -app payments/checkout
```

Render prd and find the weights:

```sh
coolctl render chart -chart ../../cool-charts/charts/cool-app -namespace payments-checkout \
  -f "$WORK/gitops/apps/payments/checkout/prd/values.json" \
  -f ../../cool-gitops/clusters/prd-eus2-appdev-aks-001/platform-values.json | grep -n weight
```

**Notice:** two Deployments, two subsets in the DestinationRule, and the
split in the VirtualService. A stage of a rolling release changes numbers
in one file, and nothing else.

## 2. The gate

Between stages the platform asks Prometheus two questions about the new
release: what share of its requests failed, and how slow was its 95th
percentile. `coolctl gate` asks them. Here `appsim` plays a release that
fails one request in five, and answers the gate's questions about itself:

```sh
PORT=9913 SIM_LATENCY_MS=5 SIM_ERROR_RATE=0.2 appsim >/dev/null 2>&1 &
for i in $(seq 1 100); do curl -s -o /dev/null localhost:9913/; done
coolctl gate -endpoint http://localhost:9913 -namespace payments-checkout -workload checkout-rddddddd
echo "exit code: $?"
```

Now a release nobody has called yet. Restart `appsim` (`kill %1`, then
start it again) and run the gate without sending requests.

**Answer:** why does a release with no traffic *fail* the gate? When would
passing it be dangerous? (`-allow-no-data` exists; when is it right?)

Heal it and watch the answer change as good requests push the bad ones out
of the window:

```sh
curl -s -X POST 'localhost:9913/sim?error_rate=0'
for i in $(seq 1 250); do curl -s -o /dev/null localhost:9913/; done
coolctl gate -endpoint http://localhost:9913 -namespace payments-checkout -workload checkout-rddddddd -max-p95-ms 200
```

## 3. Stop a bad release

Break it again, and do by hand what the workflow does when a gate fails:

```sh
curl -s -X POST 'localhost:9913/sim?error_rate=0.5'
as_ops rolling-release prd coolctl weights -env prd -weight 0
coolctl status -app payments/checkout
```

**Notice:** weight 0 does not delete the release. It is on standby, with
one replica, where someone can look at it.

## 4. Let the script do it

The `cool-rolling-release` request runs
`issueops-additions/scripts/cool/rolling-release.sh`. Run it as the execute
job would, against the broken release:

```sh
cd ../../issueops-additions
COOL_FIELDS='{"app":"payments/checkout","from":"tst","environment":"prd","stages":"10, 50, 100","settle":"0","error_budget":"1"}' \
COOL_TOKEN=$(token repository=CoolEngOrg/issueops environment=cool-prd \
  job_workflow_ref=CoolEngOrg/issueops/.github/workflows/execute-cool-request.yml@refs/heads/main) \
COOL_PROM_URL=http://localhost:9913 COOL_PROM_TOKEN=unused COOL_WAIT=0 COOL_REPORT=/tmp/report.md \
scripts/cool/rolling-release.sh; echo "exit code: $?"; cat /tmp/report.md
```

Then heal `appsim`, send it a few hundred good requests as in step 2, and
run the script again. Read the report both times: it is what the requestor
sees on the issue.

## Check yourself

- A rolling release is a sequence of `weights` commits with a question
  between them.
- The gate fails closed: over budget, or nothing to judge, both stop the
  release.
- Stopping is not rolling back. The old release never stopped serving; the
  new one just stops getting traffic.
- `kill %1` to stop `appsim`.
