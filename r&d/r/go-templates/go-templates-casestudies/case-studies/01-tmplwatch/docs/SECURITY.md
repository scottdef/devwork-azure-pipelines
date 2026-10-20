# Security

## Trust model

* **Template authors are trusted.** A template can read any environment
  variable (`env`), any file under its own directory (`file`), and decide
  the content of files written with the configured modes. Treat templates
  like code: review them, and keep them in the tmplwatch repository or
  `/etc/tmplwatch`, never in the kv repository.
* **KV data is semi-trusted.** It arrives through reviewed pull requests,
  but tmplwatch assumes a value may be wrong or hostile:
  * values are only ever *data* to `text/template`; they are never parsed as
    templates;
  * `command` is an argv vector run with `exec.Command`, and no shell is ever
    involved, so a value cannot inject shell syntax into a command. Use `shq`
    if a template itself writes a shell script;
  * HCL outputs use `toHCL`, which escapes `${` and `%{`. `FuzzRender`
    asserts no data-borne interpolation reaches `main.tf` or
    `job.nomad.hcl`;
  * YAML outputs use `toYAML`, which quotes anything YAML 1.1 or 1.2 would
    misread (`yes`, `0755`, `1:30`, `2026-01-02`, leading `-`, `: `);
  * the shipped templates validate structure with `fail` (WireGuard key
    format, nginx upstream references, Caddy site shape), so bad data stops
    the render instead of shipping a broken file;
  * an incomplete or failed render writes nothing.
* **`env` and `file` are capabilities.** `file` is rooted with `os.DirFS`
  at the template's directory, and `fs.ValidPath` rejects `..` and absolute
  paths. `env` is unrestricted, because consul-template compatibility needs
  it. Run tmplwatch with a minimal environment (the systemd unit does).

## Tokens

* **Prefer the GitHub App** (`issueops-autoadmin-app`). tmplwatch mints
  installation tokens scoped to the single kv repository with
  `contents: read`, valid for one hour and refreshed in memory. They are
  never logged or written.
* The App private key is read once at startup from `app_private_key_file`
  (mode 0400, owned by root, passed with `LoadCredential=`), or from an env
  var for containers.
* A static token is read from `$GITHUB_TOKEN`, or from the file named by
  `$GITHUB_TOKEN_FILE`, re-read on each request so rotation needs no
  restart. Use a fine-grained token with read-only Contents on one
  repository.
* Workflows: untrusted text (dispatch payloads, `workflow_dispatch`
  inputs) reaches `run:` steps only through `env:`; every workflow declares
  least-privilege `permissions:`; App tokens are minted per job by
  `actions/create-github-app-token@v3` and scoped with `repositories:`.

## Network surface

* One outbound HTTPS client: `api.github.com`, or GHES via `api_url`.
  Responses are size-limited (64 MiB for a tree, 1 MiB per value), and a
  truncated tree is refused.
* One inbound listener, loopback by default, with no authentication.
  `/render` returns rendered configuration, which may include secrets
  rendered into files, so **do not** bind it to a routable address without
  a proxy that authenticates. `-pprof` is refused on non-loopback
  listeners. Server timeouts and a 16 KiB header limit are set.

## Files

* Destinations are written via a temp file created with mode 0600, chmodded
  to `perms` before the rename. A secret file is never visible with looser
  permissions, even briefly.
* `backup: true` keeps one previous version with the same mode.
* The systemd unit runs as a `DynamicUser` with `ProtectSystem=strict`, an
  empty capability bounding set, and write access only to `ReadWritePaths`.

## Toolchain

The build convention pins **Go 1.24** (`go.mod` `go 1.24`; CI `1.24.x`;
local go1.24.7). As of October 2026 that toolchain is **outside the
upstream security support window**. Go supports the two most recent
releases, 1.26 and 1.27, and 1.24 stopped receiving fixes on 10 February
2026 with 1.24.13 ([endoflife.date/go]).

Template-related advisories in 2026 all concern **`html/template`**:

| Advisory | Package | Issue | Fixed in |
|---|---|---|---|
| CVE-2026-32289 | html/template | JS template-literal context tracked incorrectly; XSS | 1.25.9, 1.26.2 ([announcement]) |
| CVE-2026-39823 / GO-2026-4982 | html/template | URL in `<meta content>` not escaped when whitespace surrounds `=`; XSS | 1.25.10, 1.26.3 ([GO-2026-4982]) |
| CVE-2026-56858 / GO-2026-6091 | html/template | malformed input ends a JS regexp context early; XSS | 1.25.13, 1.26.6, 1.27.0 ([opencve]) |

**These do not apply to tmplwatch.** It imports `text/template` only:
no contextual auto-escaping, no HTML output, no browser. `go list -deps
./... | grep html/template` is empty for every binary. The 2026 release
announcements list no `text/template` advisories.

Other 2026 standard-library advisories **do** touch packages tmplwatch
links, for example `crypto/x509` chain-building DoS (CVE-2026-32280) and
`crypto/tls` key-update DoS (CVE-2026-32283), both in the
[April 2026 security release][announcement]. They are reachable only
through the outbound connection to GitHub, whose certificates are not
attacker-controlled in normal operation, so the risk is low but not zero.

Recommendations:

1. Set the repository variable `GO_VERSION` to a supported toolchain
   (`1.27.x` or `1.26.x`) for **release builds**. The code needs no change:
   `go.mod`'s `go 1.24` is a minimum language version, not a pin.
2. Run `govulncheck ./...` in CI when network access allows. It is not
   vendored here because it is not standard library.
3. Track [golang-announce] and the [Go vulnerability database].

[endoflife.date/go]: https://endoflife.date/go
[announcement]: https://groups.google.com/g/golang-announce/c/0uYbvbPZRWU
[GO-2026-4982]: https://pkg.go.dev/vuln/GO-2026-4982
[opencve]: https://app.opencve.io/cve/CVE-2026-56858
[golang-announce]: https://groups.google.com/g/golang-announce
[Go vulnerability database]: https://pkg.go.dev/vuln/

## Reporting

Open a private security advisory on `CoolGitOrg/tmplwatch`.
