# Architecture

## The shape

`foundry-tui` is a filter with a screen on it. It starts other programs
(`az`, `gh`, `copilot`), reads JSON from them, and writes JSON, JSONL and HTML.
It holds no state that is not a file, and no file that is not text.

| Concern | Mechanism | Package |
|---|---|---|
| Read Foundry | `az … -o json`, `az rest` for Resource Health and alerts | `internal/azure` |
| Read GitHub | `net/http` against the REST API | `internal/github` |
| Write anything | `gh workflow run FILE --json`, inputs on stdin | `internal/dispatch` |
| Decide what may be read | Foundry Owner role definition, wildcard matcher | `internal/perms` |
| Probe a model | HTTPS with an Entra token; `copilot -p` in BYOK mode | `internal/probe` |
| Remember what was done | hash-chained JSONL | `internal/audit` |
| Keep a record | `html/template` → HTML, plus JSON and SHA-256 | `internal/report` |
| Start a process | one function | `internal/run` |

### Why az for reads and Go for GitHub

`az` already owns login, token caches, clouds, proxies and conditional access.
Re-implementing that with the Azure SDK would add some forty modules to gain
nothing a Foundry owner can see. `az` prints JSON; Go parses JSON. For GitHub the
balance is the other way: the REST API is three GETs, and `net/http` does them
without starting a process per page. The token still comes from `gh` when the
environment has none, so there is one login to manage.

### Why gh for writes

The brief says so, and it is right. `gh workflow run --json` reads the inputs
object from stdin, so no user-typed text is ever placed on a command line. The
command that is sent is the command that is shown, and you can paste it into a
shell to get the same result. A Go HTTP POST would work; it could not be
inspected or replayed by the person approving it.

## The write path, step by step

1. A form (or `-set k=v`) yields `map[string]string`.
2. `Spec.Validate` checks each value against an anchored regular expression
   whose first character must be alphanumeric, so no value can pose as a flag,
   and rejects parameters the spec does not name. `dry_run` defaults to `"true"`. A `request_id`
   of the form `ftui-<UTC timestamp>-<6 hex>` is generated.
3. `templates/dispatch/<kind>.json.tmpl` is executed. Every value goes through
   the `json` template function, so it arrives as a correctly escaped JSON
   string whatever it contains.
4. The output is parsed again as `map[string]string`. If it is not a flat
   object of strings, or has more than ten keys, the request is refused.
   (github.com raised the limit to 25 inputs in December 2025; older GitHub
   Enterprise Server versions still enforce 10, so the templates stay inside it.)
5. The payload, its SHA-256 and the exact `gh` command are shown. Nothing has
   been sent.
6. On Send, `dispatch.request` is appended to the audit chain **first**, then
   `gh` runs, then `dispatch.sent` or `dispatch.error` is appended. An intent
   with no outcome is visible evidence of a crash or a kill.
7. The workflow puts `request_id` in its `run-name`. The screen finds the run by
   that string and follows it; the same id names the committed change record.

The workflow repeats the validation in bash. The client check is for the
user's convenience; the server check is the one that counts, because anyone
with `actions: write` can dispatch the workflow without this program.

## The read path and the role

`perms.Load` asks `az role definition list --name c883944f-…` for the live
Foundry Owner definition and falls back to an embedded snapshot if that fails.
`perms.Set.Allows` implements Azure's rule: an action is granted if it matches
an `actions` pattern and no `notActions` pattern (`*` matches across `/`; case
is ignored). `report.Collect` and the screen call `Can(...)` before each
section. A section outside the role is not requested; its absence and the
reason are recorded under `skipped` in the JSON and printed at the foot of the
HTML. If you hold a wider role, the role in force is still Foundry Owner unless
the live definition says otherwise: a record made by one owner should be
reproducible by the next.

## Concurrency

`report.Collect` runs its sections in goroutines and joins them; each writes
to its own field. The screen loads data off the event loop and applies it with
`QueueUpdateDraw`. The audit log serialises appends with a mutex and writes each record
as one `O_APPEND` write of one line, so lines never interleave. Two *processes*
appending in the same instant could both chain to the same predecessor;
`audit -verify` would report the fork. Give each writer its own `state_dir`
(the workflows use `records/state`, you use `~/.local/state/foundry-tui`).

## Templates

All templates are embedded with `go:embed`. Set `template_dir` to a directory
with the same layout to override them without rebuilding: a changed report
style, an extra input, a different probe prompt. HTML goes through
`html/template`, so strings from Azure and GitHub (a run title is attacker
controlled) are escaped by context. The report test feeds
`<script>alert(1)</script>` through as a run title and checks it comes out inert.
Reports carry no scripts and fetch nothing, so they read the same in ten years
from a file share.

## Testing seams

Everything that starts a process takes a `run.Runner`. `run.Fake` answers from a
table keyed by command-line prefix and records every call, so tests assert both
what was parsed and what was *not* asked (`TestMetricsAreNeverRequestedWithoutTheAction`).
GitHub and the model endpoints are `httptest` servers. The screen test runs the
real `tview` application on tcell's `SimulationScreen`.
