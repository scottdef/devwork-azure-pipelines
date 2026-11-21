# Local LLM Operations on a ThinkStation P340: Unsloth Studio to Strata

As of 20 November 2026. The live, editable version is the Docs artifact of the same title; companion code is in p340-strata.tar.gz.

## Read this first

The stock P340 can run the whole prototype plane today, but it can run Strata only in a configuration nobody has published: Strata's smallest reported GPU is 8 GB and this box has 4 GB. The guide is therefore written so the production plane works either way, and so one inexpensive card turns it into a documented Strata host.

**What I took "Strata" to mean.** [Niko1221/Strata](https://github.com/Niko1221/Strata), the MIT-licensed local inference engine for large mixture-of-experts models, first released 24 September 2026 and at version 0.2.13 (engine 0.1.59) on 9 November 2026 ([vdf.ai review, checked 13 Nov 2026](https://vdf.ai/blog/strata-local-llm/)). If you meant a different Strata, say so and I will redo the production sections.

**What changed from the May 2026 guide**

| Area | May 2026 guide | This guide |
| --- | --- | --- |
| Prototype and test | Ollama CLI, LM Studio, Open WebUI | Unsloth Studio: model hub, chat, arena, tools, linked folders, Data Recipes, training, GGUF export, API |
| Production serving | Ollama under systemd | Strata for the one large MoE model, a llama-server lane for small models, a gateway in front |
| Reference models | qwen2.5 3B/7B, llama3.2 3B | Qwen3.5-4B and 9B, gpt-oss-20b, Qwen3.6-35B-A3B, Qwen3.8-Flash-Next |
| Hardware | unchanged | unchanged: i9-10900, 32 GB DDR4-2933, Quadro P1000 4 GB, Ubuntu 20.04 |

**Four facts that shape every later section**

1. **Strata's "mixture of experts" is inside one model.** It is not a system of several models voting. Strata runs three model families (Qwen3.8-Flash-Next, Qwen3.6-35B-A3B, DeepSeek V4.1-Flash), one engine process per model, with up to 8 parallel conversation slots ([capabilities table](https://github.com/Niko1221/Strata/blob/main/docs/CAPABILITIES.md)). A multi-model council or master-slave system is built around Strata with more processes and a gateway. Sections 6 and 9 do that.
2. **The GPU floor is above this card.** Strata recommends 12 GB "as for every card", records community runs on 8 GB cards, and says a 2 GB GT 1030 "cannot hold any of the model" ([older GPUs](https://github.com/Niko1221/Strata/blob/main/docs/OLDER_GPUS.md)). The P1000 is Pascal (compute capability 6.1), which the experimental CUDA 12 engine covers, but every published Pascal number comes from 16 to 24 GB Tesla cards.
3. **32 GB of RAM fits exactly one Strata model.** Qwen3.6-35B-A3B at UD-Q4\_K\_XL is a 22.9 GB file that setup plans 32 GB for. The Flash-Next packs keep 34 to 50 GB of experts in RAM plus about 10 GB for the system ([models and memory](https://github.com/Niko1221/Strata/blob/main/docs/MODELS.md)).
4. **GPU fine-tuning on this card is a dead end.** PyTorch 2.14 is the last release with prebuilt wheels for Pascal; from 2.15 it is build-from-source only ([PyTorch notice, 4 Sep 2026](https://dev-discuss.pytorch.org/t/notice-cuda-12-6-wheels-will-no-longer-be-published-from-pytorch-2-15-drops-maxwell-pascal-volta/3432)). Unsloth Studio's chat, tools and Data Recipes run on CPU, so the prototype plane still works.

**What to do with that**

- Install Unsloth Studio now. Everything in section 3 works on this box.
- Run `strata launch --plan --family qwen36 --model UD-Q4_K_XL`. It downloads nothing, changes nothing, and prints whether the engine accepts this machine.
- Serve the same Qwen3.6 GGUF through llama-server behind the same gateway alias until Strata starts. Clients do not change when the engine does.
- The upgrade that makes this a documented Strata host is a 12 GB low-profile card. Used RTX A2000 cards sold at a median of $517 in October 2026, with the 12 GB version toward the top of a $461 to $630 range ([sold-listing tracker](https://gpusold.com/gpu/rtx-a2000)). Appendix A has the rest.

## 1. Architecture: two planes, one weight file

Unsloth Studio is where models are found, compared and shaped. A gateway in front of one large-model engine is what agents talk to. Both read the same GGUF file from disk.

```
                                   Clients: OpenCode, Claude Code, council, agent, scripts
                                                     | one address, one key
  Prototype plane                    Production plane v
 +---------------------------+      +-----------------------------------------------+
 | Unsloth Studio :8888      |      | gate :4000                                    |
 | model hub, chat, arena,   |- - ->| aliases brain, fast, embed; next upstream     |
 | tools, linked folders,    |      | on error                                      |
 | Data Recipes, training,   |      +----------+--------------------+---------------+
 | GGUF export, API          |           first |               then |
 +---------------------------+      +----------v-------+   +--------v-------------+
  Only one plane holds the          | Strata :8080     |   | llama-server :8081   |
  big model: 32 GB of RAM           | brain, 1-8 slots |   | brain, fallback,     |
  cannot hold it twice.             +------------------+   | same file            |
                                    +------------------+   +----------------------+
                                    | llama-server     |   +----------------------+
                                    | :8083 embed, CPU |   | SearXNG :8889        |
                                    +------------------+   +----------------------+
        ^ download and test                              ^ serve
 +--------------------------------------------------------------------------------+
 | One weight file on disk: Qwen3.6-35B-A3B UD-Q4_K_XL, 22.9 GB, read by all three |
 +--------------------------------------------------------------------------------+
```

The dashed line is Studio's Connections feature: Studio can chat with the production alias, so the test bench and the served system are the same thing.

| Component | Plane | Job | Port | Started by |
| --- | --- | --- | --- | --- |
| Unsloth Studio | Prototype | Find, download, compare, tool-test, build datasets, train, export | 8888 | `unsloth studio` |
| `gate` | Production | Alias routing, key check, failover between upstreams | 4000 | systemd user unit |
| Strata | Production | The large MoE model, OpenAI and Anthropic APIs | 8080 | `strata launch --preset p340 --no-tui` |
| llama-server (brain) | Production | The same file through llama.cpp when Strata is not running | 8081 | systemd user unit |
| llama-server (embed) | Production | Embeddings for retrieval, CPU only | 8083 | systemd user unit |
| SearXNG | Tool | Web search for agents | 8889 | Docker Compose |

SearXNG moves from 8888 in the May guide to 8889, because 8888 is Studio's default port. Strata and llama-server both default to 8080, so the llama lane is moved.

**One plane holds the big model at a time.** The brain keeps about 21 GB of experts in RAM and the system keeps about 4 GB, which leaves 7 GB. Loading a second multi-gigabyte model in Studio while the brain lane is up pushes the box into swap. `make proto` stops the brain lanes and `make prod-llama` or `make prod-strata` starts one again.

**Why a gateway**

- The engine behind `brain` can change from llama-server to Strata, or to another machine, without touching a client.
- Upstream keys stay on the server. Clients hold one gateway key.
- Councils and master-slave systems need one place to fan out from. An alias can list upstreams on other hosts.

`gate` is 165 lines of Go standard library in the companion package. LiteLLM, the gateway in the May guide, does the same job with more features if you would rather run Python.

## 2. Base system

The base is the May guide's: proprietary driver 580, CUDA Toolkit 12.9, Docker with the NVIDIA runtime. Two things are new: a CMake newer than Focal's, and a Go toolchain for the gateway. The commands in 2.1 and 2.2 are carried over unchanged and were not re-run in November.

### 2.1 Driver 580 and CUDA Toolkit 12.9

```bash
sudo pro attach <token> && sudo pro enable esm-infra esm-apps
sudo apt update && sudo apt -y full-upgrade
sudo apt-get -y --purge remove '^nvidia-.*' '^libnvidia-.*' '^cuda-.*'
sudo apt-get install -y build-essential dkms linux-headers-$(uname -r) curl gnupg ca-certificates git jq

printf 'blacklist nouveau\noptions nouveau modeset=0\n' | sudo tee /etc/modprobe.d/blacklist-nouveau.conf
sudo update-initramfs -u

wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb && sudo apt-get update
sudo apt-get install -y cuda-drivers-580 cuda-toolkit-12-9
sudo apt-mark hold nvidia-open nvidia-open-580
sudo reboot
```

```bash
echo 'export PATH=/usr/local/cuda-12.9/bin:$PATH' >> ~/.bashrc
nvidia-smi          # expect driver 580.xx, Quadro P1000, 4096 MiB
nvcc --version      # expect release 12.9
```

CUDA 12.9 is the ceiling, not a preference. CUDA 13 cannot compile for Pascal, which is why Strata itself ships a second engine built with CUDA 12.9 for these cards ([older GPUs](https://github.com/Niko1221/Strata/blob/main/docs/OLDER_GPUS.md)).

### 2.2 Docker and the NVIDIA runtime

```bash
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu focal stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
sudo usermod -aG docker "$USER"

curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
  | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt-get update && sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker && sudo systemctl restart docker
docker run --rm --gpus all nvidia/cuda:12.9.0-base-ubuntu22.04 nvidia-smi
```

### 2.3 Build tools

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh          # uv before pip
uv tool install cmake                                     # Focal ships CMake 3.16
cmake --version

curl -fsSLO https://go.dev/dl/go1.24.7.linux-amd64.tar.gz # check the hash against go.dev/dl
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf go1.24.7.linux-amd64.tar.gz
echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc
```

CMake matters because building llama-server for one GPU generation uses `CMAKE_CUDA_ARCHITECTURES`, which arrived in CMake 3.18. The Go programs in the companion package were built and tested with Go 1.24.7 and use only the standard library.

### 2.4 Where Ubuntu 20.04 still holds

| Component | What its documentation asks for | On this box |
| --- | --- | --- |
| Unsloth Studio | "Ubuntu 20.04+", Python 3.11 to 3.13, CUDA toolkit 12.4 or newer ([install page](https://unsloth.ai/docs/new/studio/install)) | Supported |
| Strata | "Linux x86-64 with an NVIDIA RTX GPU"; driver 580 for the CUDA 13 engines, 525 on Linux for the CUDA 12 engine ([install](https://github.com/Niko1221/Strata/blob/main/docs/INSTALL.md)) | No distribution version is stated. Not verified on Focal |
| PyTorch on Pascal | Prebuilt wheels end at 2.14 | Affects training only |

The open question is Strata's prebuilt engine on Focal's glibc 2.31. The project's Intel path already requires Ubuntu 24.04, and its Docker image for older cards asks for driver 575, which is one way around an old host. If `strata launch --plan` reports that no engine could be started, move to Ubuntu 24.04 before spending more time. Driver 580 supports the P1000 there as well.

### 2.5 Disk

| Item | Size |
| --- | --- |
| Qwen3.6-35B-A3B UD-Q4\_K\_XL, shared by both engines | 22.9 GB |
| Small models: Qwen3.5-4B, Qwen3.5-9B, gpt-oss-20b, an embedding model | about 21 GB |
| Studio environment and llama.cpp build | about 10 GB |
| Flash-Next Q2\_0, only after a RAM and GPU upgrade | 66 GB plus a 28.8 GB lookup table |

Models downloaded in Studio land in the Hugging Face cache at `~/.cache/huggingface/hub`. Set `HF_HOME` before installing if they should live elsewhere. On the SATA SSD a 23 GB model takes roughly a minute to load.

## 3. Unsloth Studio: the prototype suite

Studio replaces Ollama, Open WebUI and LM Studio as the place where models are downloaded, compared, given tools and documents, and turned into training data. On this machine it does four of the five jobs you listed at full strength. The fifth, fine-tuning on the GPU, is the weak one, and 3.6 says why.

| Job you named | Studio feature | On the P1000 |
| --- | --- | --- |
| Model and weights manager | Model hub, quantization picker, Hugging Face cache | Works |
| Configuration testbed | `unsloth run` with llama-server flags, Model Arena, API monitor | Works. Arena loads the two models one after the other |
| Toolsmithing | Built-in `python`, `terminal` and `web_search` tools, self-healing tool calls | Works |
| Knowledge gathering and aggregating | Linked folders, Data Recipes | Works. Recipes run on the CPU |
| Post-training and fine-tuning | LoRA, QLoRA, full fine-tune, GGUF export | Not on this GPU by default |

### 3.1 Install, GGUF-only

```bash
curl -fsSL https://unsloth.ai/install.sh -o unsloth-install.sh
less unsloth-install.sh                      # read it before running it
export UNSLOTH_LLAMA_CPP_BACKEND=cuda
UNSLOTH_NO_TORCH=1 UNSLOTH_PYTHON=3.12 sh unsloth-install.sh
```

`UNSLOTH_NO_TORCH=1` skips PyTorch, which leaves chat, tools, the API and Data Recipes and removes training ([install options](https://unsloth.ai/docs/new/studio/install)). That is the right default here because current PyTorch no longer ships binaries for this GPU. The installer builds llama.cpp into `~/.unsloth/llama.cpp`. If that build does not come out with Pascal support, build llama-server yourself as in 6.1 and point `unsloth run` at models through the gateway instead.

```bash
unsloth studio -H 127.0.0.1 -p 8888          # web UI; sets a password on first visit
unsloth studio reset-password                # if you lose it
```

Keep it on 127.0.0.1. Anyone holding a Studio API key with tools enabled can run code on this machine. The documentation says the same and offers `--disable-tools` for exposed servers.

### 3.2 Models and weights

Download from the Model hub tab or straight from a terminal:

```bash
unsloth run --model unsloth/Qwen3.6-35B-A3B-MTP-GGUF:UD-Q4_K_XL
```

That one file, 22.9 GB, is the file Strata serves in production, so a download in Studio is also the production download. Starter set for this box:

| Model | File | Role | Where it runs |
| --- | --- | --- | --- |
| Qwen3.6-35B-A3B MTP, UD-Q4\_K\_XL | 22.9 GB | The brain | Dense layers on the GPU, experts in RAM |
| Qwen3.5-4B, Q4\_K\_M | 2.78 GB | Fast utility, router, judge | Fully on the GPU |
| Qwen3.5-9B, Q4\_K\_M | 5.63 GB | Mid-size second opinion | Mostly CPU |
| gpt-oss-20b | 12.11 GB | Second mixture-of-experts model for comparison | CPU with a GPU assist |
| embeddinggemma 300M | under 1 GB | Embeddings for retrieval | CPU |

### 3.3 Configuration testbed

`unsloth run` passes most llama-server flags through, which makes it the place to find the settings production will use ([API guide](https://unsloth.ai/docs/basics/api)):

```bash
unsloth run --model unsloth/Qwen3.6-35B-A3B-MTP-GGUF:UD-Q4_K_XL \
  -ngl 99 --cpu-moe -t 10 -c 8192 -np 1 -ctk q8_0 -ctv q8_0 \
  --temp 0.7 --top-p 0.8 --top-k 20
```

`-ngl 99 --cpu-moe` is the setting that matters on this machine: every layer is offered to the GPU, then the expert weights are held back in system RAM. The shared layers that every token passes through sit on the P1000, and the experts, of which each token uses 8 of 256, are read from RAM. Section 8 measures what that buys.

Work through this checklist before promoting a configuration:

1. Load with the flags above and note VRAM in `nvidia-smi`. Target 3.0 to 3.6 GB.
2. Send the three standard prompts in section 5 and record tokens per second from the API monitor.
3. Raise context to 16384 and repeat. If VRAM passes 3.8 GB, stay at 8192.
4. In Model Arena, compare against Qwen3.5-4B on your own ten prompts. If the small model is good enough, production is simpler than you think.

### 3.4 Toolsmithing

Studio runs tools on the server, so a tool written once works for the chat UI and for any API client. Tools are off unless the request asks for them:

```bash
curl -s http://127.0.0.1:8888/v1/chat/completions \
  -H "Authorization: Bearer $UNSLOTH_KEY" -H 'Content-Type: application/json' -d '{
    "model": "unsloth/Qwen3.6-35B-A3B-MTP-GGUF",
    "messages": [{"role":"user","content":"How many primes are there below 10000? Compute it."}],
    "enable_tools": true,
    "enabled_tools": ["python"]
  }' | jq -r '.choices[0].message.content'
```

Create the `sk-unsloth-…` key under Settings, API. Studio also repairs malformed tool calls before they fail, which the documentation calls auto-healing ([chat](https://unsloth.ai/docs/new/studio/chat)). That repair is a Studio feature. It does not travel to Strata or llama-server, so a tool schema that only works with healing is not ready for production. Test every tool once with the Studio tools off and your own client making the calls.

Built-in web search goes to DuckDuckGo. No setting for a SearXNG backend is documented. Section 6.3 keeps SearXNG as a tool the agent calls directly, and section 7 shows the tool definition.

### 3.5 Knowledge gathering and aggregating

Two features cover this, and they do different work.

**Linked folders** are for reading. Add a folder as a source and Studio indexes it, then rechecks it every 30 seconds for added, changed and deleted files. The documented limit is 10,000 files per folder. Point it at this project's `docs/` and the model answers from them.

**Data Recipes** are for building datasets. A recipe is a graph of blocks: seed data, samplers, LLM text, LLM structured output, LLM code, an LLM judge that scores rows, expressions and validators ([data recipes](https://unsloth.ai/docs/new/studio/data-recipe)). A model provider in a recipe is any OpenAI-compatible endpoint, so recipes can generate with the production brain through the gateway at `http://127.0.0.1:4000/v1`.

A recipe that turns documents into a training set, in outline:

1. Seed block: the documents, chunked.
2. LLM structured block: for each chunk, write three question and answer pairs as JSON.
3. LLM judge block: score each pair for faithfulness to the chunk, 1 to 5.
4. Expression block: keep rows scoring 4 or higher.
5. Export as JSONL.

Recipes are stored in the browser, not on disk. Export every recipe you care about and commit the file to git, or a cleared browser profile takes it with it. At about 14 tokens per second, 1,000 pairs with judging is an overnight job.

### 3.6 Fine-tuning, and why not on this card

PyTorch 2.14 is the last release with prebuilt binaries for Pascal cards, on the CUDA 12.6 index only. From 2.15 the project stopped publishing them ([PyTorch notice](https://dev-discuss.pytorch.org/t/notice-cuda-12-6-wheels-will-no-longer-be-published-from-pytorch-2-15-drops-maxwell-pascal-volta/3432)). Add 4 GB of VRAM and the practical ceiling is a model of about 1B parameters in 4-bit. Three routes, in the order I would try them:

| Route | Cost | What it trains | Status |
| --- | --- | --- | --- |
| Free Colab Studio notebook, T4 16 GB | $0 | Up to about 22B per the Unsloth docs | Documented by Unsloth |
| Rented GPU for a few hours | A few dollars per run | Anything up to the card's size | Standard practice |
| P1000 with pinned PyTorch 2.14 | $0 | Models of about 1B parameters, QLoRA | Experiment. Not tested |

The pinned experiment, if you want to try it:

```bash
UNSLOTH_STUDIO_HOME=~/.unsloth-train \
UNSLOTH_TORCH_INDEX_URL=https://download.pytorch.org/whl/cu126 \
UNSLOTH_PYTHON=3.12 sh unsloth-install.sh
```

It installs a second Studio beside the first so a failure costs nothing. Whether the current Unsloth release accepts PyTorch 2.14 is not something I could confirm.

Whichever route trains the adapter, the way back is the same: export as GGUF from Studio's export page, copy the file to this machine, and it enters the promotion path in section 5 like any other model. One limit to know now: Strata serves three model families and nothing else. A fine-tuned small model goes to production on llama-server behind the gateway, not on Strata.

## 4. Strata: the production engine

Strata is an MIT-licensed inference engine written for one job: running large mixture-of-experts models on a consumer graphics card by keeping the layers every token uses on the GPU and the experts, of which each token needs only a few, in system RAM ([repository](https://github.com/Niko1221/Strata)). It serves one model per engine, answers up to 8 requests at once, and speaks the OpenAI and Anthropic APIs on `127.0.0.1:8080`.

### 4.1 What it documents, against what you have

|  | Strata's documentation | This P340 |
| --- | --- | --- |
| Graphics card | RTX 20 to 50 supported. Pascal through an experimental CUDA 12 engine. 12 GB recommended, 8 GB reported by users | Quadro P1000, Pascal, 4 GB |
| Smallest card it mentions | A 2 GB GT 1030 "cannot hold any of the model" | 4 GB sits between that and the smallest working reports |
| Driver | 580 for CUDA 13 engines, 525 on Linux for the CUDA 12 engine | 580 |
| RAM, Qwen3.6-35B-A3B UD-Q4\_K\_XL | Setup plans 32 GB | 32 GB. No slack |
| RAM, Flash-Next Q2\_0 | Experts 34 GB plus about 10 GB | Does not fit in 32 GB, and 48 GB is marginal |
| Model families | Qwen3.8-Flash-Next, Qwen3.6-35B-A3B, DeepSeek V4.1-Flash | One of the three fits in RAM |

Sources: [older GPUs](https://github.com/Niko1221/Strata/blob/main/docs/OLDER_GPUS.md), [models](https://github.com/Niko1221/Strata/blob/main/docs/MODELS.md), [Qwen3.6 guide](https://github.com/Niko1221/Strata/blob/main/docs/QWEN36.md).

The evidence for Pascal is real but comes from bigger cards: a Tesla P40 with 24 GB, and a Tesla P100 with 16 GB that the maintainers own and test on. Nobody has published a run on 4 GB. That makes this machine an experiment, and the next two steps are how to run it cheaply.

### 4.2 Install

```bash
mkdir -p ~/src/strata-install && cd ~/src/strata-install
curl -fsSLO https://github.com/Niko1221/Strata/releases/latest/download/install.sh
curl -fsSLO https://github.com/Niko1221/Strata/releases/latest/download/SHA256SUMS.txt
sha256sum --ignore-missing -c SHA256SUMS.txt
less install.sh
sh install.sh
strata --version          # 0.2.13 was current on 9 November 2026
```

The installer needs no root and puts `strata` on your PATH. The engine itself is fetched on first launch, matched to the card, signature-checked and test-started ([launch reference](https://github.com/Niko1221/Strata/blob/main/docs/TERMINAL_LAUNCH.md)).

### 4.3 The test that costs nothing

`--plan` shows what a launch would do and exits. It downloads nothing and starts nothing.

```bash
strata launch --plan --family qwen36 --model UD-Q4_K_XL \
  --context 8192 --parallel 1 --mtp off --vision off
```

`make plan` in the companion package runs this and two variants and saves the output with the driver and memory state beside it.

| What you see | What it means | Do this |
| --- | --- | --- |
| "Strata couldn't find an engine for this computer" | No released engine starts here. On this box the likely causes are Focal's glibc or the card class | Stay on the llama-server lane. Retest after moving to Ubuntu 24.04 or through Strata's Docker image |
| The model is marked "does not fit this computer" | A warning. Strata does not block on it | Try one launch. Expect the engine to refuse at allocation, with numbers |
| "tight" or "fits" | Strata accepts the card | Launch as in 4.4 and run the acceptance test in section 5 |

One trap: when no engine is found, `strata launch` still exits with code 0 and starts nothing. Scripts must check the endpoint, not the exit code. The systemd unit in the package does.

### 4.4 Launch settings for this machine

```bash
export STRATA_API_KEY=$(openssl rand -hex 24)
strata launch --family qwen36 --model UD-Q4_K_XL \
  --model-path "$BRAIN_GGUF" \
  --context 8192 --kv int8 --parallel 1 --mtp off --vision off \
  --tune skip --access local --port 8080 \
  --api-key-env STRATA_API_KEY \
  --save-preset p340 --no-tui
```

| Flag | Value here | Why |
| --- | --- | --- |
| `--model-path` | The GGUF Studio downloaded | One copy of the weights on disk. Pointing at the Hugging Face cache path is untested; copy the file out of the cache if Strata rejects the symlink |
| `--context` | 8192 | Conversation memory costs about 45 KB per token on the card. 8K is 0.37 GB, 32K is 1.5 GB and does not fit |
| `--kv` | `int8`, or `q4_0` if short | `q4_0` halves conversation memory |
| `--parallel` | 1 | Every slot takes its own conversation memory from the same 4 GB |
| `--mtp` | off | The draft layer needs about 1 GB of VRAM. It is Strata's biggest single speedup, 1.4x on a 12 GB card, and this card cannot afford it |
| `--vision` | off | The picture reader would run on the CPU and take 0.9 GB of RAM this box does not have spare |
| `--tune` | skip first | Tuning takes 20 minutes to 2 hours. Run `--tune quick` only after a plain launch works |
| `--access` | local | Binds to 127.0.0.1. The gateway is the only client |

After the first start:

```bash
strata launch --status              # what is serving, with which settings
strata launch --preset p340 --yes --no-tui
strata launch --stop
```

### 4.5 Calling it

```bash
AUTH="Authorization: Bearer $STRATA_API_KEY"
MODEL=$(curl -s http://127.0.0.1:8080/v1/models -H "$AUTH" | jq -r '.data[0].id')

# OpenAI style
curl -s http://127.0.0.1:8080/v1/chat/completions \
  -H "$AUTH" -H 'Content-Type: application/json' -d '{
    "model": "'"$MODEL"'",
    "messages": [{"role":"user","content":"Name three uses of a bloom filter."}],
    "reasoning_effort": "low",
    "max_tokens": 300
  }' | jq -r '.choices[0].message.content'

# Anthropic style
curl -s http://127.0.0.1:8080/v1/messages \
  -H "x-api-key: $STRATA_API_KEY" -H 'anthropic-version: 2023-06-01' \
  -H 'Content-Type: application/json' -d '{
    "model": "'"$MODEL"'", "max_tokens": 300,
    "messages": [{"role":"user","content":"Name three uses of a bloom filter."}]
  }' | jq -r '.content[0].text'
```

`reasoning_effort` takes `none`, `low`, `medium` or `high`. On a machine this slow it is the most useful dial you have: `none` for routing and extraction, `low` for chat, `high` only when a wrong answer costs more than a two-minute wait.

Clients should not carry Strata's model name. Put the name `/v1/models` reports into the `model` field of the Strata entry in `etc/routes.json`, and the gateway rewrites the alias `brain` to it on the way through. Left empty, the gateway forwards `brain` unchanged, which llama-server accepts. Whether Strata accepts a name it does not know was not tested.

Strata keeps a conversation cache with checkpoints, so a follow-up turn only reads the new text. That is why the agent-step column in section 8 is so much shorter than the cold-document column, and why agents on this box should append to a conversation instead of rebuilding the prompt each step.

### 4.6 Operating it

| Task | Command |
| --- | --- |
| Wire a coding agent | `strata connect opencode --write`, then `strata connect opencode --check` |
| List models Strata can see | `strata models list` |
| Check files you already have | `strata models import PATH --plan` |
| Look for an update | `strata update --check` |
| Update, and undo an update | `strata update`, `strata components rollback` |

Three things to hold in mind when calling this the production system:

- **It is eight weeks old.** First release 24 September 2026, releases every few days, one core maintainer, no outside security audit. Update on your schedule, not its schedule: check, read the notes, update, rerun the acceptance test, and roll back if a number drops.
- **It is local by default.** It binds to 127.0.0.1, sends no telemetry, and its update feed is signed. `--access lan` creates an API key. Without a key, anyone who can reach the port can use the model.
- **One engine holds one model.** A second `--port` runs a second engine beside the first, and on this machine there is no memory for one. Multi-model work goes through the gateway to other processes, covered in sections 6 and 9.

## 5. Promotion: from Studio to production

A model is promoted when a recorded test says it is ready, not when it feels good in a chat window. The path has four rungs and one file per model.

| Rung | Where it runs | It moves up when |
| --- | --- | --- |
| Candidate | Unsloth Studio | It fits in memory with the flags in 3.3 and answers your own prompts well |
| Qualified | Studio, tools off, called by your own client | Tool calls are well formed 10 times out of 10 without Studio's repair |
| Production, llama-server lane | `make prod-llama`, through the gateway | It passes the acceptance test below |
| Production, Strata lane | `make prod-strata`, through the gateway | Strata accepts the machine, and the same test is at least as fast as on llama-server |

The last rung is optional by design. Clients call the alias `brain` on the gateway and cannot tell which engine answered, so moving between the last two rungs changes nothing for them.

### 5.1 One manifest per model

Keep a small file in git for every model that reaches Qualified. It is the record of what was promoted and why.

```json
{
  "alias": "brain",
  "repo": "unsloth/Qwen3.6-35B-A3B-MTP-GGUF",
  "file": "Qwen3.6-35B-A3B-MTP-UD-Q4_K_XL.gguf",
  "sha256": "<output of sha256sum on the file>",
  "license": "Apache-2.0",
  "flags": "-ngl 99 --cpu-moe -t 10 -c 8192 -np 1 -fa on -ctk q8_0 -ctv q8_0 --jinja",
  "accepted": {"date": "", "engine": "", "chat_s": 0, "step_s": 0, "doc_s": 0, "vram_gb": 0}
}
```

The exact file name is whatever Studio downloaded. Record the checksum on the day you qualify the model, and check it again before each promotion. A changed checksum means a changed model.

### 5.2 The acceptance test

Three turns, the same three the simulation in section 8 uses, so measured numbers can be laid beside modelled ones.

| Turn | Prompt | Output | What it stands for |
| --- | --- | --- | --- |
| Chat turn | 1,500 tokens | 300 tokens | A question with some pasted context |
| Agent step | 500 new tokens on a cached conversation | 150 tokens | One tool result and the next action |
| Cold document | 6,000 tokens | 500 tokens | Summarize a file that fills most of the 8K window |

```bash
turn() {  # turn <alias> <prompt-file> <max_tokens>
  local t0 t1 out
  t0=$(date +%s.%N)
  out=$(jq -n --arg m "$1" --rawfile p "$2" --argjson n "$3" \
          '{model:$m, max_tokens:$n, messages:[{role:"user", content:$p}]}' |
        curl -s http://127.0.0.1:4000/v1/chat/completions \
          -H "Authorization: Bearer $GATE_KEY" -H 'Content-Type: application/json' -d @-)
  t1=$(date +%s.%N)
  jq -r --arg s "$(echo "$t1 - $t0" | bc)" \
    '"\(.usage.prompt_tokens) in, \(.usage.completion_tokens) out, \($s) s"' <<<"$out"
}

turn brain prompts/chat-1500.txt 300
turn brain prompts/doc-6000.txt  500
nvidia-smi --query-gpu=memory.used --format=csv,noheader
```

Build the prompt files from your own material, once, and keep them in the repository. Run each turn three times and keep the median.

| Gate | Pass |
| --- | --- |
| Memory | VRAM at or under 3.8 GB after the document turn. At least 3 GB of RAM still available in `free -g` |
| Speed | Chat turn in 45 seconds or less. Document turn in 2 minutes or less |
| Agreement with the model | Within 30% of the row for that pattern in section 8. A larger gap means a setting is wrong or the model is wrong; find out which |
| Tools | 10 of 10 well-formed calls to your two most used tools |
| Soak | 30 minutes of chat turns in a loop with no growth in VRAM or resident memory |
| Failover | Stop the brain's unit mid-loop. The gateway answers 502 until a lane is back, then recovers with no client change |

### 5.3 Promote and roll back

```bash
make proto          # stop production brains, free memory for Studio
make prod-llama     # production on llama-server
make prod-strata    # production on Strata
make status         # what is running, what the GPU holds, what the gateway lists
```

Rolling back is running the other target. The two brain units conflict in systemd, so starting one stops the other, and the 23 GB of weights is never loaded twice.

### 5.4 Models that Strata cannot serve

Strata serves three families. Everything else you qualify, including any small model you fine-tune, is promoted to a llama-server unit of its own and given an alias in `etc/routes.json`:

```json
"triage": [
  {"url": "http://127.0.0.1:8082", "model": "", "key_env": "LLAMA_API_KEY"}
]
```

The gateway then presents Strata's model and every llama-server model as one endpoint with one key. That gateway, not Strata, is where the multi-model system lives.

## 6. The supporting apps

Strata serves one model. Four small pieces turn that into a system: a second engine for everything Strata cannot run, a gateway that gives them one address, a search service, and an embedding lane.

| Piece | Why it is needed | Replaces from the May guide |
| --- | --- | --- |
| llama-server | Fallback brain on the same weights. Host for every model outside Strata's three families | Ollama |
| `gate` | One endpoint, one key, aliases, failover | LiteLLM with Redis and Postgres |
| `council` | Voting across models or samples | The asyncio council script |
| SearXNG | Private web search as a tool | Unchanged, moved to port 8889 |
| Embedding lane | Retrieval outside Studio | `nomic-embed-text` in Ollama |

### 6.1 llama-server, built for this card

```bash
git clone https://github.com/ggml-org/llama.cpp ~/llama.cpp && cd ~/llama.cpp
cmake -B build -DCMAKE_BUILD_TYPE=Release -DGGML_CUDA=ON \
  -DCMAKE_CUDA_ARCHITECTURES=61 \
  -DCMAKE_CUDA_COMPILER=/usr/local/cuda-12.9/bin/nvcc
cmake --build build -j 10 --target llama-server
./build/bin/llama-server --version
```

`CMAKE_CUDA_ARCHITECTURES=61` compiles for Pascal only. This build was not run on Focal; it needs the newer CMake from 2.3, and GCC 9 is at the old end of what current llama.cpp expects. If the compile fails on the compiler, install `gcc-11` from the toolchain PPA and pass `-DCMAKE_C_COMPILER=gcc-11 -DCMAKE_CXX_COMPILER=g++-11`.

The production unit starts it like this:

```bash
llama-server -m "$BRAIN_GGUF" --alias brain \
  -ngl 99 --cpu-moe -t 10 -c 8192 -np 1 -fa on -ctk q8_0 -ctv q8_0 --jinja \
  --host 127.0.0.1 --port 8081 --api-key "$LLAMA_API_KEY"
```

| Flag | Meaning on this machine |
| --- | --- |
| `-ngl 99 --cpu-moe` | All layers to the GPU, then expert weights back to RAM. About 2 GB of shared layers stay on the card |
| `-t 10` | The i9-10900's physical cores. Hyperthreads do not help memory-bound work |
| `-c 8192 -np 1` | One conversation of 8K. Slots divide the context, so `-np 3` gives three of 2,730 |
| `-fa on -ctk q8_0 -ctv q8_0` | Flash attention with 8-bit conversation memory, half the VRAM of the default |
| `--jinja` | Use the chat template in the GGUF. Tool calling depends on it |

`--n-cpu-moe N` is the finer control: it keeps the experts of the first N layers in RAM and puts the rest on the card. Use it when there is VRAM to spare, never when you are short. To free VRAM, lower `-c` first, then lower `-ngl` below the layer count.

### 6.2 gate, the alias gateway

`gate` is 165 lines of Go on the standard library. It reads `etc/routes.json`, checks one bearer key, looks at the `model` field of each request, and forwards to the first upstream for that alias that answers. A connection error or a 5xx moves it to the next. It streams responses through and reports which upstream answered in an `X-Gate-Upstream` header.

```json
{
  "brain": [
    {"url": "http://127.0.0.1:8080", "model": "", "key_env": "STRATA_API_KEY"},
    {"url": "http://127.0.0.1:8081", "model": "", "key_env": "LLAMA_API_KEY"}
  ],
  "embed": [
    {"url": "http://127.0.0.1:8083", "model": "", "key_env": ""}
  ]
}
```

```bash
make build test                       # compile; run the unit tests against mock servers
GATE_KEY=... bin/gate -listen 127.0.0.1:4000 -routes etc/routes.json
curl -s http://127.0.0.1:4000/v1/models -H "Authorization: Bearer $GATE_KEY" | jq
```

It is small on purpose. The May guide used LiteLLM with Redis and Postgres for this job, which is three services and a Python environment to hold one routing table. `gate` is one static binary with no dependencies, it fits the project's Go rules, and it refuses to listen on anything but loopback unless a key is set. What it leaves out is budgets, per-user keys and usage accounting. If you need those, LiteLLM is still the right tool and it can sit where `gate` sits.

### 6.3 SearXNG

Studio took port 8888, so SearXNG moves to 8889. The install is the May guide's:

```bash
mkdir -p ~/searxng/core-config && cd ~/searxng
curl -fsSL \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/docker-compose.yml \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/.env.example
cp .env.example .env
echo "SEARXNG_SECRET=$(openssl rand -hex 32)" >> .env
docker compose up -d
```

Publish the container on `127.0.0.1:8889` in the compose file's `ports` entry, then set these in `core-config/settings.yml` and restart:

```yaml
use_default_settings: true
search:
  formats: [html, json]     # without json, every API call gets a 403
server:
  limiter: false            # local use only
  public_instance: false
  method: "GET"
```

```bash
curl -s -G http://127.0.0.1:8889/search \
  --data-urlencode 'q=strata local inference' --data-urlencode 'format=json' \
  -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64)' | jq '.results[:3][] | {title, url}'
```

### 6.4 Embedding lane

```bash
llama-server -m "$EMBED_GGUF" --alias embed --embeddings \
  -ngl 0 -t 4 -c 2048 --host 127.0.0.1 --port 8083
```

It runs on the CPU so that it never competes with the brain for the 4 GB. Studio has its own embedding path for linked folders; this lane is for your own retrieval code and for agents.

### 6.5 What is optional now

- **Open WebUI** still works as a chat front end. Point it at `http://127.0.0.1:4000/v1` with the gateway key and it sees the aliases as models. Studio's chat covers the same ground during prototyping.
- **Ollama** is no longer needed. If you keep it, give it its own alias in `routes.json` on port 11434 and remember it holds its own copy of every model.
- **LM Studio** has no role left. Studio does its job and exports to the same GGUF files.

## 7. Tools, MCP and agents

The model in production is an API that returns text and tool calls. It runs no tools itself. Where the tools run decides whether an agent survives the move from Studio to Strata.

| Where the tool runs | Use it for | Survives promotion |
| --- | --- | --- |
| Inside Studio (`enable_tools`) | Trying an idea in minutes | No. Studio-only |
| In the client, as function calls | Production agents | Yes. Any engine behind the gateway |
| In an MCP server | Tools shared by several agents and editors | Yes. The agent hosts it, not the engine |

The rule that follows: prototype with Studio's built-in tools, then rewrite anything you keep as a client-side function or an MCP server before promotion.

### 7.1 A tool loop that travels

`agent` in the companion package is that rewrite for web research: 285 lines of Go, standard library only. It offers the model two functions and runs them itself.

```json
[
 {"type":"function","function":{"name":"web_search",
  "description":"Search the web. Returns the top results with title, URL and a snippet.",
  "parameters":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}},
 {"type":"function","function":{"name":"fetch_url",
  "description":"Fetch one web page and return its text.",
  "parameters":{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}}}
]
```

```bash
export GATE_KEY=...
bin/agent "What changed in the latest Strata release?"
bin/agent -url http://127.0.0.1:8888 -key-env UNSLOTH_KEY -model unsloth/Qwen3.6-35B-A3B-MTP-GGUF \
  "Same question, asked of Studio"
```

Three design choices in it are forced by this hardware and worth copying into any agent you write for it:

- **A size budget, not only a step budget.** Tool output is capped at 16,000 characters per run, about 4,000 tokens, and each page at 6,000 characters. When the budget is spent the next call offers no tools, so the model must answer with what it has. On an 8K context an agent without this cap fails on its third page.
- **Short schemas.** Each tool description is one sentence. Schema tokens are read on every step.
- **A guarded fetch.** `fetch_url` refuses loopback, private and link-local addresses at the moment of dialling, so a page that redirects to `http://127.0.0.1:8080` cannot make the model talk to its own engine. `-allow-private` turns the guard off for an intranet.

### 7.2 MCP

MCP servers plug into the agent, never into Strata. The configuration shape is the same one the May guide showed, with the search port corrected:

```json
{
  "mcpServers": {
    "filesystem": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/you/projects"]},
    "git":        {"command": "uvx", "args": ["mcp-server-git", "--repository", "/home/you/repo"]},
    "searxng":    {"command": "npx", "args": ["-y", "mcp-searxng"], "env": {"SEARXNG_URL": "http://127.0.0.1:8889"}}
  }
}
```

Every MCP server adds its tool schemas to every request. Three servers with a dozen tools can cost 2,000 tokens before the first user word, a quarter of this machine's context. Load only the servers a task needs. In Studio, Data Recipes take MCP servers as tool profiles for generation blocks. Studio's general MCP settings page was not read for this guide.

### 7.3 Coding agents

Both tools can write an agent's configuration for you.

```bash
# Prototype: Studio serves the model
unsloth start opencode

# Production: Strata serves the model
strata connect opencode --write
strata connect opencode --check      # one prompt and one tool call, the way that app sends them
```

`strata connect` lists OpenCode, Codex CLI, Claude Code, oh-my-pi, pi, Hermes Agent and OpenClaw. Its configurations were tested at 32K context, and this machine serves 8K, so expect to lower each agent's context setting by hand. Claude Code needs `CLAUDE_CODE_MAX_CONTEXT_TOKENS` set to match, and Hermes refuses windows under 64K unless configured otherwise ([launch reference](https://github.com/Niko1221/Strata/blob/main/docs/TERMINAL_LAUNCH.md)).

To keep an agent engine-neutral, point it at the gateway instead:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:4000     # no /v1
export ANTHROPIC_AUTH_TOKEN=$GATE_KEY
export ANTHROPIC_MODEL=brain
export CLAUDE_CODE_MAX_CONTEXT_TOKENS=32768
```

The gateway passes Anthropic-style requests through to whichever lane is up.

Be realistic about what fits. Full coding agents send a system prompt and tool schemas that typically run past 10,000 tokens before your first word, so they do not fit this machine's 8K production window at all. There are two ways to give them room, and both cost something:

- **Move conversation memory to system RAM.** On the llama-server lane, `-c 32768 --no-kv-offload` keeps the shared layers on the card and the conversation in RAM. It is slower per token and was not measured here. The first turn reads the whole system prompt, about three minutes at the modelled 87 tokens per second, and later turns reuse it from cache.
- **Use a lighter harness.** `bin/agent`, or a coding agent configured with a short system prompt and two or three tools.

Comfortable agentic coding starts at the Paper tier in Appendix A.

### 7.4 What breaks on small contexts, and the fix

| Symptom | Cause | Fix |
| --- | --- | --- |
| Agent loops on the same tool | The tool result was cut and the model cannot see the end | Lower the page cap so results are whole, not truncated mid-thought |
| Reply stops mid-sentence | Context full; stop reason `length` | Fewer tools, shorter system prompt, or summarize and restart the conversation |
| Malformed tool call | Template or grammar mismatch | Confirm `--jinja` on llama-server. On Strata, run `strata connect <app> --check` |
| Each step takes as long as the first | The client rebuilds the prompt and defeats the conversation cache | Append to the conversation; never reorder or rewrite earlier messages |

## 8. Five simulated configurations

I do not have the P340, so nothing in this section is a benchmark. Two kinds of test were run instead:

- **Modelled:** memory fit and turn time for five patterns, computed by `p340sim` from published figures and a calibration against P1000 measurements. The source ships with the guide so you can replace my constants with yours.
- **Executed:** the gateway, council and agent programs, built with Go 1.24.7 and run against mock model servers. Failover, key checks, alias rewriting, majority voting and the tool loop all pass their tests. This proves the plumbing, not the speed.

### 8.1 How the model works

Writing a token means reading weights from memory, and on this machine memory speed is the limit. So decode time per token is the bytes read for that token, divided by the effective bandwidth of the pool they sit in. Reading a prompt is limited by arithmetic instead, at 2 floating-point operations per active parameter per token.

| Constant | Value | Status |
| --- | --- | --- |
| P1000 effective bandwidth | 41.5 GB/s, plus 3.3 ms fixed cost per token | Fitted to five P1000 speeds quoted in the May guide. It is 52% of the card's 80 GB/s rating |
| CPU effective bandwidth | 25.8 GB/s at DDR4-2933, 18.8 GB/s at DDR4-2133 | 55% of dual-channel theoretical. Matches the May guide's CPU rule of thumb |
| Qwen3.6-35B-A3B, bytes read per token | 1.9 GB shared layers plus 8 of 256 experts, 0.61 GB | Published by Strata |
| Conversation memory | 45 KB per token; engine reserve 0.83 GB; draft layer 1.0 GB | Published by Strata |
| Strata against llama.cpp, same placement | 1.2x; drafts a further 1.42x | Measured by Strata on an RTX 3060 12 GB. Carried to this card as an assumption |
| Prompt-reading rate | CPU 0.42 TFLOPS, GPU half its 1.9 TFLOPS peak | Assumption. The least certain numbers in the model |
| Desktop and OS | 4 GB of RAM, 0.5 GB of VRAM | Assumption |

The fitted line reproduces its own calibration points within 14%: 53.5 modelled against 62 measured tokens per second at the small end, 17.8 against 19 at the large end.

As a second check, the same arithmetic was pointed at five speeds that Strata published for other machines:

| Machine and engine | Model | Modelled, tok/s | Published, tok/s |
| --- | --- | --- | --- |
| RTX 3060 12 GB, Strata, no drafts | Qwen3.6-35B-A3B | 55 | 59.7 |
| RTX 3060 12 GB, llama.cpp | Qwen3.6-35B-A3B | 46 | 48 to 52 |
| RTX 5070 12 GB, 64 GB DDR5, Strata with drafts | Flash-Next Q2\_0 | 84 | 87.3 |
| R9700 32 GB, every expert on the card, Strata, no drafts | Qwen3.6-35B-A3B | 111 | 83.9 |
| R9700 32 GB, llama.cpp | Qwen3.6-35B-A3B | 92 | 73.8 |

The model lands within 12% where a small card shares the work with the CPU, which is this machine's situation, and runs 25 to 32% fast when a whole model sits on one quick card. The memory speed of those test machines was not published, so DDR5-5600 and DDR5-6000 were assumed; treat the agreement as encouraging, not as proof.

### 8.2 The five patterns

|  | Pattern | The idea |
| --- | --- | --- |
| A | Strata single brain | Qwen3.6-35B-A3B, shared layers on the P1000, experts in RAM, no drafts. A+ turns drafts on |
| B | llama-server single brain | The same file and placement on llama-server. B-cpu leaves the GPU empty |
| C | Router and brain | Qwen3.5-4B on the GPU takes 70% of steps. The big model runs on the CPU and takes the rest |
| D | Council | Three different models propose on the CPU, a fourth on the GPU aggregates. D-self instead takes three samples of the brain in three slots and votes |
| E | The 16 GB stick | 48 GB at DDR4-2133, running Flash-Next Q2\_0 on Strata. E-B reruns pattern B on that memory |

### 8.3 Results

Times are seconds for a whole turn. The three turns are the ones defined in 5.2.

| ID | RAM, GB | VRAM, GB | Decode, tok/s | Prompt, tok/s | Chat turn | Agent step | Cold document | Fit |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A | 25.0 of 32 | 3.60 of 4 | 16.5 | 104 | 33 | 14 | 88 | Tight |
| A+ | 25.0 of 32 | 4.60 of 4 | 23.5 | 104 | 27 | 11 | 79 | No: VRAM |
| B | 25.0 of 32 | 3.07 of 4 | 13.8 | 87 | 39 | 17 | 105 | Fits |
| B-cpu | 26.9 of 32 | 0.50 of 4 | 9.9 | 60 | 55 | 23 | 149 | Fits |
| C | 26.9 of 32 | 3.59 of 4 | 13.0 | 101 | 40 | 17 | 149 | Tight |
| D | 24.5 of 32 | 3.59 of 4 | 14.2 | 118 | 293 | 123 | Over context | Tight |
| D-self | 25.0 of 32 | 3.25 of 4 | 20.0 | 87 | 97 | 40 | Over context | Fits |
| E | 38.3 of 48 | 3.91 of 4 | 10.2 | 38 | 69 | 28 | 206 | Tight |
| E-B | 25.0 of 48 | 3.07 of 4 | 12.3 | 87 | 42 | 18 | 110 | Fits |

Rates for C are the 70/30 blend, for D the aggregator's, and for D-self the three-slot total. "Over context" means the turn does not fit the window that pattern can afford on a 4 GB card.

### 8.4 What the numbers say

1. **Give the 4 GB to the big model's shared layers.** With those layers on the P1000, the 35B mixture-of-experts model writes about 14 tokens per second, the same speed as a 4B model that fills the card. It reads about as many bytes per token, 2.5 GB against 2.8 GB, and it is a far stronger model. That is the main result, and it is why the May guide's small-model lineup is gone.
2. **A router on the GPU buys nothing here.** Pattern C is no faster than B on short turns and 40% slower on documents, because the card can hold the router or the brain's shared layers, not both.
3. **A mixed council costs about five minutes a turn.** Pattern D takes 7.5 times as long as B. Three models share one memory bus, so their times add. Self-consistency on slots, D-self, costs 2.5 times B and needs only one model in memory. If you want a vote on this box, vote that way, and only on questions with a checkable answer.
4. **Strata's edge here is small and uncertain.** Its large gains come from caching experts on the card and from draft tokens. This card has room for neither: A+ needs 4.6 GB. What remains is a kernel advantage of about 15% on turn time, measured on a different card, on an engine that has never been reported running on 4 GB.
5. **The 16 GB stick costs speed and buys little.** At DDR4-2133 the same brain runs 11% slower, and CPU-only lanes about 26% slower. Flash-Next Q2\_0 would fit by this model's arithmetic at about 10 tokens per second, but Strata's guidance puts that model at the edge of what 48 GB holds, and it assumes a card three times this size. Treat E as a dead end. A matched DDR4-2933 pair is the right RAM upgrade.

### 8.5 Verdicts

| Pattern | Use it? |
| --- | --- |
| B, llama-server single brain | Yes. The default production lane on the stock machine |
| A, Strata single brain | If `strata launch --plan` accepts the card and the acceptance test beats B |
| D-self, self-consistency | For closed-form questions where a wrong answer is expensive |
| C, router and brain | No. Revisit with 12 GB of VRAM or more |
| D, mixed council | No, unless a 5-minute turn is acceptable for the task |
| E, 48 GB with the slow stick | No |

### 8.6 What the model leaves out

It says nothing about answer quality. On that, the published evidence the May guide cited still stands: a mixture of agents can beat its best member on open-ended tasks, and several samples of one strong model often beat a mixed council. Neither result was reproduced here.

It also ignores thermal throttling in a small chassis, competition between CPU threads when lanes overlap, and tool latency. The prompt-reading constants are the weakest part; if your measured prompt rate is half the modelled one, the cold-document column doubles and nothing else changes much.

To replace the model with measurements:

```bash
make sim                 # rerun and rewrite sim/results.md and sim/results.json
$EDITOR cmd/p340sim/main.go   # constants are at the top of main(), each with its source
```

Then run the acceptance test in 5.2 and put the measured row beside the modelled one.

## 9. Five Strata deployment patterns

Each case study gives the Strata configuration as its documentation supports it, then says plainly what the stock P340 does with it today. Every client talks to the gateway alias, so each pattern runs unchanged on the llama-server lane when Strata is not available, at the speeds in section 8.

| # | Pattern | Strata feature it leans on | Stock P340 | Works as designed from |
| --- | --- | --- | --- | --- |
| 1 | Single-brain coding desk | `strata connect`, conversation cache | Degraded: light harness only | Paper tier |
| 2 | Research desk | `reasoning_effort`, conversation cache | Works, about 90 seconds a question | Stock |
| 3 | Sub-agent harness | `--parallel` slots | One slot on Strata; three small slots on llama-server | Paper tier, better at Bronze |
| 4 | Verification council | Slots plus a second engine | Self-consistency only, about 100 seconds a vote | Bronze tier |
| 5 | Master and workers | `--access lan` or `tailscale`, API keys | Works as the master today | Any remote Strata host |

### 9.1 Single-brain coding desk

**Purpose.** One strong model behind a coding agent, with the agent's configuration written and checked by Strata.

```bash
strata launch --family qwen36 --model UD-Q4_K_XL --context 32k --parallel 1 \
  --mtp on --tune quick --access local --api-key-env STRATA_API_KEY \
  --save-preset code --no-tui
strata connect opencode --write && strata connect opencode --check
```

**What Strata measured.** On an RTX 3060 12 GB in the hybrid placement, 59.7 tokens per second, and 84.7 with drafts, against 48 to 52 for llama.cpp on the same file ([Qwen3.6 guide](https://github.com/Niko1221/Strata/blob/main/docs/QWEN36.md)).

**On the stock P340.** 32K of context and the draft layer bring the modelled VRAM need to about 5.7 GB against the card's 4. Run the 8K preset from 4.4 and a light harness, or the 32K llama-server variant in 7.3, and expect single-file work only.

**Works as designed from.** A 12 GB card: the Paper tier.

### 9.2 Research desk

**Purpose.** Questions answered from live web pages, with sources, without the question or the pages leaving the machine except as search queries.

**Layout.** `bin/agent` calls the alias `brain` on the gateway and runs its two tools against SearXNG on port 8889.

```bash
make prod-strata            # or prod-llama
bin/agent -steps 6 -budget 16000 \
  "Which PyTorch release last shipped binaries for Pascal GPUs, and what replaced them?"
```

**Tuning.** Set `reasoning_effort` to `low` for the search and fetch steps and `medium` for the final answer if you extend the agent. Keep the conversation append-only so Strata's cache holds: each step then reads only the new tool result.

**On the stock P340.** A typical run is four model calls: read the question, read five search results, read two pages of about 1,500 tokens each, write 400 tokens. That is about 4,500 tokens in context, 4,000 of them read once at 87 per second, and 520 written at 13.8 per second: roughly 85 seconds on the llama-server lane, and about 70 on Strata if it runs. This pattern fits the machine as it is.

### 9.3 Sub-agent harness

**Purpose.** A parent splits a task into independent questions, workers answer them at the same time inside one engine, and the parent combines the results.

```bash
strata launch --family qwen36 --model UD-Q4_K_XL --context 8192 --parallel 4 \
  --access local --api-key-env STRATA_API_KEY --save-preset fanout --no-tui

mkdir -p out
printf '%s\n' "What does Strata require of a GPU?" \
              "What does Unsloth Studio require of a GPU?" \
              "What changed for Pascal cards in PyTorch 2.15?" > sub.txt
xargs -P 4 -I{} sh -c 'bin/agent -steps 4 -budget 8000 "$1" > "out/$(printf %s "$1" | sha1sum | cut -c1-8).txt"' _ {} < sub.txt
bin/agent -steps 1 "Combine these findings into one short brief. $(cat out/*.txt)"
```

`-steps 1` offers no tools, so the last call only writes.

**What Strata measured.** Slots share the dense weights and every expert two conversations both route to. On the RTX 3060 12 GB without drafts, total output was 64.0, 85.1 and 99.3 tokens per second at 1, 2 and 4 requests. Setup offers 4 slots at 8K on a 12 GB card and none at 32K; a 16 GB card gets 2 slots at 32K.

**On the stock P340.** Strata has room for one slot. The llama-server lane can run `-c 12288 -np 3`, three slots of 4K, at a modelled 20 tokens per second in total. Workers must then stay under 4K each: `-budget 6000 -page 3000`.

**Works as designed from.** 12 GB for small workers, 16 GB or more for workers with real context.

### 9.4 Verification council

**Purpose.** Raise the reliability of answers that can be checked: a number, a yes or no, a choice from a list, a diff that passes tests.

A word on names first. Strata's mixture of experts is inside one model: a router picks 8 of 256 expert blocks for each token. A council is a mixture of agents: several whole models, or several samples of one, answer the same question and a rule picks the winner. Strata does the first. The second is built around it.

```bash
# Self-consistency: three samples of one model, on three Strata slots
strata launch --preset fanout --yes --no-tui
bin/council -seats brain,brain,brain -temp 0.8 \
  "A tank holds 2,400 litres and drains at 37.5 litres a minute. After how many whole minutes is it first below 500 litres?"

# Mixed council: Strata plus a second model on llama-server, a judge on a split
bin/council -seats brain,second,brain -judge brain -temp 0.7 "..."
```

`council` appends an instruction to end with one `ANSWER:` line, takes the majority ignoring case and punctuation, and exits with status 2 on a split when no judge is named. The alias `second` is a llama-server unit holding a different model, for example gpt-oss-20b on the CPU, added to `routes.json` as in 5.4.

**On the stock P340.** Self-consistency on three llama-server slots is modelled at 97 seconds for a chat-sized question. A mixed council is modelled at about 5 minutes, because a second model's weights compete for the same memory bus. Use the first, and keep it for questions that deserve the wait.

**Works as designed from.** Bronze tier: a 24 GB card holds nearly all of the brain, which leaves the CPU and system RAM free for a second model.

### 9.5 Master and workers

**Purpose.** Keep the P340 as the always-on front door and send heavy inference to a machine that can do it. This is the May guide's master-slave pattern with Strata as the remote engine.

**Layout.** The P340 runs the gateway, the agent, SearXNG, the embedding lane and its local brain as a fallback. A second machine runs Strata.

```bash
# On the worker: any Strata-supported host
strata launch --family qwen36 --model UD-Q4_K_XL --context 32k --parallel 2 \
  --access tailscale --new-key --save-preset worker --no-tui
strata launch --show-key
```

```json
"brain": [
  {"url": "http://worker.tailnet-name.ts.net:8080", "model": "", "key_env": "WORKER_API_KEY"},
  {"url": "http://127.0.0.1:8081", "model": "", "key_env": "LLAMA_API_KEY"}
]
```

This is where the gateway's ordering earns its keep. Requests go to the worker while it is up. When it sleeps or drops off the network, the dial fails within 3 seconds and the same request is answered by the local lane, slower but answered. Clients see one address the whole time.

Specialist workers, such as an image model behind ComfyUI on a third machine, are reached as tools from the agent, not through the gateway: they do not speak the chat API.

**Rules for the network.** Use `--access tailscale` or `--access lan`, never a public tunnel, for a model with tools. Every hop carries a key: client to gateway, gateway to worker. Keep the gateway itself on 127.0.0.1 unless other machines must call it, and then it refuses to start without `GATE_KEY`.

**On the stock P340.** It works today as the master. The worker is whatever Appendix A persuades you to buy.

## 10. Day-one runbook

In order. Each step ends with a check, and a failed check is a reason to stop there.

**1. Base system.** Section 2. Check: `nvidia-smi` shows driver 580 and 4096 MiB; `cmake --version` is 3.18 or newer; `go version` is 1.24.

**2. Companion package.** Unpack it into your home directory, because the systemd units expect `~/p340-strata`.

```bash
tar -xzf p340-strata.tar.gz -C ~ && cd ~/p340-strata
sha256sum -c SHA256SUMS
make build test
make preflight
```

Check: tests pass, and preflight reports no failures. Warnings about Strata and Studio not being installed are expected at this point.

**3. Unsloth Studio and the weights.** Section 3.1, then download the brain and the small utility model.

```bash
unsloth studio -H 127.0.0.1 -p 8888
# In the Model hub: unsloth/Qwen3.6-35B-A3B-MTP-GGUF, UD-Q4_K_XL, and a Qwen3.5-4B Q4_K_M
find ~/.cache/huggingface/hub -name '*UD-Q4_K_XL*.gguf'
```

Check: the brain loads in Studio with the flags in 3.3 and VRAM stays under 3.8 GB. Write down the tokens per second it reports.

**4. llama-server.** Section 6.1. Check: `llama-server --version` prints, and the build log shows CUDA.

**5. Keys and units.**

```bash
mkdir -p ~/.config/p340 && cp etc/env.example ~/.config/p340/env && chmod 600 ~/.config/p340/env
$EDITOR ~/.config/p340/env       # three keys from: openssl rand -hex 24; the two GGUF paths; LLAMA_SERVER
make install-units
```

**6. Production on llama-server.** Stop Studio's model first; there is memory for one brain.

```bash
make proto && make prod-llama
set -a; . ~/.config/p340/env; set +a
make status
```

Check: the gateway lists `brain`, `embed` and `fast`, and the acceptance test in 5.2 passes. This is a complete production system. Everything after this step is improvement.

**7. Search and the agent.** Section 6.3, then:

```bash
bin/agent "What is the current stable release of llama.cpp, and when was it tagged?"
```

Check: stderr shows a `web_search` step and at least one `fetch_url` step, and the answer names its URLs.

**8. The Strata experiment.** Section 4.2, then:

```bash
make plan                         # saved under sim/
```

Read the result against the table in 4.3. If Strata accepts the machine, create the preset in 4.4, run `make prod-strata`, repeat the acceptance test, and keep whichever lane is faster. If it does not, you have lost ten minutes and the system from step 6 is unchanged.

**9. Record it.** Commit the model manifest, the acceptance numbers and `sim/strata-plan-*.txt` to git. The next person to touch this machine, including you in three months, starts from evidence.

### Week one

- Build one Data Recipe from this project's documents and export it (3.5).
- Train one small adapter on the free Colab notebook, export GGUF, and promote it to an alias (3.6, 5.4).
- Replace three constants in `p340sim` with your measurements and rerun `make sim`.
- Decide on a tier in Appendix A with real numbers in hand.

### Keeping it running

| When | Do |
| --- | --- |
| Daily | `make status` |
| Before prototyping | `make proto`, then start Studio |
| After prototyping | Stop the Studio model, then `make prod-llama` or `make prod-strata` |
| Weekly | `strata update --check`. Read the release notes before updating. Rerun the acceptance test after |
| After any update | `git -C ~/llama.cpp log -1` and `strata --version` into the manifest |
| A number drops | `strata components rollback`, or check out the previous llama.cpp commit and rebuild |

## Appendix A. Realistic fantasy Local AI system

Realistic means parts you could order in November 2026, at prices read from dated pages. Fantasy means the budget is not this P340's. For each tier there are two picks: the most powerful option, and the most efficient one. An option is listed only if it clears at least one of three bars against the stock machine:

- **Turn time:** at least 2.5 times faster on the same brain.
- **Work it can complete:** a stronger model class, a context large enough for coding agents, or fine-tuning on the machine itself.
- **Scaling:** more requests at once, or a second machine that takes load off this one.

The stock P340 for comparison: Qwen3.6-35B-A3B at a modelled 13.8 tokens per second, 39 seconds for a chat turn, 8K of context, one request at a time, no training.

Every speed below carries its evidence:

| Tag | Meaning |
| --- | --- |
| M | Measured on that hardware and published in Strata's documentation |
| E | Estimated by a third-party buying guide from memory bandwidth |
| S | This guide's model, the method of section 8. Checked against five published figures: within 12% on 12 GB cards with experts split across card and CPU, and 25 to 32% too fast when the whole model sits on one fast card |

Prices are snapshots from October and November 2026, a period of memory-shortage pricing. Check them on the day.

### A.1 The tiers at a glance

| Tier | Pick | Hardware | About | Speed on the evidence | What it changes |
| --- | --- | --- | --- | --- | --- |
| Paper, $800 to $1,100 | Most efficient | RTX A2000 12 GB in the P340's slot, RAM to 64 GB | $1,000 | S: 51 tok/s, chat turn 14 s | 3.7x the tokens, 32K context with drafts, Flash-Next becomes runnable |
|  | Most powerful | RTX 5060 Ti 16 GB on an OCuLink dock, RAM to 64 GB | $970 to $1,150 | S: 71 tok/s, chat turn 9 s | 5x the tokens, two requests at once at 32K |
| Bronze, $1,200 to $1,800 | Most powerful | Used RTX 3090 24 GB on a dock | $1,300 to $1,750 | S: 120 to 170 tok/s | Nearly the whole brain on the card; CPU free for a second model; local fine-tuning |
|  | Most efficient | RTX 2000 Ada 16 GB in the slot, RAM to 64 GB | $1,020 to $1,350 | S: 55 tok/s, chat turn 11 s | 16 GB at 70 W with nothing outside the case |
| Silver, $2,000 to $2,800 | Most powerful | Radeon AI PRO R9700 32 GB on a dock, RAM to 64 GB | $2,400 to $2,650 | M: 117.8 tok/s; 325 tok/s across 8 requests | Every expert on the card; 8 slots |
|  | Most efficient | Strix Halo mini PC, 64 GB (GMKtec EVO-X2) | $2,200 | E: gpt-oss-20b at 42 to 51 tok/s | A quiet second machine; the P340 becomes the master |
| Gold, $3,000 to $4,500 | Most powerful | DIY tower: used RTX 3090, 64 GB DDR5 | $2,950 to $3,980 | M on a smaller card: 87.3 tok/s on Flash-Next | All three Strata families; fine-tuning up to 22B class |
|  | Most efficient | Mac Studio M5 Max, 64 GB | $3,499 | M on the 128 GB model: 108.2 tok/s on Flash-Next | A 125B-class model, about 4 s a turn, silent |
| Platinum, $5,000 and up | Most powerful | DIY tower: RTX 5090 32 GB, 64 GB DDR5 | $6,400 to $7,700 | E: 196 tok/s on the 35B brain | Fastest single stream you can buy for a desk |
|  | Most efficient | Mac Studio M5 Max, 128 GB | $5,099 | M: 108.2 tok/s on Flash-Next | Higher-quality packs and DeepSeek V4.1-Flash in one silent box |

The three hardware classes you asked for appear as follows. Apple M5: Gold and Platinum. Custom GPU systems: every tier, with NVIDIA at Paper, Bronze, Gold and Platinum and AMD at Silver. Consumer form-factor GPU PCs: Strix Halo at Silver and Gold, DGX Spark at Platinum.

### A.2 Paper: upgrade the P340 in place

**Most efficient: a 12 GB low-profile card and 64 GB of RAM.** A used RTX A2000 12 GB sold for a median of $517 in late October, typical range $461 to $630. The tracker does not separate the 6 GB and 12 GB versions, so budget the top of the range ([GPUSold](https://gpusold.com/gpu/rtx-a2000)). It draws 70 W from the slot, which matters because the P340 SFF's 310 or 380 W supply has no graphics power lead ([Lenovo PSREF](https://psref.lenovo.com/syspool/Sys/PDF/ThinkStation/ThinkStation_P340_SFF/ThinkStation_P340_SFF_Spec.html)). For RAM, add a matched 32 GB DDR4-2933 kit if two slots are free, about $130 to $260, or fit a 64 GB kit for about $370 to $550.

This is the cheapest change that turns the machine into something Strata documents: an RTX-class 12 GB card. Drafts fit, 32K of context fits, and the model puts the brain at 51 tokens per second and a chat turn at 14 seconds (S). With 64 GB, Flash-Next Q2\_0 fits in RAM by Strata's rule, at a modelled 41 tokens per second (S). That is a 125B-class model on this chassis.

**Most powerful: a 16 GB desktop card outside the case.** An RTX 5060 Ti 16 GB is about $620 new ([Llama Builds](https://www.llamabuilds.ai/best/gpus-for-local-llms)). It needs an OCuLink dock, $119 for the Minisforum DEG1 ([VDF AI](https://vdf.ai/blog/best-mini-pc-for-local-llm/)), its own power supply, and an M.2-to-OCuLink adapter in one of the P340's two M.2 slots. The last two parts were not priced from a source; allow $100 to $150. The same guide estimates this card runs the brain at about 42 tokens per second on llama.cpp (E), and this guide's model says 45 for that case and 71 on Strata with drafts (S).

The cost is tidiness and a 4-lane PCIe 3.0 link. That link is fine for Qwen3.6, where the CPU computes the experts the card does not hold. It is an open question for Flash-Next, which pulls experts to the card over PCIe and was measured on 16-lane PCIe 4.0.

### A.3 Bronze

**Most powerful: a used RTX 3090 on the same dock.** Used cards ran $1,050 to $1,450 in October by one guide and a $1,520 sold median by another, so prices are spread wide ([Llama Builds](https://www.llamabuilds.ai/best/gpus-for-local-llms), [GPUSold](https://gpusold.com/gpu/rtx-a2000)). Add the dock and a 750 W supply. At 24 GB, about 94% of the brain's experts sit on the card. The model says 173 tokens per second, which is the regime where it runs high, so plan on 120 to 170 (S). For scale, Strata measured 117.8 on a slower 32 GB card with every expert on board (M), and the buying guide estimates about 109 for a similar model on a 3090 (E).

Two further gains matter more than the speed. The CPU and system RAM are left free, so the mixed council of section 9.4 becomes practical. And current PyTorch supports this card, so Unsloth Studio can train on the machine, which the P1000 cannot do.

**Most efficient: a 16 GB, 70 W card inside the case.** A used RTX 2000 Ada 16 GB runs about $650 to $800. The new equivalent, the RTX PRO 2000 Blackwell 16 GB, is about $1,500 to $1,725. With a 64 GB RAM kit the Ada route totals $1,020 to $1,350. Modelled at 55 tokens per second and an 11-second chat turn (S), assuming a 256 GB/s memory rating that I did not confirm. It is the tidy option: no dock, no second power supply, no extra noise.

Not recommended at this price for this stack: the Mac mini with M5 Pro and 24 GB, $1,699. Strata on a Mac serves Flash-Next only and recommends 64 GB for it. Smaller Macs stream experts from the SSD, at 13.5 tokens per second on a 32 GB M4 mini, and 24 GB is below anything documented. At 24 GB it can run gpt-oss-20b in other runtimes, which is a step sideways from the brain this guide is built on.

### A.4 Silver

**Most powerful: a Radeon AI PRO R9700 32 GB on the dock.** About $1,800 for the card. This is the one option with direct Strata measurements on the exact card and model: Qwen3.6-35B-A3B UD-Q4\_K\_XL with every expert in VRAM at 117.8 tokens per second, and total output of 83.9, 173.5, 252.0 and 325.4 tokens per second at 1, 2, 4 and 8 requests without drafts ([Qwen3.6 guide](https://github.com/Niko1221/Strata/blob/main/docs/QWEN36.md)) (M). Against the stock machine that is 8.5 times the single-stream speed and 24 times the total throughput. Those figures come from Strata's host, not a 10th-generation Core on a 4-lane link, though with the whole model on the card the host matters least here.

It requires moving to Ubuntu 24.04 for AMD's ROCm stack, and Strata supports this card on Linux only. Some CUDA-only tools will not run on it.

**Most efficient: a Strix Halo mini PC with 64 GB.** The GMKtec EVO-X2 with a Ryzen AI Max+ 395 and 64 GB of unified memory was $2,199.99 on 15 November ([VDF AI](https://vdf.ai/blog/best-mini-pc-for-local-llm/)). The memory runs at 256 GB/s and most of it is addressable by the GPU. The source's estimate is 42 to 51 tokens per second on gpt-oss-20b, and it describes the box as running everything up to Qwen3.6-35B-A3B comfortably (E). Strata has guides for this chip on Linux and Windows; I found no published Flash-Next speed for it.

Its value is as a second machine. It is quiet, small and always on, it holds a 35B model with room to spare, and it turns the P340 into the master of section 9.5 with a local fallback. Memory is soldered, so buy the size you mean to keep.

### A.5 Gold

**Most powerful: a purpose-built tower with a used RTX 3090 and 64 GB of DDR5.** The card is $1,050 to $1,520. 64 GB of DDR5 is the painful line at about $1,000 to $1,360 this autumn. The processor, board, 850 W supply, case and SSD were not priced from a source; allow $900 to $1,100. Total $2,950 to $3,980.

This is the machine class Strata's own numbers come from. Flash-Next Q2\_0 measured 87.3 tokens per second, and 2,171 tokens per second reading a 32K prompt, on a 12 GB RTX 5070 with 64 GB of DDR5 (M). A 24 GB card caches far more of the model's 24,576 experts: Strata reports about 700 more for each extra gigabyte, so roughly 8,000 more than a 12 GB card. It runs all three Strata families, including DeepSeek V4.1-Flash given 240 to 275 GB of disk, and it trains: Unsloth's own guidance puts models up to about 22B within reach of a 16 GB card. A second 3090 can be added later for capacity, though Strata is plain that a split adds room, not speed.

**Most efficient: Mac Studio with M5 Max and 64 GB.** About $3,499 as configured; the 48, 64 and 128 GB options all carry the 40-core GPU and 614 GB/s of memory bandwidth. Strata recommends Flash-Next Q2\_0 from 64 GB on a Mac. Its measurement on the 128 GB version of the same chip is 108.2 tokens per second against 56.6 for llama.cpp on that Mac, with prompts read at about 1,460 tokens per second at 32K (M). That makes a chat turn about 4 seconds on a model three and a half times the size of the stock brain. The 64 GB machine itself was not measured.

The limits are the platform's. Strata on a Mac does not serve Qwen3.6, the machine cannot be upgraded, and it runs macOS.

Also in this range: the 128 GB Strix Halo box at $3,499.99, the capacity pick, estimated at about 35 tokens per second on gpt-oss-120b (E). And the RTX PRO 4000 Blackwell SFF, 24 GB at 70 W for about $3,000 to $3,200, which is the only way to put 24 GB inside the P340's case without external parts.

### A.6 Platinum

**Most powerful: a tower with an RTX 5090.** The card alone was $4,300 to $5,000 in October, more than twice its launch price. With 64 GB of DDR5 and a host built around a 1,000 W supply, $6,400 to $7,700. The estimate is 196 tokens per second on the 35B brain at a higher-quality quantization, and 80 on a dense 27B model, twice a 3090 (E). At 32 GB the whole brain fits with long context and several slots.

**Most efficient: Mac Studio with M5 Max and 128 GB.** About $5,099. This is the exact machine behind Strata's Mac figures: 108.2 tokens per second on Flash-Next Q2\_0 (M). At 128 GB Strata recommends the higher-quality UD-Q4\_K\_XL pack, and it is the Mac configuration its DeepSeek V4.1-Flash work was run on. It is also silent and draws a fraction of a GPU tower's power.

Two alternatives at this level, for different reasons:

- **DGX Spark, the CUDA appliance.** $6,950 for the 128 GB Founders Edition with 4 TB, $5,999 for the ASUS Ascent GX10 with 1 TB, and 64 GB partner models from $4,999. Strata on one Spark measured 45.5 to 46.5 tokens per second writing and 1,296 to 1,735 reading, on the IQ3\_S pack (M). That is a higher-quality pack than the Mac's Q2\_0 figure, so the two numbers are not a like-for-like race; the Q2\_0 packs do not run on the Spark at all. Buy it for NVIDIA's software stack and for fine-tuning, which NVIDIA rates up to 70B parameters, not for writing speed.
- **Four Radeon AI PRO R9700 cards, the capacity build.** 128 GB of VRAM for about $7,200 in cards. Strata's documentation reports DeepSeek V4.1-Flash at about 31 tokens per second on this arrangement (M), and the buying guide estimates gpt-oss-120b at about 37 (E).

### A.7 Buying in steps

If the money arrives in pieces, this order wastes none of it:

1. Ubuntu 24.04. Free, and it removes the open question about Strata's engines on Focal.
2. RAM to 64 GB as a matched DDR4-2933 set. Not the DDR4-2133 stick, which slows every DIMM.
3. A 12 GB or larger card. In the slot if you value tidiness, on a dock if you value speed.
4. A second machine, when you want the P340 to be the master instead of the worker.

### A.8 What to skip

| Item | Why |
| --- | --- |
| GTX 960M 2 GB | A laptop module. It cannot be installed, and 2 GB is below anything useful |
| The 16 GB DDR4-2133 stick | Costs 11 to 26% of speed and unlocks nothing (section 8) |
| 8 GB cards | Below Strata's recommended 12 GB; community-reported only |
| RTX 5080 at about $1,600 | 16 GB. A used 3090 has 24 GB for less |
| Intel Arc cards, for Strata | Linux only, Ubuntu 24.04 or newer, a model-preparation step, and no Qwen3.6 |
| Mac mini M5 Pro 24 GB, for this stack | Too little memory for the one Strata model family that runs on a Mac |

## Appendix B. Sources, versions and what was not verified

### B.1 Versions and dates

| Item | Version or date | Note |
| --- | --- | --- |
| Strata app and terminal | 0.2.13, 9 November 2026 | Engine and control 0.1.59. First release 24 September 2026 |
| Unsloth Studio | Documentation as read on 20 November 2026 | Not pinned to a release number |
| PyTorch | 2.14 is the last with Pascal binaries | CUDA 12.6 index only; none from 2.15 |
| NVIDIA driver and CUDA | 580 and 12.9 | Carried from the May guide |
| Go | 1.24.7 | Used to build and test the companion programs |
| llama.cpp | Current `master` | Not pinned. Record the commit you build |
| Prices | 14 October to 15 November 2026 | Snapshots, taken during a memory shortage |

### B.2 What was run for this guide

| Run here | Result |
| --- | --- |
| `go vet` and unit tests for `gate`, `council` and `agent` | Pass. Failover, key check, alias rewrite, majority vote, tool loop, fetch guard |
| `gate` and `council` end to end against a mock model server | Failover reported in `X-Gate-Upstream`; 401 without a key; a 2-of-3 majority returned |
| `p340sim` | The tables in section 8 and the modelled rows in Appendix A |

Nothing was run on a ThinkStation P340, on a Quadro P1000, or against a real model. Strata and Unsloth Studio were not installed here.

### B.3 Not verified

Each item is something the guide relies on or suggests, with the check that settles it.

| Open item | How to settle it |
| --- | --- |
| Strata starting at all on a 4 GB card | `make plan`, then one launch |
| Strata's released engine on Ubuntu 20.04 | Same. If it fails, Ubuntu 24.04 or Strata's Docker image |
| `--model-path` accepting a file inside the Hugging Face cache | Try it; copy the file out if refused |
| Strata accepting a model name it does not know | Call it with `"model":"brain"`; if refused, fill `model` in `routes.json` |
| The systemd unit for Strata | It is a sketch. Strata documents its own startup recipe in `docs/DETAILS.md`, which I did not read. Its Docker guide was not read either |
| Unsloth's bundled llama.cpp build supporting Pascal | Load any GGUF in Studio and watch `nvidia-smi` |
| Unsloth accepting PyTorch 2.14 from the CUDA 12.6 index | The side-by-side install in 3.6 |
| Studio reaching the gateway as a connection | Studio names llama.cpp, vLLM and Ollama servers. Add `http://127.0.0.1:4000/v1` as a llama.cpp server |
| Studio's MCP settings | Page not read |
| llama.cpp compiling on Focal with GCC 9 | Section 6.1 gives the fallback |
| Claude Code through the gateway | The environment in 7.3 follows Strata's notes and Anthropic's variable names; not run |
| Low-profile dual-slot cards fitting the P340 SFF | Carried from the earlier hardware report. Measure the bay and check the neighbouring slot is free |
| An M.2-to-OCuLink adapter working in the P340 | Not tested on this board |
| Flash-Next over a 4-lane PCIe 3.0 link | Unmeasured anywhere I could find |
| Driver, Docker and SearXNG commands from the May guide | Carried over unchanged, not rerun |

Two places where sources disagree, and what I used:

- **Flash-Next's size.** The project's paper and one review give 125B parameters with 6B active. Another review gives 180B as stored. This guide says 125B-class throughout.
- **Used RTX 3090 prices.** A buying guide gives $1,050 to $1,450 for October. A sold-listings tracker gives a $1,520 median. Appendix A quotes the whole spread.

One site that ranks well for Strata, stratallm.org, is not the project's and was not used.

### B.4 Sources

**Strata**

- [Repository](https://github.com/Niko1221/Strata) and [releases](https://github.com/Niko1221/Strata/releases)
- [Install](https://github.com/Niko1221/Strata/blob/main/docs/INSTALL.md), [launch reference](https://github.com/Niko1221/Strata/blob/main/docs/TERMINAL_LAUNCH.md), [models](https://github.com/Niko1221/Strata/blob/main/docs/MODELS.md), [Qwen3.6 guide](https://github.com/Niko1221/Strata/blob/main/docs/QWEN36.md), [capabilities](https://github.com/Niko1221/Strata/blob/main/docs/CAPABILITIES.md), [older GPUs](https://github.com/Niko1221/Strata/blob/main/docs/OLDER_GPUS.md)
- [VDF AI, Strata guide](https://vdf.ai/blog/strata-local-llm/), checked 13 November 2026
- [Llama Builds, how Strata works](https://www.llamabuilds.ai/guides/how-strata-revolutionized-local-ai-inference)
- [LocalAI engines](https://localai.io/docs/models/engines/), which lists an experimental Strata backend

**Unsloth Studio**

- [Overview](https://unsloth.ai/docs/get-started/unsloth-studio), [install](https://unsloth.ai/docs/new/studio/install), [chat](https://unsloth.ai/docs/new/studio/chat), [data recipes](https://unsloth.ai/docs/new/studio/data-recipe), [export](https://unsloth.ai/docs/new/studio/export)
- [API guide](https://unsloth.ai/docs/basics/api), [connections](https://unsloth.ai/docs/integrations/connections)
- [README](https://github.com/unslothai/unsloth)

**PyTorch on Pascal**

- [CUDA 12.6 wheels end with PyTorch 2.15](https://dev-discuss.pytorch.org/t/notice-cuda-12-6-wheels-will-no-longer-be-published-from-pytorch-2-15-drops-maxwell-pascal-volta/3432)
- [Pascal support removed from CUDA 12.8 and 12.9 builds](https://dev-discuss.pytorch.org/t/cuda-toolkit-version-and-architecture-support-update-maxwell-and-pascal-architecture-support-removed-in-cuda-12-8-and-12-9-builds/3128)

**Hardware and prices**

- [Llama Builds, best GPUs for local LLMs](https://www.llamabuilds.ai/best/gpus-for-local-llms), updated 14 October 2026: card prices, bandwidth, estimated speeds
- [VDF AI, best mini PC for local LLMs](https://vdf.ai/blog/best-mini-pc-for-local-llm/), 14 November 2026: Strix Halo, GB10, Mac mini, OCuLink dock
- [GPUSold, RTX A2000](https://gpusold.com/gpu/rtx-a2000), updated 23 October 2026: sold medians
- [Lenovo PSREF, ThinkStation P340 SFF](https://psref.lenovo.com/syspool/Sys/PDF/ThinkStation/ThinkStation_P340_SFF/ThinkStation_P340_SFF_Spec.html): slots, M.2, power supplies
- Apple: [MacRumors, M5 Max and M5 Ultra](https://www.macrumors.com/guide/m5-max-vs-m5-ultra/), [Macworld, Mac Studio buying advice](https://www.macworld.com/article/3154508/should-you-buy-a-mac-studio-now-or-wait.html), [Wavect, Mac mini and Mac Studio for local AI](https://wavect.io/blog/mac-mini-m6-vs-mac-studio-m5-local-ai/), [DriveVault, Mac Studio M5 guide](https://www.drivevault.io/blog/mac-studio-m5-buyers-guide-worth-upgrading.html)
- DGX Spark: [Physical AI Lab](https://physicalailab.io/en/nvidia-dgx-spark-price-specs-buy/), [Aliteq](https://aliteq.com/dgx-spark-price)
- Memory: [Capital and Compute, memory prices](https://capitalandcompute.net/memory-prices/), [Digital Citizen on DDR5 prices](https://www.digitalcitizen.life/ddr5-ram-prices-surge-in-2026-as-32gb-kits-reach-450-amid-global-memory-shortage/)
- Low-profile workstation cards: [VideoCardz, RTX PRO 4000 SFF and PRO 2000](https://videocardz.com/newz/nvidia-launches-rtx-pro-4000-sff-and-rtx-pro-2000-blackwell-workstation-gpus-with-70w-tdp), [Micro Center, RTX PRO 4000 Blackwell SFF](https://www.microcenter.com/product/700465/pny-nvidia-rtx-pro-4000-blackwell-sff-single-fan-ai-workstation-graphics-card), [ServeTheHome, Arc Pro B50](https://www.servethehome.com/intel-arc-pro-b50-review-a-16gb-sff-mini-gpu/)

**Carried forward**

- The May 2026 guide in this project: base-system commands, SearXNG settings, the P1000 speeds used for calibration, and the mixture-of-agents and self-consistency references (Wang et al., arXiv 2406.04692, and its 2025 follow-up).

### B.5 The companion package

`p340-strata.tar.gz` holds everything section 10 uses: four Go programs (`gate`, `council`, `agent`, `p340sim`) with tests, the systemd user units, the route table and environment template, the preflight and plan scripts, a Makefile, the simulator's output, and a checksum file. It builds with Go 1.24 and the standard library and nothing else.
