# Go templates

Every human-facing message and every generated artifact that isn't a JSON payload is a Go `text/template`. That covers issue comments, Copilot agent issue bodies and custom instructions, agent prompts, and Kubernetes manifests. Templates are embedded in the binary from [internal/tmpl/templates](../internal/tmpl/templates/), so a deployment works with no extra files. An operator can override any template without rebuilding by pointing `ISSUEOPS_TEMPLATES` (or `--templates`) at a directory of same-named files. That's useful for re-branding comments, translating them, or adding a link to an internal wiki.

JSON payloads (budget create and update, ARM parameters, workflow inputs, agent spec) are produced with `encoding/json`, never with templates.

## Catalog

| Template | Rendered when | Data |
|---|---|---|
| `comment.summary.md.tmpl` | Intake: valid request (or answers needed) | `View` |
| `comment.invalid.md.tmpl` | Intake: validation errors | `View` |
| `comment.submitted.md.tmpl` | `.submit` with approvals required | `View` (+ `Result` spec preview for agentic) |
| `comment.approval-status.md.tmpl` | An approval recorded, quorum not met (updated in place) | `View` + `Approval` |
| `comment.approved.md.tmpl` | Quorum met, auto-approval, or `.retry` | `View` + `Approval` |
| `comment.denied.md.tmpl`, `comment.cancelled.md.tmpl` | `.deny`, `.cancel` | `View` + `Actor`, `Note` |
| `comment.rejected.md.tmpl` | A command not applied (why, and who can) | `View` + `Reason`, `Command` |
| `comment.status.md.tmpl`, `comment.help.md.tmpl` | `.status`, `.help` | `View` |
| `comment.qa-status.md.tmpl` | `.answers` (updated in place) | `View` + `QA` (+ spec preview) |
| `comment.executing.md.tmpl` | Gate passed | `View` |
| `comment.completed.<type>.md.tmpl` | Execution completed (one per request type) | `View` + `Result` (type outcome) |
| `comment.failed.md.tmpl` | Execution failed | `View` + `Reason` |
| `comment.agent-followup.md.tmpl` | The agent needs input (dynamic questions) | `View` + `Result{agent, questions}` |
| `foundry.usage.md.tmpl` | Partial: how to call a new deployment | plan and endpoint |
| `agent.system-prompt.md.tmpl`, `agent.task-prompt.md.tmpl` | AKS agent prompts | `agent.Spec` |
| `agent.copilot-issue.md.tmpl`, `agent.copilot-instructions.md.tmpl` | Copilot cloud agent issue body and custom instructions | `agent.Spec` |
| `k8s.agent-configmap.yaml.tmpl`, `k8s.agent-job.yaml.tmpl` | AKS runner manifests | `agent.JobData` |
| `_partials.tmpl` | Shared blocks: `request-table`, `messages`, `rules`, `approval-progress`, `questions`, `answer-howto`, `spec`, `footer` | |

## The `View` model (comments)

```go
type View struct {
    Org, Repo, DocsURL, RunURL string
    TypeID, TypeName, TypeDesc string
    Issue int; IssueURL, Requestor, Title string
    Rows []request.Row            // summary table rows from the type handler
    Errors, Warnings []string
    Plan *policy.Plan             // rules, escalations, environment, auto-approval
    Rules []RuleView              // {Name, Min, Who}
    Mentions []string             // @CoolEngOrg/team handles to notify
    AutoApproved bool
    Approval *state.ApprovalResult // per-rule progress, ignored votes, denial
    Digest, Actor, Command, Note, Phase, Reason string
    QA *qa.Status                 // questions, answers, missing, invalid
    Info map[string]any           // handler details (e.g. current budget)
    Result any                    // type-specific outcome for completed/failed
    Stale bool
}
```

Every bot comment is `marker line + rendered template`. The engine writes the marker, so templates cannot forge state.

## Functions

| Function | Use |
|---|---|
| `safe` | **All user-supplied text in comments.** Escapes `& < >` (no raw HTML, no fake `<!-- issueops:… -->` markers) and breaks `@` mentions with a zero-width joiner |
| `mdsafe` | AI-generated Markdown (keeps code blocks; neutralizes HTML comments and mentions) |
| `cell` | Markdown table cells (`safe`, escapes `\|`, newlines → `<br>`) |
| `code`, `fence LANG` | Inline code or fenced block with a fence longer than any backtick run in the content |
| `yamlString` | Any value in YAML: JSON-quoted scalar, which cannot break out of the document structure |
| `mention` | Deliberate mentions (requestor, approver teams) |
| `usd`, `num`, `pct`, `hours` | Formatting (`$1,293.03`, `89,263.3`, `40%`, `35.0 h`) |
| `json`, `jsonIndent`, `dict`, `list`, `default`, `trunc`, `indent`, `nindent`, `lines`, `join`, `plural`, `short`, `dnsLabel`, `deref`, `isNil`, `date`, `add`, `sub`, `lower`, `upper`, `trim` | Helpers |

## Overriding a template

```bash
mkdir -p ops/templates
bin/issueops render --list                       # names
cp internal/tmpl/templates/comment.help.md.tmpl ops/templates/
$EDITOR ops/templates/comment.help.md.tmpl
bin/issueops render --template comment.help.md.tmpl --data view.json --templates ops/templates
```

In workflows, set `ISSUEOPS_TEMPLATES: ${{ github.workspace }}/ops/templates` in the job env. Overrides are parsed at start-up, and a broken override fails fast. `go test ./internal/tmpl` checks that every embedded template parses. The example transcripts show the rendered output of nearly every template.

## Writing a new template safely

- Wrap user or form text with `safe` or `cell`. Wrap model output with `mdsafe`. Put values in YAML only through `yamlString`.
- Never build JSON in a template. Put a struct in the data and let `encoding/json` do it.
- Keep marker comments out of templates; the engine adds them.
- Add or extend a scenario so the output appears in `examples/*/output/*/transcript.md`, then review the diff.
