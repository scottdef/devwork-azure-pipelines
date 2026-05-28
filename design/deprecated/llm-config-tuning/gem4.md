# Gemma 4 Local Operation User Guide

**TL;DR**
- Gemma 4 (released April 2, 2026 under Apache 2.0) ships in four sizes — E2B, E4B, 26B-A4B (MoE) and 31B Dense — all multimodal, with native function calling, thinking mode, 128K–256K context, and day‑0 support in Ollama, llama.cpp, vLLM, Transformers, MLX, LM Studio, KerasHub and LiteRT‑LM.
- For local use, `ollama pull gemma4:e4b` (≈9.6 GB, 128K ctx) is the safest starting tag; the 26B‑A4B MoE on a 24 GB GPU at Q4_K_M is the sweet spot for serious work, and the 31B Dense at Q4_K_M (≈17.4 GB) is the quality ceiling for a single 24 GB consumer card.
- Apache 2.0 makes Gemma 4 unrestricted for commercial use (no MAU cap), but the architecture (`Gemma4ForConditionalGeneration` with PLE, hybrid local/global attention, shared KV cache, p‑RoPE, MoE on 26B and conformer audio on E2B/E4B) is genuinely new — verify your runtime supports it (transformers ≥ 5.5, vLLM nightly/recipes, llama.cpp ≥ b8746, Ollama ≥ v0.20.0).

---

## Key Findings

- **Four sizes, three architectures.** E2B (≈5.1B total / 2.3B active) and E4B (≈7.9B total / 4.5B active) use **Per‑Layer Embeddings (PLE)**, where each decoder layer gets its own small embedding table per token; the **26B A4B** is a **Mixture‑of‑Experts** with 128 fine‑grained experts and **top‑8 routing** activating ≈3.8B params per token; the **31B Dense** is a wide, fully dense model with 60 layers. All four interleave **local sliding‑window attention** (512 tokens on E2B/E4B, 1024 on 31B/26B) with **full global attention**, the final layer is always global, and global layers use **unified K=V** plus **Proportional RoPE (p‑RoPE)**.
- **Genuinely multimodal.** Every variant ingests text + images with variable aspect ratios and a configurable visual token budget (70 / 140 / 280 / 560 / 1120 tokens per image). Video is handled as frame sequences (up to 60 s at 1 fps) on all sizes; **native audio (≤30 s) is only on E2B and E4B** via a conformer encoder. The 31B/26B handle video but not audio; E2B/E4B handle audio but not video natively. Output is text only.
- **Native function calling, thinking, and system prompts.** Gemma 4 ships a dedicated tool‑call protocol with custom special tokens (`<|tool_call>`, `<tool_call|>`), a `<|channel>thought` reasoning channel, and — new vs Gemma 3 — a real `system` role in the chat template. Thinking is on by default and can be disabled by adding `<|think|>` (off semantic) at the start of the system prompt; for E2B/E4B disabling thinking suppresses output, while 26B/31B still emit an empty thought block.
- **Context windows.** 128K tokens on E2B/E4B; 256K on 26B‑A4B and 31B (Google's NVFP4 31B card now documents 256K).
- **License is Apache 2.0**, confirmed on the model card (`License: apache-2.0`) and Google's docs sidebar linking the "Gemma 4 license" page directly to a verbatim Apache License 2.0 text. This is a major change from Gemma 3's "Gemma Open" terms — commercial use, redistribution, and fine‑tuning are unrestricted, with only Google's general Prohibited Use Policy still applying as a separate policy.
- **HuggingFace model_type / architectures:** `Gemma4ForConditionalGeneration` in `config.json` for all instruction‑tuned multimodal checkpoints; sub‑configs include `Gemma4TextConfig`, `Gemma4VisionConfig`, `Gemma4AudioConfig`. Tokenizer is the standard Gemma SentencePiece tokenizer with a **262,144‑token vocab**. The chat template ships as a separate `chat_template.jinja` (not embedded in `tokenizer_config.json` — a known gotcha for third‑party pipelines).
- **Day‑0 ecosystem support.** Hugging Face Transformers (TRL, Transformers.js, Candle), vLLM (NVIDIA, AMD, TPU `vllm-tpu:gemma4`, Intel Xeon 6), llama.cpp, MLX (Apple Silicon), Ollama v0.20.0+, LM Studio, Unsloth, SGLang, KerasHub, NVIDIA NIM/NeMo, Red Hat AI Inference Server, Docker, MaxText, Tunix, LiteRT‑LM, and Baseten all shipped support on launch day.

---

## Details

### 1. Model Architecture and Variants

| Variant | Total params | Active params | Architecture | Context | Modalities (in) | Audio in |
|---|---|---|---|---|---|---|
| gemma-4-E2B / E2B-it | ~5.1 B | ~2.3 B | Dense + PLE | 128 K | Text + Image + Video frames + Audio | ✅ ≤30 s |
| gemma-4-E4B / E4B-it | ~7.9 B | ~4.5 B | Dense + PLE | 128 K | Text + Image + Video frames + Audio | ✅ ≤30 s |
| gemma-4-26B-A4B / -it | 26 B | 3.8 B | MoE, 128 experts, top‑8 + shared expert | 256 K | Text + Image + Video | ❌ |
| gemma-4-31B / 31B-it | 30.7 B | 30.7 B | Dense (60 layers) | 256 K | Text + Image + Video | ❌ |

Additional checkpoints from Google:
- **Multi‑Token Prediction (MTP) drafter / assistant models** for each size (e.g. `google/gemma-4-26B-A4B-it-assistant`) — small `Gemma4AssistantForCausalLM` heads that enable speculative decoding. Per Google's official May 5, 2026 blog post: *"By using a specialized speculative decoding architecture, these drafters deliver up to a 3x speedup without any degradation in output quality or reasoning logic,"* benchmarked across LiteRT‑LM, MLX, Hugging Face Transformers and vLLM. E2B/E4B drafters use **centroids masking** that reduces lm_head compute by ~45× by selecting ~4 K candidate tokens instead of the full 262 K vocab.
- **NVFP4 / FP8 / AWQ / GPTQ / INT4 community variants** (NVIDIA, RedHat, Intel, cyankiwi, LilaRest) for production GPU serving.

**What's new vs Gemma 3.** Always‑global final layer; **K = V in global layers**; **p‑RoPE** (low‑frequency‑pruned RoPE) for long context; native `system` role; native function calling; native thinking mode with `<|channel>thought` channel; full multimodality across all sizes; MTP drafters; Apache 2.0 license; first MoE variant (26B); audio support on edge sizes. The agentic/coding jump is striking: per Google's model card cross-referenced by verdent.ai, on **τ2-bench Retail Gemma 3 27B scored 6.6 % and Gemma 4 31B scored 86.4 %**, and Codeforces ELO climbed from 110 on Gemma 3 27B to 2150 on Gemma 4 31B.

**On‑device variant ("Gemma 4n").** There is no separately‑branded "Gemma 4n" model — the edge role previously occupied by Gemma 3n is filled by **E2B** and **E4B**, both of which run on Pixel devices, Raspberry Pi 5, Jetson Orin Nano, and Android via AICore. On supported Android phones, Gemma 4 is exposed through Android AICore as **Gemini Nano 4** for production apps.

**HuggingFace config.** All instruction‑tuned multimodal checkpoints declare:
```json
{
  "architectures": ["Gemma4ForConditionalGeneration"],
  "model_type": "gemma4",
  "text_config":  { "model_type": "gemma4_text",  "vocab_size": 262144, ... },
  "vision_config":{ "model_type": "gemma4_vision", ... },
  "audio_config": { "model_type": "gemma4_audio", ... }
}
```
Pretrained‑only checkpoints (`gemma-4-E2B`, `-E4B`, `-26B-A4B`, `-31B`) use the same architecture string. Tokenizer = SentencePiece, **262,144 vocab** (shared with Gemma 2/3).

### 2. HuggingFace Repositories

Official Google org: `google/gemma-4-{E2B,E4B,26B-A4B,31B}` and `-it` variants (pre‑trained + instruction‑tuned). Audio/vision tower is bundled; load with `AutoModelForMultimodalLM` (or `AutoModelForCausalLM` for text‑only paths). Each repo also has an `-assistant` MTP drafter and litert‑lm exports under `litert-community/`.

Weights ship as **sharded safetensors** with a `model.safetensors.index.json` (13 shards for the 31B). `chat_template.jinja` is a **separate file** — many third‑party tools fail to copy it, so if you see `tokenizer.chat_template is not set`, manually load via `hf_hub_download("google/gemma-4-E2B-it", "chat_template.jinja")`.

**Community quantizations (HuggingFace):**
- **bartowski/google_gemma-4-{E2B,E4B,26B-A4B,31B}-it-GGUF** — full GGUF stack (BF16, Q8_0, Q6_K_L, Q5_K_M, Q4_K_M, Q4_K_L, Q3_K_XL, Q3_K_M, IQ3_M, Q2_K), imatrix‑calibrated, llama.cpp ≥ b8746.
- **unsloth/gemma-4-…-GGUF** — Dynamic 2.0 GGUF + 4‑bit / 8‑bit MLX dynamic quants; 8 of 9 Unsloth UD quants are on the Pareto frontier per the LocalBench KL‑divergence benchmark on 31B.
- **lmstudio-community/gemma-4-E4B-it-GGUF** — LM Studio's curated GGUF.
- **onnx-community/gemma-4-E2B-it-ONNX** — Transformers.js / browser export.
- **nvidia/Gemma-4-31B-IT-NVFP4** — NVFP4 W4A4, fits a single 24 GB GPU (≈19.3 GB), 0.25 % accuracy drop on GPQA Diamond.
- **mlx-community/gemma-4-…-{4bit,8bit,bf16}** and **mistralrs-community/gemma-4-E4B-it-UQFF**.
- **cyankiwi/gemma-4-31B-it-AWQ-4bit** (20.5 GB), **Intel/gemma-4-31B-it-int4-AutoRound**, RedHat `RedHatAI/gemma-4-…` FP8 / NVFP4 lines.

### 3. Local Inference Runtimes

#### Ollama
Requires **Ollama ≥ v0.20.0** (released same day as Gemma 4, April 3 2026). The `gemma4` library on ollama.com lists 34 tags. Common pulls:

```bash
ollama pull gemma4           # default → gemma4:e4b (~9.6 GB)
ollama pull gemma4:e2b       # 7.2–7.9 GB
ollama pull gemma4:e4b       # 9.6 GB (Q4_K_M, 128 K ctx, text+image)
ollama pull gemma4:26b       # 18 GB (MoE Q4_K_M, 256 K ctx)
ollama pull gemma4:31b       # 20 GB (Q4_K_M)
ollama pull gemma4:e4b-it-q8_0   # 12 GB (higher quality)
ollama pull gemma4:e2b-mxfp8     # 7.9 GB MLX path on Apple Silicon
ollama pull gemma4:e2b-nvfp4     # 7.1 GB NVFP4 path on Blackwell GPUs
```

Ollama also publishes a `gemma4:31b-cloud` tag — that's a remote artifact, ignore it for local planning. Ollama defaults to a **4096‑token context window**; for tool‑calling / coding agents you must bump it:

```bash
# Modelfile to extend context and set a system prompt
FROM gemma4:e4b
PARAMETER num_ctx 32768
PARAMETER temperature 1.0
PARAMETER top_p 0.95
PARAMETER top_k 64
SYSTEM "You are a careful coding assistant. Explain your answer clearly."
```
```
ollama create my-gemma4 -f Modelfile
ollama run my-gemma4
```
Recommended sampling (from Google + Unsloth): **temperature 1.0, top_p 0.95, top_k 64**. On Apple Silicon, Ollama auto‑uses MLX where available; an active issue (#15368) hangs the 31B Dense when `OLLAMA_FLASH_ATTENTION=1` and prompt >500 tokens — leave Flash Attention off on Apple Silicon until PR #15244 lands.

#### llama.cpp
GGUF conversion supports `Gemma4ForConditionalGeneration` from `llama.cpp` build **b8746+**, with imatrix calibration. Drafter (assistant) conversion via `Gemma4AssistantForCausalLM` is still being upstreamed (discussion #22735). Standard quants: BF16, Q8_0, Q6_K, Q5_K_M, **Q4_K_M (default)**, Q4_K_L (Q8_0 embed/output), Q3_K_M, IQ3_M, Q2_K.

```bash
# Build
git clone https://github.com/ggml-org/llama.cpp
cmake llama.cpp -B llama.cpp/build -DGGML_CUDA=ON   # OFF for CPU/Metal
cmake --build llama.cpp/build --config Release -j

# Interactive (downloads from HF)
./llama.cpp/build/bin/llama-cli \
  -hf unsloth/gemma-4-26B-A4B-it-GGUF:UD-Q4_K_XL \
  --temp 1.0 --top-p 0.95 --top-k 64 --jinja

# Server (OpenAI-compatible)
./llama-server \
  -m /path/gemma-4-26B-A4B-it-Q4_K_M.gguf \
  --port 1234 -ngl 99 -c 32768 -np 1 --jinja \
  -ctk q8_0 -ctv q8_0
```
**Critical flags:** `--jinja` is required so llama.cpp uses the upstream chat template (with tool‑call tokens and `<|channel>thought`); `-ctk q8_0 -ctv q8_0` quantizes the KV cache (saves ~half its memory at long context). Do **not** use CUDA 13.2 runtime — Unsloth confirmed it produces broken outputs for Gemma 4 GGUFs.

#### vLLM
Day‑0 support via vLLM recipes (`docs.vllm.ai/projects/recipes/.../Google/Gemma4.html`). Containers:
- `vllm/vllm-openai:latest` (CUDA 12.9), `:latest-cu130` (CUDA 13.0)
- `vllm/vllm-openai-rocm:latest` (AMD)
- `vllm/vllm-tpu:gemma4` (Trillium / Ironwood TPUs)
- `vllm/vllm-openai-cpu:latest-x86_64` (Intel Xeon 6)

```bash
# Single-GPU E4B with audio
uv pip install "vllm[audio]"
vllm serve google/gemma-4-E4B-it --max-model-len 131072

# 31B with tensor parallel + tool calling + thinking
vllm serve google/gemma-4-31B-it \
  --tensor-parallel-size 2 \
  --max-model-len 16384 \
  --gpu-memory-utilization 0.90 \
  --enable-auto-tool-choice \
  --tool-call-parser gemma4 \
  --reasoning-parser gemma4 \
  --chat-template examples/tool_chat_template_gemma4.jinja \
  --limit-mm-per-prompt '{"image": 4, "audio": 1}' \
  --async-scheduling \
  --host 0.0.0.0 --port 8000
```
**Quantization:** `--quantization awq` (cyankiwi 4‑bit), `--quantization modelopt` (NVIDIA NVFP4 W4A4 — `nvidia/Gemma-4-31B-IT-NVFP4` fits a single 24 GB GPU), `--quantization fp8`, `--kv-cache-dtype fp8` to halve KV cache memory. The vLLM `tool-call-parser gemma4` and `reasoning-parser gemma4` are essential — without them tool calls land in the `reasoning` field instead of `tool_calls`.

#### Hugging Face Transformers
Requires **transformers ≥ 5.5** (PEFT and bitsandbytes were patched for `Gemma4ClippableLinear`; verify before fine‑tuning). For multimodal use `AutoProcessor` + `AutoModelForMultimodalLM`; for text only `AutoTokenizer` + `AutoModelForCausalLM`.

```python
import torch
from transformers import AutoProcessor, AutoModelForMultimodalLM, BitsAndBytesConfig

MODEL_ID = "google/gemma-4-E4B-it"
bnb = BitsAndBytesConfig(load_in_4bit=True,
                         bnb_4bit_quant_type="nf4",
                         bnb_4bit_compute_dtype=torch.bfloat16,
                         bnb_4bit_use_double_quant=True)

proc  = AutoProcessor.from_pretrained(MODEL_ID)
model = AutoModelForMultimodalLM.from_pretrained(
            MODEL_ID, quantization_config=bnb, device_map="auto")

messages = [{"role": "user",
             "content": [{"type": "image",
                          "image": "https://huggingface.co/datasets/merve/vlm_test_images/resolve/main/thailand.jpg"},
                         {"type": "text", "text": "Where is this and any travel tips?"}]}]
inputs = proc.apply_chat_template(messages, add_generation_prompt=True,
                                  tokenize=True, return_tensors="pt").to(model.device)
print(proc.decode(model.generate(**inputs, max_new_tokens=512)[0], skip_special_tokens=True))
```
Use `enable_thinking=False` in `apply_chat_template` to disable the reasoning channel. For audio, `AutoModelForMultimodalLM` + `librosa` and pass `{"type": "audio", "audio": "..."}` parts (only valid on E2B/E4B).

#### KerasHub / JAX
KerasHub exposes Gemma 4 via `Gemma4CausalLM.from_preset()` (text‑only) and a multimodal pipeline; backend is selectable via `KERAS_BACKEND` env var (`jax` / `tensorflow` / `torch`). Presets follow the Kaggle naming `gemma4_instruct_e2b`, `gemma4_instruct_e4b`, etc. Notebook does not run on T4; use L4 or higher.

```python
import os; os.environ["KERAS_BACKEND"] = "jax"
import keras_hub
gemma_lm = keras_hub.models.Gemma4CausalLM.from_preset("gemma4_instruct_e4b", dtype="bfloat16")
gemma_lm.generate("Explain Gemma 4 architecture in 3 bullets.", max_length=256)
```
LoRA fine‑tuning is one line: `gemma_lm.backbone.enable_lora(rank=16)`. Multi‑backend means the same checkpoint trains on TPU (JAX) and serves on CUDA (PyTorch).

#### LM Studio
Search "Gemma 4" inside the app (⌘/Ctrl + Shift + M); LM Studio auto‑suggests a GGUF or MLX variant for your hardware. Minimum system memory reported by LM Studio: **4 GB (E2B)**, **6 GB (E4B)**, **17 GB (26B)**, **19 GB (31B)**. All variants advertise vision input + tool use + reasoning support. The built‑in server exposes the OpenAI API at `http://localhost:1234`.

#### MLX (Apple Silicon)
```bash
pip install -U mlx-lm mlx-vlm
mlx_lm.generate --model mlx-community/gemma-4-E4B-it-4bit \
  --prompt "Compare E4B and 31B in 3 bullets."
```
For multimodal use `mlx-vlm` (`mlx_vlm.generate` or OpenAI‑compatible server). Per willitrunai.com's 2026 M1–M5 benchmark series, **"MLX is 15–30 % faster at the same quantization on Apple Silicon"** versus Ollama (with ~10 % less memory); famstack.dev's M1 Max 64 GB measurements separately found Ollama's Go wrapper to be 37 % slower than raw llama.cpp on the identical GGUF. **TurboQuant** (2.5‑bit KV cache) gives +15–19 % decode speedup at 128–256 K context on M3/M4 Max. Note: Ollama #15368 currently blocks the MLX backend specifically for `Gemma4ForConditionalGeneration` until PR #15244 merges.

#### ONNX / TensorRT
- `onnx-community/gemma-4-E2B-it-ONNX` (Transformers.js, browser‑runnable).
- **onnxruntime‑genai is NOT yet supported** (issue #2062): PLE, variable head dims (256 sliding vs 512 global), and KV‑cache sharing exceed onnxruntime‑genai v0.12.2's schema. Status: feature request, no ETA.
- TensorRT‑LLM and NVIDIA NIM ship Gemma 4 day‑1; use NIM if you need TRT‑accelerated inference today.

#### Google AI Edge / LiteRT‑LM (on‑device)
LiteRT‑LM is the recommended path for Android, Raspberry Pi, and IoT. Install via `uv tool install litert-lm` or `pip install --upgrade litert-lm`. Run:
```bash
litert-lm run \
  --from-huggingface-repo=litert-community/gemma-4-E4B-it-litert-lm \
  gemma-4-E4B-it.litertlm \
  --backend=gpu --enable-speculative-decoding=true \
  --prompt="What is the capital of France?"
```
Reported performance: **Raspberry Pi 5 CPU** — 133 prefill / 7.6 decode tokens/s on E2B; **Qualcomm Dragonwing IQ8 NPU** — 3,700 prefill / 31 decode tokens/s. E2B runs in **<1.5 GB RAM** thanks to LiteRT 2‑bit / 4‑bit weights and memory‑mapped PLE tables. The legacy MediaPipe LLM Inference Engine still works (`gemma-4-E4B-it-web.task`) but is in maintenance mode.

### 4. VRAM and Hardware Sizing

**Official Google AI memory table for base weights (no KV cache):**

| Model | BF16 (16‑bit) | SFP8 (8‑bit) | Q4_0 (4‑bit) |
|---|---|---|---|
| Gemma 4 E2B | 9.6 GB | 4.6 GB | 3.2 GB |
| Gemma 4 E4B | 15 GB | 7.5 GB | 5 GB |
| Gemma 4 26B A4B | 48 GB | 25 GB | 15.6 GB |
| Gemma 4 31B | 58.3 GB | 30.4 GB | 17.4 GB |

(Google's official table does not publish FP32 figures; FP32 ≈ 2 × BF16.) For practical local use add **2–4 GB headroom** for the KV cache at 4–8 K context; at 256 K context plan on +20 GB on the 31B.

**Practical GGUF VRAM (community measurements, RTX/Mac Q4_K_M with 8–32 K context):**

| Quant | E2B | E4B | 26B A4B | 31B |
|---|---|---|---|---|
| Q2_K | ~1.5 GB | ~3 GB | ~10 GB | ~13 GB |
| Q3_K_M | ~2 GB | ~3.5 GB | ~12 GB | ~14 GB |
| Q4_K_M | ~3.2 GB | ~5–6 GB | ~14–18 GB | ~18–20 GB |
| Q5_K_M | ~4 GB | ~6.5 GB | ~17 GB | ~22 GB |
| Q6_K | ~4.5 GB | ~7 GB | ~20 GB | ~26 GB |
| Q8_0 | ~5 GB | ~12 GB (Ollama) | ~25 GB | ~30 GB |
| BF16 | ~9.6 GB | ~15 GB | ~48 GB | ~58 GB |

**Recommended GPU per model (Q4_K_M):**
- **E2B** — anything ≥ 8 GB RAM/VRAM, Raspberry Pi 5 (CPU), 8 GB integrated GPU, RTX 3050.
- **E4B** — RTX 3060 12 GB / 3080 / 4060 Ti / 4070 / M1/M2/M3 16 GB. Default starting point.
- **26B‑A4B (MoE)** — RTX 4070 Ti Super 16 GB or RTX 4080 16 GB; **RTX 4090 24 GB** is comfortable. M2 Pro 32 GB unified or better on Mac.
- **31B Dense** — RTX 4090 / 5090 / RTX 6000 Ada (single card) or M2/M3 Max 32 GB+ unified. **Below 24 GB VRAM the 31B is not a comfortable daily driver.** With NVFP4 you can squeeze it into 24 GB with full 256 K context.

**Throughput, Q4_K_M, single‑user decode (community benchmarks):**

| GPU | E4B (tok/s) | 26B‑A4B (tok/s) | 31B Dense (tok/s) |
|---|---|---|---|
| RTX 3060 12 GB | ~35 | ~22 (Q4) | n/a |
| RTX 3080 10 GB | ~55 | ~30 | n/a |
| RTX 3090 24 GB | ~70 | 64–119 | 30–34 |
| RTX 4090 24 GB | ~90 (rises to ~124 on Blackwell) | 100+ | ~40 (Q4_K_M) |
| RTX 5090 32 GB | ~150 | 180+ | ~64 |
| RTX PRO 6000 Blackwell 96 GB | n/a | n/a | 58 (Ollama Q4_K_M) / 22 (vLLM BF16) |
| M1 / M2 16 GB | ~25 | n/a (OOM) | n/a |
| M2 / M3 Max 64 GB | ~70 | ~50 | ~15 |
| M5 Max 128 GB | ~120 | ~75 | ~15 (llama.cpp; MLX not yet supported in Ollama) |
| CPU x86 dual‑Xeon DDR4 256 GB | ~7 | ~12 | ~8 |

(Numbers vary widely by build, sampler, context length, and Flash Attention status — always re‑benchmark on your stack.)

### 5. Practical Usage Patterns

**Chat template (verbatim turn structure).**
```
<bos><|turn>system
You are a helpful assistant.<turn|>
<|turn>user
Hello!<turn|>
<|turn>model
Hi there!<turn|>
```
With thinking enabled, the model emits `<|channel>thought\n...\n<channel|>` before the visible answer. **Disabling thinking:** add `<|think|>` (off semantic) as the first content of the system role, OR pass `enable_thinking=False` to `apply_chat_template`. For multi‑turn conversations, **never feed thought blocks back into history** — strip them and keep only the final answer.

**System prompt:** fully native — use the `system` role exactly like OpenAI's API.

**Tool / function calling.** Define tools using JSON‑schema; the chat template renders them and the model emits `<|tool_call>call:my_func{"arg":"value"}<tool_call|>`. vLLM, Ollama and llama.cpp (with `--jinja`) all parse this into OpenAI‑compatible `tool_calls` objects when the matching parser is enabled (`--tool-call-parser gemma4` on vLLM).

**Structured JSON output.** Both prompt‑based JSON (Pydantic schema in the system prompt + low temperature, 0.1–0.3) and **vLLM guided decoding** (`response_format={"type":"json_schema",...}`) work; Gemma 4 has strong native JSON adherence due to function‑calling training.

**Vision / image input.** Place **image (and audio) tokens before text** in the prompt for best results. Configure resolution with the visual token budget — use 70–140 for video frames and classification, 560–1120 for OCR and document parsing. Image dimensions must be divisible by 48 (patch size 16 × pooling kernel 3); aspect ratio is preserved.

- **Ollama:** `curl http://localhost:11434/api/generate -d '{"model":"gemma4","prompt":"caption","images":["<base64>"]}'`
- **Transformers/vLLM:** pass `{"type":"image","image":"<url-or-path>"}` parts.
- **MLX‑VLM:** `mlx_vlm.generate --model mlx-community/gemma-4-E4B-it-4bit --image cat.jpg --prompt "describe"`.

**Audio (E2B/E4B only).** Add `{"type":"audio","audio":"<url-or-path.wav>"}` parts; vLLM requires `vllm[audio]` extras; Transformers requires `librosa`.

**Video.** Pass `{"type":"video","video":"sample.mp4"}` to the `any-to-any` Transformers pipeline; use `load_audio_from_video=True` on E2B/E4B to also include the audio track. Max 60 s at 1 fps.

**RAG.** Use the 256 K context of 26B‑A4B / 31B for "long‑context RAG" with chunk sizes of 2–8 K; for embeddings pair with **EmbeddingGemma** (300 M, separate model) — Gemma 4 itself is decoder‑only and does not produce native pooled embeddings.

**Fine‑tuning.** QLoRA via **Unsloth** is the fastest path (2–5× speedup, 50–80 % memory savings):
```python
from unsloth import FastLanguageModel
model, tok = FastLanguageModel.from_pretrained(
    "google/gemma-4-E4B-it", load_in_4bit=True, max_seq_length=4096)
model = FastLanguageModel.get_peft_model(model, r=16,
    target_modules=["q_proj","k_proj","v_proj","o_proj",
                    "gate_proj","up_proj","down_proj"],
    lora_alpha=16, use_gradient_checkpointing="unsloth")
```
QLoRA on E2B/E4B fits in 24 GB VRAM at batch 1 / seq 4 K (verified on RTX 4090). For the 31B, ≈16 GB VRAM is enough for QLoRA but ≈80 GB is required for full fine‑tune. KerasHub `gemma_lm.backbone.enable_lora(rank=16)` is the Google‑native path. Always freeze the vision/audio towers if you only need a text adapter.

### 6. Docker Containerization

**Ollama + Open WebUI Compose:**
```yaml
services:
  ollama:
    image: ollama/ollama:latest
    ports: ["11434:11434"]
    volumes: [ollama:/root/.ollama]
    deploy: {resources: {reservations: {devices: [{driver: nvidia, count: all, capabilities: [gpu]}]}}}
    healthcheck:
      test: ["CMD","curl","-f","http://localhost:11434/api/tags"]
      interval: 30s
  webui:
    image: ghcr.io/open-webui/open-webui:main
    ports: ["3000:8080"]
    environment: [OLLAMA_BASE_URL=http://ollama:11434]
    depends_on: [ollama]
volumes: {ollama:}
```
Then `docker compose exec ollama ollama pull gemma4:e4b`.

**vLLM + Gemma 4 26B-A4B Compose:**
```yaml
services:
  vllm:
    image: vllm/vllm-openai:latest
    ports: ["8000:8000"]
    ipc: host
    shm_size: "16g"
    environment:
      - HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}
    volumes:
      - hfcache:/root/.cache/huggingface
    command: >
      --model google/gemma-4-26B-A4B-it
      --tensor-parallel-size 1 --max-model-len 16384
      --gpu-memory-utilization 0.90
      --enable-auto-tool-choice
      --tool-call-parser gemma4 --reasoning-parser gemma4
      --chat-template /workspace/tool_chat_template_gemma4.jinja
      --host 0.0.0.0 --port 8000
    deploy: {resources: {reservations: {devices: [{driver: nvidia, count: all, capabilities: [gpu]}]}}}
    healthcheck:
      test: ["CMD","curl","-f","http://localhost:8000/health"]
      interval: 30s
      start_period: 10m         # weight download + load
volumes: {hfcache:}
```
**NVIDIA Container Toolkit** is required on the host:
```bash
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=docker
sudo systemctl restart docker
docker run --rm --gpus all nvidia/cuda:12.4.0-base nvidia-smi
```
**Baking weights into images** for fast cold start:
```dockerfile
FROM vllm/vllm-openai:latest
ARG HF_TOKEN
RUN huggingface-cli login --token $HF_TOKEN && \
    huggingface-cli download google/gemma-4-26B-A4B-it \
      --local-dir /models/gemma-4-26B-A4B-it
ENV HF_HOME=/models
ENTRYPOINT ["vllm","serve","/models/gemma-4-26B-A4B-it", ...]
```
This trades a large image (~50–60 GB) for sub‑minute boot vs ~10 min HF download cold start. For Red Hat AI Inference Server, use the pre‑baked `registry.redhat.io/rhaii-preview/vllm-cuda-rhel9:gemma4` image.

### 7. Comparison with Other Models

**vs Gemma 3:** Gemma 4 introduces MoE, full multimodality across all sizes, native function calling, native thinking, native `system` role, p‑RoPE, K=V global layers, MTP drafters, audio on E2B/E4B, and Apache 2.0. On benchmarks: 31B 85.2 % MMLU‑Pro vs 27B Gemma 3's lower MMLU; AIME 2026 89.2 % vs Gemma 3 27B 20.8 %; Codeforces ELO 2150 vs 110; τ2‑bench Retail jumped from 6.6 % on Gemma 3 27B to **86.4 % on Gemma 4 31B** — an order-of-magnitude jump in agentic tool use that is the single largest generational gain in the open model space to date.

**vs Qwen 3.5 27B / 397B‑A17B:** Per the Qwen3.5-27B official HuggingFace model card (corroborated by apxml.com), Qwen3.5-27B *"achieves MMLU‑Pro (86.1 %), GPQA Diamond (85.5 %), SWE‑bench Verified (72.4 %),"* narrowly beating Gemma 4 31B on MMLU‑Pro (85.2 %) and GPQA Diamond (84.3 %) and topping it on SWE‑bench coding. Gemma 4 wins on AIME 2026 (89.2 % vs ~87 %) and Codeforces ELO 2150. Both Apache 2.0. Qwen 3.5 has a larger flagship; Gemma 4 has a real edge family (E2B/E4B) and native audio.

**vs Llama 4 Scout (109 B total / 17 B active):** Gemma 4 31B beats Scout on GPQA Diamond (84.3 vs 74.3) and most reasoning benchmarks at one‑third the active params. Llama 4 has a 10 M context window (vs 256 K) but lives under a 700 M MAU community license. For commercial deployment at scale, **Gemma 4's Apache 2.0 is strictly more permissive**.

**vs Phi‑4 14B:** Per the controlled study in arXiv:2604.07035 (April 2026) covering 8,400 evaluations across ARC‑Challenge, GSM8K, Math L1–3, and TruthfulQA MC1, *"Gemma-4-E4B remains close to the top across prompting settings while using substantially lower latency and memory, making it a strong practical operating point,"* topping the seven‑model field with a weighted accuracy of **0.675** at a mean **14.9 GB VRAM** — beating Phi‑4‑reasoning (14B) outright at roughly a third the active parameters.

**License implications.** Apache 2.0 means no MAU cap, no acceptable‑use policy required to redistribute, no Google attribution beyond standard NOTICE preservation. Google's general Prohibited Use Policy still applies as a separate policy (CSAM, hate, dangerous content, etc.), but it is not a license restriction the way Llama's community license is — you cannot be terminated for crossing a usage threshold.

---

## Recommendations

**Stage 1 — Pick the right model in one minute.**
- "I just want it to run on my laptop / Pi." → `ollama pull gemma4:e2b` or `gemma4:e4b` (Q4_K_M).
- "I have an RTX 4090 / Mac M2 Max 64 GB and want the best local quality." → **`gemma4:26b` (MoE, Q4_K_M)** is the sweet spot — near‑31B quality at 4B active speed.
- "I have ≥24 GB VRAM and want maximum quality." → `gemma4:31b` Q4_K_M, or `nvidia/Gemma-4-31B-IT-NVFP4` if you want full 256 K context on 24 GB.
- "I'm shipping to phones/embedded." → LiteRT‑LM + E2B or E4B; on Android use AICore (`Gemini Nano 4`).

**Stage 2 — Pick the runtime.**
- Personal / dev use: **Ollama** (simplest) + Open WebUI; Mac users may switch to **MLX/mlx-vlm** for ~15–30 % more throughput per willitrunai.com's benchmarks.
- Single‑GPU production serving: **vLLM** with `--tool-call-parser gemma4 --reasoning-parser gemma4`.
- Multi‑user / cost‑optimized: **vLLM + NVFP4** on Blackwell, or RedHat AI Inference Server; or NVIDIA NIM if TRT‑LLM is required.
- Fine‑tuning on consumer GPUs: **Unsloth + QLoRA** (E2B/E4B in 24 GB, 31B in 16 GB) — switch to KerasHub on TPU.

**Stage 3 — Tune for quality.**
- Always set `temperature=1.0, top_p=0.95, top_k=64` (Google's recommended sampling).
- Bump Ollama context (`num_ctx 32768` minimum for tool‑using agents — Codex CLI's system prompt alone needs ~27 K).
- Enable MTP drafters for the ~3× decode throughput Google's own May 5 2026 blog quantifies, especially when latency matters (`--speculative-config` in vLLM, `--enable-speculative-decoding=true` in LiteRT‑LM).
- Quantize the KV cache (`-ctk q8_0 -ctv q8_0` in llama.cpp, `--kv-cache-dtype fp8` in vLLM) at long context.

**Thresholds that should change your plan:**
- If `nvidia-smi` shows >95 % VRAM utilisation at idle prompts → drop one quant level (Q4_K_M → IQ3_M) or move to E4B.
- If decode <5 tok/s and you have GPU → check Ollama is actually offloading (`ollama ps`), confirm the model is "100% GPU".
- If tool calls arrive in `reasoning` field instead of `tool_calls` → update Ollama to ≥ v0.20.5 or switch to llama.cpp with `--jinja`.
- If using Apple Silicon + 31B Dense and you see prompt eval hangs → disable `OLLAMA_FLASH_ATTENTION` until PR #15244 is merged.

---

## Caveats

- **Release era.** Gemma 4 was released April 2, 2026 and is the current generation as of May 2026. Benchmark numbers and ecosystem versions are moving fast; pin `transformers`, `vllm`, `llama.cpp` build IDs and Ollama versions when running benchmarks.
- **ONNX Runtime GenAI is not yet supported** for Gemma 4 (PLE, dual head dims, KV sharing). If your stack depends on onnxruntime‑genai, stay on Gemma 3 for now (issue microsoft/onnxruntime-genai#2062).
- **Audio is only on E2B/E4B**; **video is only on 26B/31B**. No single Gemma 4 model handles text + image + video + audio in one pass.
- **Chat template is in a separate `chat_template.jinja` file** — many third‑party tools fail to copy it. If `tokenizer.chat_template is None`, load it manually.
- **CUDA 13.2 runtime produces broken outputs on Gemma 4 GGUFs** per Unsloth — use CUDA 12.x or 13.0/13.1.
- **Some marketing‑heavy third‑party sources** (gemma4.wiki, gemma4-ai.com, gemma4guide.com, gemma4all.com, aimadetools.com) make benchmark claims (e.g. "97 % of 31B quality on 26B", "182 tok/s on RTX 5090") that are not independently verified — treat exact tok/s figures as directional. The hard numbers in this guide are from Google's model card, Google blog (May 5 2026), the vLLM Recipes, the arXiv:2604.07035 controlled study, the official Qwen3.5 model card, willitrunai.com Apple Silicon benchmarks, llama.cpp KL‑divergence benchmarks, and named GitHub issues.
- **Ollama `gemma4:31b-cloud` is not a local artifact** — it points at Ollama's cloud service. Don't include it in local VRAM plans.
- **Arena AI rankings, AIME 2026 89.2 % and MMLU‑Pro 85.2 %** are Google's published numbers from the model card; expect slight variance against independent reruns.