# Approval gating

Approval policy is **configuration, not code**. Each request type in [config/issueops.json](../config/issueops.json) declares:

- who may **request** it
- which **approval rules** apply
- which **escalations** add rules (and possibly a stricter GitHub environment) when conditions match
- when a request is **auto-approved**

The engine evaluates this on every event from the issue timeline.

## Building blocks

### Selectors: who matches

A selector matches a user when **any** of its criteria matches.

| Criterion | Example | Matches when | API used |
|---|---|---|---|
| `teams` | `["copilot-admins", "OtherOrg/finops"]` | The user is an active member of the team (org defaults to CoolEngOrg) | `GET /orgs/{org}/teams/{slug}/memberships/{user}` |
| `users` | `["coolengent-billing-owner"]` | Login equals one of them | none |
| `org_roles` | `["owner"]`, `["security_manager"]` | `owner`: organization owner (`role=admin`). Otherwise holds the organization role, directly or through a team | `GET /orgs/{org}/memberships/{user}`, `/orgs/{org}/organization-roles/…` |
| `repo_permissions` | `["admin", "maintain"]` | Permission on the **IssueOps** repository | `GET /repos/{o}/{r}/collaborators/{user}/permission` |
| `target_repo_permissions` | `["maintain", "admin"]` | Permission on the request's **target** repository (from `target_repo_field`) | same, on the target repo |
| `field_users` | `["budget_target"]` | The login named in that form field (for example, the budget beneficiary) | none |
| `requestor` | `true` | The user opened the issue | none |

### Rules: how many

```json
{ "name": "Copilot administrators", "min": 1, "any_of": { "teams": ["copilot-admins"] } }
```

All rules must be satisfied. With `distinct_approvers: true`, each person counts toward **at most one** rule. The engine searches for an assignment, so a person on two teams is placed where needed. With `allow_self_approval: false`, the requestor's vote counts only for rules that explicitly name them with `requestor: true` or `field_users`. Nothing else accepts a self-approval.

### Conditions: when

```json
{ "all": [ { "field": "$environment", "op": "eq", "value": "dev" } ],
  "any": [ { "field": "foundry_sku", "op": "matches", "value": "ProvisionedManaged$" } ] }
```

A condition holds when **all** of `all` and **at least one** of `any` (if present) hold.

**Operators:** `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `not_in`, `contains`, `matches` (RE2), `empty`, `not_empty`.

- Numbers are compared numerically. `$` and `,` are ignored, so `"$2,500"` > 1000.
- Text that isn't a number never satisfies a numeric bound.

**Facts** come from two places:

- **Form fields**, keyed by element id. A single-select dropdown becomes a string; checkboxes become the list of selected labels.
- **Derived facts** prefixed with `$`, computed by the type handler:

| Type | Facts |
|---|---|
| all | `$requestor`, `$type` |
| copilot-budget-request | `$amount_usd`, `$api_scope`, `$owner_kind` |
| foundry-model-deployment | `$environment`, `$provisioned`, `$capacity` |
| agentic-task-request | `$backend`, `$backend_environment`, `$classification` |

### Escalations, auto-approval and environments

```json
"escalations": [
  { "name": "FinOps sign-off above $1,000 per month",
    "when": { "any": [ { "field": "budget_amount", "op": "gt", "value": 1000 } ] },
    "rules": [ { "name": "FinOps approvers", "min": 1, "any_of": { "teams": ["finops-approvers"] } } ],
    "environment": "copilot-budgets-high" }
],
"auto_approve": { "all": [ … ] }
```

- Matching escalations **append** rules. The last matching escalation with an `environment` sets the execution environment. Environment names may use placeholders like `foundry-{environment}` or `{backend_environment}`.
- `auto_approve` applies only when **no escalation matched**. `.submit` then records an automatic approval and execution starts.
- A type with an empty `rules` list and no matching escalation is also auto-approved (Copilot aggregate reports).
- The GitHub **environment** can add a second, native gate: required reviewers who approve the deployment in the Actions UI. Use it for high-risk paths (`copilot-budgets-high`, `foundry-prod`, `agentic-aks`).

### Requestor gating

```json
"requestors": { "any_of": { "teams": ["engineering-managers", "copilot-admins", "platform-admins"] } }
```

If the requestor doesn't match, intake marks the request **invalid** with an explanation, and `.submit` is refused. Requestor selectors use the same criteria as approvers. The agentic type requires `target_repo_permissions: [write, maintain, admin]` on the target repository.

## Gating recipes

| Requirement | Configuration |
|---|---|
| Any member of a team approves | `{"min": 1, "any_of": {"teams": ["copilot-admins"]}}` |
| Two different people from a team | `{"min": 2, "any_of": {"teams": ["platform-ai"]}}` |
| A named person (break-glass or enterprise owner) | `{"any_of": {"users": ["coolengent-billing-owner"]}}` |
| Organization owners only | `{"any_of": {"org_roles": ["owner"]}}` |
| Holders of a custom org role | `{"any_of": {"org_roles": ["security_manager"]}}`. Team-granted roles count |
| Maintainers of the affected repository | `"target_repo_field": "agent_target_repository"` + `{"any_of": {"target_repo_permissions": ["maintain","admin"]}}` |
| The person named in the form confirms | `{"any_of": {"field_users": ["budget_target"], "requestor": true}}` |
| Requestor may self-approve low-risk requests | `"allow_self_approval": true`, or an `auto_approve` condition |
| Only some people may request | `"requestors": {"any_of": {...}}` |
| Stricter path above a threshold | an escalation with `gt` plus an `environment` that has required reviewers |
| Prod needs security or governance | an escalation on `$environment == prod` with `teams` **or** `org_roles` |

## How votes are counted

- A vote is the **latest** `.approve` or `.deny` from each person, cast **after** the most recent submission for the current digest. Votes cast before `.submit`, or before a content change, don't count.
- Edited comments are ignored for `.approve`, `.deny` and `.retry`. The comment must not have been edited more than 5 seconds after creation.
- Comments by bots never count. Bot markers are trusted only from the IssueOps App bot and only on the first line.
- An eligible `.deny` denies immediately. Ineligible votes are listed under "Votes that did not count" with the reason.
- Eligibility is **recomputed at the execution gate**, so an approver removed from the team before execution no longer counts.

## Testing a policy change

1. Change `config/issueops.json`. If you changed field references, run `scripts/forms-to-json.sh`.
2. `make check-forms`: every referenced field must exist in the issue form.
3. Add or adjust a scenario under `examples/<type>/` with `expect` blocks (for example, the expected `environment`, `escalated`, or which `.approve` is rejected). Then run `make test`.
4. The PR needs an approval from each of platform-admins, AI governance and FinOps. CODEOWNERS requests them; the ruleset's required-reviewers rule on `config/issueops.json` enforces all three (see [setup-guide.md](setup-guide.md#1-repository)).
