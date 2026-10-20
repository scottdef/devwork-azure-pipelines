# Beta

How to run the library with a small group before announcing it, and how to tell
whether it is ready.

## Gating

There is no login on a static site, so the gates are about *discovery* and *processing*:

1. **Not linked.** During beta the intake page is linked only from the beta README
   (this file) and the beta channel. `issueops-library`'s catalog links to the form
   page; hold back the catalog link too if the catalog itself is not public yet.
2. **`flags.json`.** The intake page fetches `flags.json` from its own site on load. With
   `{"submit": false}` the submit buttons are disabled and a banner says submissions are
   paused. Toggle by editing `issueops-intake/flags.json` and pushing (pages.yml
   redeploys in about a minute). A missing or broken `flags.json` leaves the form
   *enabled*, so a bad deploy cannot lock everyone out.
3. **Workflows.** The flag gates the page, not GitHub: anyone who can open an issue can
   still use the form directly. To stop processing, disable the intake workflows
   (`gh workflow disable intake --repo CoolGitOrg/issueops-intake`, same for `borrow`
   and `return`).
4. **Repository visibility.** On GitHub Enterprise Cloud, make the intake repo `internal`
   (`terraform` variable `intake_visibility = "internal"`): only enterprise members can
   see the forms and file issues.
5. **Access-controlled Pages (GHEC caveat).** Enterprise Cloud can restrict a Pages site
   to people with read access to the repo ("private Pages"). That would make the catalog
   members-only, but the site then lives on a separate `*.pages.github.io` domain,
   the `BASE_URL`/`LIBRARY_BASE_URL` variables must change to match, and the `notify`
   job's anonymous `curl` poll of `library.json` gets a login redirect instead of JSON.
   `wait-live.sh` then times out after 10 minutes and the comment says the deploy is
   pending. Either accept that or keep the catalog public during beta.

## Beta checklist

Before inviting anyone:

- [ ] `terraform apply` done; App installed on both repos; `scripts/set-secrets.sh` run.
- [ ] `make preflight` green on the commit you deploy.
- [ ] Intake `pages` workflow deployed; the form loads; `flags.json` says `submit: true`.
- [ ] One manual request end to end (below), as a non-admin account.
- [ ] One invalid request: comment lists the errors, label `invalid`; editing the issue fixes it.
- [ ] One borrow, one phase-2 publish by the borrower, one refused phase-2 by someone else.
- [ ] `overdue` run manually with `grace: 0d` against a test loan (then return it).

## Running the beta load

```sh
gh auth login                                 # an account that can open issues on the intake repo
scripts/beta-run.sh -n 10 -i 10 -t caddyfile  # 10 requests, one every 10 s
```

The script labels issues `beta`, polls each item URL until it answers 200, and prints:

```
results: 10 live, 0 timed out
issue->Pages latency (s): min 118  p50 164  p90 231  max 240
```

(Illustrative numbers. Record your real ones in the table below.)

Clean up afterwards: close the beta issues, and remove the `beta-*` items from
`items/` in a PR if you do not want them in the catalog.

## Exit criteria

| Measure | Bar |
|---|---|
| issue → Pages p50 | ≤ 4 min |
| issue → Pages p90 | ≤ 8 min |
| timeouts in a 10-request run | 0 |
| invalid requests that got a useful comment | 100% |
| lost events (accepted issue never published, never commented) | 0 |
| catalog p99 at 50 req/s ([LOADTEST](LOADTEST.md)) | < 1.5 s |
| issues opened by beta users about confusing behaviour | each one answered in docs or fixed |

| Date | Run | p50 | p90 | Timeouts | Notes |
|---|---|---|---|---|---|
| | `-n 10 -i 10` | | | | |
