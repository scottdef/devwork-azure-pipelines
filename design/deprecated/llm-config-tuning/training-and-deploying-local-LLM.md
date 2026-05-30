# A complete guide to training and deploying local LLMs in 2026

**The fastest path from raw documents to a deployed specialist model now runs through three tools: Unsloth for training, GGUF for export, and Ollama for serving.** Under that stack, a domain-specialist 8B model can be fine-tuned on a single 24 GB GPU in under an hour, exported to four quantization formats in one call, and deployed locally with a 20-line Modelfile. But fine-tuning is not the right answer for most problems. The dominant 2026 pattern is hybrid: RAG handles facts that change, fine-tuning handles style and behavior that doesn't. This guide covers the full decision surface — from zero-training retrieval pipelines through QLoRA on consumer hardware to continued pretraining on private code — with concrete code, current model recommendations, and verified VRAM numbers.

---

## 1. When to use RAG, fine-tuning, or both

The first decision is almost never "should I fine-tune?" — it's "what kind of failure am I seeing?" The 2026 consensus, repeated across Anthropic, BigData Boutique, and the Unsloth/LangChain documentation, condenses to one rule: **RAG changes what the model can see; fine-tuning changes how the model behaves**. If your problem is missing knowledge, retrieval is the cure. If your problem is wrong tone, wrong format, or wrong reasoning pattern, training is the cure.

The canonical hierarchy progresses from cheapest to most expensive: **prompt engineering → RAG → fine-tuning (LoRA/QLoRA) → continued pretraining → full training**. Each step should only be taken when the prior step demonstrably fails an evaluation set. BigData Boutique's May 2026 framework formalizes this with a 2×2 matrix:

| | Stable signal | Volatile signal |
|---|---|---|
| **Knowledge-bound failure** | Continued pretraining (rare) | **RAG** |
| **Behavior-bound failure** | **Fine-tune (LoRA/QLoRA)** | Prompt engineering + few-shot |

The trade-off table that matters in practice:

| Dimension | RAG | Fine-Tuning (LoRA) | CPT | Prompt Engineering |
|---|---|---|---|---|
| Best for | Changing facts, private docs, citations | Stable behavior, style, schema | Domain vocabulary | Quick iteration |
| Knowledge freshness | Excellent (re-index) | Poor | Very poor | Excellent |
| Citation/grounding | High | Low | Low | Medium |
| Time to value | Days | Weeks | Months | Hours |
| GPU for adaptation | None (just embedding) | 1×24GB for 7-13B, 1×80GB for 70B | Multi-GPU | None |
| Per-request cost | Higher (extra tokens) | Lower (smaller prompt) | Same as base | Lowest |
| Hallucination control | Strong (grounded) | Weak | Weak | Weak |
| Primary failure mode | Bad retrieval → bad answer | Catastrophic forgetting | Severe forgetting | Brittleness |

**The hybrid pattern is now the default for any serious deployment**: fine-tune the *interface* (query rewriting, citation format, refusal behavior, output schema, domain vocabulary), retrieve the *content* (anything that changes, anything customer-specific, anything that must be cited). Quantitatively, Anthropic's contextual retrieval work showed hybrid BM25+dense retrieval cuts retrieval failures by 49%, and adding a reranker brings it to 67%. The Ovadia et al. paper (arXiv 2312.05934) demonstrated RAG consistently beats fine-tuning for factual recall — confirming that **fine-tuned models cannot reliably memorize new facts**, only patterns of behavior.

A five-item checklist gates whether you should fine-tune at all: (1) an evaluation set exists; (2) the prompt+RAG baseline measurably fails it; (3) the failure is behavioral, not knowledge-shaped; (4) you have at least a few hundred high-quality examples in production format; (5) someone owns the adapter's 12-month lifecycle with quarterly revalidation. Real lifetime cost is roughly **3-5× the initial training compute** when you account for revalidation as base models drift.

---

## 2. The local RAG pipeline, end to end

A production-quality local RAG stack in 2026 has six layers: loaders, splitters, embeddings, vector store, hybrid retrieval, and a reranker. Each has a clear default and a clear upgrade path.

### Loading and chunking

**LlamaIndex `SimpleDirectoryReader`** auto-detects PDF, DOCX, MD, PPTX, EPUB, images, audio, and notebooks — the fastest path to ingest a mixed-format folder:

```python
from llama_index.core import SimpleDirectoryReader
docs = SimpleDirectoryReader(input_dir="./data", recursive=True,
    required_exts=[".pdf", ".md", ".docx"], filename_as_id=True).load_data()
```

**LangChain** is more granular: `PyPDFLoader` for native PDFs, `UnstructuredPDFLoader` (with Poppler+Tesseract) for scanned or layout-heavy PDFs, `UnstructuredMarkdownLoader` to preserve heading structure, and `DirectoryLoader` to glob. For truly hard PDFs, **marker-pdf** (GPU-accelerated, ~11s/page, highest fidelity), **PyMuPDF4LLM** (fast native→markdown, ~0.12s/page), and **Surya OCR** (~2× Tesseract accuracy on scans) are the 2026 standard tools.

Chunking strategy follows from document type. **`RecursiveCharacterTextSplitter`** with `chunk_size=800, chunk_overlap=100` is the universal default; it splits hierarchically on `["\n\n", "\n", " ", ""]`. For structured Markdown, `MarkdownHeaderTextSplitter` preserves heading metadata. For code, `RecursiveCharacterTextSplitter.from_language(Language.GO, ...)` uses language-aware separators. Recommended sizes by content type:

| Content | chunk_size | overlap |
|---|---|---|
| Technical manuals / long-form | 512-1000 tokens | 15-20% |
| FAQ / short Q&A | 200-400 tokens | 10% |
| Code | 500-1500 chars, language-aware | 0-10% |
| Tables / dense data | 200-400 chars | 20% |

Semantic chunking (using embedding similarity to detect topic boundaries) can lift retrieval accuracy by up to 70% versus naive splitting, but at much higher compute cost — reserve it for high-stakes corpora.

### Embeddings and vector stores

The local embedding landscape in May 2026 has clear winners by tier:

| Model | Dims | Context | When to use |
|---|---|---|---|
| **nomic-embed-text** (v1.5) | 768 | 8192 | Default for laptop/CPU; `ollama pull nomic-embed-text` (274 MB) |
| **BAAI/bge-m3** | 1024 | 8192 | **Best hybrid** — dense + sparse + ColBERT in one model |
| **Qwen3-Embedding-0.6B / 8B** | up to 4096 (Matryoshka) | 32K | SOTA multilingual (70.58 MTEB); 8B needs A100/4090 |
| **NV-Embed-v2** | 4096 | 32K | Top open English (72.31 MTEB), 16 GB VRAM |
| **mxbai-embed-large** | 1024 | 512 | Strong on conceptual queries |

The cardinal rule: **use the same embedding model for indexing and querying**, and pin the model version in collection metadata. Switching requires a full reindex.

Vector store choice depends on scale:

| Store | Best for | Notes |
|---|---|---|
| **Chroma** | Development / prototyping | Easiest API, persists to disk |
| **Qdrant** | Production | Rust, fast HNSW, rich filtering, hybrid built-in |
| **FAISS** | Maximum speed, static data | Billions of vectors, no metadata server |
| **PGVector** | Already running Postgres | SQL filtering, transactional |
| **pgvectorscale** | Postgres at scale | 471 QPS vs Qdrant's 41 QPS on 50M vectors at 99% recall |

A complete local RAG pipeline with Ollama as both the LLM and embedding backend looks like this:

```python
from langchain_community.document_loaders import DirectoryLoader, PyPDFLoader
from langchain_text_splitters import RecursiveCharacterTextSplitter
from langchain_ollama import OllamaEmbeddings, ChatOllama
from langchain_chroma import Chroma
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.runnables import RunnablePassthrough
from langchain_core.output_parsers import StrOutputParser

docs   = DirectoryLoader("./docs", glob="**/*.pdf", loader_cls=PyPDFLoader).load()
chunks = RecursiveCharacterTextSplitter(chunk_size=800, chunk_overlap=100).split_documents(docs)
emb    = OllamaEmbeddings(model="nomic-embed-text")
vs     = Chroma.from_documents(chunks, embedding=emb, persist_directory="./chroma_db")
llm    = ChatOllama(model="llama3.1:8b", temperature=0, num_ctx=8192)

prompt = ChatPromptTemplate.from_template(
    "Answer ONLY from this context. If insufficient, say so.\n\nContext:\n{context}\n\nQuestion: {question}")
fmt    = lambda ds: "\n\n".join(f"[{d.metadata.get('source','')}] {d.page_content}" for d in ds)
chain  = ({"context": vs.as_retriever(search_kwargs={"k":5}) | fmt,
           "question": RunnablePassthrough()} | prompt | llm | StrOutputParser())
print(chain.invoke("What does the policy say about refunds?"))
```

### Hybrid search and reranking

Dense retrieval alone misses exact IDs, error codes, SKUs, and code clauses — anywhere lexical match matters. The fix is **`EnsembleRetriever`** combining BM25 with dense via reciprocal rank fusion, typically weighted 0.4 BM25 / 0.6 dense:

```python
from langchain_community.retrievers import BM25Retriever
from langchain.retrievers import EnsembleRetriever
bm25 = BM25Retriever.from_documents(chunks); bm25.k = 20
hybrid = EnsembleRetriever(retrievers=[bm25, vs.as_retriever(search_kwargs={"k":20})],
                           weights=[0.4, 0.6])
```

Adding a **cross-encoder reranker** is the single largest quality lever after embedding choice. The pattern is retrieve broadly (k=20-50), then rerank to top 3-10:

```python
from langchain.retrievers import ContextualCompressionRetriever
from langchain.retrievers.document_compressors import CrossEncoderReranker
from langchain_community.cross_encoders import HuggingFaceCrossEncoder
ce       = HuggingFaceCrossEncoder(model_name="BAAI/bge-reranker-v2-m3")
reranker = ContextualCompressionRetriever(
    base_compressor=CrossEncoderReranker(model=ce, top_n=5),
    base_retriever=hybrid)
```

**BAAI/bge-reranker-v2-m3** is the default open reranker — multilingual, MIT-licensed, ~10ms per pair on GPU. Cohere Rerank 3 is the managed alternative.

### Summarization and agentic patterns

LangChain's `load_summarize_chain` exposes three strategies. **Stuff** dumps all chunks into one prompt (cheapest, highest fidelity, only works if total fits in context). **Map-reduce** summarizes chunks in parallel then combines (N+1 calls, loses cross-chunk context). **Refine** walks chunks sequentially updating a running summary (slowest, best narrative coherence).

For a document agent that can search, summarize, and compare across files, **LangGraph** is the canonical 2026 pattern. The architecture: an LLM bound to tools (`search_docs`, `summarize_topic`, `compare_docs`), a `ToolNode`, and a conditional edge that loops until the LLM emits no more tool calls. Advanced variants add a document-grader node (LLM scores chunk relevance and loops back to rewrite the query if low) and a hallucination-grader node (verifies the answer is grounded in retrieved content).

### Open WebUI for no-code RAG

Open WebUI plus Ollama, deployed via Docker Compose, gives non-technical users a complete RAG interface. Critical settings in Admin Panel → Settings → Documents: embedding engine = Ollama, model = `nomic-embed-text`, chunk 1000/overlap 100, **enable Hybrid Search**, set reranking model to `BAAI/bge-reranker-v2-m3`, top-K 5-10. The single most common failure: **Ollama's default `num_ctx=2048` truncates retrieved chunks** — raise it to 8192 minimum per model, or RAG content silently disappears. Setting `RAG_SYSTEM_CONTEXT=True` keeps retrieved context in the system message, preserving KV-cache prefix between turns.

Knowledge collections are created in Workspace → Knowledge, files uploaded, then attached in chat via `#collection-name` or bound permanently to a custom model under Workspace → Models.

---

## 3. Building a domain specialist from technical manuals

A "metalworker specialist" trained on welding manuals, AWS D1.1 code, metallurgy references, and MSDS sheets demonstrates the canonical specialist creation pipeline — and reveals why the **RAG + fine-tune hybrid is mandatory for safety-critical domains**.

### Cleaning PDFs and scans

Technical manuals are typically a mix of native-text PDFs, scanned reference books, and image-heavy code documents. A 2026 benchmarked routing pipeline:

| Document type | Tool | Reasoning |
|---|---|---|
| Native PDF (most manuals) | **PyMuPDF4LLM** | Fast (~0.12s/page), markdown output, preserves headers/tables |
| Layout-heavy / mixed | **marker-pdf** | Highest fidelity, GPU-accelerated |
| Scanned books | **Surya OCR** | Line-level bboxes, ~0.97 accuracy vs Tesseract's 0.88 |
| Tables (AWS D1.1 WPS tables) | **pdfplumber** + Docling | Best for bordered tables |
| Downstream RAG chunks | **Unstructured.io** | Native semantic chunking (Title, NarrativeText) |

A `is_scanned()` heuristic — characters per page below ~100 — routes documents between PyMuPDF4LLM and Surya OCR automatically.

Always run post-extraction cleanup: collapse hyphenation at line ends, normalize Unicode (NFKC), strip repeated headers and footers across pages. **One wrong digit in a preheat table propagates to thousands of synthetic training pairs** — sample-check OCR output before generating data.

### The RAG-only path: system prompt as expertise

For most specialist use cases, RAG with a strong domain system prompt is sufficient and far cheaper than fine-tuning. The system prompt does the heavy lifting:

```
You are a Certified Welding Inspector (CWI) and metallurgist with 25 years of
structural-steel experience. You strictly follow AWS D1.1, ASME Section IX, ISO 3834.

When answering:
1. Cite the specific clause ("AWS D1.1:2020 §4.7.3") for any code-derived value.
2. Distinguish WPS, PQR, WPQ correctly.
3. Quote MSDS hazard data verbatim — never paraphrase H- or P-statements.
4. If retrieved context is insufficient, respond: "Not in my reference library."
   Never speculate on preheat temps, electrode classifications, or allowable stresses.

RETRIEVED_CONTEXT: {context}
USER QUESTION: {question}
```

For a welding corpus, **bge-m3** is the embedding model of choice — its multi-vector hybrid retrieval handles citations like "D1.1 §6.4.2" that pure dense retrieval misses. Chunk MSDS sheets by structured section header (1. Identification, 2. Hazards, ...), and AWS D1.1 by clause/subclause rather than token count.

### The fine-tuning path: synthetic Q&A from raw manuals

When you need the model to *speak welding* — using PQR, HAZ, GMAW-S, root-pass terminology naturally without retrieval — fine-tuning is warranted. The bottleneck is generating instruction-response training data from raw manuals. Three production-grade tools:

**Augmentoolkit** drops `.txt`/`.md` files into a folder and generates multi-turn Q&A pairs from each chunk, with prompt overrides to bias question style toward factual, multi-step, or diagnostic questions. **Bonito** (a Mistral-7B-based conditional task generator, arXiv 2402.18334) converts unannotated text into 16 task types — extractive QA, multiple-choice, paraphrase, NLI — without calling external APIs. **Evol-Instruct** (the WizardLM/WizardCoder pattern) iteratively evolves seed instructions to increase reasoning depth and breadth; WizardCoder generated 80K pairs from 20K seeds via ~120K API calls.

For custom pipelines, a local 70B teacher model (Qwen2.5-72B or Llama-3.3-70B served via vLLM) generates pairs with strict JSON output and per-passage grounding instructions. The critical step is **quality filtering**: drop instructions under 10 tokens, dedup by embedding cosine ≥0.95 with bge-m3, score each pair 0-5 with an LLM-as-judge in the AlpaGasus pattern and keep ≥3.5, and reverse-validate a sample by asking the teacher "is this answer in this passage?"

**Mix at least 10-20% general instruction data** (OpenHermes, ShareGPT, FineTome) into the training set. Pure domain data causes catastrophic forgetting of chat ability — the model can quote AWS clauses but loses the ability to have a normal conversation.

### Dataset formats: Alpaca, ShareGPT, ChatML

The three formats serve different purposes:

**Alpaca** for single-turn instruction following:
```json
{
  "instruction": "What is the minimum preheat for ASTM A572 Gr 50 at 1.5 inch thickness per AWS D1.1?",
  "input": "",
  "output": "Per AWS D1.1:2020 Table 5.8, A572 Gr 50 at thickness 1-1/2 in. to 2-1/2 in. requires 150°F (65°C) minimum preheat with low-hydrogen SMAW (E7018)..."
}
```

**ShareGPT** for multi-turn dialogue and tool-using agents:
```json
{"conversations": [
  {"from": "system", "value": "You are a welding inspection assistant grounded in AWS D1.1."},
  {"from": "human",  "value": "I'm seeing slag inclusions in 1G groove welds with 7018. Causes?"},
  {"from": "gpt",    "value": "Common causes ranked by frequency: 1. Inadequate inter-pass cleaning..."}
]}
```

**ChatML** is what the model actually sees post-templating — the wire format with `<|im_start|>` and `<|im_end|>` delimiters. **Continued pretraining uses plain text** with no instruction structure, just a `text` column.

### Deploying with Ollama

The deployment artifact is an Ollama Modelfile. For a RAG-only specialist:

```
FROM qwen2.5:14b-instruct-q5_K_M
PARAMETER temperature 0.2
PARAMETER num_ctx 16384
PARAMETER stop "<|im_end|>"
SYSTEM """[domain expert prompt from above]"""
MESSAGE user      "What's the preheat for 1\" A36?"
MESSAGE assistant "Per AWS D1.1:2020 Table 5.8 (Category B, low-hydrogen)..."
```

Build with `ollama create metalworker -f Modelfile.metalworker`. To attach a LoRA adapter, add `ADAPTER ./adapters/metalworker-lora` — Ollama supports GGUF-format LoRA adapters directly, but the base model in `FROM` **must match the base the adapter was trained on**, or behavior is undefined.

**Evaluation must be multi-dimensional**. For safety-critical specialists, build a custom benchmark of 50-100 hand-curated Q&A from senior practitioners, 20-30 "should refuse" out-of-scope questions, and 10-20 numerical-tolerance questions. Use **LLM-as-judge with boolean rubrics** (pass/fail per dimension) rather than 1-10 scores — judges are unreliable on fine scales. Dimensions for welding: `factually_correct`, `cites_code`, `units_correct`, `appropriate_refusal`, `safety_compliant`. Track Recall@5 and MRR separately for the retrieval layer using the Ragas library.

---

## 4. Code mastery — training a model on Go

Teaching a model to write idiomatic Go code in a specific style follows a different pipeline than text specialists: extraction is AST-driven, training data uses fill-in-the-middle format, and evaluation runs actual code through `go build` and `go test`.

### Base model selection (May 2026)

| Model | VRAM (Q4_K_M) | Strengths |
|---|---|---|
| **Qwen2.5-Coder-7B-Base** | ~4.7 GB | Best 7B for FIM fine-tuning, Apache 2.0 |
| **Qwen2.5-Coder-14B** | ~8.5 GB | RTX 4080 sweet spot |
| **Qwen2.5-Coder-32B-Instruct** | ~19 GB | HumanEval 92.7%, beats GPT-4o |
| **Qwen3-Coder-30B-A3B (MoE)** | ~17 GB | 256K context, RTX 4090 best 2026 |
| **Qwen3.6-27B** | ~16.8 GB | **SWE-bench Verified 77.2%** — strongest open ≤32B |
| **DeepSeek-Coder-V2-Lite** | ~10 GB | Native FIM at 0.5 rate |
| **StarCoder2-15B** | ~10 GB | 619 languages, repo-level FIM |
| **Codestral 22B** | ~14 GB | Strong FIM; restrictive license |

For a Go specialist on a 24 GB card, **start from Qwen2.5-Coder-7B-Base or 14B-Base** — native FIM tokens, Apache 2.0, most tested. For agentic coding (Cursor/Aider-style), Qwen3-Coder-Next or Qwen3.6-27B.

### Extracting training examples from Go code

Go has a strong advantage over other languages: the standard library's `go/ast` package gives type-accurate, generics-aware parsing for free. Use tree-sitter only when you need cross-language uniformity or tolerance for syntactically broken code.

```go
// extract.go — walk a module, emit {godoc, signature, body} JSONL
for _, decl := range f.Decls {
    fn, ok := decl.(*ast.FuncDecl)
    if !ok || fn.Doc == nil { continue }
    // signature without body
    sig := *fn; sig.Body = nil
    var sigBuf, bodyBuf strings.Builder
    printer.Fprint(&sigBuf,  fset, &sig)
    printer.Fprint(&bodyBuf, fset, fn.Body)
    enc.Encode(Example{Doc: fn.Doc.Text(), Sig: sigBuf.String(), Body: bodyBuf.String()})
}
```

The highest-signal extraction targets are: **doc → function body** (the canonical supervised pair), test → implementation (from `_test.go`), interface → implementation (via `go/types`), commit diffs for "edit code to do X" pairs, and SWE-bench-style issue+PR resolution data for agentic capability.

### FIM training format and packing

Modern code models (Qwen2.5-Coder, StarCoder2, DeepSeek-Coder, CodeLlama, Codestral) are trained with **fill-in-the-middle** tokens. Qwen2.5-Coder uses the PSM (prefix-suffix-middle) layout:

```
<|fim_prefix|>{prefix}<|fim_suffix|>{suffix}<|fim_middle|>{middle}<|endoftext|>
```

A 50/50 mix of FIM examples and standard next-token-prediction examples during continued pretraining is the established recipe. Two refinements from 2025-2026 research: **repo-level packing in dependency-graph topological order** (DeepSeek-Coder's improvement over StarCoder1's random order, arXiv 2412.16589), and **structure-aware FIM** that aligns hole boundaries to syntactic units rather than character offsets (arXiv 2506.00204) — boosting infilling Pass@1 from 84.6 to 93.6.

### Quality gates for synthetic Go data

Every synthetic Go training example must pass **hard compile gates** before joining the training set: `gofmt -d` (no formatting diff), `goimports -d` (imports clean), `go vet` (no warnings), `go build ./...` in a scaffolded module (must compile), and if a test is provided, `go test` must pass. Hash-based dedup against HumanEval, MBPP, and MultiPL-E test cases prevents benchmark contamination — DeepSeek-Coder and Qwen2.5-Coder both use 10-gram overlap filters.

### Continued pretraining vs instruction tuning for code

For teaching a model a private Go codebase's idioms, the optimal recipe is **CPT then SFT**. CPT: 1-3 epochs of raw code packed as 50% FIM and 50% NTP, with 5% general code mix (Python/Rust/SQL from The Stack v2) and 5% English instruct data as a replay buffer to prevent forgetting; learning rate 1e-5 to 3e-5, cosine schedule. SFT: 5K-50K Alpaca/ShareGPT pairs at LR 5e-6 to 2e-5. Optional DPO with build-passing/test-passing code as `chosen` and vet-warning code as `rejected`.

**Notable finding** (arXiv 2406.14833): for under 200M tokens of CPT data, 3-5 epochs over the small corpus recovers performance faster than single-epoch on a huge corpus. LoRA underperforms full fine-tune on code/math per Biderman et al.'s "LoRA Learns Less and Forgets Less" — full-finetune 7B models when feasible, LoRA only at 14B and above.

Evaluation runs **MultiPL-E for Go** (HumanEval translated to 18 languages including Go), unit-test pass rate on held-out internal functions, and `go build` / `go vet` / `gofmt -l` percentages on outputs. SWE-bench Verified is the right benchmark for agentic coders; LiveCodeBench provides decontaminated contest problems.

---

## 5. Unsloth, in depth

Unsloth is the default training stack for single-GPU and small multi-GPU fine-tuning in 2026. The library delivers **2× faster training and up to 70% less VRAM** versus a HuggingFace+PEFT baseline via custom Triton kernels, a manual-backprop engine, dynamic 4-bit quantization, and a gradient-accumulation correctness fix. The headline claim: zero accuracy loss, no approximation — exact mathematical equivalence with reference implementations.

### What's new in 2026

The library now ships in two halves. **Unsloth Core** (Apache 2.0, `pip install unsloth`) is the Python library. **Unsloth Studio** (AGPL-3.0, launched March 2026) is a self-hosted browser GUI at `localhost:8888` with Chat, Training, Data Recipes, Export, and Model Arena tabs.

The 2026 release notes highlight: **MoE training is now 12× faster with 35% less VRAM** (gpt-oss-20B trains on a 12.8 GB GPU; Qwen3-30B-A3B QLoRA fits in 17.5 GB); GRPO uses 80% less VRAM with 7-12× longer context (gpt-oss QLoRA RL hits 380K context on a single B200); long-context SFT reaches 500K tokens on a 20B model on an 80 GB GPU; FP8 training and embedding fine-tuning are now supported; multi-GPU works via `device_map="balanced_low0"`. The previously single-GPU restriction has loosened — Accelerate, DeepSpeed, FSDP, and DDP all work today; native simplified multi-GPU is "coming soon."

### Supported models

Unsloth supports essentially every modern open-weight LLM through their `unsloth/` HuggingFace namespace, which ships **tokenizer and chat-template bug fixes** that often aren't yet upstream. The families: Llama 3.x/4, Qwen 2.5/3/3.5/3.6 (including Coder variants up to 480B-A35B), Mistral and Magistral and Devstral, Phi-3/3.5/4 (including reasoning variants), Gemma 2/3/3n/4, DeepSeek V3/R1/OCR, gpt-oss (20B/120B), GLM-4.6V/4.7, Kimi-2.5/K2.6, Nemotron 3, OLMo 3, and full vision/TTS/STT/embedding stacks.

The HF naming convention matters: `*-unsloth-bnb-4bit` is **Unsloth Dynamic 4-bit** (selectively skip-quantizes ~10% of weights for near-16-bit accuracy at QLoRA memory cost) — always prefer this over the vanilla `*-bnb-4bit`.

### Installation

The 2026 canonical install uses `uv` with Python 3.13 (an older doc warning that 3.13 isn't supported is obsolete):

```bash
# Linux / WSL / macOS
curl -LsSf https://astral.sh/uv/install.sh | sh
uv venv unsloth_env --python 3.13 && source unsloth_env/bin/activate
uv pip install unsloth --torch-backend=auto
```

Minimum CUDA compute capability is 7.0 (T4, V100 and up). RTX 50-series (Blackwell), DGX Spark, B200, and RTX 6000 Blackwell are supported through `--torch-backend=auto`. The latest PyPI release at research time is `unsloth==2026.5.8`.

### Loading models

```python
from unsloth import FastLanguageModel
model, tokenizer = FastLanguageModel.from_pretrained(
    model_name      = "unsloth/Meta-Llama-3.1-8B-unsloth-bnb-4bit",
    max_seq_length  = 2048,        # Unsloth auto-handles RoPE scaling
    dtype           = None,        # auto: fp16 on T4/V100, bf16 on Ampere+
    load_in_4bit    = True,        # QLoRA path (NF4 + double quant)
    full_finetuning = False,       # mutually exclusive with load_in_*
    fast_inference  = False,       # enable vLLM-backed inference (for GRPO)
)
```

Exactly one of `load_in_4bit`, `load_in_8bit`, `load_in_16bit`, `full_finetuning` may be true. The library handles paged optimizers automatically when 4-bit is selected.

### Configuring LoRA

```python
model = FastLanguageModel.get_peft_model(
    model, r = 16,                                  # 8/16/32/64/128
    target_modules = ["q_proj","k_proj","v_proj","o_proj",
                      "gate_proj","up_proj","down_proj"],
    lora_alpha = 16,                                # r or 2*r
    lora_dropout = 0,                               # 0 is kernel-optimized
    bias = "none",                                  # "none" is kernel-optimized
    use_gradient_checkpointing = "unsloth",         # -30% VRAM vs True
    random_state = 3407,
    use_rslora = False,                             # rsLoRA: alpha/sqrt(r)
    loftq_config = None,                            # LoftQ init via SVD
)
```

Rank selection follows task complexity: **r=8 for classification or style transfer**, r=16 (the industry default) for instruction tuning and most domain adaptation, **r=32-64 for complex reasoning or code**, and r=128+ only when validation loss stagnates at lower ranks. Keep alpha/r ≥ 1; `alpha = r` or `alpha = 2*r` are the proven conventions. With `use_rslora=True`, effective scaling becomes `alpha / sqrt(r)`.

The QLoRA paper's data shows targeting **all major linear layers** (attention + FFN) is essential to match full fine-tune quality — applying LoRA to attention only loses meaningful performance. For continued pretraining, add `"lm_head"` and `"embed_tokens"` to target modules, but use 2-10× smaller learning rates for those (Unsloth exposes `embedding_learning_rate` in `UnslothTrainingArguments`).

### Training with SFTTrainer

The full SFT script — Llama 3.1 8B on Alpaca, ~45-90 minutes on a 4090:

```python
from trl import SFTTrainer
from transformers import TrainingArguments
from unsloth import FastLanguageModel, is_bfloat16_supported
from unsloth.chat_templates import get_chat_template, standardize_sharegpt, train_on_responses_only
from datasets import load_dataset

# 1. Model + LoRA
model, tok = FastLanguageModel.from_pretrained(
    "unsloth/Meta-Llama-3.1-8B-Instruct-unsloth-bnb-4bit",
    max_seq_length=2048, load_in_4bit=True)
model = FastLanguageModel.get_peft_model(
    model, r=16, lora_alpha=16, lora_dropout=0, bias="none",
    target_modules=["q_proj","k_proj","v_proj","o_proj","gate_proj","up_proj","down_proj"],
    use_gradient_checkpointing="unsloth", random_state=3407)

# 2. Dataset → Llama-3.1 chat template
tok = get_chat_template(tok, chat_template="llama-3.1")
ds  = standardize_sharegpt(load_dataset("mlabonne/FineTome-100k", split="train"))
def fmt(ex):
    return {"text":[tok.apply_chat_template(c, tokenize=False) for c in ex["conversations"]]}
ds = ds.map(fmt, batched=True)

# 3. Trainer
trainer = SFTTrainer(
    model=model, tokenizer=tok, train_dataset=ds,
    dataset_text_field="text", max_seq_length=2048, packing=False,
    args=TrainingArguments(
        per_device_train_batch_size=2, gradient_accumulation_steps=8,
        warmup_ratio=0.05, num_train_epochs=1,
        learning_rate=2e-4, lr_scheduler_type="linear",
        optim="adamw_8bit", weight_decay=0.01,
        bf16=is_bfloat16_supported(), fp16=not is_bfloat16_supported(),
        output_dir="outputs", seed=3407))

# 4. Train only on assistant tokens (~1% RougeL gain per QLoRA paper)
trainer = train_on_responses_only(trainer,
    instruction_part="<|start_header_id|>user<|end_header_id|>\n\n",
    response_part   ="<|start_header_id|>assistant<|end_header_id|>\n\n")
trainer.train()
```

The hyperparameter cheat sheet that matters in practice: **learning rate 2e-4 for LoRA/QLoRA** (5e-6 for RL/DPO/GRPO), 1-3 epochs, effective batch size 16 (per-device 2 × grad-accum 8), warmup 5-10% of total steps, weight decay 0.01, optimizer `adamw_8bit` (or `paged_adamw_8bit` if OOMing on QLoRA), seed 3407. A target training loss is 0.5-1.0 — **below 0.2 strongly suggests overfitting**.

The gradient-accumulation correctness fix Unsloth shipped in 2025 is now table stakes: `batch_size=1 × grad_accum=16` produces identical loss curves to `batch_size=16 × grad_accum=1`, which was historically not the case in HF Transformers.

---

## 6. Data recipes and chat templates

Unsloth bundles three pieces of data infrastructure that materially change preparation ergonomics: **`get_chat_template`** registers correct templates for every supported family (Llama-3.1, Qwen 2.5, Phi-4, Gemma-3, ChatML, Alpaca, Vicuna, etc.); **`standardize_sharegpt`** converts ShareGPT-style `{from, value}` data to OpenAI `{role, content}`; and **`to_sharegpt`** auto-builds prompts from multi-column CSV/tabular data, with `[[...]]` optional-block syntax for missing values and `conversation_extension` to synthesize multi-turn dialogues from single-turn rows.

### Studio Data Recipes

The headline 2026 feature is **Unsloth Studio Data Recipes**, a visual node-graph dataset builder powered by NVIDIA NeMo DataDesigner. Block types include: **Seed** blocks (HuggingFace Hub, local files, unstructured PDFs/DOCX with auto-chunking, GitHub repos); **LLM** blocks for text, structured data, and code generation, callable against hosted APIs (OpenAI/Anthropic) or local vLLM/Ollama/llama.cpp/Unsloth backends; **Sampler** blocks (Category, Gaussian, Person) for deterministic synthetic variables; **Validator** blocks running Python, SQL, or OXC (JS/TS) linters to auto-filter bad rows; **Expression** blocks for Jinja2 transforms without LLM calls; and **Tool profile** blocks sharing MCP tool configurations across LLM blocks. Outputs land as Parquet ready to feed into `SFTTrainer`.

### Dataset size and mixing rules

Unsloth's own documentation gives explicit dataset size guidance: under 100 rows is usually too small; 100-300 rows works for fine-tuning Instruct models; 300-1000 rows works for base or instruct; 1000+ rows favors training on a base model. **Always mix domain data with general instruction data** (FineTome-100k, ShareGPT, OpenHermes) to avoid catastrophic forgetting — the docs make this explicit. For reasoning preservation when fine-tuning Qwen3 or DeepSeek-R1-distilled models, keep a minimum of 75% reasoning examples in the mix.

### Continued pretraining data

CPT requires no instruction template — just a `text` column of raw documents. Use `UnslothTrainer` with `UnslothTrainingArguments` to set `embedding_learning_rate=1e-5` alongside the main `learning_rate=5e-5`, since the embedding and lm_head layers should learn slower than middle layers during knowledge injection.

### Notable notebooks

The `unslothai/notebooks` repo ships 250+ ready-to-run Colab/Kaggle notebooks. The essential ones for this guide: `Llama3.1_(8B)-Alpaca.ipynb` (canonical SFT), `Llama3_(8B)-Ollama.ipynb` (full train→export→Ollama deploy), `Mistral_v0.3_(7B)_CPT.ipynb` (continued pretraining), `Meta_Synthetic_Data_Llama3_2_(3B).ipynb` (PDF/video to QA pairs), `gpt-oss-(20B)-Fine-tuning.ipynb`, `Qwen3_(4B)-GRPO.ipynb`, and the Kaggle dual-T4 Magistral notebook demonstrating multi-GPU.

---

## 7. From training to deployment

The export-and-deploy pipeline is now a single function call followed by a Modelfile.

### GGUF export, four quants in one call

```python
# LoRA adapters only (smallest, fastest to share)
model.save_pretrained("finetuned_lora"); tokenizer.save_pretrained("finetuned_lora")

# Merged 16-bit safetensors (vLLM serving, HF sharing)
model.save_pretrained_merged("model_16bit", tok, save_method="merged_16bit")

# GGUF: four quants in one call (Ollama / LM Studio / llama.cpp)
model.save_pretrained_gguf("model", tok,
    quantization_method=["q4_k_m", "q5_k_m", "q8_0", "f16"])
```

Quantization selection (verified inference VRAM for 7B):

| Quant | Bits | 7B VRAM | Quality | Use |
|---|---|---|---|---|
| **f16** | 16 | ~14 GB | Lossless | Reference / further training |
| **q8_0** | 8 | ~7 GB | ~Lossless | Best quality if you have VRAM |
| **q5_k_m** | 5 | ~5 GB | ~99% retention | Quality-first deploy |
| **q4_k_m** | 4 | ~4-5 GB | 1-2% MMLU drop | **Default Ollama deploy** |
| **IQ4_XS / IQ4_NL** | 4 | ~4 GB | Better than q4_k_m | Quality king at 4-bit (slower decode) |
| **UD-Dynamic 2.0** | mixed | varies | SOTA per-layer | Unsloth's own dynamic quant — best when available |

GGUF conversion on Windows no longer requires compiling llama.cpp for 16-bit/8-bit saves, though the `q4_k_m` family can occasionally still trigger a fallback build.

### Ollama Modelfile and deployment

```
FROM ./model.gguf
TEMPLATE """{{ if .System }}<|im_start|>system
{{ .System }}<|im_end|>
{{ end }}{{ if .Prompt }}<|im_start|>user
{{ .Prompt }}<|im_end|>
{{ end }}<|im_start|>assistant
"""
SYSTEM """You are a domain-tuned assistant for X."""
PARAMETER temperature 0.2
PARAMETER num_ctx 16384
PARAMETER stop "<|im_end|>"
ADAPTER ./lora-adapter.gguf
```

Then `ollama create mymodel -f Modelfile && ollama run mymodel`. Two non-obvious requirements: **the `TEMPLATE` must exactly match the chat template used during training** (mismatched templates cause silent quality regression or outright gibberish — copy from `ollama show base-model --modelfile`), and **stop tokens must include both the EOS token and any chat-template delimiters** like `<|im_end|>` or `<|eot_id|>`.

The full Docker stack for Ollama + Open WebUI is roughly 20 lines of Compose YAML; with that running, custom models created via Modelfile appear automatically in the WebUI model picker, and Knowledge collections give non-technical users RAG without code.

### Merge vs adapter trade-off

Keeping LoRA adapters separate is best for multi-tenant serving (vLLM/TGI multi-LoRA on a shared base) and rapid iteration — adapter files are 10-200 MB versus multi-GB merged models. Merging is best for single-task production deploy: simpler artifact, slightly faster inference, single file to version-control.

### Iteration and versioning

Resuming training from a saved adapter is one line: `trainer.train(resume_from_checkpoint=True)` after reloading via `FastLanguageModel.from_pretrained("path/to/lora")`. The 2026 standard versioning stack: **HuggingFace Hub** for weights and adapters (via `hf-transfer` for large files), **Git LFS** for self-hosted repos, **DVC** for reproducible dataset pipelines, and **W&B or MLflow** for experiment tracking with artifact storage. Production evaluation needs a held-out eval dataset (10-20% of training data), MMLU delta tracking to detect catastrophic forgetting, and string-match or LLM-as-judge regression tests on a fixed prompt set.

---

## 8. Hardware reality check

Unsloth publishes the most authoritative VRAM table for QLoRA training (May 2026), assuming `batch_size=1` and `use_gradient_checkpointing="unsloth"`:

| Model | QLoRA 4-bit | LoRA 16-bit | Comfortable GPU |
|---|---|---|---|
| 3B | **3.5 GB** | 8 GB | RTX 3060 12 GB |
| 7B | **5 GB** | 19 GB | RTX 3060 12 GB |
| 8B | **6 GB** | 22 GB | RTX 3060 12 GB |
| 14B | **8.5 GB** | 33 GB | RTX 4060 Ti 16 GB |
| 27B | 22 GB | 64 GB | RTX 3090 / 4090 24 GB |
| 32B | **26 GB** | 76 GB | RTX 5090 32 GB (tight on 4090) |
| 70B | **41 GB** | 164 GB | A100 80 GB or 2×24 GB FSDP+QLoRA |
| 405B | 237 GB | 950 GB | Multi-node H100 cluster |

Real-world budgets add 20-40% for batch > 1 and longer context — every doubling of context length roughly doubles attention memory unless gradient checkpointing absorbs it.

### Throughput by tier

Approximate training throughput on 7B QLoRA at 2K context (varies by model and packing):

| GPU | VRAM | tok/s (7B QLoRA) | 5K examples × 3 epochs |
|---|---|---|---|
| RTX 3060 12 GB | 12 | ~800-1,500 | 6-10 hours |
| RTX 3090 24 GB | 24 | ~2,500-4,000 | 1.5-3 hours |
| RTX 4090 24 GB | 24 | ~4,500-7,000 | 45-90 minutes |
| **RTX 5090 32 GB** | 32 | ~7,500-12,000 | **30-60 minutes** |
| A100 80 GB | 80 | ~6,000-10,000 | 40-70 min |
| H100 80 GB | 80 | ~12,000-20,000 | 15-25 min |
| H200 141 GB | 141 | ~15,000-25,000 | 12-20 min |

The RTX 5090 (released, $1,999-$2,499 MSRP) is roughly **72% faster than the 4090 on NLP workloads** and crosses the 32B QLoRA threshold comfortably for the first time at consumer pricing. A 1000W+ PSU is required (575W TDP).

### Cloud GPU pricing, May 2026

Verified spot/community pricing across providers:

| GPU | Vast.ai | RunPod Community | Lambda Labs | AWS/GCP on-demand |
|---|---|---|---|---|
| RTX 4090 | $0.29-0.44/hr | $0.34/hr | — | — |
| A100 80GB | $0.90-1.39/hr | — | $1.79/hr | $3.67+/hr |
| H100 80GB | $1.49-2.50/hr | $1.99/hr | $2.49-2.99/hr | $6.88-12.29/hr |
| H200 | — | $3.59/hr | — | — |
| B200 | — | $4.99/hr | $5.29/hr | $14.24/hr |

Practical example: Llama 3.1 8B QLoRA on 5K examples completes in ~1 hour on H100, costing roughly **$2 on RunPod community pricing or $10 on AWS on-demand**. Spot/interruptible tiers run 30-70% cheaper if your training script handles preemption.

For free options: **Kaggle's 30 GPU-hours/week of P100 or dual T4 with 9-12 hour sessions is the most generous free tier** for serious training, more practical than Colab Free's T4 with ~15-22 hours/week and 12-hour limits. Colab Pro at $9.99/mo buys ~57 T4-hours or ~7 A100-hours of compute units monthly.

### Apple Silicon: MLX is real, but slow

The MLX-LM and MLX-LM-LoRA frameworks (Gökdeniz Gülmez, 2025) now support LoRA, DoRA, QLoRA, full fine-tune, SFT, DPO, ORPO, GRPO, and quantization-aware training on Apple Silicon. The unified-memory advantage is real — an M3 Ultra with 192 GB lets the GPU see ~140 GB for model weights, so **70B QLoRA training fits on a Mac Studio**. Throughput is the catch: MLX QLoRA training is roughly **1-3 tok/s on M3 Max versus 30-50 tok/s on H100** — fine for prototyping or 7B-32B work, impractical for production-scale 70B+. Apple's M5 (announced WWDC 2025) is reported to be 3.5-4× faster than M4 for LLM workloads. MLX is roughly 30-50% faster than llama.cpp on the same Apple Silicon.

```bash
python -m mlx_lm.lora --model microsoft/Phi-3.5-mini-instruct \
    --train --data ./data --iters 1000 --batch-size 4
python -m mlx_lm.fuse --model base --adapter-path ./adapters \
    --save-path ./fused --de-quantize
# Then convert to GGUF via llama.cpp for Ollama deployment
```

### Multi-GPU status

The historical "Unsloth is single-GPU only" framing no longer holds. **Accelerate, DeepSpeed (ZeRO 1/2/3), FSDP v1/v2, and DDP all work today** via `accelerate launch train.py` or `torchrun --nproc_per_node N`. Official simplified multi-GPU is still labeled "coming soon," but the building blocks are functional — and the Magistral-2509 Kaggle notebook demonstrates dual-T4 training of a 24B model. For full multi-GPU LoRA/QLoRA in production, **Axolotl** (YAML-driven, mature FSDP+QLoRA support, sequence parallelism) and Red Hat's **Training Hub** (Unsloth backend as of v0.4.0, April 2026) are more polished options than raw Unsloth + Accelerate.

CPU training above ~1B parameters remains impractical — llama.cpp's CPU LoRA path runs at under 10 tok/s, meaning multi-day epochs even on tiny models. Use CPUs for data prep, evaluation, and RAG; not gradient updates.

---

## Conclusion: how the pieces fit

The 2026 stack converges on a single recommended path for most users: **start with prompt engineering, add RAG when the answer is volatile or must be cited, fine-tune with Unsloth+LoRA when behavior is the gap, and stack RAG over a fine-tuned base for safety-critical or citation-required domains.** A practitioner with an RTX 4090 and a folder of PDFs can now ingest documents through Unstructured.io or PyMuPDF4LLM, generate synthetic Q&A with Augmentoolkit or a local Qwen2.5-72B teacher, fine-tune an 8B Qwen2.5 with QLoRA in under an hour, export q4_k_m GGUF in one call, deploy via Ollama with a 20-line Modelfile, and serve it with hybrid retrieval and reranking through Open WebUI — entirely offline, for under $2,000 in hardware.

The non-obvious insights that separate working deployments from failing ones: **retrieval failure is the dominant RAG bug**, fixed by hybrid BM25+dense plus a cross-encoder reranker (a 67% improvement, not a 10% one); **synthetic data without filtering is worse than less data**, so AlpaGasus-style LLM-judge gating and embedding dedup are non-optional; **chat-template mismatch at deploy time silently destroys fine-tune gains**, so the Modelfile `TEMPLATE` must match training exactly; and **fine-tuned models cannot reliably memorize new facts**, so anything that must be cited belongs in retrieval, not weights. The frontier is moving fast — MoE training got 12× faster in early 2026, 500K-context SFT is now possible on a single 80 GB card, and Apple Silicon training is increasingly viable for hobbyists — but the core decision framework, RAG for facts and fine-tuning for behavior, has stabilized into the consensus default.