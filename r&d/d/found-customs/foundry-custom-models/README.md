# foundry-custom-models

Deploy **custom-weight open models** (your fine-tunes, adapters, or private
Hugging Face repositories) to **Azure ML managed online endpoints running
vLLM**. Then use them from **Foundry Agent Service**, **Microsoft Agent
Framework**, **Copilot CLI** and **GitHub agentic workflows**. Every change runs
through GitHub Actions, triggered by manual dispatch or IssueOps.

Three worked examples:

| | Base weights | Customisation | Used by | Trigger |
|---|---|---|---|---|
| [01 Gemma 4 agent](examples/01-gemma4-agent/) | `google/gemma-4-E4B-it` | LoRA, merged | Foundry prompt agent (BYOM) | dispatch |
| [02 Qwen3.8 harness](examples/02-qwen38-harness/) | `Qwen/Qwen3.8-27B` | LoRA adapter served beside base | Agent Framework agents, workflow, eval | dispatch + IssueOps |
| [03 Muse](examples/03-muse-glimmer/) | `meta-models/Muse-Glimmer-30B` (+ `microsoft/wham`) | your own HF repository | Copilot CLI BYOK, gh-aw | dispatch (+ IssueOps) |

Qwen3.7 has no open weights (it is API-only), so example 2 uses Qwen3.8-27B,
the newest Apache-2.0 Qwen. "Muse" means two models; example 3 covers both.
**[GUIDE.md](GUIDE.md)** explains why, and everything else.

## Quick start

```sh
make check                              # ruff, shellcheck, actionlint, 56 tests; no Azure needed
FTM_SET_HF_REVISION=$(shared/bin/ftm resolve gemma4) shared/bin/ftm plan gemma4

scripts/bootstrap-azure.sh              # prints the Azure + GitHub setup
scripts/bootstrap-azure.sh --apply      # does it (workspace, clusters, OIDC identity, roles, environments)
scripts/preflight.sh

gh workflow run ex1-gemma4.yml -f action=plan   -f reason="first plan"
gh workflow run ex1-gemma4.yml -f action=deploy -f reason="dry run"              # dry_run defaults to true
gh workflow run ex1-gemma4.yml -f action=deploy -f dry_run=false -f reason="first deploy"
```

## How it works

```
Hugging Face ─fetch job (in Azure)─▶ <model>-base:<rev>
     ─LoRA job (merge | adapter)─▶ <model>:<content hash>
     ─▶ deployment v<hash> at 0% ─smoke test (pinned header)─▶ 100% (previous kept for rollback)
     ─▶ Foundry ModelGateway connection ─▶ agents use "<connection>/<model>"
```

* **One driver:** [`shared/bin/ftm`](shared/bin/ftm) (`plan resolve infra fetch train deploy smoke promote
  rollback connect info record destroy all`). Workflows and people run the same commands.
  `FTM_DRY_RUN=1` prints every mutating `az` call.
* **One config per example:** `model.env`, validated by [`ftmcfg.py`](shared/tools/ftmcfg.py),
  which also derives content-hash versions and renders the Azure ML YAML. Nothing is random, so a
  plan names exact bytes and an unchanged re-run deploys nothing.
* **One reusable workflow:** [`ftm-pipeline.yml`](.github/workflows/ftm-pipeline.yml). The plan job
  never touches Azure; the apply job runs in `ftm-plan` (dry runs) or `ftm-prod` (reviewers) with OIDC.
  Every apply commits a signed-off record under `records/`.
* **One serving image:** `vllm/vllm-openai:v0.30.0` plus a short launcher that finds merged weights
  or base + adapters under the Azure ML mount.

## Layout

```
GUIDE.md                           the guide
shared/bin/ftm                     the driver (bash)
shared/tools/                      ftmcfg (config, versions, render), issueform, smoke_test, connection_body
shared/train/                      fetch.py, sft_lora.py, sftdata.py (run inside Azure ML jobs)
shared/containers/{serve,train,wham}/   Dockerfiles built by Azure ML
shared/templates/                  Azure ML YAML templates (@{KEY} placeholders)
shared/config/site.env             resource group, workspace, Foundry account/project
examples/0N-*/                     model.env, data, consumer code, README
.github/workflows/                 ftm-pipeline (reusable), ex1/ex2/ex3 (dispatch), issueops, ci
.github/ISSUE_TEMPLATE/            model-request form
scripts/                           bootstrap-azure.sh, preflight.sh
tests/                             pytest: config, IssueOps, data, endpoints, harness, ftm CLI
records/                           committed by the pipeline, one JSON per apply
```

## Assumptions

Enterprise `CoolGitEnterprise`, organisation `CoolGitOrg`, repository
`CoolGitOrg/foundry-custom-models`, resource group `rg-coolgit-copilot` in
`eastus2`, Foundry account `ais-coolgit-copilot-prod` with project
`proj-coolgit-agents`, Azure ML workspace `mlw-coolgit-foundry`. Optionally,
the GitHub App `issueops-autoadmin-app` makes the record commits. Change
`shared/config/site.env`, or set the repository variables of the same names.

## Verification

Tested without Azure: 56 tests (config and rendering; IssueOps parsing and
injection cases; SFT masking; smoke test, Agent Framework harness and workflow
against a local OpenAI-compatible fake; the WHAM shim; the whole `ftm all`
sequence in dry run against a stub `az`); ruff, shellcheck and actionlint are
clean. **Not run against a live subscription or GPU.** GUIDE.md §10 lists
exactly what that leaves unverified.
