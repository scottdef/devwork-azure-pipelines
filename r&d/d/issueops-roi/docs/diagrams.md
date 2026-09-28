# Diagrams

GitHub renders these Mermaid diagrams inline. The platform-wide component, state-machine and sequence diagrams are in [architecture.md](architecture.md).

## Workflow and job topology

```mermaid
flowchart LR
  subgraph T["Triggers"]
    O["issues: opened · edited · reopened"]
    C["issue_comment: created"]
    D["workflow_dispatch (issue, force)"]
    S["schedule (hourly)"]
  end
  O --> IO["issue-opened.yml<br/>intake job"]
  C --> CR["comment-router.yml<br/>handle job"]
  CR -- "execute=true · type" --> EB["execute-copilot-budget.yml"]
  CR --> ER["execute-copilot-report.yml"]
  CR --> EF["execute-foundry-deploy.yml"]
  CR --> EA["execute-agentic-task.yml"]
  D --> EB & ER & EF & EA
  S --> OM["ops-metrics.yml"]
  subgraph X["every execute-*.yml"]
    G["gate<br/>(concurrency per issue)"] --> E["execute<br/>environment · OIDC · --expect-digest"] --> R["report<br/>finish + apply-decision"]
  end
```

## Composite actions used by every job

```mermaid
flowchart TB
  SI["setup-issueops<br/>create-github-app-token (narrowed) · setup-go · build CLI"] --> CLI["issueops &lt;command&gt;<br/>decision.json · comment.md · step outputs"]
  CLI --> AD["apply-decision<br/>issue-ops/labeler (remove, add) · find-comment · create-or-update-comment · close"]
  CLI -. gate .-> GA["gate<br/>issueops gate · executing marker · refusal comment"]
  CLI -. finish .-> FA["finish<br/>issueops finish · apply-decision"]
```

## Digest binding

```mermaid
sequenceDiagram
  actor R as Requestor
  actor A as Approver
  participant E as Engine (timeline rebuild)
  R->>E: .submit  (body B1 → digest D1)
  E-->>R: submitted {digest: D1}
  A->>E: .approve
  E-->>A: counts (vote after submission for D1)
  R->>E: edits body (B2 → D2)
  E-->>R: summary: "changed after it was submitted" — approvals void
  A->>E: .approve
  E-->>A: rejected: no submission for current content
  R->>E: .submit (D2)
  A->>E: .approve
  E-->>A: approved {digest: D2} → execute
  Note over E: gate and execution jobs refuse unless fetched content hashes to D2
```

## Copilot budget execution

```mermaid
flowchart LR
  P["budget plan<br/>scope → API scope/entity/SKU"] --> L["GET …/settings/billing/budgets"]
  L --> M{"matching budget?<br/>scope · entity · user · SKU"}
  M -- no --> C["POST …/budgets"]
  M -- "yes, different" --> U["PATCH …/budgets/{id}"]
  M -- "yes, same" --> N["unchanged"]
  C & U & N --> F["finish: before → after, budget id"]
  subgraph tokens
    OT["org scopes: App token<br/>organization administration: write"]
    ET["cost-center: ENTERPRISE_BILLING_PAT<br/>(classic PAT, env-scoped)"]
  end
```

## Copilot ROI report data join

```mermaid
flowchart LR
  U["users-1-day NDJSON<br/>per user · day"] --> J{{"join on user_id + day"}}
  T["user-teams-1-day NDJSON"] --> J
  J --> AG["aggregate<br/>active · engaged · agentic · credits · LOC"]
  RP["repos-1-day NDJSON"] --> RR["repo rows<br/>Copilot-authored PRs"]
  S["search: merged PRs · reviews"] --> OUT["output<br/>merged PRs · cycle time · reviews"]
  ST["commits · code_frequency"] --> RR
  SE["copilot/billing seats"] --> COST["cost<br/>credits × price + seats × monthly × days/30"]
  AG --> COST
  AG & OUT & COST & RR --> REP["report.json"]
  REP --> H["copilot-roi.html (go-echarts)"] & CSV["CSV"] & PR["metrics.prom → Pushgateway → Grafana"] & CM["completion comment (Go template)"]
```

## Foundry deployment and identities

```mermaid
flowchart TB
  subgraph GH["GitHub · environment foundry-prod (reviewers)"]
    J["deploy job<br/>id-token: write"]
  end
  J -- "OIDC token<br/>sub = repo:CoolEngOrg/issueops:environment:foundry-prod" --> FC["Entra app issueops-gh-foundry-prod<br/>federated credential"]
  FC --> RG["rg-cooleng-ai-prod<br/>Cognitive Services Contributor"]
  FC --> SUB["subscription<br/>IssueOps Foundry Quota Reader"]
  J --> PF["pre-flight<br/>list-models · usage list · deployment list"] --> WI["what-if"] --> BI["az deployment group create<br/>model-deployment.bicep"]
  BI --> ACC["aif-cooleng-prod-eus2<br/>accounts/deployments"]
  subgraph AKS["AKS · issueops-agents"]
    SA["ServiceAccount issueops-agent-runner"] -- "projected token" --> MI["managed identity id-issueops-agent-runner<br/>Cognitive Services OpenAI User"]
  end
  MI --> ACC
```

## Agentic task: Q&A loop and backends

```mermaid
flowchart TB
  O["opened"] --> Q["summary: questions Q1…Qn<br/>awaiting-answers"]
  Q --> A[".answers by requestor<br/>validated: choices · patterns · lengths · secrets"]
  A --> C{"complete?"}
  C -- no --> Q
  C -- yes --> V["validated + spec preview"] --> SUB[".submit (digest = body + answers)"] --> AP["approvals<br/>target maintainer (+ platform-ai, + ai-governance)"]
  AP --> B{"backend"}
  B -- copilot-cloud-agent --> CA["issue in target repo<br/>assigned to copilot-swe-agent[bot] → draft PR"]
  B -- agentic-workflow-catalog --> WF["workflow_dispatch *.lock.yml<br/>inputs from answers"]
  B -- aks-foundry-agent --> K["ConfigMap + Job (Go templates)<br/>kubectl 1.30"] --> RUN["agent-runner<br/>draft → critique → revise"]
  RUN -- needs_input --> DQ["follow-up questions Q(n+1)…"] --> A
  RUN -- completed --> DEL["deliverable comment"]
  CA & WF --> DONE["completion comment"]
```
