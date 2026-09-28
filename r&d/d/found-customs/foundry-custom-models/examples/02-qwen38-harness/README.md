# Example 2: Qwen3.8-27B + a LoRA adapter, driven by an Agent Framework harness

**What it shows:** custom weights as a **LoRA adapter served beside its base**
on one vLLM server (clients pick by model name). A **Microsoft Agent Framework
harness** uses both: the base model investigates with tools, the adapter
decides, wired as an Agent Framework **workflow**. It also runs an **eval**.
Triggers: **manual dispatch** (`ex2-qwen38.yml`) and **IssueOps** (open a
*Model deployment request* issue, get a plan, a maintainer comments
`/approve`).

**Why Qwen3.8 and not 3.7:** Qwen3.7 shipped only as hosted API models
(Plus, Max); there are no `Qwen/Qwen3.7*` weights on Hugging Face. Qwen3.8-27B
(August 2026) is the newest open-weight Qwen under Apache-2.0. Its larger
siblings, 2.4T-A95B and Flash-Next, carry other licenses. `Qwen/Qwen3.6-27B` is
a drop-in fallback: change `HF_REPO` and use `--tool-call-parser qwen3_coder`.

| | |
|---|---|
| Base | [`Qwen/Qwen3.8-27B`](https://huggingface.co/Qwen/Qwen3.8-27B), Apache-2.0, 27.8B dense, image + video + text, 262K native context |
| Customisation | LoRA adapter `coolgit-ops`, kept separate (`TRAIN_MODE=adapter`) |
| Data | 24 governance reviews from [`data/build.py`](data/build.py): reply is one JSON verdict |
| Training compute | `gpu-h100x2` (2 x H100 80 GB) |
| Serving | `Standard_NC40ads_H100_v5`, FP8 KV cache, `--tool-call-parser qwen3_xml --reasoning-parser qwen3`, LoRA enabled |
| Clients send | `qwen38-27b` (base) or `coolgit-ops` (adapter) |
| Foundry agent model | `ftm-qwen38/coolgit-ops` (both names are listed on the connection) |

## Files

```
model.env                    Qwen3.8 settings, adapter mode
data/build.py, train.jsonl   seed reviews
governance/policy.md         the policy the tools expose
governance/changes/*.yml     proposed repository changes to review
governance/cases.json        expected decisions + accuracy bar for eval
harness/tools.py             read-only @tool functions (no network, no writes)
harness/run.py               review | workflow | eval
```

## The harness

```
run.py review FILE      reviewer agent (adapter) -> {"decision", "risks", "checks"}
run.py workflow FILE    WorkflowBuilder: investigator (base + tools) -> reviewer (adapter)
run.py eval             reviewer over governance/cases.json; exit 1 below min_accuracy
```

`OpenAIChatCompletionClient` talks Chat Completions to the vLLM endpoint.
In Agent Framework 1.x `OpenAIChatClient` now means the Responses API; the
Chat Completions client is the one to use against vLLM. Thinking is turned off
for the reviewer with `extra_body={"chat_template_kwargs": {"enable_thinking": false}}`,
and left on for the tool-using investigator.

```sh
pip install -r examples/02-qwen38-harness/harness/requirements.txt
export FTM_BASE_URL=https://ftm-qwen38-coolgit.eastus2.inference.ml.azure.com
export FTM_KEY=$(az ml online-endpoint get-credentials -n ftm-qwen38-coolgit -g rg-coolgit-copilot -w mlw-coolgit-foundry --query primaryKey -o tsv)
python examples/02-qwen38-harness/harness/run.py eval
python examples/02-qwen38-harness/harness/run.py workflow contractor-portal.yml
```

`tests/test_harness.py` runs the same harness against a local fake endpoint,
so the wiring is tested in CI without a GPU.

## IssueOps

1. New issue, template *Model deployment request*, Example `qwen38`, Action
   `deploy`, a reason. The workflow validates every field and comments a plan:
   pinned revision, deployment name, the model name agents will use.
2. Edit the issue or comment `/plan` to re-plan.
3. A maintainer other than the requester comments `/approve`. The same
   `ftm-pipeline` runs with `dry_run=false`; `ftm-prod` reviewers still gate it.
4. The issue gets the result and a link to the committed record, is labelled
   `applied`, and closes. `/cancel` closes it without changes.

## Serving several adapters

Put more adapters under `adapters/<name>/` in the registered bundle (train with
different `ADAPTER_NAME`s into the same output, or assemble the folder and
`az ml model create` it). `serve.sh` registers every `adapters/*/` it finds
with `--lora-modules`, and each becomes a model name on the same endpoint.
Raise `--max-loras` in `VLLM_ARGS` to keep more of them resident.
