# Runbook — Copilot BYOK on Foundry

## Emergency fallback: Copilot credits exhausted
1. Confirm: `gh api /enterprises/CoolGitEnterprise/settings/billing/ai_credit/usage` (classic PAT, manage_billing:copilot).
2. Is `qwen3-32b` deployed on `prod-eastus2`? `ls deployments/prod-eastus2/`. If not: open "Foundry model: deploy",
   model `qwen3-32b`, account `prod-eastus2`, untick Dry run → `/approve`. ~3 minutes.
3. Is it registered in AI controls? If not, enterprise owner adds it (UI, see README). Enable for CoolGitOrg.
4. Announce in #platform-copilot: "Switch model picker to *CoolGitOrg / qwen3-32b*". Completions are unaffected (free).
5. Watch spend: Azure budget alerts on `rg-coolgit-copilot`; daily cost report workflow in the infra repo.

## Add agentic coding tier (managed compute)
Open deploy issue for `qwen3-coder-next`, **Dry run first**, review the printed PUT body against the current
Hugging Face-on-Foundry docs, then re-approve without dry run. Decommission when idle: GPU-hours bill regardless.

## Rotate the Foundry key
Rotation invalidates the key registered in AI controls. Order: rotate key2 → update AI controls → rotate key1.
`az cognitiveservices account keys regenerate -g rg-coolgit-copilot -n ais-coolgit-copilot-prod --key-name key2`

## Decommission
Remove from AI controls **first** (users get 401 otherwise), then open "Foundry model: decommission".

## Approvers
Team `CoolGitOrg/foundry-approvers`. `/approve` requires membership (via GH_ADMIN_TOKEN) or OWNER/MEMBER fallback.
Production additionally requires the `foundry-prod` environment reviewers.
