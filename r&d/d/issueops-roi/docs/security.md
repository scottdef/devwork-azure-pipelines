# Security and threat model

The platform turns issue comments into privileged actions: billing changes, cloud deployments, and agent runs with repository write. The controls below map each threat to where it is mitigated in this implementation.

## Threats and controls

| # | Threat | Control | Where |
|---|---|---|---|
| T1 | **Spoofed state.** A user posts `<!-- issueops:v1:approved … -->` | Markers are trusted only from the App bot login and only on the first line. Template output escapes `<` and `>`. The marker JSON is `encoding/json`, which escapes `<`, `>` and `&` | `state.Build`, `tmpl.Safe`, `Marker.Format` |
| T2 | **Approve-then-change (TOCTOU).** The body or answers are edited after approval | SHA-256 digest of the normalized body plus answers. Approvals only count for the submitted digest. The gate and execution jobs re-hash the fetched content (`--expect-digest`). The execution job parses the same body it hashes | `state.Digest`, `engine.Gate`, `verified()` |
| T3 | **Edited approval.** An old comment is edited into `.approve` | Decision commands edited more than 5 s after creation are ignored | `state.Build` |
| T4 | **Self-approval / collusion.** | `allow_self_approval: false`; distinct approvers across rules; requestor opt-in only for explicit beneficiary rules | `state.EvaluateApprovals` |
| T5 | **Stale eligibility.** An approver leaves the team | Eligibility is recomputed at the gate from live team, role and permission data | `engine.Gate` |
| T6 | **Script injection** through issue text in `run:` | Issue and comment text never appear in `${{ }}` inside `run:`; only in `with:` and `env:`. Outputs use random heredoc delimiters. actionlint runs in CI | workflows, `gha.SetOutput` |
| T7 | **Mention spam / phishing** through quoted text | `safe` breaks `@` mentions and escapes HTML. Only approver teams from policy are mentioned | `tmpl` |
| T8 | **Over-privileged tokens** | Default `GITHUB_TOKEN` is read-only. App tokens are minted per job, narrowed per workflow, expire in 1 h, and are revoked post-job. Enterprise PAT and Copilot user token are environment secrets | `setup-issueops`, environments |
| T9 | **Cloud credential theft** | No Azure secrets: OIDC federated credentials per environment (`repo:CoolEngOrg/issueops:environment:<env>`), roles scoped to one RG, namespace or registry. Production environments require reviewers and `main`-only branches | `setup-oidc.sh` |
| T10 | **Workflow tampering** (a PR changes policy or workflows) | `issues` and `issue_comment` run workflow files from the default branch. Ruleset on `main`: CODEOWNERS review, plus a required-reviewers rule so policy changes need platform-admins, AI governance **and** FinOps (CODEOWNERS alone accepts any one owner). No `pull_request_target` | CODEOWNERS, ruleset |
| T11 | **Supply chain** | Actions pinned to commit SHAs (Dependabot updates them). Go standard library only plus go-echarts (enforced in CI). Custom validators use Node built-ins only. Distroless runner image with SBOM and provenance | CI, Dockerfile |
| T12 | **Prompt injection** into agents | The agent spec contains validated fields and answers only. The system prompt treats spec text as data. Secrets are refused. Agent follow-up questions go through Q&A and re-approval. The runner has no GitHub credentials. Copilot agent output is a draft PR reviewed by humans | `agent.BuildSpec`, prompts |
| T13 | **Data exfiltration by agents** | Classification ceilings per backend. Confidential data only goes to `confidential_deployments`. NetworkPolicy allows DNS and 443 only; add FQDN egress control at the firewall. PSA `restricted`, read-only root filesystem, no service-account token | registry, `deploy/aks` |
| T14 | **Runaway cost** | Budget ceiling and FinOps escalation. Capacity caps per model and environment. PTU requires FinOps. Agent iteration and time bounds. Namespace quota | registry, runner |
| T15 | **Double execution** | Per-issue concurrency on gate jobs. The `executing` marker blocks re-entry unless `force` is set through `workflow_dispatch` | execute workflows |
| T16 | **Denial of service by comment flood** | The router ignores non-commands and bots. Each handler is idempotent (timeline rebuild) | router |
| T17 | **Leaking per-user data** | Per-user report detail requires a Copilot-admin approval. Artifacts expire after 30 days. Metrics pushed to Grafana are aggregate (team or scope) only | policy, report |
| T18 | **Pushgateway abuse** | Internal ingress with basic auth; never public | `deploy/pushgateway` |

## Residual risks and recommendations

- **GitHub Agentic Workflows and the Copilot cloud agent are evolving products.** Review their permissions, safe outputs and firewall settings on every catalog change. Keep target repositories protected with rulesets and required reviews.
- **Enterprise PAT.** The enterprise budgets endpoints need a classic PAT. Keep it on a dedicated machine account, scoped to billing, in the `copilot-budgets-enterprise` environment with required reviewers. Rotate it quarterly.
- **Search-based output metrics** are limited by search API caps (1,000 results per query). The report notes when a cap is hit. Prefer shorter periods or repositories scope for very large orgs.
- **Model output is untrusted.** Deliverables are posted in `mdsafe` form with an AI-generated notice. Treat commands in runbooks as proposals until reviewed.
- **Static analysis.** Consider adding `zizmor` and CodeQL for GitHub Actions to CI (see the [roadmap](roadmap.md)).
