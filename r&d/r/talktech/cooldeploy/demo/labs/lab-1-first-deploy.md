# Lab 1: From push to production

**You need:** this repository, Go 1.24, git, curl, jq. **Time:** 30 minutes.

**You will:** deploy the sample app three times, promote it, and read what
the platform wrote, so that a `values.json` holds no mysteries.

## Set up

Open two terminals in the repository root. In both:

```sh
. demo/lib.sh
WORK=/tmp/coollab && mkdir -p "$WORK/gitops/apps"
```

In the first, start the control plane and leave the terminal open:

```sh
build
cp -R cool-gitops/teams cool-gitops/clusters "$WORK/gitops/"
start "$WORK/gitops"
```

In the second, only put the tools on the PATH: `export PATH=$BIN:$PATH`.
Work there, in `examples/checkout`.

`as_ci` and `as_ops` run a command with the token of a pipeline job or of
an approved request. In real life GitHub mints those tokens; here the fake
issuer does.

## 1. Register and deploy

```sh
cd examples/checkout
coolctl validate
as_ops app-create dev coolctl register -app payments/checkout -repo CoolEngOrg/payments-checkout \
  -identity dev=aaaaaaaa-0000-4000-8000-00000000d0e1 \
  -identity tst=aaaaaaaa-0000-4000-8000-0000000075f2 \
  -identity prd=aaaaaaaa-0000-4000-8000-00000000b4d3
as_ci CoolEngOrg/payments-checkout coolctl deploy -env dev -digest "$(digest a)" -sha "$(commit a)"
```

Read what that wrote:

```sh
jq -M . "$WORK/gitops/apps/payments/checkout/dev/values.json"
git -C "$WORK/gitops" log -1
```

**Find:** the release ID. Where does `raaaaaaa` come from? Which keys came
from `cool.json`, and which did `cool-api` add? (The three `-identity`
values stand for the managed identities a real `cool-app-create` request
makes; the app needs one to read its secret from Key Vault.)

## 2. Deploy again, twice

```sh
as_ci CoolEngOrg/payments-checkout coolctl deploy -env dev -digest "$(digest b)" -sha "$(commit b)"
as_ci CoolEngOrg/payments-checkout coolctl deploy -env dev -digest "$(digest c)" -sha "$(commit c)"
coolctl status -app payments/checkout
```

**Answer before you look:** after three deploys, how many releases does
`dev` hold? What happened to `raaaaaaa`?

## 3. Environment overrides

`cool.json` gives `dev` its own scale and log level. Compare:

```sh
jq -M '{scale, env}' cool.json
jq -M '{scale, env}' "$WORK/gitops/apps/payments/checkout/dev/values.json"
```

## 4. Promote

```sh
as_ops deploy-promote tst coolctl promote -from dev -to tst
as_ops deploy-promote prd coolctl promote -from tst -to prd
diff <(jq -M . "$WORK/gitops/apps/payments/checkout/tst/values.json") \
     <(jq -M . "$WORK/gitops/apps/payments/checkout/prd/values.json")
```

**Notice:** the digest is the same in all three environments. Promotion
copies a release; it does not rebuild. The scale differs, because `prd` has
its own in `cool.json`.

## 5. From values to Kubernetes

```sh
coolctl render chart -chart ../../cool-charts/charts/cool-app -namespace payments-checkout \
  -f "$WORK/gitops/apps/payments/checkout/dev/values.json" \
  -f ../../cool-gitops/clusters/dev-eus2-appdev-aks-001/platform-values.json | less
```

**Find:** the two Deployments. Which has an autoscaler, and why not the
other? What does the VirtualService send where?

## Check yourself

- A release is named after a commit and pinned to an image digest. Tags
  play no part.
- An environment keeps the live release and one standby.
- `values.json` is `cool.json` with the environment's overrides applied,
  plus the release list. Nobody edits it by hand.
- Stop the first terminal with `stop` when you are done, or keep it for lab 2.
