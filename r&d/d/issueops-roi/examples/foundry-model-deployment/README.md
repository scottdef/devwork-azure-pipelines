# Example: Microsoft Foundry model deployment

## 1. Dev deployment, auto-approved ([transcript](output/scenario/transcript.md))

`lea-ml` deploys `gpt-4.1-mini` (2025-04-14, GlobalStandard, 10K TPM) as `gpt-4.1-mini-ml-eval` on `aif-cooleng-dev-eus2`. The request matches `auto_approve` (dev, standard SKU, capacity ≤ 10), so `.submit` approves it and execution runs in `foundry-dev`:

- [parameters.json](output/scenario/execution/parameters.json): ARM parameters for [deploy/bicep/model-deployment.bicep](../../deploy/bicep/model-deployment.bicep)
- [preflight.json](output/scenario/execution/preflight.json): model and SKU offered, quota `OpenAI.GlobalStandard.gpt-4.1-mini` has 1,960 available, create mode
- The completion comment has the endpoint and a keyless usage snippet

## 2. Production PTU with three approvals, quota failure and `.retry` ([transcript](output/scenario-prod-ptu/transcript.md))

A 100-PTU `GlobalProvisionedManaged` `gpt-4.1` deployment for the agent runner on `aif-cooleng-prod-eus2`.

**Policy:**

- base rule: platform-ai
- **prod** escalation: ai-governance **or** the `security_manager` organization role
- **PTU** escalation: FinOps

Execution runs in `foundry-prod`, where platform-ai are required reviewers.

1. `.submit`, then `priya-ai` approves (platform-ai). The requestor runs `.status`. `sec-sofia` approves through her **org role**, then `fran-finops` approves, and the request is approved.
2. The first execution fails the pre-flight: `OpenAI.GlobalProvisionedManaged` has 50 PTU available and 100 are requested. Nothing is deployed, and the failure comment explains why.
3. The requestor's `.retry` is refused (an approver command). After the quota increase, `tom-ai` comments `.retry`. The gate re-verifies the **unchanged** digest and the recomputed approvals, and the deployment succeeds.

## Fixtures

| File | Stands in for |
|---|---|
| [az-list-models.json](fixtures/az-list-models.json) | `az cognitiveservices account list-models -n <account> -g <rg> -o json` |
| [az-usage-dev.json](fixtures/az-usage-dev.json), [az-usage-prod-before.json](fixtures/az-usage-prod-before.json), [az-usage-prod-after.json](fixtures/az-usage-prod-after.json) | `az cognitiveservices usage list -l eastus2 -o json` |
| [az-deployments-dev.json](fixtures/az-deployments-dev.json), [az-deployments-prod.json](fixtures/az-deployments-prod.json) | `az cognitiveservices account deployment list -o json` |
