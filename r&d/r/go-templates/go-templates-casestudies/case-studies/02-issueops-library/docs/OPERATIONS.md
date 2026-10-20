# Operations

## Moving parts

| Where | Workflow | Trigger | Does |
|---|---|---|---|
| intake | `intake.yml` | issue opened/edited/reopened with `request` | parse, validate, dispatch `library-publish` |
| intake | `borrow.yml` | same, `borrow` | parse, validate, dispatch `library-borrow` |
| intake | `return.yml` | same, `return` | parse, validate, dispatch `library-return` |
| intake | `pages.yml` | push to forms/schema/site code, manual | render and deploy the form page (+ wasm lab) |
| library | `publish.yml` | `repository_dispatch` (3 types) | apply, then deploy, then notify |
| library | `overdue.yml` | daily 06:17 UTC, manual | warn and auto-return late loans, deploy |
| library | `pages.yml` | push to templates/code, manual | rebuild `site/`, commit, deploy |
| both | `ci.yml`, `release.yml` | push/PR, tag `v*` | preflight + fuzz; cross builds + `SHA256SUMS` + release |

Configuration (all set by `terraform/`): variables `APP_CLIENT_ID`, `GO_VERSION`,
`LIBRARY_BASE_URL` (library only); secret `APP_PRIVATE_KEY` (by `scripts/set-secrets.sh`).

## Health at a glance

```sh
gh run list --repo CoolGitOrg/issueops-library --workflow publish --limit 20
gh issue list --repo CoolGitOrg/issueops-intake --label accepted --state open   # dispatched, not yet published
gh issue list --repo CoolGitOrg/issueops-intake --label invalid --state open
curl -s https://coolgitorg.github.io/issueops-library/library.json | jq '{generated, count, open: (.open_loans|length)}'
curl -s https://coolgitorg.github.io/issueops-library/ledger/open.json | jq -r '.[] | [.item,.borrower,.due] | @tsv'
```

An `accepted` issue that stays open for more than ~10 minutes means the library run
failed or is stuck: look at the newest `publish` runs.

## Runbook

### A request is stuck on `accepted`

1. Find the run: `gh run list --repo CoolGitOrg/issueops-library --workflow publish --json databaseId,displayTitle,conclusion`.
2. If **apply** failed, the run comments "Library failed" on the issue. Read the log. Common causes:
   - `payload sha256 … does not match`: the event was hand-crafted or altered. Ask the requester to edit the issue (re-dispatches).
   - template error: the payload lacks a key the template needs. The requester gets a refusal comment; nothing to do.
   - push failed 5 times: unusual contention. Re-run the job; apply is idempotent.
3. If **deploy** failed (Pages outage), re-run the failed jobs: `gh run rerun <id> --failed`. The commit is already on `main`.
4. If **notify** timed out, the comment says so; the next deploy will publish it. Nothing to fix.

### Replay an event

Every event is reproducible from the issue. Edit the issue (any edit) to re-run intake,
or dispatch by hand with the same binary the workflow uses:

```sh
cd issueops-intake && make build
gh issue view 42 --repo CoolGitOrg/issueops-intake --json body --jq .body |
  bin/issueparse -form .github/ISSUE_TEMPLATE/request.yml > /tmp/req.json
bin/validate -schema schema/request.json /tmp/req.json
GH_TOKEN=$(gh auth token) GITHUB_REPOSITORY=CoolGitOrg/issueops-intake \
  bin/dispatch -repo CoolGitOrg/issueops-library -event library-publish -issue 42 \
  -requester "$(gh issue view 42 --repo CoolGitOrg/issueops-intake --json author --jq .author.login)" /tmp/req.json
```

A personal token works for `POST /dispatches` if it has `contents: write` on the library;
the run then shows you as the sender, which is fine for a replay.

Re-publishing an item id replaces it in place (same path, new `published_at`, new sums).

### Rebuild the site without an event

`gh workflow run pages --repo CoolGitOrg/issueops-library`, or locally:

```sh
cd issueops-library && make build
bin/pagegen -site site -items items -ledger ledger/ledger.json && git diff --stat site
```

### Fix the ledger by hand

The ledger is plain JSON in git; humans change it through a PR.

```sh
bin/ledger -f ledger/ledger.json list -all                  # what is there
bin/ledger -f ledger/ledger.json return -item X -by ops -force   # operator return
bin/ledger -f ledger/ledger.json overdue -grace 7d -n       # what tonight's sweep would do
```

Commit `ledger/ledger.json` and the rebuilt `site/` together.

### Remove an item

Delete `items/<source>/<template>/<issue>/` in a PR, close any open loan on it with
`ledger return -force`, and rebuild the site. Git history keeps the old files; if the
removal is for content reasons (secrets in a payload), also follow GitHub's guidance for
removing sensitive data from history, and rotate whatever leaked.

### Pause submissions (beta gate)

Set `{"submit": false}` in `issueops-intake/flags.json` and push; `pages.yml` redeploys
and the form page disables its button. The flag only gates the page: anyone can still
file an issue on GitHub directly. To stop processing too, disable the intake workflows
(`gh workflow disable intake --repo CoolGitOrg/issueops-intake`, likewise `borrow`,
`return`); issues filed meanwhile are processed when you re-enable and edit them.

### Rotate the App key

Generate a new private key on the App's settings page, run `scripts/set-secrets.sh new.pem`,
confirm a run succeeds, then delete the old key in the App settings.

## Limits worth knowing

| Limit | Value | Where it bites |
|---|---|---|
| `client_payload` top-level keys | 10 | `dispatch.Build` refuses more |
| our payload limit | 32 KiB | `validate` (`maxLength`), `dispatch`, `event.Decode` |
| issue body | 65,536 chars | GitHub rejects longer bodies |
| prefill URL | 7,000 chars (our guard) | the form page tells users to attach payloads instead |
| App installation token | ≥5,000 REST requests/hour, 1 h lifetime | we use ~10 calls per request |
| write pacing | ≥1 s between writes | `dispatch` paces itself; `overdue.sh` sleeps 1 s per comment |
| Pages deploy | ~1-3 min, serialized | `docs/LOADTEST.md`, `docs/BETA.md` |
| Pages site size | 1 GB; soft bandwidth 100 GB/month | items are KBs; not close |
