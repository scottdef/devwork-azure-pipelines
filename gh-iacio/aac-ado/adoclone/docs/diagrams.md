# adoclone diagrams

GitHub and Azure DevOps both render the Mermaid blocks.

## Architecture

```mermaid
flowchart LR
  subgraph Runner["Runner: GitHub Actions / Azure Pipelines / AKS Job"]
    CLI[adoclone CLI] --> CFG[config: flags, ADO_PAT / ADO_PAT_FILE]
    CLI --> CL[client: retries, Retry-After, rate limits, paging, dry run]
    CLI --> ST[(state.json + journal: ID maps, phases)]
    CLI --> TODO[(todo.json: manual follow-ups)]
    CLI --> CLN[28 components in 9 phases]
    CLN --> GIT[git mirror / import requests]
    CLI --> VER[verify: counts + work item sample] --> REP[report: go-echarts HTML]
    CLI --> MET["/metrics"]
  end
  KV[(Azure Key Vault: ado-pat)] -. CSI + Workload ID .-> CFG
  CL <--> SRC[(CoolADO / source project)]
  CL <--> TGT[(CoolADO / target project)]
  GIT --> TGT
  MET -.-> PROM[Managed Prometheus] --> GRAF[Grafana OSS 12]
```

## Data flow and ID mapping

```mermaid
flowchart TB
  S[List source objects] --> K{already in the ID map?}
  K -->|yes| N[skip: rerun-safe]
  K -->|no| T[strip server-owned fields; remap GUIDs, numeric IDs, project names, area paths]
  T --> R{every reference mapped?}
  R -->|no| D[skip and record in todo.json, or defer to a second pass]
  R -->|yes| P[create in target]
  P --> W[record source->target ID in state.json]
```

Checkpoint shape:

```
state.json
{ "sourceProjectId": "...", "targetProjectId": "...",
  "maps": { "workitem": {"123":"4567"}, "repo": {"<src guid>":"<tgt guid>"}, "repoName": {}, "repoContent": {},
            "team": {}, "teamSubject": {}, "subject": {}, "identity": {}, "identityDescriptor": {},
            "classNode": {}, "classNodeId": {}, "query": {}, "dashboard": {}, "deliveryPlan": {},
            "buildDef": {}, "releaseDef": {}, "releaseEnv": {}, "taskGroup": {}, "queue": {},
            "vargroup": {}, "vargroupShared": {}, "endpoint": {}, "endpointShared": {}, "secureFile": {},
            "environment": {}, "deploymentGroup": {}, "policy": {}, "testPlan": {}, "testSuite": {},
            "testConfig": {}, "testVariable": {}, "suiteCases": {}, "wiki": {}, "wikiContent": {},
            "feed": {}, "feedView": {}, "hook": {}, "wicomments": {}, "wilinks": {} },
  "phases": { "workitems": {"status":"done"} } }
state.json.journal   one {"k","s","t"} line per mapping since the last fold
todo.json            [{"component","item","action"}]
```

## Phase sequence

```mermaid
sequenceDiagram
  participant Op as Operator
  participant T as adoclone
  participant A as Azure DevOps REST
  Op->>T: preflight, plan, clone (dry run)
  T->>A: P1 project (poll operation), groups
  T->>A: P2 nodes, teams, boards
  T->>A: P3 repos (mirror or import)
  T->>A: P4 endpoints (share), vargroups, securefiles, queues, deploymentgroups, envs
  T->>A: P5 taskgroups, pipelines, releases, checks, permissions, policies, settings
  T->>A: P6 workitems: pass 1 create, pass 2 comments, attachments, links
  T->>A: P7 testplans
  T->>A: P8 queries, dashboards, plans, wiki, feeds
  T->>A: P9 security (memberships, ACLs, role assignments), hooks
  T->>Op: exit code, todo.json
  Op->>T: verify (exit 4 on problems), report.html
```

## Exit codes and retries

```mermaid
flowchart LR
  R[adoclone clone] -->|0| OK[done]
  R -->|1 component error| RR[fix and rerun: resumes from state.json; the AKS Job retries]
  R -->|2 bad settings| FIX[fix flags or env; Job fails at once]
  R -->|3 item failures| TD[work through todo.json; Job fails at once]
  V[adoclone verify] -->|4| INV[investigate missing objects or differing work items]
```
