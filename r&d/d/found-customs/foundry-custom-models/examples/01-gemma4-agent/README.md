# Example 1: Gemma 4, fine-tuned, behind a Foundry agent

**What it shows:** custom weights *made by you* (LoRA fine-tune merged into
full weights), served on an Azure ML managed online endpoint, and used by a
**Foundry Agent Service prompt agent** through a ModelGateway connection.
Trigger: **manual `workflow_dispatch` with typed inputs**.

| | |
|---|---|
| Base | [`google/gemma-4-E4B-it`](https://huggingface.co/google/gemma-4-E4B-it), Apache-2.0, not gated, text + image + audio in, 128K context |
| Customisation | LoRA r=16 on the language model's attention and MLP projections, merged (`TRAIN_MODE=merge`) |
| Data | 24 runbook Q&A pairs from [`data/build.py`](data/build.py): answers always have Summary / Steps / Rollback |
| Training compute | `gpu-a100` (1 x A100 80 GB), roughly 10 to 20 minutes |
| Serving | `Standard_NC24ads_A100_v4`, vLLM v0.30.0, `--tool-call-parser gemma4 --reasoning-parser gemma4` |
| Clients send | `model: gemma4-coolgit` |
| Foundry agent model | `ftm-gemma4/gemma4-coolgit` (connection / model) |

## Files

```
model.env              everything that varies: repo, revision, SKU, vLLM flags, training
data/build.py          seed dataset generator (edit RUNBOOKS, run make data)
data/train.jsonl       its output; the training input
agent/create_agent.py  new Foundry prompt-agent version on the custom model
agent/ask.py           ask the agent via the Responses API; --expect for tests
agent/direct.py        call the endpoint with the plain OpenAI SDK, no Foundry
```

## Run it from GitHub

1. `gh workflow run ex1-gemma4.yml -f action=plan -f reason="first look at the plan"`
   Resolves `main` to a commit, renders every Azure ML YAML, and writes a plan
   table to the run summary. No Azure login.
2. `gh workflow run ex1-gemma4.yml -f action=deploy -f reason="dry run of first deploy"`
   `dry_run` defaults to true: every `az` change is printed, none is made.
3. `gh workflow run ex1-gemma4.yml -f action=deploy -f dry_run=false -f reason="first deploy"`
   Waits for a reviewer on `ftm-prod`, then fetch, train, register, deploy (0%),
   smoke-test the new deployment, promote to 100%, connect Foundry, commit a
   record. The `agent` job then publishes a new agent version and asks it a
   question end to end.

Each step skips itself when its output already exists. The model version is a
hash of the base revision, the data, the trainer code and the hyperparameters.
So a re-run with nothing changed deploys nothing, and changing one line of data
produces a new version and a new deployment.

## Run it from a terminal

```sh
az login && az extension add -n ml
export FTM_SET_HF_REVISION=$(shared/bin/ftm resolve gemma4)
shared/bin/ftm plan gemma4
FTM_DRY_RUN=1 shared/bin/ftm all gemma4     # print the az commands
shared/bin/ftm all gemma4                   # do it
export FTM_BASE_URL=$(shared/bin/ftm info gemma4 | python3 -c 'import json,sys; print(json.load(sys.stdin)["base_url"])')
export FTM_KEY=$(az ml online-endpoint get-credentials -n ftm-gemma4-coolgit -g rg-coolgit-copilot -w mlw-coolgit-foundry --query primaryKey -o tsv)
python examples/01-gemma4-agent/agent/direct.py "How do I archive an inactive repository?"
```

Then the agent (needs a Foundry project role that can create agents):

```sh
pip install -r examples/01-gemma4-agent/agent/requirements.txt
cd examples/01-gemma4-agent/agent
python create_agent.py --model ftm-gemma4/gemma4-coolgit
python ask.py "Runbook: revoke a leaked personal access token."
```

## Make it yours

* Replace `RUNBOOKS` in `data/build.py`, or write `train.jsonl` directly
  (`{"messages": [system?, user, assistant]}` per line; the last turn is the
  answer and the only one that carries loss). `make data`, commit, deploy.
* Bigger model: `HF_REPO=google/gemma-4-31B-it`,
  `INSTANCE_TYPE=Standard_NC80adis_H100_v5`, add `--tensor-parallel-size 2`
  to `VLLM_ARGS`, `TRAIN_COMPUTE=gpu-h100x2`.
* Serve the base without training: dispatch with `train=false`.

## Cost

The endpoint bills for its VM while it exists, at any traffic level; the
training cluster scales to zero. `action=destroy` removes the endpoint and the
connection; the registered models stay, so the next deploy skips fetch and
training.
