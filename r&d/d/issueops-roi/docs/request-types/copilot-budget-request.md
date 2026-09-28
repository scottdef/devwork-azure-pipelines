# A. Copilot AI budget request (`copilot-budget-request`)

This request creates or raises a **GitHub Copilot AI-credit budget**. Since usage-based billing (June 2026), Copilot consumption is metered in AI credits. Budgets cap or monitor that spend per organization, repository, user, all users, or enterprise cost center. The platform manages them through the billing budgets REST API.

- Form: [.github/ISSUE_TEMPLATE/copilot-budget-request.yml](../../.github/ISSUE_TEMPLATE/copilot-budget-request.yml)
- Handler: [internal/budget](../../internal/budget/budget.go)
- Workflow: [execute-copilot-budget.yml](../../.github/workflows/execute-copilot-budget.yml)
- Examples: [examples/copilot-budget-request](../../examples/copilot-budget-request/)

## Form fields

| Field id | Label | Notes |
|---|---|---|
| `budget_scope` | Budget scope | `organization`, `repository`, `user`, `all-users`, `cost-center` |
| `budget_target` | Scope target | Blank for organization or all-users. `repo` or `CoolEngOrg/repo`. `@login`. Cost-center name |
| `budget_product` | Metered product | `AI credits` (SKU `ai_credits`) or `Premium requests (legacy)` (`premium_requests`) |
| `budget_amount` | Requested monthly budget (USD) | Whole dollars; `$` and `,` are accepted |
| `budget_enforcement` | Enforcement | *Stop usage when the budget is reached* (`prevent_further_usage: true`) or *Alert only* |
| `budget_alert_recipients` | Alert recipients | GitHub logins. Defaults to `default_alert_recipients` |
| `budget_expires_on` | Budget expiry date | `YYYY-MM-DD`; user scope only |
| `budget_cost_center` | Chargeback cost center | `CC-nnnn[-NAME]` |
| `budget_justification` | Business justification | Shown to approvers |
| `budget_acknowledgements` | Acknowledgements | Both boxes required |

## Scope mapping

| Form scope | API endpoint | `budget_scope` | `budget_entity_name` | Extra |
|---|---|---|---|---|
| organization | `/organizations/CoolEngOrg/settings/billing/budgets` | `organization` | `""` (the API echoes the org) | |
| repository | same | `repository` | `CoolEngOrg/<repo>` | |
| user | same | `user` | `""` | `user: <login>` is required (400 otherwise). Must stop usage. No alerts |
| all-users | same | `multi_user_customer` | `""` | Must stop usage. No alerts |
| cost-center | `/enterprises/CoolEngEnt/settings/billing/budgets` | `cost_center` | cost-center name | Classic PAT `ENTERPRISE_BILLING_PAT`. Disabled unless `enterprise_scopes_enabled` |

All budgets use `budget_type: BundlePricing` with the product SKU above. The API version is sent as `X-GitHub-Api-Version: 2026-03-10`.

## Validation

Validation is layered:

- `issue-ops/validator` checks the form.
- Custom validators check the amount, dates, logins, cost center and secrets.
- `budget.BuildPlan` enforces:
  - the scope and target combination
  - amount ≤ `max_amount_usd` (10,000)
  - enforcement rules for user and all-users budgets
  - no alert recipients on user or all-users budgets
  - expiry only for user scope, and in the future
  - enterprise scopes enabled
- With the API available at intake, it also checks that the repository exists, that the user is an org member, and looks up the **current budget**. The summary shows the change, e.g. `$800.00 (change +$1,700.00)`.

## Approval policy (default)

| Rule | When | Who |
|---|---|---|
| Copilot administrators | always | `@CoolEngOrg/copilot-admins` |
| FinOps approvers | `budget_amount > 1000` → environment `copilot-budgets-high` | `@CoolEngOrg/finops-approvers` |
| Enterprise billing owners | `budget_scope == cost-center` → environment `copilot-budgets-enterprise` | `coolengent-billing-owner` |
| Budget beneficiary | `budget_scope == user` | The user named in `budget_target`, or the requestor. This explicit opt-in is the only way a requestor-related person counts |

Approvers are distinct: one person satisfies at most one rule. The requestor never counts toward admin or FinOps rules.

## Execution

1. **Gate:** re-verifies the digest-bound approval and posts `executing`.
2. **Apply**, in the environment chosen by policy (`copilot-budgets`, `-high` or `-enterprise`):
   - `issueops budget plan` writes `plan.json` with the create and update payloads.
   - `issueops budget apply` lists budgets and finds one with the same scope, entity, user and SKU. It then:
     - **updates** it (`PATCH …/budgets/{id}`), or
     - **creates** one (`POST …/budgets`), or
     - reports **unchanged**.
3. **Report:** comments the before and after amounts and the budget ID, then closes the issue as completed.

The App token for the apply job is narrowed to `organization-administration: write` and `issues: read`.

Example payload (from the [FinOps escalation scenario](../../examples/copilot-budget-request/output/scenario/execution/budget-plan.json)):

```json
{
  "budget_amount": 2500,
  "prevent_further_usage": true,
  "budget_alerting": { "will_alert": true, "alert_recipients": ["dana-dev", "gia-pay"] },
  "budget_scope": "repository",
  "budget_entity_name": "CoolEngOrg/payments-api",
  "budget_type": "BundlePricing",
  "budget_product_sku": "ai_credits"
}
```

## Settings (`config/issueops.json`)

| Key | Default | Meaning |
|---|---|---|
| `max_amount_usd` | 10000 | Self-service ceiling. Larger requests go to FinOps outside the platform |
| `enterprise_scopes_enabled` | false | Allows cost-center budgets (requires `ENTERPRISE_BILLING_PAT`) |
| `products` | AI credits, premium requests | Form label → SKU and budget type |
| `default_alert_recipients` | `copilot-billing-bot` | Used when the form leaves recipients blank (org, repo, cost center) |

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| `403` on list or create | The App lacks organization **Administration: write**, or the installation token wasn't minted with it |
| `400 user is required` | User scope without a target login (validation normally prevents this) |
| Enterprise scope: `401`/`403` | App and fine-grained tokens are rejected. Use the classic PAT of an enterprise billing manager |
| Budget created but usage is still blocked | Enforcement blocks at the budget amount. Raise the amount, or choose *Alert only* for monitoring |
| Licensed products | Budgets on license-based products only monitor. `prevent_further_usage` applies to metered AI credits and premium requests |
