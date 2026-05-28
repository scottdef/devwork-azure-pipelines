# The Hermes Agent Operations Guide

**Running NousResearch Hermes 3 as a Local Agentic Workhorse**

*"Simplicity is prerequisite for reliability." — Edsger Dijkstra, but Rob Pike would've said it if Dijkstra hadn't gotten there first.*

---

Hermes 3 is the NousResearch fine-tune of Llama 3.1 purpose-built for agentic work: tool calling, structured JSON output, multi-turn function orchestration, and deep system prompt adherence. On Ollama it ships as `hermes3:8b` — an 8-billion-parameter model that sits in the partial-offload tier on constrained hardware and runs fully on-GPU on anything with 8+ GB VRAM. This guide covers pulling it, customizing it, wiring it into agent frameworks, and five concrete use cases that teach skills, harnesses, and orchestration from the ground up.

---

## 1 · Local Usage

### 1.1 Pull and Verify

```bash
# Pull the model
ollama pull hermes3:8b

# Verify it loaded and check its template
ollama show hermes3:8b --modelfile

# Interactive smoke test
ollama run hermes3:8b "What is a 9P file server?"

# Confirm tool-calling capability
curl -s http://localhost:11434/api/chat -d '{
  "model": "hermes3:8b", "stream": false,
  "messages": [{"role":"user","content":"What time is it in Berlin?"}],
  "tools": [{"type":"function","function":{
    "name":"get_time",
    "description":"Get the current time in a city",
    "parameters":{"type":"object","required":["city"],
                  "properties":{"city":{"type":"string"}}}}}]
}' | jq '.message.tool_calls'
```

If that `jq` output shows a `tool_calls` array with `get_time` and `{"city":"Berlin"}`, the model is alive and its function-calling template is working. If you get plain text instead, you pulled a broken quant or Ollama is too old — update with `curl -fsSL https://ollama.com/install.sh | sh`.

### 1.2 Hardware Tiers

Hermes 3 at 8B Q4_K_M weighs ~4.7 GB. Where it fits:

| VRAM | Strategy | Expected tok/s | Notes |
|------|----------|----------------|-------|
| 8+ GB | `num_gpu 99` (all layers on GPU) | 20-40 | Sweet spot. Full speed. |
| 4 GB | `num_gpu 14-18` of 33 layers | 4-8 | Partial offload — interactive but not fast |
| CPU-only | `num_gpu 0` | ~6 | 32 GB RAM minimum. DDR4-2933 ≈ 6 tok/s |

Critical Ollama environment for constrained hardware:

```ini
# /etc/systemd/system/ollama.service.d/override.conf
[Service]
Environment="OLLAMA_NUM_PARALLEL=1"
Environment="OLLAMA_MAX_LOADED_MODELS=2"
Environment="OLLAMA_KEEP_ALIVE=15m"
Environment="OLLAMA_KV_CACHE_TYPE=q8_0"
Environment="OLLAMA_FLASH_ATTENTION=1"
```

The `q8_0` KV cache quantization halves KV memory with negligible quality loss. Flash attention is required for it to activate. Reload with `sudo systemctl daemon-reload && sudo systemctl restart ollama`.

### 1.3 The REST API — Everything You Need

Base URL: `http://localhost:11434`. Two endpoints matter for agents:

**`/api/chat`** — the tool-calling endpoint. Send `messages` + `tools`, get back `tool_calls` or plain text. Loop until the model stops calling tools:

```bash
# Structured output via JSON schema (kills the malformed-JSON failure class)
curl -s http://localhost:11434/api/chat -d '{
  "model": "hermes3:8b",
  "stream": false,
  "messages": [{"role":"user","content":"List 3 Unix commands for file management"}],
  "format": {
    "type": "object",
    "required": ["commands"],
    "properties": {
      "commands": {
        "type": "array",
        "items": {
          "type": "object",
          "required": ["name","description"],
          "properties": {
            "name": {"type":"string"},
            "description": {"type":"string"}
          }
        }
      }
    }
  }
}' | jq '.message.content | fromjson'
```

**`/v1/chat/completions`** — OpenAI-compatible. Every agent framework that speaks OpenAI (which is all of them) can point at `http://localhost:11434/v1` and it works.

### 1.4 The Python SDK

```python
from ollama import chat
from pydantic import BaseModel

class ToolResult(BaseModel):
    action: str
    reasoning: str

resp = chat(
    model="hermes3:8b",
    messages=[{"role": "user", "content": "Should I use bind or rfork for this namespace?"}],
    format=ToolResult.model_json_schema(),
)
result = ToolResult.model_validate_json(resp.message.content)
print(result.action, result.reasoning)
```

The Ollama Python SDK auto-generates JSON schemas from Python callables if you pass functions with Google-style docstrings directly to the `tools=` parameter. This is the lowest-friction way to get tool calling working.

---

## 2 · Customizing the Agent

### 2.1 The Modelfile — Your Agent's Genome

Ollama Modelfiles use a Dockerfile-like syntax: `FROM`, `PARAMETER`, `SYSTEM`, `TEMPLATE`, `MESSAGE`, `ADAPTER`, `LICENSE`. Build with `ollama create`, inspect with `ollama show`.

```dockerfile
# Modelfile.hermes-devops
FROM hermes3:8b

PARAMETER temperature 0.1
PARAMETER num_ctx 8192
PARAMETER num_predict 2048
PARAMETER num_gpu 99
PARAMETER stop "<|im_end|>"

SYSTEM """You are Hermes, a senior infrastructure engineer fluent in Terraform,
Bash, Python, Go, and the Unix tradition from ed(1) through Plan 9. You have
internalized the 9P philosophy: everything is a file, every resource is a
server, composition happens through namespaces.

When given a task:
1. Think step by step about the minimal correct solution
2. Prefer composition of small tools over monolithic scripts
3. Use native tool calling when functions are available
4. Return structured JSON when asked; never wrap JSON in markdown fences
5. Say "I don't know" rather than hallucinate an API

You operate on Ubuntu with Ollama as your inference server, Docker for
containers, and GitHub Actions for CI/CD. You respect the rule of least
privilege and never suggest running containers as root without justification."""
```

```bash
ollama create hermes-devops -f Modelfile.hermes-devops
ollama run hermes-devops "Write a Makefile target that validates YAML with jsonschema"
```

### 2.2 Critical Tuning Parameters

These are not suggestions — they are the difference between a working agent and a hallucination machine:

`num_gpu` is a **layer count**, not a fraction. For Hermes 3 with 33 layers on 4 GB VRAM, set 14-18 to split between GPU and CPU. `99` or `-1` = all on GPU. `0` = CPU only.

`num_thread` should equal your **physical core count**. Never set higher — hyperthreads hurt because LLM inference is memory-bandwidth bound, not compute bound. Check with `lscpu | grep "Core(s) per socket"`.

`num_ctx` directly drives KV-cache memory. Keep ≤ 4096 on 4 GB VRAM unless `OLLAMA_KV_CACHE_TYPE=q8_0` is active. For agent loops with tool results, 8192 is the minimum useful context — tool results eat tokens fast.

`temperature 0.1-0.2` in the agent loop. This is non-negotiable. Higher temperature on small models produces hallucinated tool names, malformed JSON args, and format drift mid-loop. Raise it only inside writer/summary nodes where creativity matters.

### 2.3 System Prompt Engineering for Agents

Hermes 3 was specifically fine-tuned for strong system prompt adherence. Exploit this. The system prompt is the single most powerful customization lever:

```dockerfile
# Role-specialist: GitHub IssueOps operator
SYSTEM """You are an IssueOps automation agent. You process GitHub issue
bodies as structured requests. When you receive an issue:

1. Parse the YAML front matter for parameters
2. Validate inputs against allowed values
3. Call the appropriate tool to execute the operation
4. Return a structured status report

You never execute destructive operations without explicit confirmation.
You always include the issue number and actor in your responses.
Format all output as GitHub-Flavored Markdown suitable for issue comments."""
```

```dockerfile
# Role-specialist: Terraform plan reviewer
SYSTEM """You are a Terraform plan review agent. When given terraform plan
output, you analyze it for:

- Resource deletions (always flag these)
- Security group changes (flag any 0.0.0.0/0 rules)
- IAM policy modifications (flag any admin/wildcard permissions)
- Cost implications (estimate monthly delta when possible)

Respond with a structured review using severity levels: CRITICAL, WARNING, INFO.
Never approve plans that delete stateful resources without explicit confirmation."""
```

### 2.4 MCP — The USB-C for Agent Tools

Hermes 3 doesn't speak MCP natively (Ollama isn't an MCP host), but three patterns bridge the gap:

**Pattern 1: `mcpo` proxy (recommended)** — wraps any MCP server in REST/OpenAPI:
```bash
uvx mcpo --port 8000 --api-key "secret" --config ./mcpo_config.json --hot-reload
```

**Pattern 2: Open WebUI native MCP** — point-and-click in Settings → Tools → MCP Servers.

**Pattern 3: `ollama-mcpo-adapter`** — translates MCPO's OpenAPI shape into Ollama's `tools=[...]` parameter for direct `ollama.chat()` calls.

The standard MCP config shape (works identically in Claude Desktop, Cursor, LM Studio, Cline):

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/you/projects"]
    },
    "git": {
      "command": "uvx",
      "args": ["mcp-server-git", "--repository", "/home/you/repo"]
    },
    "searxng": {
      "command": "npx",
      "args": ["-y", "mcp-searxng"],
      "env": {"SEARXNG_URL": "http://localhost:8888"}
    }
  }
}
```

---

## 3 · Five Use Cases — From Skills to Orchestration

Each example teaches a different layer of the agent stack. They're ordered by complexity, and each one introduces concepts the next one builds on.

---

### Use Case 1: Single-Agent Tool Caller — "The `cat(1)` of Agents"

**Concept taught: Skills (tool definitions + the tool-calling loop)**

This is the atomic unit. One model, one or two tools, a tight loop. Like `cat` — it does one thing and does it well. The skill here is understanding how the universal tool-calling loop works: send messages + tools → if response has `tool_calls`, execute locally → append result as `role: tool` → loop until the model returns plain text.

```python
#!/usr/bin/env python3
"""hermes_weather.py — single-agent tool caller.
Concept: the tool-calling loop is the fundamental agent primitive."""

import json
import httpx
from ollama import chat

def get_weather(city: str) -> str:
    """Get current weather for a city using wttr.in."""
    r = httpx.get(f"https://wttr.in/{city}?format=j1", timeout=10)
    data = r.json()
    current = data["current_condition"][0]
    return json.dumps({
        "city": city,
        "temp_c": current["temp_C"],
        "description": current["weatherDesc"][0]["value"],
        "humidity": current["humidity"],
        "wind_kmph": current["windspeedKmph"],
    })

def get_time(city: str) -> str:
    """Get the current time in a city's timezone."""
    # Simplified — real implementation uses timeapi.io or similar
    return json.dumps({"city": city, "time": "14:32", "timezone": "CET"})

# The universal tool-calling loop
messages = [{"role": "user", "content": "What's the weather in Berlin and what time is it there?"}]

while True:
    response = chat(
        model="hermes3:8b",
        messages=messages,
        tools=[get_weather, get_time],  # SDK auto-generates schemas from docstrings
    )

    if not response.message.tool_calls:
        print(response.message.content)
        break

    # Execute each tool call and feed results back
    messages.append(response.message)
    for tc in response.message.tool_calls:
        func = {"get_weather": get_weather, "get_time": get_time}[tc.function.name]
        result = func(**tc.function.arguments)
        messages.append({"role": "tool", "content": result, "name": tc.function.name})
```

**Key lessons:**

The loop is the agent. Everything else — frameworks, graphs, supervisors — is scaffolding around this loop. Keep tool surfaces small (≤ 5 tools per agent). Hermes 3 at 8B handles 3-5 tools reliably; beyond that, hallucinated tool names creep in.

---

### Use Case 2: Research Pipeline — "The Shell Pipeline of Agents"

**Concept taught: Harnesses (LangGraph StateGraph as the orchestration runtime)**

A harness is the thing that runs the loop and manages state between steps. LangGraph's `StateGraph` is the right harness for production work — it gives you typed state, conditional edges, and checkpointing for free. This is the agent equivalent of a Unix pipeline: `plan | search | filter | synthesize | write`.

```python
#!/usr/bin/env python3
"""hermes_research.py — multi-step research pipeline.
Concept: the StateGraph harness manages typed state through a DAG of agent steps."""

import asyncio, json, httpx, trafilatura
from typing import TypedDict, List
from langchain_ollama import ChatOllama
from langgraph.graph import StateGraph, START, END

# Hermes 3 for heavy reasoning; a smaller model for filtering
hermes  = ChatOllama(model="hermes3:8b", temperature=0.2, num_ctx=8192)
filterer = ChatOllama(model="llama3.2:3b", temperature=0)
SEARXNG  = "http://localhost:8080/search"

class ResearchState(TypedDict):
    topic: str
    subqueries: List[str]
    hits: List[dict]
    docs: List[dict]
    notes: str
    report: str

def plan(state: ResearchState) -> dict:
    """Break topic into 4-6 search queries."""
    raw = hermes.invoke(
        f"Break this topic into 4-6 web-search queries. "
        f"Return ONLY a JSON array of strings, no other text.\n\n"
        f"Topic: {state['topic']}"
    ).content
    queries = json.loads(raw[raw.find('['):raw.rfind(']')+1])
    return {"subqueries": queries}

async def _search(query: str) -> List[dict]:
    async with httpx.AsyncClient(timeout=20) as c:
        r = await c.get(SEARXNG, params={"q": query, "format": "json"},
                        headers={"User-Agent": "Mozilla/5.0 (X11; Linux x86_64)"})
        return [{"url": h["url"], "title": h["title"], "snippet": h.get("content", "")}
                for h in r.json()["results"][:8]]

def search(state: ResearchState) -> dict:
    """Fan-out parallel searches, deduplicate by URL."""
    batches = asyncio.run(asyncio.gather(*[_search(q) for q in state["subqueries"]]))
    seen, unique = set(), []
    for hit in [x for batch in batches for x in batch]:
        if hit["url"] not in seen:
            seen.add(hit["url"])
            unique.append(hit)
    return {"hits": unique}

def read_and_filter(state: ResearchState) -> dict:
    """Fetch pages, extract text, filter for relevance."""
    docs = []
    for hit in state["hits"][:10]:  # cap to avoid context blowup
        try:
            downloaded = trafilatura.fetch_url(hit["url"])
            text = trafilatura.extract(downloaded, include_comments=False) or ""
            if len(text) > 200:  # skip empty/tiny pages
                docs.append({"url": hit["url"], "title": hit["title"],
                             "content": text[:3000]})  # truncate aggressively
        except Exception:
            continue
    return {"docs": docs}

def synthesize(state: ResearchState) -> dict:
    """Compress all docs into structured notes."""
    doc_text = "\n\n---\n\n".join(
        f"## {d['title']}\n{d['content']}" for d in state["docs"]
    )
    notes = hermes.invoke(
        f"Synthesize these sources into structured research notes about "
        f"'{state['topic']}'. Be specific, cite sources by title.\n\n{doc_text}"
    ).content
    return {"notes": notes}

def write(state: ResearchState) -> dict:
    """Produce the final report from notes."""
    report = hermes.invoke(
        f"Write a polished Markdown research report on '{state['topic']}' "
        f"using these notes. Include a summary, key findings, and sources.\n\n"
        f"{state['notes']}"
    ).content
    return {"report": report}

# Wire the graph — this is the harness
graph = StateGraph(ResearchState)
graph.add_node("plan", plan)
graph.add_node("search", search)
graph.add_node("read", read_and_filter)
graph.add_node("synthesize", synthesize)
graph.add_node("write", write)
graph.add_edge(START, "plan")
graph.add_edge("plan", "search")
graph.add_edge("search", "read")
graph.add_edge("read", "synthesize")
graph.add_edge("synthesize", "write")
graph.add_edge("write", END)

app = graph.compile()

# Run it
result = app.invoke({"topic": "Plan 9 influence on Linux containers and Go"})
print(result["report"])
```

**Key lessons:**

The harness (`StateGraph`) owns the control flow. Each node is a function that reads typed state and returns a partial update. This is the Plan 9 philosophy applied to agents: each node is a server that reads and writes a well-defined namespace (the state dict). Truncate tool results aggressively — most agent blowups come from a search tool returning 50 KB of HTML into a 8K context window.

---

### Use Case 3: Code-as-Action Agent — "The `rc(1)` of Agents"

**Concept taught: Skills via code generation (smolagents CodeAgent)**

The most reliable agent pattern on small local models is not the textual ReAct loop but **code-as-action agents**. Instead of emitting JSON tool-call blobs (which 8B models frequently malform), the model emits Python code that calls tools directly. This is more in-distribution for trained models and avoids JSON parsing entirely. smolagents is ~1 kLOC and purpose-built for this.

```python
#!/usr/bin/env python3
"""hermes_code_agent.py — code-as-action agent.
Concept: emit Python instead of JSON tool calls. More reliable on small models."""

from smolagents import CodeAgent, Tool, LiteLLMModel

# Point at Hermes 3 via Ollama's OpenAI-compat endpoint
model = LiteLLMModel("ollama_chat/hermes3:8b", api_base="http://localhost:11434")

class ShellTool(Tool):
    name = "run_shell"
    description = "Execute a shell command and return stdout. Use for file operations, git, terraform, make."
    inputs = {"command": {"type": "string", "description": "The shell command to run"}}
    output_type = "string"

    def forward(self, command: str) -> str:
        import subprocess
        # SECURITY: allowlist commands in production. Never shell=True with untrusted input.
        allowed_prefixes = ["ls", "cat", "grep", "find", "wc", "head", "tail",
                            "git log", "git status", "git diff", "terraform plan",
                            "make", "yamllint", "python -c"]
        if not any(command.strip().startswith(p) for p in allowed_prefixes):
            return f"BLOCKED: command '{command.split()[0]}' not in allowlist"
        result = subprocess.run(command, shell=True, capture_output=True, text=True, timeout=30)
        return result.stdout[:4000] or result.stderr[:2000]  # truncate

class FileReadTool(Tool):
    name = "read_file"
    description = "Read a file and return its contents."
    inputs = {"path": {"type": "string", "description": "Path to the file"}}
    output_type = "string"

    def forward(self, path: str) -> str:
        with open(path) as f:
            content = f.read()
        return content[:8000]  # truncate for context budget

agent = CodeAgent(
    tools=[ShellTool(), FileReadTool()],
    model=model,
    max_steps=8,  # step budget prevents runaway loops
)

# The agent writes Python code that calls these tools
result = agent.run(
    "Check the git status of /home/user/infra-repo, then read the Makefile "
    "and tell me what targets are available."
)
print(result)
```

**Key lessons:**

smolagents `CodeAgent` emits Python rather than JSON tool-call blobs — this collapses the malformed-JSON failure class that ruins 3B-8B tool calling. Always set `max_steps` (6-12 range) to prevent runaway loops. Always command-allowlist shell tools. Treat any shell-execution tool as untrusted code execution: run inside Docker, `firejail`, or `bubblewrap`. Never give a writable filesystem + an HTTP-fetch tool to the same agent.

---

### Use Case 4: Supervisor-Worker Orchestration — "The `rfork` of Agents"

**Concept taught: Orchestration (supervisor pattern with handoffs)**

In Plan 9, `rfork` creates a new process with a selectively shared namespace. The supervisor pattern does the same thing for agents: a supervisor agent decides which worker gets the next task and hands off control. Each worker has its own tools and system prompt (its own "namespace"). The supervisor sees all their outputs but the workers don't see each other.

```python
#!/usr/bin/env python3
"""hermes_supervisor.py — supervisor/worker orchestration.
Concept: a supervisor routes tasks to specialist workers via handoff tools."""

from langchain_ollama import ChatOllama
from langgraph.graph import StateGraph, MessagesState, START, END
from langgraph.prebuilt import create_react_agent
from langchain_core.tools import tool

# --- Worker agents (each with its own tool namespace) ---

@tool
def run_terraform_plan(working_dir: str) -> str:
    """Run terraform plan in the specified directory and return the output."""
    import subprocess
    result = subprocess.run(
        ["terraform", "plan", "-no-color"],
        cwd=working_dir, capture_output=True, text=True, timeout=120
    )
    return (result.stdout + result.stderr)[:6000]

@tool
def validate_yaml(file_path: str) -> str:
    """Validate a YAML file for syntax errors."""
    import yaml
    try:
        with open(file_path) as f:
            yaml.safe_load(f)
        return f"VALID: {file_path}"
    except yaml.YAMLError as e:
        return f"INVALID: {file_path}: {e}"

@tool
def check_git_status(repo_path: str) -> str:
    """Check git status and recent commits in a repository."""
    import subprocess
    status = subprocess.run(["git", "status", "--short"], cwd=repo_path,
                            capture_output=True, text=True).stdout
    log = subprocess.run(["git", "log", "--oneline", "-5"], cwd=repo_path,
                         capture_output=True, text=True).stdout
    return f"STATUS:\n{status}\nRECENT COMMITS:\n{log}"

# Create specialist workers
infra_worker = create_react_agent(
    ChatOllama(model="hermes3:8b", temperature=0.1, num_ctx=8192),
    tools=[run_terraform_plan, validate_yaml],
    prompt="You are an infrastructure specialist. Run terraform plans and validate configs."
)

git_worker = create_react_agent(
    ChatOllama(model="hermes3:8b", temperature=0.1, num_ctx=8192),
    tools=[check_git_status],
    prompt="You are a git operations specialist. Check repository states and report changes."
)

# --- Supervisor ---

def supervisor(state: MessagesState):
    """Route to the appropriate worker based on the task."""
    supervisor_llm = ChatOllama(model="hermes3:8b", temperature=0, num_ctx=4096)
    last_msg = state["messages"][-1].content

    routing = supervisor_llm.invoke(
        f"Given this request, respond with ONLY one word — either 'infra' or 'git' or 'done'.\n"
        f"- 'infra' for terraform, YAML validation, infrastructure tasks\n"
        f"- 'git' for repository status, commits, branches\n"
        f"- 'done' if the task is complete\n\n"
        f"Request: {last_msg}"
    ).content.strip().lower()

    if "infra" in routing:
        return {"next": "infra_worker"}
    elif "git" in routing:
        return {"next": "git_worker"}
    return {"next": "done"}

# Wire the orchestration graph
builder = StateGraph(MessagesState)
builder.add_node("supervisor", supervisor)
builder.add_node("infra_worker", infra_worker)
builder.add_node("git_worker", git_worker)

builder.add_edge(START, "supervisor")
builder.add_conditional_edges("supervisor", lambda s: s.get("next", "done"), {
    "infra_worker": "infra_worker",
    "git_worker": "git_worker",
    "done": END,
})
builder.add_edge("infra_worker", "supervisor")  # report back
builder.add_edge("git_worker", "supervisor")    # report back

app = builder.compile()
```

**Key lessons:**

The `langgraph-supervisor` package provides `transfer_to_<agent_name>` handoff tools wired automatically. The supervisor LLM picks the next worker each step. This gives more accurate routing than CrewAI hierarchical, with cleaner observability via `app.stream(stream_mode="updates")`. Keep the worker count low — planner + 2-3 specialists is the sweet spot for 8B models. Each worker return to the supervisor creates a decision point, not a free-for-all.

---

### Use Case 5: Mixture-of-Agents Council — "The `/proc` of Agents"

**Concept taught: Multi-model orchestration (MoA voting council)**

In Plan 9, `/proc` exposes every process as files — you can read any process's state from any other. A Mixture-of-Agents council does the same thing: multiple models each propose an answer, then an aggregator model reads all proposals and synthesizes a refined final answer. The empirical foundation is strong — diverse small models, each shown its peers' outputs, can collectively reach GPT-4-level quality on benchmarks.

```python
#!/usr/bin/env python3
"""hermes_council.py — mixture-of-agents voting council.
Concept: multiple proposers + one aggregator. Diversity beats size."""

import asyncio
import json
import httpx
from collections import Counter

OLLAMA = "http://localhost:11434/api/chat"

# Proposers: diverse models for diverse perspectives
PROPOSERS = ["hermes3:8b", "qwen2.5:7b", "llama3.1:8b"]
AGGREGATOR = "hermes3:8b"  # Hermes aggregates — strong instruction following

async def propose(client: httpx.AsyncClient, model: str, prompt: str) -> str:
    """Get a single proposal from a model."""
    resp = await client.post(OLLAMA, json={
        "model": model, "stream": False,
        "messages": [{"role": "user", "content": prompt}],
        "options": {"temperature": 0.7, "num_ctx": 8192},
    }, timeout=120)
    return resp.json()["message"]["content"]

async def aggregate(client: httpx.AsyncClient, prompt: str, proposals: list[str]) -> str:
    """Aggregator refines from all proposals."""
    proposals_text = "\n\n---\n\n".join(
        f"### Proposal {i+1}\n{p}" for i, p in enumerate(proposals)
    )
    resp = await client.post(OLLAMA, json={
        "model": AGGREGATOR, "stream": False,
        "messages": [{"role": "user", "content":
            f"You are a senior reviewer. Below are {len(proposals)} independent responses "
            f"to the question: '{prompt}'\n\n"
            f"Critically evaluate each response. Recognize that some may be biased or "
            f"incorrect. Do not merely replicate — synthesize the best insights into a "
            f"single authoritative answer. If responses conflict, explain the disagreement "
            f"and state which position the evidence supports.\n\n{proposals_text}"
        }],
        "options": {"temperature": 0.2, "num_ctx": 16384},
    }, timeout=120)
    return resp.json()["message"]["content"]

async def council(prompt: str) -> str:
    """Run the full MoA pipeline: parallel propose → aggregate."""
    async with httpx.AsyncClient() as client:
        # Fan-out: all proposers run concurrently
        proposals = await asyncio.gather(
            *[propose(client, model, prompt) for model in PROPOSERS]
        )
        # Aggregate
        return await aggregate(client, prompt, proposals)

# --- For structured/closed-form questions, use majority voting instead ---
async def majority_vote(prompt: str, k: int = 5) -> str:
    """Self-consistency: k samples from one model, majority-vote the answer."""
    async with httpx.AsyncClient() as client:
        proposals = await asyncio.gather(
            *[propose(client, AGGREGATOR, prompt + "\nEnd with: ANSWER: <your answer>")
              for _ in range(k)]
        )
        # Extract final-line answers and vote
        answers = []
        for p in proposals:
            for line in reversed(p.strip().split("\n")):
                if "ANSWER:" in line:
                    answers.append(line.split("ANSWER:")[-1].strip())
                    break
        if answers:
            winner = Counter(answers).most_common(1)[0][0]
            return f"Consensus answer ({len(answers)}/{k} votes): {winner}"
        return proposals[0]  # fallback

if __name__ == "__main__":
    # Open-ended: use MoA council
    result = asyncio.run(council(
        "Compare the namespace model in Plan 9 with Linux mount namespaces. "
        "Which is more composable and why?"
    ))
    print(result)
```

**Key lessons:**

Pin the aggregator on GPU; run proposers on CPU. Total RAM ≈ 12-14 GB, VRAM ≈ 4 GB for the aggregator. End-to-end latency is the slowest proposer plus the aggregator — typically 30-60 seconds for ~500-token answers. Use MoA (multiple diverse models) for open-ended questions. Use self-consistency (k samples from one model, majority vote) for closed-form questions — it's cheaper and equally effective. For deduplication before voting, embed each draft with `nomic-embed-text` and drop near-duplicates above cosine similarity of 0.92.

Tune Ollama for concurrent model loading:

```ini
Environment="OLLAMA_MAX_LOADED_MODELS=5"
Environment="OLLAMA_NUM_PARALLEL=2"
Environment="OLLAMA_KEEP_ALIVE=30m"
```

---

## 4 · Common Architectures for Agent Orchestration

These are the four patterns that actually work in production. Everything else is a combination of these.

### 4.1 Single-Agent ReAct Loop

```
┌─────────────────────────────────────┐
│  User Prompt                        │
│       │                             │
│       ▼                             │
│  ┌─────────┐    ┌──────────────┐    │
│  │ Hermes 3 │───▶│ Tool Calls?  │    │
│  └─────────┘    └──────┬───────┘    │
│       ▲                │            │
│       │           Yes  │  No        │
│       │                │   │        │
│  ┌────┴─────┐          │   ▼        │
│  │ Tool     │◀─────────┘  Done      │
│  │ Executor │                       │
│  └──────────┘                       │
└─────────────────────────────────────┘
```

The atom. One model, N tools, a while loop. Use when the task is well-scoped and ≤ 5 tools cover it. Hermes 3 at 8B handles this cleanly. Constrain with `max_iterations=6-12` and `temperature=0.1`. Use `format=<schema>` for structured output to kill malformed JSON.

### 4.2 Pipeline / DAG (StateGraph)

```
┌──────┐   ┌────────┐   ┌──────┐   ┌───────────┐   ┌───────┐
│ Plan │──▶│ Search │──▶│ Read │──▶│ Synthesize│──▶│ Write │
└──────┘   └────────┘   └──────┘   └───────────┘   └───────┘
    │                                                    │
    └── State flows forward; each node updates it ───────┘
```

The shell pipeline for agents. State is a typed dict; each node reads it and returns a partial update. LangGraph `StateGraph` is the runtime. Use when the task decomposes into sequential stages with well-defined inputs and outputs. Add conditional edges for branching (e.g., "if plan has > 10 subqueries, summarize first"). LangGraph checkpointers (`SqliteSaver`, `PostgresSaver`) persist state across runs and enable time-travel debugging via `.get_state_history()`.

### 4.3 Supervisor / Worker

```
                ┌────────────┐
                │ Supervisor │
                │ (router)   │
                └─────┬──────┘
           ┌──────────┼──────────┐
           ▼          ▼          ▼
      ┌────────┐ ┌────────┐ ┌────────┐
      │Infra   │ │  Git   │ │ Review │
      │Worker  │ │ Worker │ │ Worker │
      └────┬───┘ └────┬───┘ └────┬───┘
           │          │          │
           └──────────┼──────────┘
                      ▼
               Back to Supervisor
```

The `rfork` pattern. Supervisor decides which worker gets the next step via a routing prompt or `transfer_to_<worker>` handoff tools. Three framework implementations:

**LangGraph supervisor** (`langgraph-supervisor` package) — handoff tools wired automatically. Supervisor LLM picks the next worker each step. Cleanest observability.

**CrewAI hierarchical** — `Process.hierarchical` with `manager_llm=hermes3:8b`. Mitigate the manager-does-all-work bug by providing an explicit `manager_agent=Agent(...)` with a step-by-step system prompt.

**AutoGen 0.4 / Magentic-One** — orchestrator with a Task Ledger + Progress Ledger (shared blackboard). Expects a strong reasoning model; with Hermes 3 at 8B, keep tasks short and worker count low.

### 4.4 Mixture-of-Agents (MoA) Council

```
     ┌───────────┐  ┌───────────┐  ┌───────────┐
     │ Hermes 3  │  │ Qwen 2.5  │  │ Llama 3.1 │
     │ (proposer)│  │ (proposer)│  │ (proposer)│
     └─────┬─────┘  └─────┬─────┘  └─────┬─────┘
           │              │              │
           └──────────────┼──────────────┘
                          ▼
                   ┌─────────────┐
                   │ Aggregator  │
                   │ (Hermes 3)  │
                   └──────┬──────┘
                          ▼
                    Final Answer
```

Diversity beats size. Three proposers run concurrently (CPU is fine — they're embarrassingly parallel). The aggregator (pinned to GPU) reads all proposals and synthesizes a refined answer. The aggregator prompt must include "critically evaluate — do not merely replicate" to prevent parroting.

For LAN-distributed setups, front everything with LiteLLM as a gateway:

```
MASTER (Hermes 3 on Ollama)
├── LiteLLM proxy :4000 → fan-out
├── Ollama :11434 (hermes3, qwen2.5, nomic-embed-text)
├── Redis :6379 (cache + rate-limit state)
└── Postgres :5432 (virtual keys, budgets)

Tailscale mesh (WireGuard, MagicDNS)
├── Slave A (RTX 4090)  vLLM :8000  qwen2.5-32b-awq
├── Slave B (any GPU)   Stable Diffusion :7860
└── Slave C (CPU)       whisper.cpp :9000, Piper TTS :10200
```

LiteLLM routing strategies: `simple-shuffle`, `least-busy`, `usage-based-routing-v2`, `latency-based-routing`. Reliability primitives: `num_retries`, `allowed_fails`/`cooldown_time` (circuit breaker), `fallbacks` (cross-group fallback chains), `context_window_fallbacks` (auto-escalate long prompts to the bigger model).

---

## 5 · Memory and State

An agent without memory is a function. An agent with memory is a system.

**Short-term (within a run):** Hermes 3 with `num_ctx=8192` gives you 8K tokens of working memory. That sounds like a lot until a tool result dumps 3K tokens of HTML into it. Mitigations: truncate tool results to 3-4K tokens, use sliding windows of last N turns, and periodically summarize the conversation so far into a compressed state.

**Cross-session (between runs):** LangGraph checkpointers (`SqliteSaver`, `PostgresSaver`) persist agent state across runs. `mem0` bolts cross-session, self-editing memory onto any framework. **Letta** (formerly MemGPT) is the right pick if memory *is* the product — it manages an OS-style core/recall/archival tier inside its runtime, and the agent uses tool calls (`core_memory_replace`, `archival_memory_insert`) to manage its own state.

**Vector stores for long-term recall:** Chroma or FAISS for prototypes, Qdrant in Docker for production. Embed with `nomic-embed-text` (270 MB, fits alongside any chat model on the GPU).

---

## 6 · The Plan 9 Lesson

Plan 9 taught us that the right abstraction is the one that composes. Files compose because they have a universal interface — open, read, write, close. 9P makes every resource a file server, so every resource composes with every other resource through the same protocol.

Agents are the same. The right agent abstraction is one that composes:

- **Tools** are the files — they have a universal interface (name, description, parameters, return value).
- **MCP** is the 9P — it makes every tool source a server with a standard protocol (JSON-RPC 2.0 over stdio or HTTP).
- **Per-process namespaces** are per-agent tool scoping — each worker sees only its own tools, the supervisor sees the workers.
- **`rfork`** is the handoff — the supervisor creates a new execution context with selectively shared state.

The system that wins is the one where adding a new capability means writing a new server, not rewriting the orchestrator. That's what Plan 9 got right. That's what MCP gets right. That's what your agent architecture should get right.

```
"Those who do not understand Plan 9 are condemned to reinvent it, poorly."
                                                    — the systems tradition
```
