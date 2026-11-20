# Lab 2: Who may do what

**You need:** the set-up of lab 1, still running. **Time:** 20 minutes.

**You will:** try to deploy things you should not be able to, and see
exactly which claim stops each attempt.

## The rule

There is no deploy secret anywhere on the platform. A job asks GitHub for
an OIDC token and sends it to `cool-api`. GitHub signs the token, so the job
cannot lie about what is in it. `cool-api` reads three claims:

| Claim | Says |
| --- | --- |
| `repository` | which repository the workflow run belongs to |
| `job_workflow_ref` | which workflow file defines the job |
| `environment` | which GitHub environment the job runs in, if any |

and allows:

| A job defined by | May |
| --- | --- |
| `CoolEngOrg/cool-actions/.github/workflows/*@refs/tags/v1` | deploy **its own repository's** app to **dev**, and manage its previews |
| `CoolEngOrg/issueops/.github/workflows/execute-cool-*@refs/heads/main`, in environment `cool-<env>` | everything else, in `<env>` |

The whole policy is `Authorize` in `coolctl/internal/oidc/oidc.go`. Read it
now; it is short.

## Mint your own tokens

`token` asks the fake issuer for a token with the claims you name:

```sh
T=$(token repository=CoolEngOrg/payments-checkout \
  job_workflow_ref=CoolEngOrg/cool-actions/.github/workflows/cool-pipeline.yml@refs/tags/v1)
cut -d. -f2 <<<"$T" | tr '_-' '/+' | base64 -d 2>/dev/null | jq -M .
```

Use one with `COOL_TOKEN=$T coolctl ...`.

## Try each of these, and predict the answer first

Run them from `examples/checkout`.

1. **Another team's pipeline deploys your app.**
   `repository=CoolEngOrg/someone-else`, the platform's pipeline as `job_workflow_ref`.
2. **Your own repository, your own workflow file.** Keep the repository;
   set `job_workflow_ref=CoolEngOrg/payments-checkout/.github/workflows/deploy.yml@refs/heads/main`.
3. **The platform's pipeline, from a branch.** `...cool-pipeline.yml@refs/heads/my-fix`
   instead of `@refs/tags/v1`.
4. **The pipeline promotes to prd.** A valid pipeline token, with
   `coolctl promote -from dev -to prd`.
5. **A request approved for tst acts on prd.**
   `as_ops deploy-promote tst coolctl promote -from tst -to prd`.
6. **No token at all.** `COOL_TOKEN=nonsense coolctl status` and then
   `COOL_TOKEN=nonsense coolctl rollback -env dev`.

For each: which claim made `cool-api` say no? Number 6 has a twist: one of
the two commands works. Why is that all right?

## Think

- Number 2 is the important one. A developer who can push to an app
  repository can write any workflow they like there. Why does that not let
  them deploy to production? What would they have to change instead, and
  who reviews changes there?
- Number 3: what does protecting the tag `v1` of `cool-actions` protect?
- The `environment` claim only appears when GitHub has let the job into
  that environment. Where are the people who can approve that listed?

## Check yourself

- Trust rests on *which workflow file defines the job*, not on which
  repository asked. That is what lets one pipeline serve every app.
- Production rights come from a GitHub environment, which only an approved
  request's job reaches.
- Reads are open at `cool-api` because API Management stands in front of
  them and asks for a sign-in. Writes are checked by `cool-api` itself.
