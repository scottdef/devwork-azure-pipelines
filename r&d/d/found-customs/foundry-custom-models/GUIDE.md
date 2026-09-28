# Custom-Weight Models in Microsoft Foundry: A Deployment Guide

*Gemma 4, Qwen3.8 and Muse, from Hugging Face weights to Foundry agents, agent
harnesses and GitHub workflows. Researched and written 28 September 2026.*

This guide explains the decisions. The repository around it carries them out:
[`shared/bin/ftm`](shared/bin/ftm) plus four workflows and three examples. Read
§1 to §3 to choose a path, §4 to §7 to understand the pipeline, §8 to operate
it. §10 lists what was verified and what was not.

---

## 1. What "custom weights in Foundry" means in 2026

Foundry has four ways to run an open model. Only one of them accepts arbitrary
weights, and it lives in Azure Machine Learning, not in the Foundry resource.

| Path | Your own weights? | Models | Billing | Foundry agents can use it |
|---|---|---|---|---|
| **Managed compute** (Foundry, preview) | **No**: Hugging Face collection models from the `azure-huggingface` registry only | the curated catalog | per GPU hour, `GlobalManagedCompute` | yes, admin-connected |
| **Fireworks import** ("bring your own weights") | yes, full weights or LoRA | **only architectures Fireworks supports** | Global Provisioned (PTU) | yes, as a Foundry deployment |
| **Serverless fine-tuning** of open models | only weights trained *inside* Foundry | Ministral-3B, Qwen-32B, Llama-3.3-70B, gpt-oss-20b | Standard / Global Standard / Developer tier / PTU | yes |
| **Azure ML managed online endpoint + your container** | **yes, anything vLLM (or your server) loads** | any | per VM hour | **yes, through a ModelGateway connection** |

Sources: managed compute overview and how-to, Fireworks import, fine-tuning
deployment (links in §11).

This repository uses the fourth path, for three reasons. Every target here needs
parsers and chat templates that ship in vLLM v0.30.0 (§2). Two of the three
fine-tuning variants (merged and adapter) must be served exactly as trained.
And WHAM is not an OpenAI-compatible model at all. Nothing else accepts all of
that.

Two alternatives are worth knowing about:

* **Azure Container Apps serverless GPU** (A100, T4) scales to zero, which
  managed online endpoints cannot. It is limited to one GPU per replica, so it
  cannot run the 2×H100 Muse Glimmer configuration.
* **AKS + KAITO** suits a platform team already running AKS. Its custom-model
  template uses the Hugging Face runtime.

The vLLM container in `shared/containers/serve` runs unchanged on either one.

### How the pieces connect

```
 Hugging Face ──(fetch job, in Azure)──▶ registered model  <name>-base:<rev12>
                                              │
                                   (train job: LoRA, merge | adapter)
                                              ▼
                                    registered model  <name>:<content-hash>
                                              │
        ftm-serve environment ──────▶ managed online deployment  v<hash10>   (0% traffic)
        (vLLM v0.30.0 + serve.sh)             │  smoke test via azureml-model-deployment header
                                              ▼
                                    promote to 100%; previous kept at 0% for rollback
                                              │
             ┌────────────────────────────────┼─────────────────────────────────────┐
             ▼                                ▼                                     ▼
  Foundry ModelGateway connection    Agent Framework harness            Copilot CLI (BYOK), gh-aw codex
  → prompt agents "<conn>/<model>"   OpenAIChatCompletionClient         engine with OPENAI_BASE_URL
```

## 2. The three base models

All three families were checked against Hugging Face metadata, their model cards,
and the vLLM v0.30.0 source tree (`vllm/model_executor/models/registry.py` and
the tool and reasoning parser registries).

| | Gemma 4 | Qwen3.8 | Muse Glimmer | Microsoft Muse (WHAM) |
|---|---|---|---|---|
| Repo used | `google/gemma-4-E4B-it` | `Qwen/Qwen3.8-27B` | `meta-models/Muse-Glimmer-30B` | `microsoft/wham` |
| Released | 2 Apr 2026 (12B on 3 Jun) | Aug 2026 | 10 Aug 2026 | Feb 2025 |
| License / gated | Apache-2.0 / no | Apache-2.0 / no | Apache-2.0 / no | MS Research License / no |
| Architecture | `Gemma4ForConditionalGeneration` | `Qwen3_5ForConditionalGeneration`, 27.8B | `MuseGlimmerForConditionalGeneration`, ~30B | WHAM transformer + VQGAN |
| Inputs | text, image, audio (E2B/E4B/12B) | text, image, video | text, image | game frames, controller actions |
| Context | 128K (E-sizes), 256K (12B+) | 262K native | 128K+ | 10 frame/action steps |
| vLLM tool parser | `gemma4` | `qwen3_xml` | `muse_glimmer` | n/a |
| vLLM reasoning parser | `gemma4` | `qwen3` | `muse_glimmer` | n/a |
| Minimum transformers | 5.5 (vLLM image ships ≥5.10.4) | n/a | n/a | n/a |
| GPU here | 1×A100 80 GB | 1×H100 80 GB, FP8 KV | 2×H100 80 GB, TP=2 | 1×A100 80 GB |

Details that change what you deploy:

* **Qwen "3.7" has no open weights.** Qwen3.7-Plus and Qwen3.7-Max are
  API-only; the Hugging Face `Qwen` organisation has no `Qwen3.7*` repositories.
  The newest open generation is **Qwen3.8**, but only `Qwen3.8-27B` is
  Apache-2.0. `Qwen3.8-2.4T-A95B` carries the `qwen3.8-max` license and
  `Qwen3.8-Flash-Next` the `qwen-community-1.0` license. At least one news
  report says the 2.4T is Apache-2.0; the repository metadata says otherwise.
  Trust the metadata. `Qwen/Qwen3.6-27B` (April, Apache-2.0) is the fallback.
  Its model card uses `--tool-call-parser qwen3_coder`.
* **"Muse" is two models.** Foundry's catalog entry called *Muse* is
  **Microsoft WHAM**, a research world model for the game *Bleeding Edge*. It
  predicts frames and controller actions and cannot back an agent. **Meta's
  Muse Glimmer 30B** (Apache-2.0) is the Muse you can build agents on. Meta's
  Muse Spark is proprietary; Meta has promised an open 1.2 but given no date.
  Example 3 deploys both usable ones.
* **Muse Glimmer in vLLM.** Meta's own vLLM page still says support is an
  unmerged pull request and points at a special image tag. That is out of date:
  the v0.30.0 tag contains `muse_glimmer.py`, both parsers, and the registry
  entry. The pinned upstream image is enough. Meta's card also warns against
  greedy decoding for this reasoning model; every client here uses
  temperature ≥ 0.2.
* **Gemma 4 tool calling** uses a chat template that ships in the vLLM image
  (`/vllm-workspace/examples/tool_chat_template_gemma4.jinja`). Example 1 passes
  it with `--chat-template`.
* **Qwen3.8 thinks by default.** Send
  `chat_template_kwargs: {"enable_thinking": false}` per request when you want
  short answers. The smoke test and the reviewer agent do.

## 3. Choosing a path per model

| You want | Do this |
|---|---|
| A model already in the Foundry HF collection, unchanged | Foundry managed compute; skip this repository |
| Your fine-tune of a Fireworks-supported base, with PTU pricing | Fireworks import |
| Your weights, any architecture, or a server that isn't OpenAI-compatible | this repository |
| An agent in Foundry on those weights | this repository + ModelGateway connection (§6) |
| Scale to zero between uses | same container on Azure Container Apps GPU |

## 4. Making and bringing custom weights

### One trainer for three families

All three language models load through `AutoModelForImageTextToText`, and all
three keep their text decoder under a `language_model` module with the usual
`q/k/v/o_proj` and `gate/up/down_proj` layers. This was checked in the
transformers 5.17 modelling code. So
[`shared/train/sft_lora.py`](shared/train/sft_lora.py) uses one LoRA target:

```python
TARGET = r".*language_model.*\.(q_proj|k_proj|v_proj|o_proj|gate_proj|up_proj|down_proj)$"
```

That adapts the language model and leaves the vision and audio towers frozen.
Qwen3.8's linear-attention layers have no `q_proj`, so they are skipped too,
which is the conservative choice. `mm_token_type_ids` is optional in all three
`forward()` signatures, so text-only fine-tuning needs only `input_ids`,
`attention_mask` and `labels`.

Loss is computed on the final assistant turn only
([`sftdata.py`](shared/train/sftdata.py)). The prompt is found by templating
every turn but the last with `add_generation_prompt=True`. If a chat template
rewrites earlier turns (some strip reasoning), the prefix check fails and the
row falls back to whole-sequence loss instead of silently mislabelling it.

### Two output layouts, one server

| `TRAIN_MODE` | Output | Served as | Use when |
|---|---|---|---|
| `merge` | full weights at the root | one model name | one customisation, simplest client story (Example 1) |
| `adapter` | `base/` + `adapters/<name>/` | base name **and** adapter name on one server | several customisations share one GPU; A/B against the base (Example 2) |

[`serve.sh`](shared/containers/serve/serve.sh) finds the shallowest
`config.json` under the Azure ML mount (the mount adds a directory level you
don't control). If that directory is called `base`, it registers every
`adapters/*/adapter_config.json` with `--enable-lora --lora-modules name=path`
and sets `--max-lora-rank` to the largest rank it finds.

### Bringing weights made elsewhere

Point `HF_REPO` at your repository (Example 3 does this through a workflow
input). For private repositories, put a read token in Key Vault and set
`KEY_VAULT_URL`. The fetch job reads it through the cluster's user-assigned
identity. The token never goes to the GitHub runner, the job definition, or the
logs.

### Versions are content hashes

[`ftmcfg.py`](shared/tools/ftmcfg.py) derives every name:

* **Base version:** the first 12 characters of the Hugging Face commit. `main`
  is resolved to a commit before planning, so a plan always names exact bytes.
* **Trained version:** a SHA-256 over the base version, repository, mode,
  adapter name, hyperparameters, the training image hash, the trainer source,
  and the data file.
* **Environment versions:** hashes of their Docker build context.
* **Deployment name:** `v` plus the first 10 characters of the served model's
  version.

A re-run with nothing changed finds everything already registered and deploys
nothing. Change one line of training data and you get a new model, a new
deployment, and a blue/green swap. `tests/test_ftmcfg.py` holds this property.

### Weights never touch the runner

A 30B checkpoint is about 60 GB, and a GitHub-hosted runner has a fraction of
that free. The fetch job runs **inside Azure ML** on `cpu-fetch`
(`Standard_D16ds_v5`, 600 GiB temp disk). It calls `snapshot_download` at a
pinned commit, writes `SHA256SUMS` and `ftm-fetch.json` beside the weights, and
uploads the result as a job output that is then registered. `huggingface_hub`
2.0 ships only the `hf` CLI (the `huggingface-cli` entry point is gone) and
downloads through `hf-xet`. `HF_HUB_ENABLE_HF_TRANSFER` is deprecated, so the
images set `HF_XET_HIGH_PERFORMANCE=1` instead.

## 5. Serving on Azure ML managed online endpoints

### Container

[`shared/containers/serve/Dockerfile`](shared/containers/serve/Dockerfile) is
`vllm/vllm-openai:v0.30.0` plus `serve.sh`. Azure ML builds it into the
workspace registry from the environment YAML. The training image uses the same
base, so tokenizer, chat template and transformers version match between
training and serving. It clears the vLLM `ENTRYPOINT` so Azure ML can run its
own command.

### Routes

```yaml
inference_config:
  liveness_route:  {port: 8000, path: /health}
  readiness_route: {port: 8000, path: /health}
  scoring_route:   {port: 8000, path: /}
```

With the scoring route at `/`, `https://<endpoint>.<region>.inference.ml.azure.com/v1/chat/completions`
reaches vLLM unchanged. So does `/v1/models`, and so do `/v1/responses` and
`/v1/messages` for clients that need them. `model_mount_path: /models` puts the
registered model at a fixed path.

### Limits that shape the design

* **`request_timeout_ms` tops out at 180 000.** Long generations must stream.
  The smoke test and clients use timeouts just under that.
* **Probes.** Loading 30B of weights and compiling CUDA graphs takes minutes.
  The deployment template gives liveness a 900 s initial delay and readiness 120
  retries at 10 s intervals.
* **SKUs.** The managed online endpoint SKU list includes NC24/48/96ads A100 v4,
  ND96amsr A100 v4, NC40ads/NC80adis H100 v5 and ND96isr H100 v5. It lists no
  H200. Quota is per region and VM family; check it before the first deploy
  (`bootstrap-azure.sh` prints the command).
* **Auth.** `auth_mode: key` is what a Foundry ModelGateway connection with
  `ApiKey` can present. `aad_token` endpoints (audience `https://ml.azure.com`)
  are stronger for direct callers, but whether ModelGateway's OAuth2 mode
  works with them is unverified.

### Blue/green without a load balancer

Each new model version becomes a new deployment at 0% traffic, except the very
first, which takes `--all-traffic`. `ftm smoke` sends the header
`azureml-model-deployment: v<hash>`, which pins requests to that deployment
whatever the traffic split. It checks `/v1/models`, a chat round trip, and (for
`SMOKE_KIND=tools`) that the model calls an offered tool. Only then does
`ftm promote` move 100% of traffic. The previous deployment stays at 0% for
`ftm rollback`; anything older is deleted.

## 6. Using the model in Foundry

### The connection

Foundry Agent Service reaches outside models through **Bring your own model**
connections. The category is `ModelGateway` (or `ApiManagement` for APIM). The
upstream must speak **OpenAI Chat Completions**: Foundry appends
`chat/completions` to the connection target. The ARM body that
[`connection_body.py`](shared/tools/connection_body.py) builds follows the
`foundry-samples` ModelGateway contract, where every metadata value is a string:

```json
{"properties": {
  "category": "ModelGateway", "authType": "ApiKey", "isSharedToAll": false,
  "target": "https://ftm-gemma4-coolgit.eastus2.inference.ml.azure.com/v1",
  "credentials": {"key": "<endpoint key>"},
  "metadata": {
    "models": "[{\"name\":\"gemma4-coolgit\",\"properties\":{\"model\":{\"name\":\"gemma4-coolgit\",\"version\":\"1\",\"format\":\"OpenAI\"}}}]",
    "deploymentInPath": "false",
    "authHeaderName": "Authorization",
    "authHeaderFormat": "Bearer {api_key}",
    "customHeaders": "{}"}}}
```

`ftm connect` PUTs it with `az rest` to
`…/accounts/<account>/projects/<project>/connections/<name>?api-version=2025-04-01-preview`.
It re-runs after each promotion, so a rotated endpoint key reaches Foundry on
the next deploy.

### Prompt agents

Agents name the model `<connection>/<model>`:

```python
from azure.ai.projects import AIProjectClient            # azure-ai-projects 2.7.0
from azure.ai.projects.models import PromptAgentDefinition
from azure.identity import DefaultAzureCredential

with AIProjectClient(endpoint="https://ais-coolgit-copilot-prod.services.ai.azure.com/api/projects/proj-coolgit-agents",
                     credential=DefaultAzureCredential()) as project:
    agent = project.agents.create_version(
        agent_name="coolgit-runbooks",
        definition=PromptAgentDefinition(model="ftm-gemma4/gemma4-coolgit", instructions="..."))
    reply = project.get_openai_client().responses.create(
        input="Runbook: rotate the app key.",
        extra_body={"agent_reference": {"name": "coolgit-runbooks", "type": "agent_reference"}})
```

The signatures were checked against the installed 2.7.0 package. Clients still
call the agent through Responses; the Responses restriction in the docs is
about the *upstream* model API, which must be Chat Completions.

### What BYOM does not cover

* **Only prompt agents** are listed as supporting BYOM connections.
* **Hosted agents** are your own containers (Agent Framework, LangGraph and so
  on). They don't need a connection: their code calls the endpoint directly, as
  the Example 2 harness does.
* **Foundry workflows** (preview) **are being retired on 1 December 2026.**
  Microsoft recommends **Agent Framework workflows**, declarative YAML or code.
  Example 2's `WorkflowBuilder` pipeline is that replacement, running on your
  model.

## 7. Harnesses and workflows on the endpoint

The endpoint is plain OpenAI-compatible HTTP, so any harness that accepts a
base URL can use it.

| Harness | Setting | Example |
|---|---|---|
| **Microsoft Agent Framework** 1.19 (`agent-framework-core`, `agent-framework-openai` 1.14.4) | `OpenAIChatCompletionClient(model=..., base_url=".../v1", api_key=...)`. In 1.x, `OpenAIChatClient` means the **Responses** API; for vLLM use the Chat Completions client. Pass vLLM extras as `default_options={"extra_body": {...}}`. | 2 |
| **Copilot CLI** BYOK | `COPILOT_PROVIDER_BASE_URL=.../v1`, `COPILOT_PROVIDER_TYPE=openai`, `COPILOT_PROVIDER_API_KEY`, `COPILOT_MODEL`, `COPILOT_OFFLINE=true`. The model must support tool calling and streaming. | 3 |
| **gh-aw** agentic workflows | `engine: {id: codex, env: {OPENAI_BASE_URL: .../v1}}` + `model:` + `network.allowed` host. The api-proxy injects `OPENAI_API_KEY`; setting a base URL disables model fallback. | 3 |
| Plain OpenAI SDK | `OpenAI(base_url=".../v1", api_key=key)` | 1 (`direct.py`), 3 (`vision.py`) |

### GitHub Actions design

* **One reusable workflow, several triggers.**
  [`ftm-pipeline.yml`](.github/workflows/ftm-pipeline.yml) is the only code
  path to Azure. `ex1-gemma4.yml`, `ex2-qwen38.yml` and `ex3-muse.yml` are
  `workflow_dispatch` front-ends with typed inputs.
  `issueops-model-request.yml` is the IssueOps front-end.
* **`plan` never logs in to Azure.** It validates inputs, pins the revision,
  renders the YAML, uploads it as an artifact, and writes a plan table to the
  run summary.
* **`apply` runs in an environment.** `ftm-prod` (required reviewers) for real
  changes, `ftm-plan` for dry runs, `ftm-verify` for post-deploy checks. Each has
  its own OIDC federated credential on `uami-ftm-deploy`; there are no Azure
  secrets anywhere.
* **Inputs reach bash only through `env:`.** No `${{ }}` appears inside a
  `run:` block. Every value is validated twice: in bash, then by `ftmcfg.py`'s
  regular expressions, which reject quotes, newlines and anything that could
  pose as a flag.
* **IssueOps is two-person by construction.** The issue form is parsed and
  validated by [`issueform.py`](shared/tools/issueform.py). Opening or editing
  the issue plans. `/approve` needs `admin` or `maintain` and must come from
  someone other than the requester, and `ftm-prod` reviewers still gate the
  apply. The result, with a link to the committed record, lands back on the
  issue. Plans triggered by an issue resolve the Hugging Face revision
  anonymously; the `HF_TOKEN` secret is used only by dispatches and approved
  applies, so an issue cannot make the workflow probe private repositories
  with your token.
* **Records are commits.** Every apply writes `records/<example>/<time>-<request>.json`
  and commits it with `git commit -s` (optionally as `issueops-autoadmin-app`
  so rulesets can allow it), retrying the push with rebase.
* **Long jobs.** GitHub-hosted jobs stop at 6 hours. Azure ML jobs keep
  running, and because versions are content hashes, re-dispatching picks up the
  registered result instead of retraining.

## 8. Operating it

**First deploy**

1. `scripts/bootstrap-azure.sh` prints the plan; add `--apply` to create the
   workspace, clusters, identities, federation, roles, environments, variables
   and labels.
2. Add required reviewers to `ftm-prod`, and restrict all three environments to
   `main`.
3. Check GPU quota for managed online endpoints in the region.
4. `scripts/preflight.sh`.
5. Dispatch `ex1-gemma4` with `action=plan`, then `deploy` (dry run), then
   `deploy` with `dry_run=false`.

**Every day**

| Task | Command |
|---|---|
| What will a change do? | `ftm plan EX` or dispatch `action=plan` |
| Deploy new data or weights | commit, then dispatch `deploy` (or IssueOps) |
| Bad model in production | `ftm rollback EX` or dispatch `action=rollback` (the previous deployment is still warm) |
| Stop paying for a GPU | `ftm destroy EX`. Registered models stay, so the next deploy skips fetch and training. |
| Who changed what | `git log -- records/` (each commit has the run URL and reason) |
| vLLM metrics | `GET <endpoint>/metrics` with the key should return vLLM's Prometheus text through the same scoring route (untested); Azure ML's own endpoint metrics are in Azure Monitor |

**Cost.** Clusters scale to zero after 15 idle minutes. **Endpoints bill for
their VMs at any traffic level**, including the 0%-traffic rollback deployment
(`FTM_KEEP_PREVIOUS=0` drops it). Destroy endpoints you are not using.

**Troubleshooting**

| Symptom | Likely cause |
|---|---|
| deployment stuck in `Creating`, then fails its probes | the model is still loading: raise `liveness_probe.initial_delay`; or out of GPU memory: lower `--max-model-len` or `--gpu-memory-utilization` |
| `serve.sh: no config.json under /models` | registered the wrong folder; `az ml model download` it and look |
| `InsufficientQuota` / `OutOfQuota` | managed online endpoint quota for the VM family in that region |
| smoke `tools` fails, `chat` passes | wrong or missing `--tool-call-parser`, or (Gemma 4) missing `--chat-template` |
| Foundry agent: 404 from the model | the connection target must end in `/v1` with `deploymentInPath: "false"`; model name must be listed in `metadata.models` |
| Foundry agent: 401 | endpoint key rotated since the last `ftm connect`; run it again |
| fetch job fails on a private repo | `KEY_VAULT_URL` / `HF_TOKEN_SECRET` unset, or the cluster identity lacks *Key Vault Secrets User* |
| AADSTS70021 in `azure/login` | federated subject mismatch: environment name or repository differs from bootstrap |

## 9. The examples

| | Example 1 | Example 2 | Example 3 |
|---|---|---|---|
| Model | Gemma 4 E4B | Qwen3.8-27B | Muse Glimmer 30B (+ WHAM) |
| Custom weights | LoRA, merged | LoRA adapter beside base | your Hugging Face repository |
| Consumer | **Foundry prompt agent** (BYOM) | **Agent Framework harness + workflow + eval** | **Copilot CLI BYOK + gh-aw** |
| Trigger | dispatch | dispatch + **IssueOps** | dispatch (+ IssueOps via the shared form) |
| GPU (serve) | 1×A100 | 1×H100 | 2×H100 (WHAM: 1×A100) |

Each has its own README with commands, files, and how to adapt it.

## 10. What was verified, and what was not

**Verified while writing this (28 Sep 2026).**

* Hugging Face ids, licenses, gating and architectures for all four repositories.
* vLLM v0.30.0 source: model classes for Gemma 4, `qwen3_5` and Muse Glimmer;
  parsers `gemma4`, `qwen3_xml`, `qwen3_coder`, `qwen3` and `muse_glimmer`;
  LoRA support in all three; the `--lora-modules name=path` and
  `--default-chat-template-kwargs` flags; `/v1/responses` and `/v1/messages`
  routes; the Gemma 4 tool template in the image.
* transformers 5.17: auto-class mappings and module layout used by the trainer.
* agent-framework-core 1.19.0 and azure-ai-projects 2.7.0 signatures, by
  introspecting installed packages.
* The foundry-samples ModelGateway connection contract.
* Current major tags for every action used.

**Tested in this repository without Azure.** 56 tests, run with pytest (the
2 harness tests only when agent-framework is installed):

* configuration, versioning and rendering (every rendered file parses as YAML);
* IssueOps parsing and validation, including injection attempts;
* the SFT data masking;
* `smoke_test.py` and the Agent Framework harness and workflow against a local
  OpenAI-compatible fake;
* the WHAM shim;
* the full `ftm all` sequence in dry-run mode against a stub `az`.

`ruff`, `shellcheck` and `actionlint` 1.7.12 are clean.

**Not verified.** Expect to adjust these on first contact:

* Nothing has run against a live Azure subscription or GPU. Probe timings,
  memory headroom and training time are estimates.
* The exact path under the Azure ML model mount. `serve.sh` searches for it
  rather than assuming it.
* Whether a ModelGateway connection streams, and whether its OAuth2 mode works
  with `aad_token` endpoints.
* The `az ... project connection create --file` schema. This repository uses
  ARM PUT instead.
* Muse Glimmer's Foundry catalog listing, and the Foundry "Muse" (WHAM)
  endpoint's request schema versus the local `run_server.py` one.
* WHAM's `/new_job` reply format. `wham_client.py` accepts either `job_id` or a
  bare id.
* The gh-aw codex engine against vLLM's Responses API. It is documented as
  supported for custom base URLs but not tested here.
* The minimum vLLM version for Qwen3.8. The recipe gives two, 0.17.0 and
  0.26.1rc1, which disagree; this repository pins v0.30.0.
* Docker Hub tag `vllm/vllm-openai:v0.30.0`. The git tag exists; the image tag
  is assumed to follow it.

## 11. Sources

**Microsoft (Foundry, Azure ML, Agent Framework)**

* [Managed compute overview](https://learn.microsoft.com/en-us/azure/foundry/concepts/managed-compute-overview) · [Deploy with managed compute](https://learn.microsoft.com/en-us/azure/foundry/how-to/deploy-models-managed)
* [Import custom models (Fireworks)](https://learn.microsoft.com/en-us/azure/foundry/how-to/fireworks/import-custom-models) · [Deploy fine-tuned models](https://learn.microsoft.com/en-us/azure/foundry/openai/how-to/fine-tuning-deploy)
* [Bring your own model to Foundry Agent Service](https://learn.microsoft.com/en-us/azure/foundry/agents/how-to/ai-gateway) · [foundry-samples connections](https://github.com/microsoft-foundry/foundry-samples/tree/main/infrastructure/infrastructure-setup-bicep/01-connections)
* [Hosted agents](https://learn.microsoft.com/en-us/azure/foundry/agents/concepts/hosted-agents) · [Workflows (retiring 1 Dec 2026)](https://learn.microsoft.com/en-us/azure/foundry/agents/concepts/workflow)
* [Foundry RBAC and role IDs](https://learn.microsoft.com/en-us/azure/foundry/concepts/rbac-foundry) · [AI + ML built-in roles](https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/ai-machine-learning)
* [Custom container deployment](https://learn.microsoft.com/en-us/azure/machine-learning/how-to-deploy-custom-container?view=azureml-api-2) · [Deployment YAML schema](https://learn.microsoft.com/en-us/azure/machine-learning/reference-yaml-deployment-managed-online?view=azureml-api-2) · [Environment YAML schema](https://learn.microsoft.com/en-us/azure/machine-learning/reference-yaml-environment?view=azureml-api-2) · [Endpoint SKU list](https://learn.microsoft.com/en-us/azure/machine-learning/reference-managed-online-endpoints-vm-sku-list?view=azureml-api-2) · [Endpoint auth](https://learn.microsoft.com/en-us/azure/machine-learning/how-to-authenticate-online-endpoint?view=azureml-api-2) · [Manage models](https://learn.microsoft.com/en-us/azure/machine-learning/how-to-manage-models?view=azureml-api-2)
* [Container Apps serverless GPU](https://learn.microsoft.com/en-us/azure/container-apps/gpu-serverless-overview) · [AKS KAITO custom models](https://learn.microsoft.com/en-us/azure/aks/kaito-custom-inference-model)
* [Agent Framework OpenAI providers](https://learn.microsoft.com/en-us/agent-framework/agents/providers/openai) · [OpenAI-compatible endpoints](https://learn.microsoft.com/en-us/agent-framework/integrations/openai-endpoints)
* [Connect GitHub Actions with OIDC](https://learn.microsoft.com/en-us/azure/developer/github/connect-from-azure-openid-connect) · [Azure/login](https://github.com/Azure/login)
* [Gemma 4 in Microsoft Foundry](https://techcommunity.microsoft.com/blog/azure-ai-foundry-blog/gemma-4-now-available-in-microsoft-foundry/4510984) · [Foundry catalog: Muse](https://ai.azure.com/catalog/models/Muse)

**Models**

* Gemma 4: [announcement](https://blog.google/innovation-and-ai/technology/developers-tools/gemma-4/) · [12B](https://blog.google/innovation-and-ai/technology/developers-tools/introducing-gemma-4-12b/) · [gemma-4-31B-it card](https://huggingface.co/google/gemma-4-31B-it) · [vLLM Gemma 4 recipe](https://docs.vllm.ai/projects/recipes/en/stable/Google/Gemma4.html)
* Qwen: [Qwen3.8-27B](https://huggingface.co/Qwen/Qwen3.8-27B) · [vLLM Qwen3.8 recipe](https://recipes.vllm.ai/Qwen/Qwen3.8-27B) · [Qwen3.6-35B-A3B card](https://huggingface.co/Qwen/Qwen3.6-35B-A3B) · [Qwen3.7-Plus launch](https://www.marktechpost.com/2026/06/02/alibabas-qwen-team-launches-qwen3-7-plus-adding-vision-deep-reasoning-tool-invocation-and-autonomous-iteration-on-the-bailian-platform/) · [Qwen3.8 open weights](https://the-decoder.com/alibabas-qwen-team-releases-qwen-3-8-models-with-open-weights-under-the-apache-2-0-license/)
* Muse: [Muse Glimmer announcement](https://research.meta.ai/blog/introducing-muse-glimmer-open-agentic-model) · [Muse-Glimmer-30B card](https://huggingface.co/meta-models/Muse-Glimmer-30B) · [Hugging Face blog](https://huggingface.co/blog/muse-glimmer) · [vLLM recipe](https://recipes.vllm.ai/meta-models/Muse-Glimmer-30B) · [Meta vLLM page (stale on support)](https://dev.meta.ai/docs/muse-glimmer/vllm/) · [Muse Spark](https://en.wikipedia.org/wiki/Muse_Spark) · [microsoft/wham](https://huggingface.co/microsoft/wham) · [WHAM run_server.py](https://huggingface.co/microsoft/wham/blob/main/run_server.py) · [Foundry Labs: Muse](https://labs.ai.azure.com/innovations/muse/)

**Tools**

* [vLLM](https://github.com/vllm-project/vllm) (tag v0.30.0) · [vLLM Docker deployment](https://github.com/vllm-project/vllm/blob/main/docs/deployment/docker.md)
* [huggingface_hub CLI](https://github.com/huggingface/huggingface_hub/blob/main/docs/source/en/guides/cli.md) · [environment variables](https://github.com/huggingface/huggingface_hub/blob/main/docs/source/en/package_reference/environment_variables.md)
* [Copilot CLI BYOK](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-byok-models)
* gh-aw engine and network reference (`gh-aw-llms-full`, project files)
