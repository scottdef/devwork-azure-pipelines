# Example: Copilot AI budget request

## 1. Raise a repository budget with FinOps escalation ([transcript](output/scenario/transcript.md))

`dana-dev` (an engineering manager) asks to raise the `payments-api` AI-credit budget from $800 to **$2,500/month**:

1. **Intake:** valid. The amount is over $1,000, so the *FinOps sign-off* escalation adds a rule and moves execution to `copilot-budgets-high`. The request is labeled `issueops:escalated`.
2. `.submit` records the digest and mentions `@CoolEngOrg/copilot-admins` and `@CoolEngOrg/finops-approvers`.
3. `dana-dev` tries `.approve` and is **rejected**: requestors cannot approve their own request.
4. `alex-admin` approves (1 of 2 rules), then `fran-finops` approves, and the request is **approved**.
5. **Execution:** the gate re-verifies. `budget plan` shows the [payload](output/scenario/execution/budget-plan.json). `apply` finds the existing budget and **updates** it (PATCH). The completion comment shows $800.00 → $2,500.00, and the issue closes.

The issue as submitted: [issue-body.md](output/scenario/issue-body.md). As parsed: [parsed.json](output/scenario/parsed.json).

## 2. Temporary user budget: invalid → fixed, stale approval ([transcript](output/scenario-user-budget/transcript.md))

1. A user-scope budget with alert recipients is **invalid**: GitHub disables alerts for user budgets. `.submit` is refused.
2. The requestor edits the issue. The summary comment is updated in place, and the request is now valid.
3. `.submit`, then a Copilot admin approves.
4. The requestor **edits the amount**. The digest changes, so the submission and approval are void. The beneficiary's `.approve` is refused with an explanation.
5. `.submit` again. The **beneficiary** (`sam-contract`, named in the form) confirms and a Copilot admin approves. The budget is **created** with `user` and `expires_at`.

## Try variations

Edit `scenario.json` and run `bin/issueops simulate --scenario examples/copilot-budget-request/scenario.json --out /tmp/b`:

- `"budget_scope": ["cost-center"]`: invalid unless `enterprise_scopes_enabled`. With it enabled, the enterprise-owner escalation applies and the environment becomes `copilot-budgets-enterprise`.
- `"budget_amount": "25000"`: rejected above the $10,000 self-service ceiling.
