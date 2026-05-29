# SearXNG with Ollama, LM Studio, and Open WebUI on Ubuntu 20.04

**Private Web Search for Your Local AI Stack**

*"The whole system is itself a kind of search engine — only the queries are issued by programs instead of people, and the index is the live web instead of a crawl."*

---

## What This Guide Covers

SearXNG is a self-hosted metasearch engine that fans queries out to Google, DuckDuckGo, Brave, Bing, Wikipedia, GitHub, arXiv, and 70+ other sources, aggregates the results, strips tracking, and returns clean JSON. It gives your local LLMs the ability to search the web without sending your prompts to any external AI service. The only outbound traffic is SearXNG scraping public search engines — your conversations, system prompts, and agent state never leave your machine.

This guide wires SearXNG into three local AI runtimes on Ubuntu 20.04:

- **Open WebUI** — native first-class integration via `SEARXNG_QUERY_URL`
- **Ollama** — via the tool-calling loop, MCP servers, and agent frameworks
- **LM Studio** — via MCP (`~/.lmstudio/mcp.json`) and the OpenAI-compatible API

All three share a single SearXNG instance running in Docker.

---

## 1 · Prerequisites

### 1.1 Ubuntu 20.04 Baseline

```bash
sudo apt update && sudo apt upgrade -y
sudo apt install -y curl git jq docker.io docker-compose-plugin
sudo usermod -aG docker $USER && newgrp docker
```

Verify Docker Compose v2:

```bash
docker compose version
# Docker Compose version v2.x.y
```

If your system has the old `docker-compose` (v1), install the v2 plugin:

```bash
sudo apt install docker-compose-plugin
```

### 1.2 Node.js (for MCP servers)

MCP servers like `mcp-searxng` run via `npx` and need Node.js ≥ 18:

```bash
curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
sudo apt install -y nodejs
node --version  # v20.x
```

### 1.3 Python 3.11 (for agent frameworks)

Open WebUI and the agent stack require Python 3.11:

```bash
sudo add-apt-repository ppa:deadsnakes/ppa -y && sudo apt update
sudo apt install -y python3.11 python3.11-venv python3.11-dev
```

### 1.4 Ollama

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama pull qwen2.5:7b nomic-embed-text
```

### 1.5 LM Studio on Ubuntu 20.04

LM Studio officially targets 22.04+; on Focal the AppImage requires `libfuse2`:

```bash
sudo apt install -y libfuse2
wget https://releases.lmstudio.ai/linux/x86/LM-Studio-latest.AppImage
chmod +x LM-Studio-latest.AppImage
./LM-Studio-latest.AppImage
```

For headless server-only use, LM Studio ships a `llmster` daemon. For model exploration the GUI is superior; for production serving prefer Ollama.

---

## 2 · SearXNG Installation

### 2.1 Repo Status (2026)

The old `github.com/searxng/searxng-docker` was **archived in March 2026**. The recommended deployment is now the in-tree `container/` template at `github.com/searxng/searxng`, which uses **Valkey 9** instead of Redis and ships without Caddy.

### 2.2 Deploy

```bash
mkdir -p ~/searxng/core-config && cd ~/searxng

# Fetch the latest compose template from the main repo
curl -fsSL \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/docker-compose.yml \
  -O https://raw.githubusercontent.com/searxng/searxng/master/container/.env.example

cp .env.example .env
echo "SEARXNG_SECRET=$(openssl rand -hex 32)" >> .env

docker compose up -d
```

Settings are auto-generated in `core-config/settings.yml` on first boot. Edit them and restart:

```bash
nano ~/searxng/core-config/settings.yml
docker compose restart
```

### 2.3 Critical `settings.yml` Configuration

This is the file that makes or breaks your integration. The two most common failures are **403 on JSON requests** (you forgot `formats: [html, json]`) and **bot detection blocking your client** (you left `limiter: true` or sent a bare `python-requests/2.x` User-Agent).

```yaml
use_default_settings: true

search:
  formats: [html, json]         # *** REQUIRED for programmatic access ***
  default_lang: "en"
  autocomplete: "google"

server:
  secret_key: "paste-your-secret-here"   # from SEARXNG_SECRET in .env
  limiter: false                 # false for local/agent use; true for public
  public_instance: false
  bind_address: "0.0.0.0"       # within container; host binding is in docker-compose
  method: "GET"                  # curl/agent-friendly

# Override specific engines
engines:
  - name: google
    disabled: false
  - name: duckduckgo
    disabled: false
  - name: brave
    disabled: false
  - name: wikipedia
    disabled: false
  - name: github
    disabled: false
    tokens: []                   # optional GitHub API token for higher rate limits
  - name: arxiv
    disabled: false
```

Key points:

- `search.formats: [html, json]` — without `json` in this list, every `?format=json` request returns 403. This is the single most common misconfiguration.
- `server.limiter: false` — the rate limiter uses a `limiter.toml` to detect bots. For local use with agents hammering it, disable it entirely. If you leave it on, override `limiter.toml` to passlist your Docker network CIDR.
- `server.method: "GET"` — some clients default to POST; GET is more universally compatible with query-string encoding.
- `server.public_instance: false` — disables public instance features (analytics, instance info endpoint).

### 2.4 Verify

```bash
# HTML works?
curl -s 'http://localhost:8080/' | head -20

# JSON API works?
curl -s -G 'http://localhost:8080/search' \
  --data-urlencode 'q=hello world' \
  --data-urlencode 'format=json' \
  -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64)' | jq '.results[:2]'
```

If you get results with `url`, `title`, `content`, `engine`, and `score` fields, SearXNG is alive.

### 2.5 JSON API Reference

```bash
curl -s -G 'http://localhost:8080/search' \
  --data-urlencode 'q=ollama benchmarks' \
  --data-urlencode 'format=json' \
  --data-urlencode 'categories=general,it' \
  --data-urlencode 'engines=google,duckduckgo,github' \
  --data-urlencode 'language=en' \
  --data-urlencode 'pageno=1' \
  --data-urlencode 'time_range=month' \
  --data-urlencode 'safesearch=0' \
  -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64)' | jq .
```

Response structure:

| Field | Contents |
|-------|----------|
| `results[]` | `url`, `title`, `content`, `engine`, `score`, `publishedDate` |
| `answers[]` | Direct answers from plugins (Calculator, unit conversion) |
| `infoboxes[]` | Wikipedia-style summary panels |
| `suggestions[]` | Related search terms |
| `corrections[]` | Spelling corrections |
| `unresponsive_engines[]` | Engines that timed out or returned errors — **always check this when you get empty results** |

Brave and DuckDuckGo rate-limit quickly; if you see them in `unresponsive_engines` frequently, reduce request frequency or add a delay between agent queries.

---

## 3 · Integration with Open WebUI

This is the simplest path — Open WebUI has native SearXNG support.

### 3.1 The Full Stack: Docker Compose

This single `docker-compose.yml` brings up Ollama, Open WebUI, and SearXNG on one Docker network so they can address each other by container name:

```yaml
# ~/ai-stack/docker-compose.yml
services:

  # --- Ollama inference server ---
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

  # --- SearXNG metasearch ---
  searxng:
    image: searxng/searxng:latest
    container_name: searxng
    restart: unless-stopped
    ports: ["127.0.0.1:8080:8080"]
    volumes:
      - ./searxng:/etc/searxng:rw
    environment:
      - SEARXNG_SECRET=${SEARXNG_SECRET}
    cap_drop: [ALL]
    cap_add: [CHOWN, SETGID, SETUID]

  # --- Valkey (SearXNG's rate-limit/cache backend) ---
  valkey:
    image: valkey/valkey:9-alpine
    container_name: valkey
    restart: unless-stopped
    command: valkey-server --save 30 1 --loglevel warning
    volumes: [valkey-data:/data]

  # --- Open WebUI ---
  open-webui:
    image: ghcr.io/open-webui/open-webui:main
    container_name: open-webui
    restart: unless-stopped
    depends_on: [ollama, searxng]
    ports: ["3000:8080"]
    environment:
      - OLLAMA_BASE_URL=http://ollama:11434
      - WEBUI_AUTH=true
      - WEBUI_SECRET_KEY=${WEBUI_SECRET_KEY}
      - DEFAULT_USER_ROLE=pending
      #--- RAG ---
      - RAG_EMBEDDING_ENGINE=ollama
      - RAG_EMBEDDING_MODEL=nomic-embed-text
      - RAG_OLLAMA_BASE_URL=http://ollama:11434
      - ENABLE_RAG_HYBRID_SEARCH=true
      #--- Web Search ---
      - ENABLE_WEB_SEARCH=true
      - WEB_SEARCH_ENGINE=searxng
      - SEARXNG_QUERY_URL=http://searxng:8080/search?q=<query>
    volumes: [open-webui:/app/backend/data]

volumes:
  ollama: {}
  open-webui: {}
  valkey-data: {}
```

Create the `.env` file:

```bash
mkdir -p ~/ai-stack/searxng && cd ~/ai-stack
cat > .env << 'EOF'
SEARXNG_SECRET=$(openssl rand -hex 32)
WEBUI_SECRET_KEY=$(openssl rand -hex 32)
EOF
# Re-source to expand the $(…) or just paste literal values
```

Create the SearXNG settings:

```bash
cat > ~/ai-stack/searxng/settings.yml << 'YAML'
use_default_settings: true
search:
  formats: [html, json]
server:
  secret_key: "change-me-to-match-your-env"
  limiter: false
  public_instance: false
  method: "GET"
YAML
```

Bring it up:

```bash
cd ~/ai-stack
docker compose up -d
docker compose exec ollama ollama pull qwen2.5:7b nomic-embed-text
```

### 3.2 Configuring Open WebUI for SearXNG

Two levels of activation are required — a global admin toggle and a per-conversation user toggle:

**Step 1: Admin activation**

1. Browse to `http://localhost:3000` and register (first user becomes admin)
2. Go to **Admin Panel → Settings → Web Search**
3. Toggle **Enable Web Search** to ON
4. Set **Web Search Engine** to `searxng`
5. Set **SearXNG Query URL** to: `http://searxng:8080/search?q=<query>`

The literal `<query>` placeholder is **mandatory** — Open WebUI replaces it with the actual search terms at runtime. Forgetting it is the second most common failure after the missing `formats: [html, json]`.

**Step 2: Per-conversation activation**

Web search is not automatic. In each chat conversation, click the **globe icon** (🌐) on the input bar to toggle web search for that conversation. The model will then have access to SearXNG results as context for its responses.

**Step 3: Verify**

Ask the model something that requires current information:

> "What were the latest Ollama releases this month?"

If web search is working, you'll see a "Searched N results" indicator above the response, and the answer will include current information with source links.

### 3.3 Troubleshooting Open WebUI + SearXNG

| Symptom | Cause | Fix |
|---------|-------|-----|
| "Web search failed" in chat | SearXNG not reachable from Open WebUI container | Check `docker compose logs searxng`; verify they share the same Docker network |
| 403 error in logs | `formats: [html, json]` missing in `settings.yml` | Add it and `docker compose restart searxng` |
| Empty results, model says "I couldn't find anything" | `limiter: true` blocking the Open WebUI User-Agent | Set `limiter: false` in `settings.yml` |
| SearXNG works in browser but not from Open WebUI | Wrong URL — using `localhost` instead of container name | Use `http://searxng:8080/search?q=<query>` (the Docker DNS name) |
| Globe icon not visible in chat | Web search not enabled globally | Admin → Settings → Web Search → Enable |
| Globe icon visible but search never triggers | Per-conversation toggle not activated | Click the globe icon on the input bar |

---

## 4 · Integration with Ollama

Ollama itself is not an MCP host and has no built-in web search. There are three patterns to wire SearXNG into Ollama-powered workflows, from simplest to most powerful.

### 4.1 Pattern 1: Native Tool Calling (Direct API)

The simplest approach — write a Python function that calls SearXNG, and pass it to Ollama's tool-calling API. No frameworks needed.

```python
#!/usr/bin/env python3
"""searx_tool.py — Ollama + SearXNG via native tool calling."""

import json
import httpx
from ollama import chat

SEARXNG = "http://localhost:8080/search"
HEADERS = {"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}

def web_search(query: str, num_results: int = 5) -> str:
    """Search the web using SearXNG and return the top results."""
    r = httpx.get(SEARXNG, params={
        "q": query, "format": "json", "categories": "general",
        "language": "en", "pageno": 1,
    }, headers=HEADERS, timeout=15)
    results = r.json().get("results", [])[:num_results]
    return json.dumps([{
        "title": hit["title"],
        "url": hit["url"],
        "snippet": hit.get("content", "")[:300],
        "engine": hit["engine"],
    } for hit in results], indent=2)


# The universal tool-calling loop
messages = [{"role": "user", "content": "What are the latest changes in SearXNG?"}]

while True:
    response = chat(
        model="qwen2.5:7b",
        messages=messages,
        tools=[web_search],   # SDK auto-generates schema from docstring
    )

    if not response.message.tool_calls:
        print(response.message.content)
        break

    # Execute each tool call and feed results back
    messages.append(response.message)
    for tc in response.message.tool_calls:
        if tc.function.name == "web_search":
            result = web_search(**tc.function.arguments)
            messages.append({
                "role": "tool",
                "content": result,
                "name": tc.function.name,
            })
```

The Ollama Python SDK auto-generates JSON schemas from Python callables with Google-style docstrings when you pass them to the `tools=` parameter. This is the lowest-friction way to get search working.

**Models that reliably call tools** (verified on ollama.com with the `tools` capability filter):

- `qwen2.5:3b`, `qwen2.5:7b`, `qwen2.5-coder:3b` — best small tool callers
- `qwen3:4b`, `qwen3:8b` — hybrid thinking; disable `/think` mode in agent loops
- `llama3.1:8b`, `llama3.2:3b` — Meta's tool template, reliable for ≤ 3 tools
- `gemma3:4b`, `gemma4:e4b` — native tool calling, good multilingual
- `hermes3:8b` — strong function-calling fine-tune
- `mistral-nemo:12b`, `mistral:7b` — Mistral tool template

### 4.2 Pattern 2: MCP Server (mcp-searxng)

MCP (Model Context Protocol) is the standardized way to expose tools to LLMs. Three actively maintained SearXNG MCP servers exist:

| Package | Install | Features |
|---------|---------|----------|
| `mcp-searxng` (ihor-sokoliuk) | `npx -y mcp-searxng` | `web_search` + `read_url` (URL→markdown), pagination, time/language/safety filters |
| `mcp-searxng-scrape` (wfkpk) | `npx -y mcp-searxng-scrape` | Search + article scraping |
| `searxng-mul-mcp` (jae-jae) | Docker `ghcr.io/jae-jae/searxng-mul-mcp` | Multi-query parallel search, StreamableHTTP |

**Ollama is not an MCP host**, so you need a bridge. Three working patterns:

**Bridge 1: `mcpo` (MCP-to-OpenAPI proxy, recommended)**

Wraps any MCP server in a REST/OpenAPI HTTP server with Swagger docs:

```bash
# Install mcpo
pip install mcpo

# Create config
cat > mcpo_config.json << 'JSON'
{
  "mcpServers": {
    "searxng": {
      "command": "npx",
      "args": ["-y", "mcp-searxng"],
      "env": {
        "SEARXNG_URL": "http://localhost:8080"
      }
    }
  }
}
JSON

# Run the proxy
uvx mcpo --port 8000 --api-key "secret" --config ./mcpo_config.json --hot-reload
```

Now `http://localhost:8000` exposes an OpenAPI endpoint. Add it as a **Tool Server** in Open WebUI (Settings → Tools → Tool Servers), or call it from any HTTP client.

**Bridge 2: Open WebUI native MCP**

Recent Open WebUI versions support MCP directly: Settings → Tools → MCP Servers. Set the transport (stdio), command (`npx`), args (`["-y", "mcp-searxng"]`), and env (`SEARXNG_URL=http://searxng:8080`). Set `WEBUI_SECRET_KEY` so encrypted credentials survive restarts.

**Bridge 3: `ollama-mcpo-adapter` in Python**

Translates MCPO's OpenAPI shape into the Ollama `tools=[...]` parameter so plain `ollama.chat()` calls can invoke MCP tools programmatically.

### 4.3 Pattern 3: LangChain / LangGraph Agent

For production agent pipelines, wire SearXNG into a LangGraph `StateGraph`:

```python
#!/usr/bin/env python3
"""research_agent.py — LangGraph + Ollama + SearXNG research pipeline."""

import asyncio, json, httpx, trafilatura
from typing import TypedDict, List
from langchain_ollama import ChatOllama
from langgraph.graph import StateGraph, START, END

# Models — heavy reasoning on 7B, light filtering on 3B
planner  = ChatOllama(model="qwen2.5:7b", temperature=0.2, num_ctx=8192)
filterer = ChatOllama(model="llama3.2:3b", temperature=0)
writer   = ChatOllama(model="qwen2.5:7b", temperature=0.5, num_ctx=16384)

SEARXNG = "http://localhost:8080/search"
HEADERS = {"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}

class ResearchState(TypedDict):
    topic: str
    subqueries: List[str]
    hits: List[dict]
    docs: List[dict]
    report: str

def plan(state: ResearchState) -> dict:
    """Break topic into 4-6 search queries."""
    raw = planner.invoke(
        f"Break this topic into 4-6 web search queries. "
        f"Return ONLY a JSON array of strings.\n\nTopic: {state['topic']}"
    ).content
    queries = json.loads(raw[raw.find('['):raw.rfind(']')+1])
    return {"subqueries": queries}

async def _search(query: str) -> List[dict]:
    async with httpx.AsyncClient(timeout=20) as c:
        r = await c.get(SEARXNG, params={"q": query, "format": "json"},
                        headers=HEADERS)
        return [{"url": h["url"], "title": h["title"],
                 "snippet": h.get("content", "")}
                for h in r.json().get("results", [])[:8]]

def search(state: ResearchState) -> dict:
    """Fan-out parallel searches, deduplicate by URL."""
    batches = asyncio.run(asyncio.gather(
        *[_search(q) for q in state["subqueries"]]
    ))
    seen, unique = set(), []
    for hit in [x for batch in batches for x in batch]:
        if hit["url"] not in seen:
            seen.add(hit["url"])
            unique.append(hit)
    return {"hits": unique}

def read(state: ResearchState) -> dict:
    """Fetch pages, extract text, truncate for context budget."""
    docs = []
    for hit in state["hits"][:10]:
        try:
            downloaded = trafilatura.fetch_url(hit["url"])
            text = trafilatura.extract(downloaded, include_comments=False) or ""
            if len(text) > 200:
                docs.append({"url": hit["url"], "title": hit["title"],
                             "content": text[:3000]})
        except Exception:
            continue
    return {"docs": docs}

def write(state: ResearchState) -> dict:
    """Produce the final report."""
    doc_text = "\n\n---\n\n".join(
        f"## {d['title']}\n{d['content']}" for d in state["docs"]
    )
    report = writer.invoke(
        f"Write a polished Markdown research report on '{state['topic']}' "
        f"using these sources. Include key findings and source links.\n\n{doc_text}"
    ).content
    return {"report": report}

# Wire the graph
graph = StateGraph(ResearchState)
graph.add_node("plan", plan)
graph.add_node("search", search)
graph.add_node("read", read)
graph.add_node("write", write)
graph.add_edge(START, "plan")
graph.add_edge("plan", "search")
graph.add_edge("search", "read")
graph.add_edge("read", "write")
graph.add_edge("write", END)

app = graph.compile()
result = app.invoke({"topic": "SearXNG 2026 deployment best practices"})
print(result["report"])
```

**LangChain also ships a native wrapper:**

```python
from langchain_community.utilities import SearxSearchWrapper

searx = SearxSearchWrapper(
    searx_host="http://localhost:8080",
    headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"},
)
results = searx.results("Ollama latest release", num_results=5)
```

Use `SearxSearchResults` as a LangChain `Tool` for agent integration.

---

## 5 · Integration with LM Studio

LM Studio integrates with SearXNG through two paths: MCP (native since mid-2025) and the OpenAI-compatible API on port 1234.

### 5.1 MCP Configuration

LM Studio reads MCP server configs from `~/.lmstudio/mcp.json`:

```json
{
  "mcpServers": {
    "searxng": {
      "command": "npx",
      "args": ["-y", "mcp-searxng"],
      "env": {
        "SEARXNG_URL": "http://localhost:8080"
      }
    },
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/you/projects"]
    }
  }
}
```

After saving, restart LM Studio. The MCP tools appear in the chat interface and are available to any loaded model that supports tool calling.

The `mcp-searxng` server (ihor-sokoliuk) exposes two tools:

- **`searxng_web_search`** — execute web searches with pagination, time range, language, and safesearch filters
- **`web_url_read`** — fetch a URL and convert its content to Markdown with heading extraction and section filtering

### 5.2 Using LM Studio's OpenAI API with SearXNG

LM Studio exposes an OpenAI-compatible server on port 1234. You can use the same Python patterns from §4.1, just point at a different base URL:

```python
from openai import OpenAI
import json, httpx

client = OpenAI(base_url="http://localhost:1234/v1", api_key="lm-studio")

def web_search(query: str) -> str:
    """Search the web using SearXNG."""
    r = httpx.get("http://localhost:8080/search", params={
        "q": query, "format": "json",
    }, headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}, timeout=15)
    hits = r.json().get("results", [])[:5]
    return json.dumps([{"title": h["title"], "url": h["url"],
                        "snippet": h.get("content", "")[:300]}
                       for h in hits])

tools = [{
    "type": "function",
    "function": {
        "name": "web_search",
        "description": "Search the web using SearXNG",
        "parameters": {
            "type": "object",
            "required": ["query"],
            "properties": {"query": {"type": "string"}},
        },
    },
}]

messages = [{"role": "user", "content": "What's new in LM Studio 0.4?"}]

response = client.chat.completions.create(
    model="qwen2.5-7b-instruct",   # whatever is loaded in LM Studio
    messages=messages,
    tools=tools,
)

# Handle tool calls
if response.choices[0].message.tool_calls:
    for tc in response.choices[0].message.tool_calls:
        result = web_search(**json.loads(tc.function.arguments))
        messages.append(response.choices[0].message)
        messages.append({
            "role": "tool",
            "tool_call_id": tc.id,
            "content": result,
        })
    final = client.chat.completions.create(
        model="qwen2.5-7b-instruct", messages=messages
    )
    print(final.choices[0].message.content)
```

### 5.3 LM Studio Performance Settings for Ubuntu 20.04

LM Studio on Focal runs but has quirks. Key settings:

| Setting | Recommendation |
|---------|---------------|
| GPU offload layers | Maximum if the green rocket icon appears |
| Context length | 2048–4096 (each extra 1K ≈ 50–100 MB KV cache) |
| CPU threads | Physical cores only (check `lscpu | grep "Core(s) per socket"`) |
| Flash Attention | ON (required for KV cache quantization) |
| K-cache / V-cache quantization | q8_0 (V-cache requires Flash Attention) |

Estimate VRAM before loading: `lms load qwen2.5-7b-instruct --estimate-only`

---

## 6 · Advanced: SearXNG as a smolagents Tool

For code-as-action agents (the most reliable pattern on small local models), use smolagents with a SearXNG tool:

```python
#!/usr/bin/env python3
"""smolagent_search.py — code-as-action agent with SearXNG."""

from smolagents import CodeAgent, Tool, LiteLLMModel
import httpx, json

class SearXNGTool(Tool):
    name = "web_search"
    description = "Search the web via SearXNG. Returns titles, URLs, and snippets."
    inputs = {
        "query": {"type": "string", "description": "The search query"},
        "num_results": {"type": "integer", "description": "Number of results (default 5)",
                        "nullable": True},
    }
    output_type = "string"

    def forward(self, query: str, num_results: int = 5) -> str:
        r = httpx.get("http://localhost:8080/search", params={
            "q": query, "format": "json", "categories": "general",
        }, headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}, timeout=15)
        hits = r.json().get("results", [])[:num_results]
        return json.dumps([{
            "title": h["title"], "url": h["url"],
            "snippet": h.get("content", "")[:300],
        } for h in hits], indent=2)

class FetchURLTool(Tool):
    name = "fetch_url"
    description = "Fetch a URL and extract its text content."
    inputs = {"url": {"type": "string", "description": "The URL to fetch"}}
    output_type = "string"

    def forward(self, url: str) -> str:
        import trafilatura
        downloaded = trafilatura.fetch_url(url)
        text = trafilatura.extract(downloaded, include_comments=False) or ""
        return text[:4000]  # truncate for context budget

model = LiteLLMModel("ollama_chat/qwen2.5:7b",
                      api_base="http://localhost:11434")

agent = CodeAgent(
    tools=[SearXNGTool(), FetchURLTool()],
    model=model,
    max_steps=8,
)

result = agent.run(
    "Find the latest SearXNG release notes and summarize the key changes."
)
print(result)
```

smolagents `CodeAgent` emits Python rather than JSON tool-call blobs — this collapses the malformed-JSON failure class that plagues 3B–8B models doing traditional ReAct.

---

## 7 · Advanced: CrewAI with SearXNG

```python
from crewai import LLM, Agent, Task, Crew, Process
from crewai.tools import BaseTool
import httpx, json

class SearXNGSearchTool(BaseTool):
    name: str = "SearXNG Web Search"
    description: str = "Search the web using a local SearXNG instance."

    def _run(self, query: str) -> str:
        r = httpx.get("http://localhost:8080/search", params={
            "q": query, "format": "json",
        }, headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}, timeout=15)
        hits = r.json().get("results", [])[:5]
        return "\n\n".join(
            f"**{h['title']}**\n{h['url']}\n{h.get('content', '')[:200]}"
            for h in hits
        )

llm = LLM(model="ollama/qwen2.5:7b",
           base_url="http://localhost:11434",
           temperature=0.2)

researcher = Agent(
    role="Research Analyst",
    goal="Find current, accurate information on the given topic",
    backstory="You are a meticulous researcher who always cites sources.",
    llm=llm,
    tools=[SearXNGSearchTool()],
)

task = Task(
    description="Research the current state of SearXNG development in 2026.",
    expected_output="A structured summary with key findings and source URLs.",
    agent=researcher,
)

crew = Crew(agents=[researcher], tasks=[task], process=Process.sequential)
result = crew.kickoff()
print(result)
```

---

## 8 · SearXNG Engine Tuning for AI Agents

### 8.1 Recommended Engine Profiles

Different agent tasks benefit from different engine configurations. Specify engines in the API call via `&engines=google,duckduckgo,arxiv`:

| Use Case | Engines | Categories |
|----------|---------|------------|
| General knowledge | `google,duckduckgo,brave,wikipedia` | `general` |
| Technical / code | `google,github,stackoverflow` | `it` |
| Academic research | `google_scholar,arxiv,semantic_scholar` | `science` |
| News / current events | `google_news,duckduckgo,brave,bing_news` | `news` |
| Images (for VLM context) | `google_images,duckduckgo_images,brave_images` | `images` |

### 8.2 Rate Limiting and Caching

SearXNG caches results via Valkey (Redis fork). For agent workloads that repeat similar queries:

```yaml
# In settings.yml
server:
  limiter: false              # off for local use

# Valkey/Redis URL (auto-configured in the container template)
redis:
  url: redis://valkey:6379/0
```

For high-volume agent pipelines (100+ queries/hour), add delays between requests to avoid upstream engine bans:

```python
import time

def search_with_backoff(query, retries=3, delay=2.0):
    for attempt in range(retries):
        try:
            return web_search(query)
        except Exception:
            if attempt < retries - 1:
                time.sleep(delay * (attempt + 1))
    return "[]"
```

### 8.3 User-Agent Spoofing

SearXNG itself handles User-Agent rotation when talking to upstream engines. But **your client talking to SearXNG** must also send a plausible User-Agent, or the limiter (if enabled) blocks it:

```python
# Always do this
headers = {"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"}

# Never do this
headers = {"User-Agent": "python-requests/2.31.0"}  # instant block
```

---

## 9 · Troubleshooting Reference

| Problem | Diagnosis | Solution |
|---------|-----------|----------|
| 403 on `?format=json` | Missing JSON format in settings | Add `search.formats: [html, json]` to `settings.yml` |
| Empty results array | Engines rate-limited or timed out | Check `unresponsive_engines[]` in response; add delay between queries |
| "Connection refused" from Open WebUI | SearXNG not on same Docker network | Verify both are in the same `docker-compose.yml` or share a named network |
| Results but model ignores them | Context too short | Increase `num_ctx` in Ollama (minimum 8192 for search+response) |
| SearXNG starts but no engines work | DNS resolution failure in container | Add `dns: [8.8.8.8, 1.1.1.1]` to the searxng service in docker-compose |
| MCP `mcp-searxng` fails to start | Node.js < 18 | Upgrade to Node.js 20 LTS |
| Tool calls return malformed JSON | Model too small for reliable tool calling | Use `qwen2.5:7b` or larger; or switch to smolagents CodeAgent |
| SearXNG very slow (>10s per query) | Too many engines enabled | Disable slow engines (Brave, Mojeek) or set `timeout: 5` per engine |
| Open WebUI shows "Searched 0 results" | Limiter blocking Docker internal traffic | Set `server.limiter: false` |

### Quick Health Check Script

```bash
#!/bin/bash
# health_check.sh — verify the full stack

echo "=== Ollama ==="
curl -sf http://localhost:11434/api/tags | jq '.models[].name' 2>/dev/null \
  || echo "FAIL: Ollama not responding"

echo -e "\n=== SearXNG (HTML) ==="
curl -sf http://localhost:8080/ > /dev/null \
  && echo "OK" || echo "FAIL: SearXNG HTML not responding"

echo -e "\n=== SearXNG (JSON) ==="
curl -sf -G 'http://localhost:8080/search' \
  --data-urlencode 'q=test' \
  --data-urlencode 'format=json' \
  -H 'User-Agent: Mozilla/5.0 (X11; Linux x86_64)' \
  | jq '.results | length' 2>/dev/null \
  && echo "results returned" \
  || echo "FAIL: JSON API broken (check formats: [html, json])"

echo -e "\n=== Open WebUI ==="
curl -sf http://localhost:3000/ > /dev/null \
  && echo "OK" || echo "FAIL: Open WebUI not responding"

echo -e "\n=== LM Studio ==="
curl -sf http://localhost:1234/v1/models > /dev/null \
  && echo "OK" || echo "INFO: LM Studio not running (optional)"
```

---

## 10 · Architecture Summary

```
┌────────────────────────────────────────────────────────────────────┐
│                        Ubuntu 20.04 Host                           │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                    Docker Network                             │  │
│  │                                                              │  │
│  │  ┌────────────┐   ┌────────────┐   ┌──────────────────────┐ │  │
│  │  │   Ollama    │   │  SearXNG   │   │     Open WebUI       │ │  │
│  │  │  :11434     │   │   :8080    │   │      :3000           │ │  │
│  │  │             │   │            │   │                      │ │  │
│  │  │  qwen2.5:7b │   │  Google    │   │ SEARXNG_QUERY_URL=   │ │  │
│  │  │  llama3.1:8b│   │  DDG       │   │  http://searxng:8080 │ │  │
│  │  │  nomic-emb  │   │  Brave     │   │  /search?q=<query>   │ │  │
│  │  │             │   │  GitHub    │   │                      │ │  │
│  │  └──────┬──────┘   │  arXiv     │   │  RAG + Web Search    │ │  │
│  │         │          │  Wikipedia │   │  Tools, Functions     │ │  │
│  │         │          └──────┬─────┘   │  MCP Servers          │ │  │
│  │         │                 │         └──────────┬───────────┘ │  │
│  │         │          ┌──────┴─────┐              │             │  │
│  │         │          │   Valkey   │              │             │  │
│  │         │          │   :6379    │              │             │  │
│  │         │          └────────────┘              │             │  │
│  └─────────┼──────────────────────────────────────┼─────────────┘  │
│            │                                      │                │
│  ┌─────────┴──────────────────────────────────────┴─────────────┐  │
│  │                    Agent Layer (host)                         │  │
│  │                                                              │  │
│  │  Python 3.11 venv:                                           │  │
│  │    • ollama SDK (native tool calling)                        │  │
│  │    • langchain-ollama + langgraph (StateGraph pipelines)     │  │
│  │    • smolagents (CodeAgent, best for small models)           │  │
│  │    • crewai (role-based crews)                               │  │
│  │    • httpx + trafilatura (direct SearXNG + page extraction)  │  │
│  │                                                              │  │
│  │  MCP Servers (npx, stdio):                                   │  │
│  │    • mcp-searxng → SearXNG JSON API                          │  │
│  │    • mcpo proxy → REST/OpenAPI bridge                        │  │
│  │                                                              │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                    LM Studio (AppImage)                       │  │
│  │                                                              │  │
│  │  ~/.lmstudio/mcp.json → mcp-searxng                          │  │
│  │  OpenAI API :1234 → tool calling via OpenAI SDK              │  │
│  └──────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────┘
```

The design follows the Plan 9 principle: each component is a server with a well-defined interface (HTTP REST, JSON-RPC, OpenAI-compatible). Adding a new capability — a new search engine, a new model, a new agent framework — means adding a new server, not rewriting the stack. SearXNG is the `/net` of this system: it provides a uniform interface to a heterogeneous set of search engines, the same way Plan 9's `/net` provides a uniform interface to heterogeneous network protocols. The agent layer composes these servers through their interfaces, and each piece can evolve independently.
