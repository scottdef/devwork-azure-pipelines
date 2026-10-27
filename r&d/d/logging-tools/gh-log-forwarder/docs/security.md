# Security

## Credentials and their reach

| Credential | Lives in | Can do | Narrowed by |
|---|---|---|---|
| `issueops-autoadmin-app` key | forwarder repo secret `GHLOG_APP_PRIVATE_KEY` | mint installation tokens | every token is minted with `permission-actions: read` (+ metadata) only |
| Installation token | step env, ~1 h | read Actions runs/jobs/logs | permissions requested at mint time |
| `DT_LOGS_TOKEN` | forwarder repo secret | `logs.ingest` only | Terraform `scopes = ["logs.ingest"]`, optional expiry |
| `GITHUB_TOKEN` | per job | `contents: read`, or `write` only in jobs that push `forwarder-state` | job-level `permissions:`; workflow default `{}` |
| Dispatch app key (push mode) | each push-mode repo | `repository_dispatch` to the forwarder | separate app installed on the forwarder repo alone with Contents: write |

**Never put the issueops-autoadmin-app key in target repositories.** Any
workflow there could mint a token with all of that app's permissions. Push
mode therefore requires a second, single-purpose app (Terraform refuses
push mode without it).

Consider a dedicated read-only app for the forwarder as well
(Actions: read, Metadata: read) so a leaked forwarder secret is worth
nothing beyond log reading.

## Secret handling in the binary

- Tokens are read from the environment only, never flags (no `ps` leak),
  trimmed, and never logged; `--print-config` elides them.
- The job-log download is a 302 to a signed URL on another host.
  `CheckRedirect` drops `Authorization` whenever host or port changes and
  refuses https→http; a test asserts the blob host never sees the token.
- `ghlog2dt token --github-output` emits `::add-mask::` on stdout before
  writing the token to `$GITHUB_OUTPUT`.
- `scripts/dql.sh` passes its bearer token to curl via `--config -` on
  stdin, not argv.

## Redaction (defence in depth)

GitHub masks registered secrets in logs, but tools print credentials
GitHub never knew about. Before batching, every line is matched against
`redact` patterns and matches become `[REDACTED]`:

GitHub tokens (`ghp_ gho_ ghu_ ghs_ ghr_`, `github_pat_`), Dynatrace
tokens (`dt0c01.…`), AWS access keys, PEM private-key headers,
`Authorization:`/`X-Api-Key:` header values, Slack tokens. Add your own in
`ghlog2dt.json` (setting `redact` replaces the list, so copy the
defaults from `examples/ghlog2dt.full.json`).

The optional OpenPipeline pipeline adds a server-side masking processor
as a second net. Redactions are counted per run (`redactions` in the
summary); a sudden rise means someone is printing secrets.

## Workflow hardening

- `permissions: {}` at workflow level; each job asks for what it needs.
- `persist-credentials: false` on every checkout that does not push.
- No `${{ }}` interpolation inside `run:` scripts for untrusted values:
  dispatch payloads and dispatch inputs go through `env:` and are
  validated with anchored regexes (`owner/name`, numeric IDs).
- `forward-event.yml` accepts only repositories in `GHLOG_TARGET_REPOS`
  (unless `*`), so anyone able to send a dispatch cannot make the
  forwarder export arbitrary repositories.
- Pushes to `forwarder-state` use `GITHUB_TOKEN`, which cannot trigger
  further workflows: no loops.
- `checkpoint.yml` and sweeps share a concurrency group; neither cancels
  the other mid-flight.

## Supply chain

- `ghlog2dt` depends on the Go standard library only; `make preflight`
  fails the build on any other module.
- Releases are static (`CGO_ENABLED=0`, `-trimpath`, empty build ID),
  published with `SHA256SUMS` and GitHub build-provenance attestations:

  ```bash
  gh attestation verify ghlog2dt_linux_amd64 --repo CoolGitOrg/gh-log-forwarder
  ```

- `install-ghlog2dt.sh` installs a pinned version only if its line in
  `SHA256SUMS` verifies (`sha256sum --check --strict`).
- Third-party actions are referenced by major tag for readability. Pin
  them to full commit SHAs in production (Dependabot or Renovate keeps
  pins current); enterprise policy can require it.

## Data governance

- Build logs land in their own Grail bucket (`ci_github_actions_logs`)
  with their own retention; IAM policies can grant `storage:logs:read`
  on that bucket to the teams that need it.
- Every record carries `dt.security_context` (default `ci`) for
  record-level permissions; set it per team with `--attr` or config.
- Terraform state holds the ingest token in plain text: use an encrypted
  backend with restricted access.
