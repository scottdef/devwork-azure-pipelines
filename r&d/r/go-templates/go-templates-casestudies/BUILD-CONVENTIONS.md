# Build conventions (binding for every case-study repo)

These rules apply to every repository under `case-studies/` and to `cookbook/`.

## Toolchain and dependencies

- `go.mod`: `go 1.24`. Local toolchain here is go1.24.7; CI pins `go-version: "1.24.x"`
  behind a `GO_VERSION` variable so operators can lift it without touching language level.
- **Dependencies:** Go standard library only, plus `github.com/go-echarts/go-echarts/v2`
  (vendored at `case-studies/03-tmplforge/third_party/go-echarts`, v2.7.3, consumed via
  `replace`). `upspin.io/*` is permitted but not required. Nothing else. No `golang.org/x/*`.
- **There is no module-proxy network access in this build environment.** Every module must
  build with `GOFLAGS=-mod=mod GOPROXY=off`. Do not add `require` lines you cannot satisfy.
- Static builds: `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$VERSION"`.
- Targets: `linux/amd64 linux/arm64 darwin/arm64 plan9/amd64`. Every binary must
  cross-compile for `GOOS=plan9` — so **no `syscall.Termios`, no `syscall.SIGHUP` constants
  outside `//go:build !plan9` files, no `os/user`-heavy code**. Use build tags where an OS
  differs (`signal_unix.go` / `signal_plan9.go`).

## Shell, scripts, workflows

- bash scripts start `#!/usr/bin/env bash` + `set -euo pipefail`; must pass `bash -n`.
- Python only via `uv run` (never pip). Avoid Python unless it earns its place.
- GitHub Actions: pin majors `actions/checkout@v6`, `actions/setup-go@v6`,
  `actions/configure-pages@v5`, `actions/upload-pages-artifact@v5`, `actions/deploy-pages@v5`,
  `actions/create-github-app-token@v3` (inputs `client-id`, `private-key`, `repositories`).
  Prefer `gh` + our own Go binaries over `actions/github-script`.
- Least-privilege `permissions:` on every workflow. Untrusted text (issue bodies, inputs)
  flows through `env:` and never into `run:` interpolation.
- Commits from workflows: `git config user.name "github-actions[bot]"`,
  `git config user.email "41898282+github-actions[bot]@users.noreply.github.com"`.
- Release: `sha256sum * > SHA256SUMS` in `dist/`, then `gh release create "$TAG" dist/* --verify-tag`.

## Standing GitHub assumptions

- Enterprise `CoolGitEnterprise`, org `CoolGitOrg`, GitHub App `issueops-autoadmin-app`
  (variable `APP_CLIENT_ID`, secret `APP_PRIVATE_KEY`).
- Terraform: `integrations/github` provider `~> 6.13`, `required_version = ">= 1.6"`.
  `terraform` is **not installed** in this environment — write HCL carefully, keep it
  `terraform fmt`-clean by hand (2-space indent, aligned `=`).

## Repo skeleton (every case study)

```
<repo>/
├── README.md            # what, why, quickstart, diagrams (Mermaid) + link to docs/
├── Makefile             # all, build, test, vet, lint, cross, release, loadtest, clean
├── mkfile               # Plan 9 mk equivalent of the Makefile (tabs!)
├── go.mod
├── cmd/<bin>/main.go
├── internal/...
├── testdata/golden/...  # golden tests with -update flag
├── scripts/preflight.sh # vet, test, bash -n, go build for all GOOS, checksums
├── scripts/loadtest.sh  # vegeta/hey if present, else the repo's own Go load tool
├── terraform/{versions.tf,main.tf,variables.tf,outputs.tf}
├── .github/workflows/{ci.yml,release.yml,...}
├── docs/{DESIGN.md,OPERATIONS.md,SECURITY.md,LOADTEST.md,FUNCS.md}
├── docs/diagrams/*.mmd  # Mermaid sources
├── docs/images/*.svg    # hand-written SVG architecture images (no external tools)
└── LICENSE              # MIT
```

- Makefile and mkfile recipes use TABS. `mkfile` syntax: `target:V: deps` for phony, `$O`
  object char not needed; keep rules simple shell lines (`rc` not required — use `#!/bin/sh`-
  compatible lines or `rc` idioms sparingly).
- `scripts/preflight.sh` must exit 0 in this environment (no network, no terraform binary —
  skip `terraform validate` when `terraform` is absent and say so).
- Tests: `go test ./...` green; golden files committed; at least one `httptest`-based test
  per HTTP server; fuzz targets where the code parses untrusted input.
- Docs are Markdown. Diagrams: Mermaid `.mmd` in `docs/diagrams/` **and** embedded in README.
- Every rendered artifact / every output format has a golden file.
- Write real code, not stubs. Every `main.go` must build and do what the docs say.

## Style (Pike/Thompson)

- Small packages, plain names, no frameworks, no reflection tricks beyond `text/template`'s own.
- Flags are single-dash (`-a addr`, `-c config`), Plan 9 style; `flag` package.
- Errors are values; wrap with `%w`; no panics outside `template.Must` at init.
- Text is the interface: every binary reads stdin/args and writes stdout; exit codes matter.
- Comments explain *why*. Doc comments on every exported identifier.
