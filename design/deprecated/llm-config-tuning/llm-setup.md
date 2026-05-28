# Local LLM operations on a Quadro P1000 workstation

This documentation is the end-to-end operational guide for running modern local LLMs on a **Lenovo ThinkStation P340 SFF** (Ubuntu 20.04, Intel **i9-10900** 10C/20T, **32 GB DDR4**, **NVIDIA Quadro P1000 4 GB GDDR5**, Pascal compute capability 6.1). Everything below is tuned for that exact box and verified against May 2026 documentation for Ollama, Open WebUI, LM Studio, SearXNG, LiteLLM, LangGraph/LangChain, CrewAI, AutoGen 0.4+, smolagents, PydanticAI, and the Model Context Protocol.

The single hardest constraint is **4 GB of VRAM**. It dictates almost every decision: which models you load, how you quantize the KV cache, how you partition work between this box and any optional GPU slaves on the LAN, and which agent frameworks behave reliably. The good news is that the i9-10900 with 32 GB of RAM is a *very* capable CPU-inference partner, so a hybrid approach — a 3–4 B model fully on GPU as the orchestrator plus 7 B models spilling to CPU for harder work — is realistic and productive.

Two 2025–2026 caveats are repeated through the document because they affect every install command:

1. **Pascal is on the last-ever driver branch.** NVIDIA driver **R580 LTSB** (supported through mid-2028) is the last branch supporting GP107 (Quadro P1000). Stay on **CUDA Toolkit 12.9** for any native build; CUDA 13.x dropped offline `compute_<7.5` compilation and its APT repo no longer ships a Focal channel.
2. **Ubuntu 20.04 is on ESM.** Standard support ended April 2025; Ubuntu Pro ESM (free for personal/up-to-5 machines) extends security maintenance to April 2030. Attach Pro and enable `esm-infra esm-apps` *before* installing CUDA.

---

## 1. Setup of required software

### 1.1 NVIDIA driver 580 LTSB and CUDA Toolkit 12.9

The P1000 is GP107, compute capability **6.1**, 640 CUDA cores, no Tensor cores, no BF16 fast-path. Use the proprietary `cuda-drivers-580` package, **not** `nvidia-open` (the open kernel module only supports Turing and later).

```bash
# Pre-flight
sudo pro attach <token> && sudo pro enable esm-infra esm-apps
sudo apt update && sudo apt -y full-upgrade
sudo apt-get -y --purge remove '^nvidia-.*' '^libnvidia-.*' '^cuda-.*' '^libcudnn.*'
sudo apt-get -y autoremove
sudo apt-get install -y build-essential dkms linux-headers-$(uname -r) \
    pkg-config libglvnd-dev curl gnupg ca-certificates software-properties-common

# Blacklist nouveau
sudo tee /etc/modprobe.d/blacklist-nouveau.conf >/dev/null <<'EOF'
blacklist nouveau
options nouveau modeset=0
EOF
sudo update-initramfs -u

# NVIDIA's ubuntu2004 repo (CUDA 12.x channel)
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-ubuntu2004.pin
sudo mv cuda-ubuntu2004.pin /etc/apt/preferences.d/cuda-repository-pin-600
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb
sudo apt-get update
sudo apt-get install -y cuda-drivers-580 cuda-toolkit-12-9
sudo apt-mark hold nvidia-open nvidia-open-580   # block the open metapackage
sudo reboot
```

After reboot, add to `~/.bashrc`:
```bash
export PATH=/usr/local/cuda-12.9/bin:$PATH
export LD_LIBRARY_PATH=/usr/local/cuda-12.9/lib64:$LD_LIBRARY_PATH
export CUDA_HOME=/usr/local/cuda-12.9
```

Verify:
```bash
nvidia-smi        # Driver 580.xx, "Quadro P1000", 4096 MiB
nvcc --version    # release 12.9
```

If Secure Boot is on (`mokutil --sb-state`), either disable it in BIOS or complete MOK enrollment at the next boot. The most common post-install error — `Failed to initialize NVML: Driver/library version mismatch` — is fixed by a reboot.

### 1.2 Docker CE, Docker Compose, and the NVIDIA Container Toolkit

```bash
sudo apt-get remove -y docker docker-engine docker.io containerd runc
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | \
    sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
https://download.docker.com/linux/ubuntu focal stable" | \
    sudo tee /etc/apt/sources.list.d/docker.list >/dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin
sudo usermod -aG docker $USER && newgrp docker
sudo systemctl enable --now docker

# NVIDIA Container Toolkit
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | \
  sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
sudo apt-get update && sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker

# Verify — use a 12.x CUDA base image (13.x is too new for the P1000 toolchain)
docker run --rm --gpus all nvidia/cuda:12.9.0-base-ubuntu22.04 nvidia-smi
```

### 1.3 Ollama with a hardware-tuned systemd override

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama -v
sudo systemctl edit ollama.service
```

Paste into the override editor (`/etc/systemd/system/ollama.service.d/override.conf`):

```ini
[Service]
Environment="OLLAMA_HOST=0.0.0.0:11434"
Environment="OLLAMA_MODELS=/usr/share/ollama/.ollama/models"
Environment="OLLAMA_KEEP_ALIVE=15m"
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=2"
Environment="OLLAMA_FLASH_ATTENTION=1"
Environment="OLLAMA_KV_CACHE_TYPE=q8_0"
Environment="OLLAMA_GPU_OVERHEAD=536870912"
Environment="OLLAMA_CONTEXT_LENGTH=4096"
Environment="OLLAMA_ORIGINS=*"
```

```bash
sudo systemctl daemon-reload && sudo systemctl restart ollama
journalctl -u ollama -n 50 | grep -iE 'cuda|gpu|inference'
# Confirm: "inference compute … NVIDIA Quadro P1000 … 4.0 GiB"
```

**Why these values:** With only 4 GB of VRAM, parallel requests are death — every parallel slot multiplies the KV cache. `OLLAMA_FLASH_ATTENTION=1` together with `OLLAMA_KV_CACHE_TYPE=q8_0` is the single biggest free win on a Pascal card (it roughly halves KV-cache memory at no perceptible quality cost). `OLLAMA_GPU_OVERHEAD=536870912` reserves 512 MB so your desktop session won't OOM the GPU; set to 0 if headless.

### 1.4 Open WebUI

Bundled Docker stack (Ollama + Open WebUI together) — `~/ai-stack/docker-compose.yml`:

```yaml
services:
  ollama:
    image: ollama/ollama:latest
    container_name: ollama
    restart: unless-stopped
    ports: ["127.0.0.1:11434:11434"]
    volumes: [ollama:/root/.ollama]
    environment:
      - OLLAMA_FLASH_ATTENTION=1
      - OLLAMA_KV_CACHE_TYPE=q8_0
      - OLLAMA_KEEP_ALIVE=15m
      - OLLAMA_NUM_PARALLEL=1
      - OLLAMA_MAX_LOADED_MODELS=2
    deploy:
      resources:
        reservations:
          devices:
            - {driver: nvidia, count: all, capabilities: [gpu]}

  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    container_name: open-webui
    restart: unless-stopped
    depends_on: [ollama]
    ports: ["3000:8080"]
    environment:
      - OLLAMA_BASE_URL=http://ollama:11434
      - WEBUI_AUTH=true
      - WEBUI_SECRET_KEY=change-me-openssl-rand-hex-32
      - RAG_EMBEDDING_ENGINE=ollama
      - RAG_EMBEDDING_MODEL=nomic-embed-text
      - RAG_OLLAMA_BASE_URL=http://ollama:11434
      - ENABLE_RAG_HYBRID_SEARCH=true
      - ENABLE_WEB_SEARCH=true
      - WEB_SEARCH_ENGINE=searxng
      - SEARXNG_QUERY_URL=http://searxng:8080/search?q=<query>
    volumes: [open-webui:/app/backend/data]

volumes: {ollama: {}, open-webui: {}}
```

Pip-install path (requires Python **3.11** exactly — hard pin):
```bash
sudo add-apt-repository ppa:deadsnakes/ppa -y && sudo apt update
sudo apt-get install -y python3.11 python3.11-venv python3.11-dev
python3.11 -m venv ~/.venvs/openwebui && source ~/.venvs/openwebui/bin/activate
pip install -U pip open-webui
DATA_DIR=~/.open-webui OLLAMA_BASE_URL=http://127.0.0.1:11434 open-webui serve
# or with uv: uvx --python 3.11 open-webui@latest serve
```

The first user to register becomes admin; set `DEFAULT_USER_ROLE=pending` and disable `ENABLE_SIGNUP` after that.

### 1.5 LM Studio on Ubuntu 20.04

LM Studio officially targets 22.04+; on Focal the AppImage usually runs after installing libfuse2 and a few GTK libs. For a server-style deployment, prefer the **`llmster` headless daemon**.

```bash
sudo apt-get install -y libfuse2 libatk1.0-0 libatk-bridge2.0-0 libcups2 \
  libgdk-pixbuf2.0-0 libgtk-3-0 libpango-1.0-0 libcairo2 libxcomposite1 \
  libxdamage1 libasound2 libatspi2.0-0 libnss3 libxrandr2 libxkbcommon0
wget -O ~/LM-Studio.AppImage "https://lmstudio.ai/download/latest/linux/x64?format=AppImage"
chmod +x ~/LM-Studio.AppImage

# Or headless / CLI only
curl -fsSL https://lmstudio.ai/install.sh | bash
exec $SHELL -l
lms daemon up
lms server start --port 1234 --cors
```

### 1.6 Python environments

Standardize on **Python 3.11** for the whole AI stack — it is the only version that satisfies Open WebUI's hard pin and is also the comfortable middle for LangChain, LlamaIndex, CrewAI, and AutoGen 0.4+.

The Rust-based **`uv`** has effectively replaced pyenv+pip+venv for new projects in 2026 because it is 10–100× faster and ships prebuilt CPython:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
uv python install 3.11
uv venv --python 3.11 && source .venv/bin/activate
uv pip install ollama langchain langchain-ollama langgraph \
    llama-index llama-index-llms-ollama llama-index-embeddings-ollama \
    crewai 'crewai[tools]' langchain-mcp-adapters httpx trafilatura
```

### 1.7 Sanity check the full stack

```bash
nvidia-smi | head -n 10
nvcc --version
docker run --rm --gpus all nvidia/cuda:12.9.0-base-ubuntu22.04 nvidia-smi
systemctl is-active ollama
curl -s http://localhost:11434/api/tags | jq
curl -s http://localhost:3000 >/dev/null && echo "openwebui OK"
```

---

## 2. Using Ollama directly

### 2.1 Models that fit and run well

Real measured throughput on the **Quadro P1000** at Q4_K_M (sources: databasemart.com Ollama-P1000 benchmark, May 2026):

| Model | VRAM used | GPU util | tok/s |
|---|---|---|---|
| `tinyllama:1.1b` | ~1.3 GB | 93 % | **62** |
| `qwen2.5:0.5b` | ~0.8 GB | 80 % | **55** |
| `qwen2.5:1.5b` | ~1.5 GB | 89 % | **34** |
| `codegemma:2b` | ~2.1 GB | 96 % | **31** |
| `llama3.2:1b` | ~2.1 GB | 92 % | **29** |
| `llama3.2:3b` | ~3.2 GB | 95 % | **20** |
| `gemma2:2b` | ~2.9 GB | 89 % | **20** |
| `phi3.5` (3.8B) | ~3.0 GB | 97 % | **19** |
| `qwen2.5:3b` | ~2.4 GB | 95 % | **18** |

**Practical workhorse picks for this hardware:**

- **`qwen2.5:3b`** — general chat with the best tool calling under 4 B parameters.
- **`qwen2.5-coder:3b`** — code, completes at ~30 tok/s.
- **`llama3.2:3b`** — Meta tool template; very reliable for ≤3 tools.
- **`gemma3:4b`** or **`qwen3:4b`** — push VRAM ceiling; use `num_ctx ≤ 4096`.
- **`nomic-embed-text`** — 270 MB, RAG/embeddings, fits alongside any chat model.
- **`llava-phi3`** — the only vision model that fits comfortably (3 GB).

Models that **partially offload** (32 GB RAM makes this viable, but slow):

| Model | Strategy | Expected tok/s |
|---|---|---|
| `mistral:7b`, `qwen2.5:7b`, `llama3.1:8b` (Q4_K_M) | `num_gpu 14–18` of 33 layers | 4–8 |
| `gemma2:9b` | `num_gpu 10` | 2–4 |
| `qwen2.5:14b` | `num_gpu 5` or CPU-only | 1–3 |
| `gemma2:27b`, `qwen3:32b` (Q4_K_M ~16–20 GB) | **CPU-only**, fits in 32 GB RAM | ~1.5 |

Bandwidth-bound i9-10900 with DDR4-2933 yields ≈6 tok/s for 7 B Q4 CPU-only, ≈3 tok/s for 13 B Q4, ≈1.5 tok/s for 27 B Q4.

### 2.2 Modelfile customization

Full instruction set: `FROM`, `PARAMETER`, `SYSTEM`, `TEMPLATE`, `MESSAGE`, `ADAPTER`, `LICENSE`. Build with `ollama create <name> -f Modelfile`; inspect any model with `ollama show <model> --modelfile`.

**Example: P1000-tuned coding assistant**
```dockerfile
FROM qwen2.5-coder:3b
PARAMETER temperature 0.2
PARAMETER num_ctx 4096
PARAMETER num_predict 1024
PARAMETER num_thread 10           # physical cores of the i9-10900
PARAMETER num_gpu 99              # offload all layers (fits in 4 GB)
PARAMETER stop "<|im_end|>"
SYSTEM """You are a senior software engineer specializing in Python, Go,
and Bash on Ubuntu 20.04. Reply with concise, runnable code first, then a
brief explanation. Always include error handling and type hints. Say
"I'm not sure" if uncertain rather than fabricating library APIs."""
```

**Critical tuning parameters:**

- `num_gpu` is a **layer count**, not a fraction. For a 7 B Q4 model with 33 layers, set 14–18 to mix GPU and CPU. `99` or `-1` = all on GPU; `0` = CPU only.
- `num_thread = 10` (physical cores). Never set higher than physical cores on llama.cpp — hyperthreads hurt because LLM inference is memory-bandwidth bound.
- `num_ctx` directly drives KV-cache memory. Keep ≤ 4096 for 3 B+ models on 4 GB unless `OLLAMA_KV_CACHE_TYPE=q8_0` is active.

### 2.3 REST API at a glance

Base URL `http://localhost:11434`. NDJSON streaming unless `"stream": false`. Endpoints:

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/generate` | single-prompt completion |
| POST | `/api/chat` | chat with tool calls, images |
| POST | `/api/embed` | batched embeddings |
| GET  | `/api/tags`, `/api/ps`, `/api/version` | list, loaded, version |
| POST | `/api/show`, `/api/copy`, `/api/pull`, `/api/create` | model ops |
| DELETE | `/api/delete` | remove |
| POST | `/v1/chat/completions`, `/v1/embeddings`, `/v1/models` | OpenAI-compatible |

Tool calling example (returns `message.tool_calls[]` for the caller to execute):
```bash
curl -s http://localhost:11434/api/chat -d '{
  "model": "qwen2.5:3b", "stream": false,
  "messages": [{"role":"user","content":"Weather in Tokyo?"}],
  "tools": [{"type":"function","function":{
    "name":"get_temperature",
    "parameters":{"type":"object","required":["city"],
                  "properties":{"city":{"type":"string"}}}}}]
}'
```

Structured (JSON-schema-constrained) output is the most reliable way to get JSON from a small model — far better than prompting "respond in JSON":
```python
from ollama import chat
from pydantic import BaseModel
class Pet(BaseModel): name: str; age: int
class Pets(BaseModel): pets: list[Pet]
resp = chat(model="qwen2.5:3b",
    messages=[{"role":"user","content":"I have Luna (5) and Loki (2)."}],
    format=Pets.model_json_schema())
pets = Pets.model_validate_json(resp.message.content)
```

### 2.4 Concurrent serving — be conservative on 4 GB

| Variable | Recommended | Why |
|---|---|---|
| `OLLAMA_NUM_PARALLEL` | **1** | each parallel slot doubles KV-cache cost |
| `OLLAMA_MAX_LOADED_MODELS` | **2** (only if one is the 270 MB embedding model) | VRAM cannot hold two chat models |
| `OLLAMA_KEEP_ALIVE` | **15m** (or `-1` for permanent, `0` to evict immediately) | avoids cold-load latency |
| `OLLAMA_KV_CACHE_TYPE` | **`q8_0`** | halves KV memory, negligible quality loss |
| `OLLAMA_FLASH_ATTENTION` | **`1`** | required to use KV cache quantization |

A subtle reality check from community benchmarks: even when Ollama loads 35/36 layers onto GPU for a 7 B model, the giant final output projection often stays on CPU, so the real-world speedup vs. CPU-only is closer to **2.5×** than the theoretical 5–10×. Set realistic expectations.

---

## 3. Open WebUI with Ollama

Open WebUI is the right front end for this box: it speaks Ollama natively, supports OIDC/LDAP/SCIM/trusted-header SSO, exposes an OpenAI-compatible API for downstream apps, ships RAG with hybrid search and reranking, and integrates with SearXNG out of the box. The `:main` image is correct on this hardware — keep the 4 GB VRAM reserved for Ollama, not for Open WebUI's built-in SentenceTransformers.

### 3.1 Auth, API keys, and SSO

The first registered user is automatically the admin. Set `WEBUI_SECRET_KEY` (`openssl rand -hex 32`) so JWTs survive restarts, and set `DEFAULT_USER_ROLE=pending` so subsequent signups require admin approval. Create an API key under **Settings → Account → API Keys** and use it as a Bearer token against `/api/chat/completions` — the endpoint is fully OpenAI-compatible and routes through every active Function/filter/pipe.

OIDC integration is single-provider only (`OPENID_PROVIDER_URL`) but supports merging accounts by email, role mapping via a configurable claim, and group provisioning. **`WEBUI_URL` must be set before the first OAuth login** — the persistent config caches the value and a wrong URL bricks the SSO flow.

### 3.2 RAG with hybrid search

Switch embeddings from the bundled CPU `all-MiniLM-L6-v2` to **Ollama `nomic-embed-text`** — it is faster and shares the GPU intelligently. Set `RAG_EMBEDDING_ENGINE=ollama`, `RAG_EMBEDDING_MODEL=nomic-embed-text`, enable `ENABLE_RAG_HYBRID_SEARCH=true` (BM25 + dense merged), and add a reranker (`BAAI/bge-reranker-v2-m3`). Default vector DB is Chroma; switch to **PGVector** or **Qdrant** the moment you want multi-worker scaling — Chroma corrupts under `UVICORN_WORKERS > 1`.

For chunks, 1000/200 with the Tiktoken splitter is a good default; raise the overlap to 200 for code/technical docs and consider the Markdown Header Splitter for structured documentation. When you change the embedding model, click **Reset Vector Storage / Re-index** in the admin panel — only Knowledge collections are auto-reindexed; chat-uploaded files retain their old embeddings until re-uploaded.

### 3.3 Web search with SearXNG

Toggle **Web Search** in Admin → Settings → Web Search, select `searxng`, and set `SEARXNG_QUERY_URL=http://searxng:8080/search?q=<query>`. The literal `<query>` placeholder is mandatory; the typical failure mode is forgetting it. Also ensure your SearXNG `settings.yml` has `search.formats: [html, json]` or every request gets a 403. Users must additionally toggle **Web Search** on the chat input bar per conversation — the global flag only authorizes it, the chat-level toggle activates it.

### 3.4 Tools, Functions, and Pipelines

Open WebUI exposes four extension surfaces:

- **Tools** — Python functions the LLM can call mid-conversation (requires a tool-capable model like Llama 3.1+, Qwen2.5+).
- **Functions** — Pipes (appear as virtual "models"), Filters (modify inlet/outlet messages), Actions (per-message buttons).
- **Pipelines** — a separate FastAPI server (`ghcr.io/open-webui/pipelines:main`, port 9099) for heavy or pip-dependent logic, exposed back to Open WebUI as an OpenAI-compatible endpoint.
- **MCP** — native Settings → Tools → MCP Servers in recent versions, or via `mcpo` (MCP-to-OpenAPI proxy) for older builds.

A minimal Tool that fetches a URL and calculates an expression is enough to demonstrate the format. The frontmatter at the top of the file declares `title`, `author`, `version`, `required_open_webui_version`, and `requirements` — set `ENABLE_PIP_INSTALL_FRONTMATTER_REQUIREMENTS=false` in production to block in-process pip installs.

### 3.5 Model management through the UI

Pull Ollama tags from **Admin → Settings → Models**. Build *Model Presets* in **Workspace → Models** — each preset binds a base model, a system prompt, advanced parameters, attached Knowledge collections, Tools, and Filters/Actions, plus visibility settings (private/group/public). Per-chat overrides are available by clicking the model name at the top of any conversation.

---

## 4. LM Studio

LM Studio's strength on this hardware is **model exploration** — a polished GUI with a Hugging Face-backed search, a "Full GPU Offload Possible" badge that tells you instantly whether a GGUF fits, inline performance overlays, and per-file quantization filters. Its weakness is that it is an Electron app intended for desktops and only loosely supported on Ubuntu 20.04. For a server-style deployment, prefer Ollama; use LM Studio for picking which models to deploy.

### 4.1 Performance settings for 4 GB VRAM

The model loader exposes the same knobs as Ollama, just as sliders:

| Setting | P1000 recommendation |
|---|---|
| GPU offload layers | maximum if the green rocket appears; partial otherwise |
| Context length | **2048–4096** (each extra 1k ≈ 50–100 MB KV) |
| CPU threads | **10** (physical cores; not 20) |
| Flash Attention | **ON** (required for V-cache quantization) |
| K-cache / V-cache quantization | **q8_0** (V-cache requires Flash Attention) |
| Limit to Dedicated GPU Memory | **ON** — prevents driver spill to shared RAM |
| Max concurrent predictions | **1–2** |

Inspect VRAM cost before loading: `lms load qwen2.5-3b-instruct --estimate-only`. The OpenAI-compatible server at port 1234 supports `/v1/chat/completions`, `/v1/embeddings`, `/v1/completions`, `/v1/responses`, and (in 0.4.1+) `/v1/messages` for Anthropic compatibility.

### 4.2 LM Studio vs Ollama

| Dimension | LM Studio | Ollama |
|---|---|---|
| Interface | Full Electron GUI + headless `llmster` | CLI + REST; brings own UI via Open WebUI |
| Best for | desktop model exploration, multi-quant comparisons | headless servers, systemd, automation |
| OpenAI compat | port 1234, plus Anthropic `/v1/messages` | port 11434, native `/api/*` and `/v1/*` |
| Model registry | any GGUF from Hugging Face | ollama.com tags, can import any GGUF |
| Auth on server | persistent toggle via GUI; no CLI flag (issue #489) | none; front with reverse proxy or Open WebUI |
| Idle overhead | ~250–400 MB | ~50 MB Go binary |

The pragmatic split on this box: **Ollama on the host as the inference daemon** (systemd-managed, exposed to Open WebUI/LiteLLM/agents), **LM Studio on a developer workstation** for browsing models. Once you find a good GGUF, `ollama pull` it for the server.

---

## 5. Customizing models with tools, skills, and MCP

### 5.1 Native tool calling on small models

The small models that *actually* support tool calling reliably in May 2026 — verified against the "tools" capability filter on ollama.com — and that fit fully on 4 GB VRAM:

- **`qwen2.5:3b`** and `qwen2.5-coder:3b` — best small tool callers; rarely hallucinate schemas.
- **`qwen3:4b`** — hybrid thinking; **disable** the `/think` mode inside agent loops to avoid context bloat.
- **`llama3.2:3b`** — Meta's tool template; very reliable for ≤3 tools.
- **`gemma3:4b`** — native tool calling, good multilingual.

For 7 B+ tool callers (partial offload territory): `qwen2.5:7b`, `llama3.1:8b`, `hermes3:8b`, `mistral-nemo:12b` (CPU-only), `functionary` family.

The universal tool-calling loop is: send `messages + tools` → if the response has `tool_calls`, execute locally → append the result as `{"role":"tool","tool_name":"...","content":...}` → loop until the model returns plain text. The Ollama Python SDK auto-generates the JSON schema if you pass Python callables directly with Google-style docstrings.

### 5.2 Model Context Protocol (MCP)

MCP — introduced by Anthropic in November 2024 and now broadly adopted by 2026 (OpenAI, Google, LM Studio, Open WebUI, Cursor, Cline, Continue all speak it) — is the "USB-C for AI tools" pattern. Architecturally it is **JSON-RPC 2.0** over either **stdio** (local subprocess, the safe default) or **Streamable HTTP** (remote, supports OAuth 2.1). A server exposes three primitives: **tools** (LLM-invocable functions), **resources** (data to read), and **prompts** (templates). A host (the LLM app) talks to multiple servers via one client per server.

The standard config shape — used identically by Claude Desktop, Cursor, LM Studio (`~/.lmstudio/mcp.json`), and Cline — looks like this:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/you/projects"]
    },
    "git":       {"command": "uvx", "args": ["mcp-server-git", "--repository", "/home/you/repo"]},
    "fetch":     {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-fetch"]},
    "time":      {"command": "uvx", "args": ["mcp-server-time", "--local-timezone=Europe/Berlin"]},
    "searxng":   {"command": "npx", "args": ["-y", "mcp-searxng"], "env": {"SEARXNG_URL": "http://localhost:8888"}},
    "playwright":{"command": "npx", "args": ["-y", "@playwright/mcp@latest"]}
  }
}
```

### 5.3 Wiring MCP into Ollama

Ollama itself is not an MCP host. The three working patterns:

1. **`mcpo` — MCP-to-OpenAPI proxy** (recommended). Wraps any MCP server in a REST/OpenAPI HTTP server with Swagger docs:
   ```bash
   uvx mcpo --port 8000 --api-key "secret" --config ./mcpo_config.json --hot-reload
   ```
   Then add the URL as a Tool Server in Open WebUI, or hit it from any HTTP client.

2. **Open WebUI native MCP** — Settings → Tools → MCP Servers (OAuth 2.1, Static, Bearer, None). Set `WEBUI_SECRET_KEY` so encrypted credentials survive restarts.

3. **`ollama-mcpo-adapter`** in Python — translates MCPO's OpenAPI shape into the Ollama `tools=[...]` parameter so plain `ollama.chat()` calls can invoke MCP tools.

### 5.4 LangChain / LlamaIndex / CrewAI essentials

All three frameworks have first-class Ollama support. Minimal patterns:

**LangChain + LangGraph ReAct agent** (the modern replacement for `AgentExecutor`):
```python
from langchain_ollama import ChatOllama
from langchain_core.tools import tool
from langgraph.prebuilt import create_react_agent

@tool
def web_search(query: str) -> str:
    "Search the web."
    return f"results for: {query}"

agent = create_react_agent(
    model=ChatOllama(model="qwen2.5:3b", num_ctx=8192, temperature=0),
    tools=[web_search],
)
agent.invoke({"messages":[("user","Latest Mars rover news?")]})
```

**LlamaIndex FunctionAgent** with global Settings:
```python
from llama_index.core import Settings
from llama_index.llms.ollama import Ollama
from llama_index.embeddings.ollama import OllamaEmbedding
from llama_index.core.agent.workflow import FunctionAgent
Settings.llm = Ollama(model="qwen2.5:3b", request_timeout=120.0, context_window=8192)
Settings.embed_model = OllamaEmbedding(model_name="nomic-embed-text")
```

**CrewAI** uses LiteLLM under the hood — keep the `ollama/` prefix:
```python
from crewai import LLM, Agent, Task, Crew, Process
llm = LLM(model="ollama/qwen2.5:3b", base_url="http://localhost:11434", temperature=0.2)
# Then build Agent(role=..., goal=..., backstory=..., llm=llm, ...)
```

**External system access** is best done via MCP servers with sandboxing baked in (filesystem scoped to one directory; Postgres in a read-only role) rather than raw `subprocess.run`. Treat any shell-execution tool as untrusted code execution: allow-list commands, run inside Docker/`firejail`/`bubblewrap`, never `shell=True`, and never give a writable filesystem + an HTTP-fetch tool to the same agent.

---

## 6. Agentic operation

### 6.1 Framework survey (May 2026)

| Framework | Style | Local-Ollama fit | Best for |
|---|---|---|---|
| **AutoGen 0.4+** | actor-model async; `agentchat`+`core`+`ext` | `OllamaChatCompletionClient` or OpenAI-compat | Magentic-One orchestrator, GroupChat |
| **CrewAI** | role-based Crews + event-driven Flows | LiteLLM `ollama/<model>` | role ergonomics, fast prototypes |
| **LangGraph** | StateGraph runtime, supervisor/swarm | `ChatOllama` | production, HITL, persistence |
| **smolagents** | minimalist (~1 kLOC); CodeAgent + ToolCallingAgent | `LiteLLMModel("ollama_chat/...")` | **best for small models** (code-as-action) |
| **PydanticAI** | typed-first | `OpenAIChatModel + OllamaProvider` | strict structured outputs, eval-driven |
| **OpenAI Agents SDK** | Agent + handoffs + Runner | `AsyncOpenAI(base_url=ollama/v1)` | minimal multi-agent, tracing |
| **AgentScope / Atomic / PocketFlow** | research/educational | any | visible ReAct, embedded use |
| **Letta** (formerly MemGPT) | agent runtime with OS memory | native Ollama | unbounded effective memory |

### 6.2 ReAct on small models — what actually breaks and how to fix it

Common failure modes on 3 B–8 B models: hallucinated tool names, malformed JSON args, format drift mid-loop, tool-storm repetition, premature "Final Answer" before tool results land. The mitigations that actually work:

1. **Prefer native function calling** over text-ReAct. Pick a model with `function_calling=True`.
2. **Constrain output by schema** — Ollama's `format=<schema>`, PydanticAI's `output_type`, `instructor` with retries. This kills the malformed-JSON failure class.
3. **Use code agents** for reasoning — smolagents `CodeAgent` emits Python rather than JSON tool-call blobs, which is more in-distribution for trained models and avoids JSON parsing entirely.
4. **Keep tool surfaces small** (≤5 tools per agent); use a two-stage *category → tool* selection if you have many.
5. **Step budgets** — `max_iterations`, `recursion_limit` of 6–12 prevents runaway loops.
6. **Temperature 0–0.2** in the agent loop; raise only inside writer/summary nodes.
7. **Caveat**: strict constrained decoding can *hurt* reasoning ("structure snowballing", Zhou 2026). The fix is *draft-then-constrain*: let the model draft freely, then enforce schema on a second pass.

### 6.3 Memory and state

Short-term context budget on this box is **4–8 K tokens** for 7 B partial-offload, comfortably 8–16 K for 3 B models with q8 KV cache. Strategies that matter: a hard `num_ctx` cap (default 2048 is too small for agents), sliding windows of last N turns, periodic summarization buffers, and aggressive truncation of tool results (most blow-ups come from a search tool returning 50 KB of HTML).

For long-term memory, **Chroma** or **FAISS** are right for prototypes; **Qdrant** in Docker for production. LangGraph **checkpointers** (`SqliteSaver`, `PostgresSaver`) persist agent state across runs and enable time-travel via `.get_state_history()`. **mem0** bolts cross-session, self-editing memory onto any framework. **Letta** is the right pick if memory *is* the product — it manages an OS-style core/recall/archival tier inside its runtime, and the agent uses tool calls (`core_memory_replace`, `archival_memory_insert`) to manage its own state.

---

## 7. SearXNG for local web search

**Repo status as of 2026:** `github.com/searxng/searxng-docker` was archived in March 2026. The recommended deployment is now the in-tree `container/` template at `github.com/searxng/searxng`, which uses **Valkey 9** instead of Redis and ships *without* Caddy.

### 7.1 Install (current path)

```bash
mkdir -p ~/searxng/core-config && cd ~/searxng
curl -fsSL \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/docker-compose.yml \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/.env.example
cp .env.example .env
echo "SEARXNG_SECRET=$(openssl rand -hex 32)" >> .env
docker compose up -d
```

Settings are auto-generated in `core-config/settings.yml` on first boot. Edit them and `docker compose restart`.

### 7.2 Critical `settings.yml` flags

```yaml
use_default_settings: true
search:
  formats: [html, json]   # *** REQUIRED for programmatic access ***
server:
  secret_key: "<replace>"  # from $SEARXNG_SECRET
  limiter: false           # false for local/agent use; true for public
  public_instance: false
  bind_address: "127.0.0.1"
  method: "GET"            # curl/agent-friendly
```

The two most common failure modes are **403 on JSON requests** (you forgot `formats: [html, json]`) and **bot detection blocking your client** (set `server.limiter: false` for local use, or override `limiter.toml` to pass-list your network, and spoof a normal browser User-Agent — never send raw `python-requests/2.x`).

### 7.3 JSON API

```bash
curl -s -G 'http://localhost:8080/search' \
  --data-urlencode 'q=ollama benchmarks' \
  --data-urlencode 'format=json' \
  --data-urlencode 'categories=general,it' \
  --data-urlencode 'engines=google,duckduckgo,github' \
  --data-urlencode 'language=en' \
  --data-urlencode 'pageno=1' \
  -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64)' | jq .
```

Response contains `results[]` (url, title, content, engine, score), `answers[]` (direct answers from plugins like Calculator), `infoboxes[]` (Wikipedia-like panels), `suggestions[]`, `corrections[]`, and `unresponsive_engines[]` — always check the last one when you get empty results (Brave and DuckDuckGo rate-limit quickly).

### 7.4 Integrating with the agent stack

**Open WebUI** — set `SEARXNG_QUERY_URL=http://searxng:8080/search?q=<query>`. The `<query>` placeholder is literal.

**LangChain** — `from langchain_community.utilities import SearxSearchWrapper` and use `SearxSearchResults` as a tool. Multi-tool variant (one tool per engine: `google,duckduckgo,wikipedia` / `github` / `arxiv`) gives the LLM specialized routing.

**CrewAI / smolagents** — wrap a `requests.get` in `BaseTool._run` or a smolagents `Tool.forward` (see code in §8.1 below).

**MCP** — `mcp-searxng` (npm, Docker `isokoliuk/mcp-searxng`) is the actively-maintained MCP server with `web_search`, `read_url` (URL→markdown), pagination, time-range, language, and safesearch. It supports both stdio and HTTP transports.

---

## 8. Example use cases — agent orchestration

### 8.1 Automated research with local LLMs + SearXNG

Architecture: **Planner → Searcher → Reader → Synthesizer → Writer**. Heavy nodes (planner, synthesis, writer) run on `qwen2.5:7b` partial-offload; light nodes (URL-relevance filter) run on `llama3.2:3b` fully on GPU.

A compact LangGraph implementation feeds SearXNG's JSON `/search` endpoint, runs URL relevance through the small filter model, fetches and extracts content via `trafilatura.fetch_url → trafilatura.extract`, synthesizes notes, then writes a polished Markdown report. The whole flow uses `httpx.AsyncClient` for parallel searches across subqueries and deduplicates hits by URL.

```python
import asyncio, json, httpx, trafilatura
from typing import TypedDict, List
from langchain_ollama import ChatOllama
from langgraph.graph import StateGraph, START, END

planner  = ChatOllama(model="qwen2.5:7b", temperature=0.2, num_ctx=8192)
filterer = ChatOllama(model="llama3.2:3b", temperature=0)
synth    = ChatOllama(model="qwen2.5:7b", temperature=0.3, num_ctx=16384)
writer   = ChatOllama(model="qwen2.5:7b", temperature=0.5, num_ctx=16384)
SEARXNG  = "http://localhost:8080/search"

class S(TypedDict):
    topic: str; subqueries: List[str]; hits: List[dict]
    docs: List[dict]; notes: str; report: str

def plan(s):
    raw = planner.invoke(f"Break the topic into 4-6 web-search queries as JSON list. "
                         f"Topic: {s['topic']}").content
    return {"subqueries": json.loads(raw[raw.find('['):raw.rfind(']')+1])}

async def _search(q):
    async with httpx.AsyncClient(timeout=20) as c:
        r = await c.get(SEARXNG, params={"q": q, "format":"json"})
        return [{"url":h["url"],"title":h["title"],"snippet":h.get("content","")}
                for h in r.json()["results"][:8]]

def search(s):
    batches = asyncio.run(asyncio.gather(*[_search(q) for q in s["subqueries"]]))
    seen, uniq = set(), []
    for h in [x for b in batches for x in b]:
        if h["url"] not in seen: seen.add(h["url"]); uniq.append(h)
    return {"hits": uniq}
# read/synthesize/write nodes follow the same pattern; full code in research_graph.py
```

### 8.2 File-organizer agent

Treats file content as deterministic text-extraction (`pypdf` for PDFs, mimetype sniff for the rest, optional `moondream` for image captioning), then asks `qwen2.5:7b` with `format="json"` to return `{"category": ..., "new_name": ...}` and moves the file accordingly. Categories: invoices, contracts, papers, code, photos, personal, misc.

### 8.3 Software-development assistance

**`qwen2.5-coder:7b`** for chat/edit + **`qwen2.5-coder:1.5b-base`** for tab completion is the proven Continue.dev configuration on this hardware. Continue.dev's `~/.continue/config.yaml` declares both models with roles `[chat, edit, apply]` and `[autocomplete]` respectively, points `apiBase` at `http://localhost:11434`, sets `contextLength: 8192`, and binds `nomic-embed-text` for repo retrieval. Aider with `--model ollama_chat/qwen2.5-coder:7b --map-tokens 1024` is the equivalent CLI option. The `ollama_chat/` prefix forces the chat endpoint (better instruction-following than legacy `generate`).

`deepseek-coder-v2:16b` is a tempting MoE alternative but won't fit comfortably under 4 GB VRAM and is slow on CPU offload — stick with Qwen2.5 Coder.

### 8.4 Web scraping with LLM extraction

**Crawl4AI** is the right tool. Use a small model (`llama3.2:3b`) for schema-filling extraction with `chunk_token_threshold=1500` so small contexts don't overflow. Pydantic schema definition + `extraction_type="schema"` gives validated `Product`/`Article` objects. For stable layouts, prefer Crawl4AI's `JsonCssExtractionStrategy` (no LLM) — its LLM-assisted *schema generator* makes that switch one call away.

### 8.5 Master-subagent harness

Three idiomatic patterns:

**CrewAI hierarchical** — manager is implicit; `agents=[workers...]`, `manager_llm=qwen2.5:7b`, the manager is *not* in the workers list. Long-running issue (#4783) where the manager sometimes does all the work itself is mitigated by an explicit `manager_agent=Agent(...)` with a step-by-step system prompt.

**LangGraph supervisor** (`langgraph-supervisor` package) — `transfer_to_<agent_name>` handoff tools wired automatically; supervisor LLM picks the next worker each step. More accurate routing than CrewAI hierarchical, with cleaner observability via `app.stream(stream_mode="updates")`.

**AutoGen 0.4 / Magentic-One** — orchestrator with a Task Ledger + Progress Ledger (the "shared blackboard"). Microsoft warns Magentic-One expects a strong reasoning model; with `qwen2.5:7b` keep tasks short and the worker count low (planner + coder + executor is the sweet spot).

---

## 9. Multi-model systems

### 9.1 Mixture-of-Experts council (single host)

The empirical foundation is the **Mixture-of-Agents** paper (Wang et al., arXiv 2406.04692, ICLR 2025 Spotlight) — diverse small models, each shown its peers' outputs, can collectively reach GPT-4-Omni-level quality on AlpacaEval 2.0 (65.1 %). The 2025 follow-up (**Self-MoA**, Li et al.) finds that *k* samples from one strong model often beats heterogeneous MoA by 3–7 points on common benchmarks. So the practical recipe on this box is:

- **Closed-form** (math/MCQ/structured) → **Self-Consistency** with k=5–10 samples on `phi-4-mini`, majority-vote the final-line answer. Cheaper than MoA, equally effective.
- **Open-ended** (drafting, summarization, analysis) → **3-proposer + 1-aggregator MoA**, optionally with a 2nd refinement layer.
- **Code** → 3 proposers + LLM-as-judge that picks the answer that compiles or passes tests.

Hardware allocation: pin the **aggregator** (`qwen3:4b` or `phi-4-mini`) on the GPU; run 3–4 **proposers** (`phi-4-mini`, `qwen3:4b`, `llama3.2:3b`, `gemma3:4b`, `deepseek-r1-distill-qwen:1.5b`) on CPU. Total RAM ≈ 12–14 GB, VRAM ≈ 3 GB. Concurrent throughput on CPU is roughly **5–15 tok/s per 3 B Q4 model** on the i9-10900, and running 3 in parallel shares cores so each gets ~⅓. End-to-end latency is the slowest proposer plus the aggregator — typically 30–60 s for ~500-token answers.

Tune Ollama for this:
```ini
Environment="OLLAMA_MAX_LOADED_MODELS=5"
Environment="OLLAMA_NUM_PARALLEL=2"
Environment="OLLAMA_KEEP_ALIVE=30m"
Environment="OLLAMA_FLASH_ATTENTION=1"
```

The voting council implementation is a straightforward `asyncio.gather` over `/api/chat` calls, followed by either majority voting (extract final-line ANSWER and `Counter.most_common(1)`) or an aggregator pass with a *Refine* system prompt instructing the aggregator to "critically evaluate the information, recognize that some may be biased or incorrect; do not merely replicate; refine." For dedup before voting, embed each draft with `nomic-embed-text` and drop near-duplicates above a cosine similarity of 0.92.

### 9.2 Master-slave architecture (LAN-distributed)

**Topology** when you can deploy LAN slaves:

```
MASTER (this P1000 box)
├─ Open WebUI :3000  →  LiteLLM proxy :4000  →  fan-out
├─ Ollama :11434       (phi4-mini, qwen3:4b, qwen2.5-coder:3b, nomic-embed-text)
├─ Redis :6379          (LiteLLM cache + rate-limit state)
└─ Postgres :5432       (virtual keys, budgets)

Tailscale mesh (WireGuard, MagicDNS) — no port-forwards to the internet
├─ Slave A (RTX 4090)   vLLM :8000  qwen2.5-32b-awq / llama-3.3-70b-awq
├─ Slave B (any GPU)    A1111 :7860 / ComfyUI :8188  (SDXL / Flux)
└─ Slave C (CPU)        whisper.cpp :9000, Piper TTS :10200, TEI :8080
```

**LiteLLM is the right gateway** for this pattern. One OpenAI-compatible endpoint (`/v1/chat/completions`, `/v1/embeddings`, `/v1/images/generations`, `/health`) on the master, fanning out to ~100 backend types. A representative `litellm_config.yaml` declares aliases like `local-fast` (Ollama `phi4-mini`), `local-aggregator` (Ollama `qwen3:4b`), `remote-smart` (vLLM Qwen2.5-32B-AWQ on Slave A — declared twice for load balancing), `code` (`qwen2.5-coder:3b`), `embed` (TEI on Slave C), and `image` (A1111 via an adapter shim).

Routing strategies available: `simple-shuffle`, `least-busy`, `usage-based-routing-v2`, `latency-based-routing`. Reliability primitives include `num_retries`, `allowed_fails`/`cooldown_time` (the circuit breaker), `fallbacks` (cross-group fallback chains, e.g. *remote-smart down → local-aggregator → local-fast*), and `context_window_fallbacks` (auto-escalate long prompts to the 70 B). With Postgres wired up, `/key/generate` mints per-app virtual keys with `max_budget`, `rpm_limit`, `tpm_limit`.

**Stable Diffusion as a tool**: AUTOMATIC1111 with `--api --api-auth user:pass --xformers` exposes `/sdapi/v1/txt2img`; the wrapper tool is ~10 lines (POST JSON, base64-decode the returned PNG). ComfyUI uses `/prompt` with a workflow JSON exported from the GUI in *Save (API Format)* mode and supports a real-time WebSocket at `/ws?clientId=...` for streaming progress. Flux.1-schnell/dev now runs in ~12 GB VRAM via ComfyUI's GGUF nodes.

**Security non-negotiables**: bind Ollama to the tailnet IP via `OLLAMA_HOST=100.x.y.z:11434`, not `0.0.0.0`; deny inbound 11434 at the host firewall (`ufw deny 11434`); never use Tailscale Funnel for Ollama (no built-in auth); put bearer tokens on every slave endpoint; rotate `LITELLM_MASTER_KEY` and `LITELLM_SALT_KEY` periodically; pin model digests (`ollama pull qwen2.5:7b@<digest>`) — silent autoupdates have broken prompt templates before.

**When does the master-slave pattern pay off?** When you can reuse existing hardware (the P1000 box stays a productive orchestrator), need independent scaling (add a GPU host without touching the master), want failure isolation (image-gen crash ≠ chat down), and prefer to keep loud GPUs in a closet on Wake-on-LAN. The proxy overhead is 5–15 ms per request; LAN RTT is sub-millisecond; gigabit bandwidth is wildly over-provisioned for token streaming. Power matters: a 4090 box at full inference draws 300–450 W and is loud; the P1000 master idles at ~30 W, and a master-side `wake` tool the LLM can call lets you save 5–8 kWh/day.

---

## 10. What to actually do on day one

The fastest path from a bare Ubuntu 20.04 P340 to a productive local-LLM workstation:

```bash
# 1. Driver + CUDA (§1.1)
# 2. Docker + NVIDIA Container Toolkit (§1.2)
# 3. Ollama with hardware-tuned override (§1.3)
ollama pull qwen2.5:3b qwen2.5-coder:3b llama3.2:3b nomic-embed-text llava-phi3

# 4. Open WebUI + SearXNG bundled docker-compose (§1.4 + §7.1)
mkdir -p ~/ai-stack && cd ~/ai-stack
# paste docker-compose.yml from §1.4, add SearXNG service with the formats:[html,json] config
docker compose up -d

# 5. uv + Python 3.11 + agent stack (§1.6)
curl -LsSf https://astral.sh/uv/install.sh | sh
uv venv --python 3.11 && source .venv/bin/activate
uv pip install ollama langchain langchain-ollama langgraph llama-index \
   llama-index-llms-ollama llama-index-embeddings-ollama \
   crewai 'crewai[tools]' langchain-mcp-adapters httpx trafilatura

# 6. MCP servers on demand (no install needed; npx/uvx pull on first use)
npx -y @modelcontextprotocol/server-filesystem ~/projects
uvx mcp-server-time --local-timezone=Europe/Berlin
```

Then point your browser at `http://localhost:3000`, create the admin user, enable Web Search (Engine = `searxng`, URL = `http://searxng:8080/search?q=<query>`), upload your first knowledge base, and either drive the box via the Open WebUI chat UI or hit `/api/chat/completions` with a Bearer token from any agent framework.

## Closing observations

Three findings deserve emphasis because they reshape the design decisions on this hardware. **First**, the 4 GB VRAM ceiling is more flexible than it looks if you embrace partial offload and `OLLAMA_KV_CACHE_TYPE=q8_0` — a 7 B Q4 model with hybrid CPU/GPU runs at 6–12 tok/s on this i9-10900, which is interactive enough for orchestration work, and self-consistency or MoA councils on 3 B models genuinely close the quality gap with single larger models on many tasks. **Second**, the most reliable agent pattern on small local models is not the textual ReAct loop that early LangChain documentation popularized but **code-as-action agents** (smolagents `CodeAgent`) plus **schema-constrained outputs** (Ollama `format=<schema>`, PydanticAI `output_type`) — these collapse the malformed-JSON failure class that ruins 3 B–8 B tool calling. **Third**, the right scaling story for someone on this box who eventually adds more hardware is not "buy a bigger GPU and replace this one" but **LiteLLM-fronted master-slave over Tailscale**: the P1000 box stays a productive orchestrator and embedding/cache server, while a single LAN GPU host running vLLM provides the heavy-model layer. That architecture composes cleanly, isolates failures, and lets each piece evolve independently — which is exactly the property you want from a system that has to keep working as the local-LLM ecosystem continues its monthly churn through 2026 and beyond.