# adoclone

Duplicates an Azure DevOps project into a new project in the same organization
(default org: `CoolADO`). Go 1.21, standard library plus go-echarts. Design,
capability matrix and limitations are in the implementation guide;
`docs/runbook.md` is the operator's checklist.

Every component from the guide is implemented (28, listed below). The code
builds, passes `go vet`, and has unit and fake-server tests (`go test ./...`).
It has **not** been run against a live organization: dry-run first, then do a
real run against a scratch target project.

## Build and test

```
go test ./...
go build -ldflags "-X main.version=1.1.0" -o bin/adoclone ./cmd/adoclone
```

## Run

```
export ADO_PAT=...                       # or ADO_PAT_FILE=/path/to/file; scopes in docs/runbook.md
SRC=SourceProj TGT=TargetProj bash scripts/preflight.sh
./bin/adoclone plan  -source SourceProj > plan.json                      # source inventory, read-only
./bin/adoclone clone -source SourceProj -target TargetProj               # dry run (default): logs every write
./bin/adoclone clone -source SourceProj -target TargetProj -dry-run=false
./bin/adoclone verify -source SourceProj -target TargetProj > verify.json  # exits 4 on problems
./bin/adoclone report -source SourceProj -target TargetProj -out report.html
CONFIRM=TargetProj ./bin/adoclone cleanup -source SourceProj -target TargetProj -dry-run=false
```

Rerunning `clone` resumes: `state.json` (plus its `.journal`) maps every
source ID to its target ID, and anything already mapped is skipped. Dry runs
never write to the checkpoint; their follow-ups go to `todo-dryrun.json`.
A checkpoint belongs to one source and one target: `clone` refuses a target
project that already exists unless this checkpoint created it (or you pass
`-adopt-existing`), so a mistyped `-target` can't touch somebody else's project. Run part of the job with `-components
teams,boards` or leave parts out with `-skip security,hooks`;
`adoclone components` lists them in run order.

| Flag | Default | |
|---|---|---|
| `-org` | `$ADO_ORG` or `CoolADO` | organization |
| `-source`, `-target` | | project names (must differ) |
| `-components` / `-skip` | `all` / none | component selection |
| `-dry-run` | `true` | log writes instead of sending them |
| `-adopt-existing` | `false` | let `clone`/`cleanup` use a target project this checkpoint didn't create |
| `-bypass-rules` | `true` | keep created/changed dates and people on work items (needs the Bypass rules permission) |
| `-visibility` | `private` | `private`, `public`, or `source` to copy the source's |
| `-repo-mode` | `mirror` | `mirror` (git clone/push, with LFS when git-lfs is installed) or `import` (server-side import requests; no git needed, no LFS) |
| `-vargroup-mode` | `copy` | `copy` (secrets from `SECRET_<GROUP>_<VAR>` or todo.json) or `share` |
| `-securefiles-dir` | none | directory of secure files to upload by name |
| `-state` | `state.json` | checkpoint; `todo.json` is written next to it |
| `-work-dir` | temp dir | scratch space for repo mirrors and large attachments |
| `-metrics-addr`, `-metrics-hold` | off | Prometheus `/metrics` and `/healthz`; keep serving for a while after the run |
| `-sample` | `5` | percent of copied work items that verify/report field-check |
| `-out` | `report.html` | report path |

Exit codes: `0` ok · `1` a component stopped (fix and rerun; it resumes) ·
`2` bad invocation · `3` finished with item failures (listed in `todo.json`) ·
`4` verify found missing objects or work item differences.

Logs are JSON on stderr; stdout carries only the JSON results of `plan` and `verify`.

## Components

| Phase | Component | What it copies | Notes |
|---|---|---|---|
| 1 | `project` | project with the same process and source-control type | private by default |
| 1 | `groups` | custom project security groups | memberships come in `security` |
| 2 | `nodes` | area and iteration paths, iteration dates | only missing nodes are created |
| 2 | `teams` | teams, iterations, area values, settings, members | default team maps to the target's |
| 2 | `boards` | board columns, swimlanes, card settings, card rules | matched by name |
| 3 | `repos` | repos, all branches and tags, LFS, default branch | never deletes target-only branches; the source default repo maps to the target's |
| 4 | `endpoints` | shares service connections | no secrets read or copied |
| 4 | `vargroups` | variable groups | Key Vault-linked groups need no secrets |
| 4 | `securefiles` | secure files from `-securefiles-dir` | contents can't be read from the API |
| 4 | `queues` | agent queues for the same pools | |
| 4 | `deploymentgroups` | deployment groups on the same deployment pool, target tags | machines aren't re-registered |
| 4 | `envs` | environments and Kubernetes resources | VM resources need their agent re-registered |
| 5 | `taskgroups` | task groups, nested ones first | |
| 5 | `pipelines` | build folders and YAML/classic build definitions | build-completion triggers fixed in a second pass |
| 5 | `releases` | release folders and classic release definitions | artifacts re-pointed at copied pipelines and repos |
| 5 | `checks` | approvals and checks on environments, queues, variable groups, secure files, repos | shared resources keep their checks |
| 5 | `permissions` | pipeline authorizations on resources | |
| 5 | `policies` | branch and repository policies | skipped (todo.json) if the repo or pipeline wasn't copied |
| 5 | `settings` | pipeline general and retention settings | |
| 6 | `workitems` | work items, comments, attachments, links | see below |
| 7 | `testplans` | test variables, configurations, plans, suites, static-suite test cases | run after `workitems` |
| 8 | `queries` | Shared Queries tree | personal "My Queries" can't be created for other users |
| 8 | `dashboards` | project and team dashboards and widgets | widget settings remapped |
| 8 | `plans` | delivery plans | rows for other projects' teams kept |
| 8 | `wiki` | project wiki content (git) and code wikis | |
| 8 | `feeds` | project-scoped feeds, views, upstreams, permissions | packages must be re-published (todo.json) |
| 9 | `security` | group memberships, ACLs (8 namespaces), role assignments | run last; merged into the target's ACLs; source-only identities are never granted anything |
| 9 | `hooks` | service hook subscriptions | ones with masked secrets are created disabled (todo.json) |

Work items: created at their current state with a hyperlink back to the
source item. With `-bypass-rules` the original created/changed dates and
people are kept. Comments are copied oldest first with the original author and
date in the text. Attachments are streamed, and chunked above 128 MiB. Work
item links are added once (from the lower source ID; the server adds the
reverse). Links to items in other projects of the org keep pointing at them.
Commit and branch links are re-pointed at the copied repos. Pull request,
build and wiki links keep pointing at the source, which stays as the archive.
Test case shared-step and shared-parameter references are remapped. Board
fields (`WEF_*`), `System.Parent` and computed counters are skipped. Reruns
don't duplicate: each comment and attachment is checkpointed, and an item
whose link partner wasn't copied yet stays open until a rerun can add it.

Security: groups map by name, the source's build service maps to the
target's, and any other source-project identity is dropped and listed in
`todo.json` rather than granted access to the target. ACL entries are merged
into the target's existing ACLs, so the target keeps its own entries.

## Not copied

Pull requests, work item revision history, build/release/test run history,
secret values (variable group secrets, secure file contents, service hook
credentials), packages, personal queries, and extension data. Keep the source
project read-only as the archive. `todo.json` lists every item that needs a
hand.

## Deploy

- **GitHub Actions:** `.github/workflows/adoclone.yml` (`workflow_dispatch`). Set the `ADO_PAT` secret; add `SECRET_<GROUP>_<VAR>` env lines to the Clone step for variable group secrets. The checkpoint is cached per run attempt, so a new run or a "Re-run jobs" resumes.
- **Azure Pipelines:** `azure-pipelines.yml`, with a variable group `adoclone-secrets` holding `ADO_PAT`. Secret variables only reach the script when mapped in the Clone step's `env:` (the file shows how).
- **AKS Job:** `Dockerfile` (Alpine with git and git-lfs, uid 65532) and `deploy/k8s` (`kubectl apply -k deploy/k8s`). The PAT comes from Key Vault through the Secrets Store CSI driver and Workload ID; the checkpoint and scratch space are on a 50 GiB PVC. Setup commands are in `docs/runbook.md`.
- **Grafana OSS 12:** `deploy/k8s/podmonitor.yaml` scrapes the Job's `/metrics`; import `deploy/grafana/adoclone-dashboard.json`.

Metrics: `adoclone_items_total{component,side}`, `adoclone_errors_total`,
`adoclone_throttle_seconds_total`, `adoclone_requests_total{method,code}`,
`adoclone_phase_status{component}` (0 pending, 1 running, 2 done, 3 failed),
`adoclone_info`.

## API details still to confirm on a live org

The endpoints were checked against Microsoft Learn and the REST specs. These
weren't fully documented, so the code handles them defensively:

- Secure files are documented only at `api-version=7.2-preview.1`; that's what the tool sends.
- Kubernetes environment resources: the 7.1 body doesn't list `serviceEndpointId`. The tool sends it and falls back to the 7.2 `pipelines/environments` route on a 400 or 404.
- Pipeline permission resource type names beyond `queue` and `environment`, and the `{projectId}.{repoId}` repository ID, come from Microsoft's Terraform provider, not Learn.
- Import mode's temporary connection (`type: git`, `UsernamePassword`) isn't named on Learn. `mirror` is the default.
- Service hook `status` on create isn't confirmed, so subscriptions with masked secrets are created, then disabled with a PUT.
- Query folder ACL tokens appear as `$/{project}/...` (Terraform) and `/{project}/...` (Learn); both prefixes are read.
- New board columns are sent without an `id`; chunked attachment uploads use 16 MiB chunks. Neither is specified on Learn.

## Layout

```
cmd/adoclone/main.go          CLI and exit codes
internal/config               flags and environment
internal/client               REST client: retries, Retry-After, rate limits, paging, streaming, dry run
internal/state                checkpoint with journal, todo.json
internal/metrics              Prometheus text format
internal/jx                   JSON editing and GUID remapping helpers
internal/clone                one file per area; components.go has the run order
internal/verify               counts and work item field sample
internal/report               go-echarts HTML report
scripts/                      preflight, standalone git and wiki mirrors
.github/workflows, azure-pipelines.yml
deploy/k8s, deploy/grafana, Dockerfile
docs/runbook.md, docs/diagrams.md
```

`go.sum` was generated from the go-echarts v2.3.3 tag on GitHub because the Go
checksum database wasn't reachable when it was built. To have it verified,
delete it and run `go mod tidy`.
