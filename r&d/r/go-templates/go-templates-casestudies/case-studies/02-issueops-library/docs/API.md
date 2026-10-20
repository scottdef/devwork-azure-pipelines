# API surface

Every call the system makes, who makes it, and with which token. REST version header
`X-GitHub-Api-Version: 2022-11-28` throughout.

## GitHub REST

| Call | Caller | Token | Notes |
|---|---|---|---|
| `POST /repos/{owner}/{repo}/dispatches` | `bin/dispatch` (intake workflows) | App installation | `204 No Content` on success. Retries 5xx/429 with 2 s, 4 s, 8 s... backoff, honours `Retry-After`, ≥1 s between writes. |
| `POST /repos/{o}/{r}/issues/{n}/comments` | `gh issue comment` | `GITHUB_TOKEN` (intake), App (library) | invalid / dispatched / published / refused / overdue comments |
| `POST /repos/{o}/{r}/issues/{n}/labels`, `DELETE .../labels/{name}` | `gh issue edit --add-label/--remove-label` | same | `invalid`, `accepted`, `published`, `overdue` |
| `PATCH /repos/{o}/{r}/issues/{n}` (`state: closed`) | `gh issue close` | App | request issue after publish; borrow issue after return/next phase/auto-return |
| `GET /repos/{o}/{r}/issues/{n}` | `gh issue view` (overdue sweep) | App | to skip already-warned loans |
| `POST /app/installations/{id}/access_tokens` | `create-github-app-token@v3` | App JWT | per job; `repositories` limits the token |
| git smart HTTP (`fetch`, `push`) | `apply-event.sh`, `overdue.sh`, library `pages.yml` | App installation | push to `main` via ruleset bypass |
| Pages deployment API | `deploy-pages@v5` | `GITHUB_TOKEN` + OIDC (`id-token: write`) | via `upload-pages-artifact@v5` artifact |

## `repository_dispatch` payloads

Envelope sent by `bin/dispatch`:

```json
{"event_type": "library-publish", "client_payload": { ... }}
```

| event_type | client_payload keys (≤10) |
|---|---|
| `library-publish` | `source_id`, `template_id`, `phase` (string "1".."3"), `issue` (int), `repo` ("owner/name" of intake), `requester` (login), `payload` (compact JSON object as a string, ≤32 KiB), `sha256` (hex of `payload`) |
| `library-borrow` | `item_id` ("source/template/issue"), `days` ("7"/"14"), `reason` (≤1024 bytes), `issue`, `repo`, `requester` |
| `library-return` | `item_id`, `issue`, `repo`, `requester` |

Example (from `issueops-intake/testdata/golden/dispatch/publish.dry.json`):

```json
{
  "event_type": "library-publish",
  "client_payload": {
    "issue": 42,
    "payload": "{\"name\":\"payments-api\",\"domain\":\"pay.coolgit.org\",\"port\":8443,\"routes\":[\"/pay\",\"/refund\"]}",
    "phase": "1",
    "repo": "CoolGitOrg/issueops-intake",
    "requester": "alice",
    "sha256": "a696a2834d3ebfb35453652f13154128e629d5097639c12528b7b2a56aeef858",
    "source_id": "payments-api",
    "template_id": "caddyfile"
  }
}
```

The library receives it as `github.event` (`action` = event type, `client_payload` as
above) and pipes `toJSON(github.event)` into `pagegen -event -`.

## Prefill URLs (GitHub web, not REST)

```
https://github.com/CoolGitOrg/issueops-intake/issues/new
  ?template=request.yml
  &title=%5Brequest%5D%20payments-api%20caddyfile
  &source-id=payments-api
  &payload=%7B%22name%22%3A%22payments-api%22%7D
  &notes=...
```

`template` selects the form file; other parameters are matched to form fields by `id`.
The library's Borrow/Return buttons build the same kind of URL with `item-id`.

## Static JSON on Pages (read-only)

| URL | Shape |
|---|---|
| `/issueops-library/library.json` | `{generated, base, count, items: [{id, source_id, template_id, phase, issue, published_at, url, artifact, borrowed, due?, files: [{name, size, sha256}]}], open_loans: [...]}` |
| `/issueops-library/ledger/open.json` | `[{item, borrower, borrow_issue, borrowed_at, due, returned_at: null}]` |
| `/issueops-library/items/<id>/config.json` | item config incl. `requested_by`, `payload`, `payload_sha256`, `template_sha256`, `supersedes?` |
| `/issueops-library/items/<id>/SHA256SUMS`, `/issueops-library/SHA256SUMS` | `sha256sum -c` format |
| `/issueops-library/feed.xml` | Atom 1.0, newest 50 items |
| `/issueops-library/sitemap.txt` | one URL per line |
| `/issueops-intake/flags.json` | `{"submit": bool}` beta gate |
| `/issueops-intake/forms.json` | the parsed forms the page was built from |

## Exit status and JSON results (command-line API)

| Binary | 0 | 1 | 2 | 3 |
|---|---|---|---|---|
| `issueparse` | parsed | bad form / no fields | usage | |
| `validate` | valid | invalid (report on stdout) | usage / I/O | |
| `dispatch` | sent (receipt on stdout) | build or HTTP failure | usage | |
| `pagegen` (library) | applied (result on stdout) | failure | usage | refused by a rule (result says why) |
| `ledger` | done | failure | usage | refused by a rule |
| `tpl` | rendered | failure | usage | |
| `siteload` | within budget | over p99 / error budget | usage | |
