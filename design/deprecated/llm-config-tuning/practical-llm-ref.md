# A Practitioner's Reference Guide to Choosing and Packaging AI/ML/DL/RL Models — The Hugging Face Format, 2025–2026 Edition

## TL;DR
- **Treat `config.json` + `model.safetensors` (+ `tokenizer.json` and a `README.md` YAML front matter) as the canonical Hugging Face contract**: the `model_type` field maps to a Transformers AutoModel class, the `architectures` field maps to a vLLM/TGI runtime class, and `pipeline_tag` plus `library_name` drive Hub discoverability. Get those four fields right, and any modern serving stack will load your model.
- **For 2025–2026 deployments, default to safetensors at BF16 for training/fine-tuning, GGUF Q4_K_M for laptop/CPU inference via llama.cpp/Ollama, AWQ-INT4 or FP8 for high-throughput GPU serving via vLLM, and ONNX/TensorRT only when you need cross-framework portability.** Q4_K_M is the empirically validated sweet spot: Will-It-Run AI's 2026 guide reports "Q4_K_M saves 72% VRAM with minor quality loss," and PromptQuorum measures the quality loss at "1–3% on MMLU benchmarks compared to FP16 — imperceptible in most practical tasks."
- **Re-packaging is a layered problem: Modelfile (logical config) → GGUF/safetensors (weights) → container image (runtime) → orchestration.** Use Ollama Modelfiles for local/edge, vLLM `vllm/vllm-openai` for GPU servers, and NVIDIA NIM (`nvcr.io/nim/...`) when you need vendor-supported containers. Always bake model weights into the image for production cold-start.

---

## Key Findings

1. **The Hugging Face format is a folder convention, not a file format.** Every modern repo has the same skeleton: `config.json`, one or more `model-*.safetensors` shards plus a `model.safetensors.index.json` weight map, `tokenizer.json` + `tokenizer_config.json` + `special_tokens_map.json` for text models, and `README.md` with YAML front matter.
2. **The single most important field in `config.json` is `model_type`.** Transformers, vLLM, TGI, llama.cpp, and Ollama all dispatch architecture classes off this string. If `model_type` is missing, loaders fall back to `auto_map` and require `--trust_remote_code` — a security boundary.
3. **Safetensors has replaced `pytorch_model.bin` as the default.** Per ngxson's HF blog: "New models released on Hugging Face are all stored in safetensors format, including Llama, Gemma, Phi, Stable-Diffusion, Flux, and many others."
4. **GGUF is a single-file container, not a quantization algorithm.** The Q-suffixes (Q2_K…Q8_0) name the per-block quantization scheme inside.
5. **For LLMs ≥7B, the meaningful precision tiers are FP16/BF16, FP8, INT8/Q8_0, INT4 (Q4_K_M, AWQ, GPTQ-4), and sub-4-bit.** Below 4 bits, quality degrades noticeably for models <30B.
6. **TGI is now in maintenance mode.** Its GitHub README states: "text-generation-inference is now in maintenance mode. Going forward, we will accept pull requests for minor bug fixes, documentation improvements and lightweight maintenance tasks… we recommend [going forward]: vllm, SGLang, as well as local engines with inter-compatibility such as llama.cpp or MLX."
7. **NVIDIA NIM 2.0 adopts a "one container, one backend" model.** Each NIM is a Docker container wrapping a single inference engine (vLLM, TensorRT-LLM, or SGLang).
8. **Ollama Modelfiles are Dockerfile-like blueprints with eight instructions: FROM, PARAMETER, TEMPLATE, SYSTEM, ADAPTER, LICENSE, MESSAGE, REQUIRES.** Only FROM is required.

---

## Details

### 1. The Hugging Face Specification of Models

#### 1.1 `config.json`

The Transformers docs describe the base class: "Common attributes present in all config classes are: `hidden_size`, `num_attention_heads`, and `num_hidden_layers`. Text models further implement: `vocab_size`."

Canonical fields for any modern decoder LLM:

| Field | Purpose | Example |
|---|---|---|
| `model_type` | Short string for AutoConfig/AutoModel registries | `"llama"` |
| `architectures` | List of class names (used by vLLM/TGI) | `["LlamaForCausalLM"]` |
| `hidden_size` | Residual stream width | `4096` |
| `num_hidden_layers` | Transformer depth | `32` |
| `num_attention_heads` | Q heads | `32` |
| `num_key_value_heads` | KV heads (GQA) | `8` |
| `intermediate_size` | FFN inner dim | `14336` |
| `vocab_size` | Tokenizer vocabulary | `128256` |
| `max_position_embeddings` | Trained sequence length | `131072` |
| `rope_theta` / `rope_scaling` | RoPE base & scaling (YaRN, dynamic, linear) | `500000.0` |
| `hidden_act` | FFN activation | `"silu"` |
| `rms_norm_eps` | RMSNorm epsilon | `1e-5` |
| `tie_word_embeddings` | Share output head with embeddings | `false` |
| `torch_dtype` | Saved-weights dtype | `"bfloat16"` |
| `bos/eos/pad_token_id` | Required for generation | `128000` / `128009` |
| `is_encoder_decoder` | Architecture flag | `false` |
| `auto_map` | Custom Python modules; needs `trust_remote_code` | `{"AutoModelForCausalLM": "modeling_deepseek.Deepseek..."}` |
| `quantization_config` | Embedded quantization metadata (§3) | `{"quant_method": "awq", "bits": 4, ...}` |

For MoE: `num_experts`, `num_experts_per_tok`, `moe_intermediate_size`. For VLMs: `image_size`, `patch_size`, `vision_config`, `text_config`.

**The `model_type` → AutoModel mapping.** When you call `AutoModelForCausalLM.from_pretrained(repo)`, Transformers reads `config.model_type` and instantiates the matching class. Per vLLM's docs: "vLLM uses the architectures field in the config object to determine the model class to initialize, as it maintains the mapping from architecture name to model class in its registry."

#### 1.2 Tokenizer files

- **`tokenizer.json`** — Self-contained `tokenizers` (Rust) serialization with sections `model` (BPE/WordPiece/Unigram), `normalizer`, `pre_tokenizer`, `post_processor`, `decoder`, `added_tokens`, plus optional `truncation` and `padding`. Per HF docs: "The Normalizer: in charge of normalizing the text… The PreTokenizer: in charge of creating initial words splits in the text… The Model: in charge of doing the actual tokenization… The PostProcessor: in charge of post-processing the Encoding to add anything relevant that… a language model would need, such as special tokens."
- **`tokenizer_config.json`** — Python-side config: `tokenizer_class`, `model_max_length`, `padding_side` (left for generation, right for classification), `clean_up_tokenization_spaces`, `add_bos_token`, `add_eos_token`, special-token entries, and crucially **`chat_template`** (Jinja2 string for OpenAI-style messages). Real Llama-3.1 example:
  ```json
  {"bos_token":"<|begin_of_text|>","eos_token":"<|end_of_text|>",
   "clean_up_tokenization_spaces":true,
   "model_input_names":["input_ids","attention_mask"],
   "model_max_length":131072,"tokenizer_class":"PreTrainedTokenizerFast"}
  ```
- **`special_tokens_map.json`** — Per the Transformers source, the recognized attributes are `["bos_token","eos_token","unk_token","sep_token","pad_token","cls_token","mask_token","additional_special_tokens"]`. Values can be bare strings or `AddedToken` objects with `content/lstrip/normalized/rstrip/single_word`.

Legacy SentencePiece models (Llama 1/2, Mistral v0.1, Gemma 1) also ship a `tokenizer.model` binary.

#### 1.3 Model card YAML front matter (`README.md`)

```yaml
---
library_name: transformers           # required for transformers repos created after Aug 2024
pipeline_tag: text-generation
language: [en, fr]
license: apache-2.0                  # or "other" with license_name/license_link
base_model: meta-llama/Llama-3.1-8B
base_model_relation: finetune        # finetune | quantized | adapter | merge
datasets: [HuggingFaceH4/ultrachat_200k]
tags: [llama-3, instruct, function-calling]
model-index:
  - name: my-model
    results:
      - task: { type: text-generation }
        dataset: { name: MMLU, type: cais/mmlu }
        metrics: [{ type: accuracy, value: 0.72 }]
---
```

Per the HF Hub docs: "For model repos created after August 2024, this is not the case anymore, so you need to set `library_name: transformers` explicitly."

Standard `pipeline_tag` values: `text-generation`, `text-classification`, `token-classification`, `question-answering`, `fill-mask`, `summarization`, `translation`, `text2text-generation`, `feature-extraction` (embeddings), `sentence-similarity`, `image-classification`, `object-detection`, `image-segmentation`, `image-to-text`, `image-text-to-text` (VLMs), `text-to-image`, `text-to-video`, `automatic-speech-recognition`, `text-to-speech`, `audio-classification`, `zero-shot-classification`, `reinforcement-learning`, `robotics`, `any-to-any`.

#### 1.4 Safetensors, sharding, and the index file

Per the current Transformers docs (v5/main): "save_pretrained() automatically shards checkpoints larger than 50GB. This keeps shard counts low for large models and simplifies file management." (The older 5GB/10GB defaults are from the legacy v4.18.0–v4.47.x `big_models` documentation; the `max_shard_size` default has since been raised to 50GB.)

`model.safetensors.index.json` has exactly two top-level keys:
```json
{
  "metadata": { "total_size": 28966928384 },
  "weight_map": {
    "model.embed_tokens.weight": "model-00001-of-00006.safetensors",
    "lm_head.weight": "model-00006-of-00006.safetensors"
  }
}
```
Diffusers uses the analogous `diffusion_pytorch_model.safetensors.index.json`.

#### 1.5 File-format trade-offs

| Format | Best for | Pros | Cons |
|---|---|---|---|
| **safetensors** | Training, fine-tuning, public sharing | Safe (no code execution), zero-copy mmap, fast | Weights only; framework-specific |
| **pytorch_model.bin** (pickle) | Legacy / internal | Stores Python objects | Arbitrary code execution risk; deprecated |
| **GGUF** | Local LLM inference (llama.cpp, Ollama, LM Studio) | Single file w/ tokenizer+metadata; built-in quant; fast mmap | Mostly LLMs; harder to fine-tune from |
| **ONNX** | Cross-framework, edge accelerators | Computation graph included; vendor-neutral | Larger files; op-coverage gaps |
| **TensorRT engine (.plan)** | Max NVIDIA throughput | Compiled, fused; FP8 | GPU-arch locked; rebuild per generation |

#### 1.6 Repo & branch conventions

- `main` is canonical; tags & commit SHAs are first-class revisions.
- PR previews via `revision="refs/pr/<n>"`.
- Variants in branches or sibling files (`model.fp16.safetensors`).
- Quantized derivatives go in **separate repos** linking back via `base_model:`. The HF release checklist: "Use separate repositories for different model weights… Prefer safetensors over pickle for weight serialization."

---

### 2. Comprehensive Model Property Table (2025–2026)

#### LLMs (text-generation)

| Family | `model_type` | Sizes | License | Tool calling | GGUF on HF |
|---|---|---|---|---|---|
| GPT-2 | `gpt2` | 124M / 355M / 774M / 1.5B | MIT | No | Yes |
| GPT-J | `gptj` | 6B | Apache-2.0 | No | Yes |
| GPT-NeoX / Pythia | `gpt_neox` | 1.4B – 20B | Apache-2.0 | No | Yes |
| BLOOM | `bloom` | 560M – 176B | RAIL | No | Yes |
| LLaMA 1 | `llama` | 7B / 13B / 33B / 65B | Non-commercial research | No | Yes |
| Llama 2 | `llama` | 7B / 13B / 70B | Llama-2 Community | No (community) | Yes |
| Llama 3 | `llama` | 8B / 70B | Llama-3 Community | Yes | Yes |
| Llama 3.1 | `llama` | 8B / 70B / 405B | Llama-3.1 Community | Yes | Yes |
| Llama 3.2 | `llama` (text), `mllama` (vision) | 1B / 3B / 11B-V / 90B-V | Llama-3.2 Community | Yes | Yes |
| Llama 3.3 | `llama` | 70B | Llama-3.3 Community | Yes | Yes |
| Llama 4 (Scout / Maverick) | `llama4` | 17B-active/16E, 17B-active/128E | Llama-4 Community | Yes | Limited |
| Mistral 7B | `mistral` | 7B | Apache-2.0 | Yes | Yes |
| Mixtral 8x7B / 8x22B | `mixtral` | 46.7B/12.9B active; 141B/39B active | Apache-2.0 | Yes | Yes |
| Mistral Nemo | `mistral` | 12B | Apache-2.0 | Yes | Yes |
| Falcon | `falcon` | 7B / 40B / 180B | Apache-2.0 / TII custom | Limited | Yes |
| Phi-2 | `phi` | 2.7B | MIT | No | Yes |
| Phi-3 (mini/small/medium) | `phi3` | 3.8B / 7B / 14B | MIT | Limited | Yes |
| Phi-4 | `phi3` | 14B | MIT | Yes | Yes |
| Phi-4-reasoning / reasoning-plus | `phi3` | 14B | MIT | Yes | Yes |
| Qwen 1 | `qwen` | 1.8B – 72B | Tongyi Qianwen | Yes | Yes |
| Qwen2 | `qwen2` | 0.5B – 72B | Apache-2.0 | Yes | Yes |
| Qwen2.5 | `qwen2` | 0.5B – 72B | Apache-2.0 (3B is community) | Yes | Yes |
| Qwen3 | `qwen3`, `qwen3_moe` | 0.6B – 32B dense; 30B-A3B / 235B-A22B MoE | Apache-2.0 | Yes | Yes |
| DeepSeek-V2 | `deepseek_v2` | 236B / 21B active | DeepSeek License | Yes | Yes |
| DeepSeek-V3 | `deepseek_v3` | 671B / 37B active | MIT + commercial-use Model License | Yes | Yes |
| DeepSeek-R1 (+ distills) | `deepseek_v3` / `llama` / `qwen2` | 671B + 1.5B/7B/8B/14B/32B/70B | MIT | Yes | Yes |
| Gemma 1 | `gemma` | 2B / 7B | Gemma License | No | Yes |
| Gemma 2 | `gemma2` | 2B / 9B / 27B | Gemma License | Limited | Yes |
| Gemma 3 | `gemma3` | 1B / 4B / 12B / 27B | Gemma License | Workarounds only | Yes |
| Gemma 3n | `gemma3n` | E2B / E4B | Gemma License | Limited | Yes |
| Command R / R+ | `cohere` | 35B / 104B | CC-BY-NC-4.0 | Yes | Yes |
| Yi 1 / 1.5 | `llama` | 6B / 9B / 34B | Apache-2.0 (1.5) | Limited | Yes |
| InternLM 2 / 2.5 | `internlm2` | 1.8B / 7B / 20B | Apache-2.0 | Yes | Yes |
| Baichuan 1 / 2 | `baichuan` | 7B / 13B | Baichuan License | Limited | Yes |
| StarCoder / StarCoder2 | `gpt_bigcode`, `starcoder2` | 3B / 7B / 15B | BigCode OpenRAIL-M | No | Yes |
| CodeLlama | `llama` | 7B / 13B / 34B / 70B | Llama-2 Community | Limited | Yes |

#### Vision encoders & VLMs

| Family | `model_type` | Sizes | License | Pipeline tag |
|---|---|---|---|---|
| ViT | `vit` | base/large/huge | Apache-2.0 | `image-classification` |
| DINOv2 | `dinov2` | small/base/large/giant | Apache-2.0 | feature extraction |
| CLIP | `clip` | B/32, B/16, L/14 | MIT | `zero-shot-image-classification` |
| SigLIP / SigLIP 2 | `siglip`, `siglip2` | base/large/so400m | Apache-2.0 | `zero-shot-image-classification` |
| Florence-2 | `florence2` | 0.23B / 0.77B | MIT | `image-text-to-text` |
| PaliGemma / PaliGemma 2 | `paligemma`, `paligemma2` | 3B / 10B / 28B | Gemma | `image-text-to-text` |
| LLaVA / LLaVA-Next / OneVision | `llava`, `llava_next`, `llava_onevision` | 7B / 13B / 34B | Apache-2.0 | `image-text-to-text` |
| InternVL 2 / 2.5 / 3 | `internvl_chat` | 1B – 78B | MIT / custom | `image-text-to-text` |
| Qwen-VL / Qwen2-VL / 2.5-VL / 3-VL | `qwen2_vl`, `qwen2_5_vl`, `qwen3_vl` | 2B / 7B / 32B / 72B | Apache-2.0 (most) | `image-text-to-text` |
| Idefics2 / Idefics3 | `idefics2`, `idefics3` | 8B | Apache-2.0 | `image-text-to-text` |
| Fuyu | `fuyu` | 8B | CC-BY-NC-4.0 | `image-text-to-text` |
| CogVLM / CogVLM2 | trust_remote_code | 17B / 19B | custom | `image-text-to-text` |
| BLIP-2 | `blip-2` | 2.7B / 6.7B base | MIT | `image-text-to-text` |
| SmolVLM | `idefics3` variant | 256M / 500M / 2.2B | Apache-2.0 | `image-text-to-text` |

#### Audio / Speech

| Family | `model_type` | Sizes | License | Pipeline tag |
|---|---|---|---|---|
| Whisper | `whisper` | tiny – large-v3 / v3-turbo | MIT | `automatic-speech-recognition` |
| Wav2Vec2 | `wav2vec2` | base / large / xls-r (300M/1B/2B) | Apache-2.0 / MIT | `automatic-speech-recognition` |
| HuBERT | `hubert` | base / large / x-large | Apache-2.0 | `automatic-speech-recognition` |
| WavLM | `wavlm` | base / large | MIT | feature extraction |
| Bark | `bark` | ~1B | MIT | `text-to-speech` |
| MusicGen | `musicgen`, `musicgen_melody` | small/medium/large | CC-BY-NC-4.0 | `text-to-audio` |
| EnCodec | `encodec` | 24 / 32 / 48 kHz | MIT | audio compression |
| SeamlessM4T | `seamless_m4t` | medium / large | CC-BY-NC-4.0 | `translation` |
| Parler-TTS | custom | mini / large | Apache-2.0 | `text-to-speech` |

#### NLP classics (encoder & seq2seq)

| Family | `model_type` | Sizes | License | Tasks |
|---|---|---|---|---|
| BERT | `bert` | base / large | Apache-2.0 | classification, NER, QA |
| RoBERTa | `roberta` | base / large | MIT | classification, NER |
| DeBERTa-v3 | `deberta-v2` | base / large / xlarge | MIT | GLUE/SuperGLUE SOTA |
| ALBERT | `albert` | base – xxlarge | Apache-2.0 | parameter-efficient BERT |
| XLNet | `xlnet` | base / large | MIT | classification (legacy) |
| T5 / FLAN-T5 | `t5` | small – xxl | Apache-2.0 | text2text |
| mT5 | `mt5` | small – xxl | Apache-2.0 | multilingual text2text |
| BART | `bart` | base / large | Apache-2.0 | summarization |
| mBART / mBART-50 | `mbart` | large | MIT | translation |
| Pegasus | `pegasus` | base / large | Apache-2.0 | summarization |
| LED | `led` | base / large-16384 | Apache-2.0 | long-doc summarization |

#### Image generation (diffusers)

| Family | Pipeline class | Sizes | License | Tag |
|---|---|---|---|---|
| SD 1.5 | `StableDiffusionPipeline` | ~0.98B UNet | CreativeML OpenRAIL-M | `text-to-image` |
| SD 2.1 | `StableDiffusionPipeline` | ~0.86B | OpenRAIL++-M | `text-to-image` |
| SDXL | `StableDiffusionXLPipeline` | 2.6B + refiner 6.6B | OpenRAIL++-M | `text-to-image` |
| SDXL Turbo | same | — | SAI Non-Commercial Research | `text-to-image` |
| SD 3 / 3.5 | `StableDiffusion3Pipeline` | 2B / 8B MMDiT | Stability Community | `text-to-image` |
| FLUX.1 (dev/schnell/pro) | `FluxPipeline` | 12B DiT | Apache-2.0 (schnell); non-commercial (dev) | `text-to-image` |
| Kandinsky 2.1 / 2.2 / 3 | `Kandinsky*Pipeline` | ~1B–3B | Apache-2.0 | `text-to-image` |
| PixArt-α / Σ | `PixArtAlphaPipeline` | 0.6B DiT | Open RAIL++ | `text-to-image` |

#### Reinforcement learning

| Format | `library_name` | Notes |
|---|---|---|
| Decision Transformer | `decision_transformer` (in Transformers) | Offline-RL sequence model |
| Stable-Baselines3 | `stable-baselines3` | zip files; `pipeline_tag: reinforcement-learning` |
| CleanRL | `cleanrl` | Single-file PPO/DQN/SAC; PyTorch state_dicts |
| sample-factory | `sample-factory` | Async PPO |
| ML-Agents (Unity) | `ml-agents` | `.onnx` policies |

#### Embedding models (sentence-transformers)

| Family | Dim | Params | License |
|---|---|---|---|
| all-MiniLM-L6-v2 | 384 | 22M | Apache-2.0 |
| all-mpnet-base-v2 | 768 | 110M | Apache-2.0 |
| BGE base/large-en-v1.5 / M3 | 768/1024/1024 | 109M/335M/568M | MIT |
| E5 small/base/large-v2; e5-mistral-7b-instruct | 384–4096 | 33M–7B | MIT |
| GTE base/large-en-v1.5 / gte-Qwen2 | 768/1024/1536+ | 109M–7B | Apache-2.0 |
| Nomic v1 / v1.5 / v2 (MoE) | 768 | 137M / MoE | Apache-2.0 |
| Jina v2 / v3 | 768/1024 | 137M/570M | Apache-2.0 / CC-BY-NC |
| Qwen3-Embedding | 1024/2560/4096 | 0.6B/4B/8B | Apache-2.0 |

---

### 3. Quantized vs Non-Quantized Models

#### 3.1 The precision ladder

| Precision | Bytes/param | Quality vs FP32 | Use case |
|---|---|---|---|
| FP32 | 4 | Reference | Scientific only |
| FP16 | 2 | ≈ FP32 inference | Legacy inference |
| BF16 | 2 | ≈ FP32, wider exponent | **Default training dtype on A100/H100** |
| FP8 (E4M3/E5M2) | 1 | 99% retention | H100/H200 inference |
| INT8 / Q8_0 | ~1 | 99%+ | "Near lossless" |
| INT4 / Q4_K_M / AWQ-4 / GPTQ-4 | ~0.5–0.6 | 92–96% | **Default local inference** |
| Q3_K_M | ~0.4 | 80–90% | When Q4 won't fit |
| Q2_K / AQLM / QuIP# | ~0.3 | 70–85% | Extreme compression (>30B only) |

A January 2026 Jarvis Labs benchmarking study by Jaydev Tonde ran the comparison on **Qwen2.5-32B-Instruct on an H200 GPU** against an FP16 baseline of perplexity 6.56 (lower is better). Their headline result: "Best for Quality: BitsandBytes — Lowest perplexity increase (6.67 vs 6.56 baseline)." GGUF Q4_K_M came in at 6.74, AWQ / Marlin-AWQ at 6.84, and GPTQ / Marlin-GPTQ at 6.90–6.97 — "all methods stay within ~6% of baseline perplexity." Throughput on the same study: "Marlin-AWQ is the fastest overall at 741 tok/s output throughput, followed closely by Marlin-GPTQ at 712 tok/s. Both are faster than baseline FP16!" (FP16 at 461 tok/s).

#### 3.2 Quantization methods compared

| Method | Bits | Needs calibration | Best for | Library |
|---|---|---|---|---|
| **GPTQ** | 2/3/4/8 | Yes — Hessian + 128 samples | GPU inference, accuracy | GPTQModel (AutoGPTQ archived April 2025) |
| **AWQ** | 4 (and 3) | Yes — activation magnitudes | GPU + Marlin kernels (fastest) | AutoAWQ; in Transformers |
| **GGUF K-quants** | 2/3/4/5/6/8 | Optional (imatrix) | CPU + consumer GPU | llama.cpp |
| **bitsandbytes** | 4 (NF4/FP4), 8 (LLM.int8) | No | Fine-tuning (QLoRA), drop-in load | bitsandbytes |
| **AQLM** | 2 | Yes | Extreme compression of large models | AQLM |
| **QuIP#** | 2 | Yes | Research-grade 2-bit | QuIP# |
| **HQQ** | 1–8 | No | Data-free; fast | HQQ |
| **EXL2** | 2–8 (mixed) | Yes | ExLlamaV2 fast Ampere/Ada | exllamav2 |
| **SqueezeLLM** | 3/4 | Yes | Mixed sparse + dense | reference impl |

Practical rule (Best AI Web): "If you need to run on a laptop or CPU, choose GGUF. If you need to fine-tune, bitsandbytes is the only option. If you are unsure, AWQ at 4-bit is the safest default for serving."

#### 3.3 GGUF naming cheat-sheet

| Suffix | Bits | Family | Quality vs FP16 | 7B size | Use when |
|---|---|---|---|---|---|
| F32 | 32 | float | 100% (ref) | ~28 GB | Debug only |
| F16 | 16 | float | 100% | ~14 GB | 24GB+ VRAM |
| Q8_0 | 8 | legacy | ~99% | ~7.2 GB | 16 GB GPU; production |
| Q6_K | 6.5 | K-quant | ~98% | ~5.5 GB | 12–16 GB |
| Q5_K_M | 5.5 | K-quant medium | ~96–98% | ~4.8 GB | 12 GB GPU sweet spot |
| Q5_K_S | 5 | K-quant small | ~95–97% | ~4.5 GB | Slightly smaller |
| **Q4_K_M** | ~4.8 | K-quant medium | **~92–96%** | **~4.1 GB** | **Universal default** |
| Q4_K_S | ~4.5 | K-quant small | ~90–94% | ~3.9 GB | When Q4_K_M doesn't fit |
| Q4_0 | 4 | legacy | ~88% | ~3.8 GB | Legacy; prefer Q4_K_M |
| IQ4_XS | ~4.25 | I-quant | ~91% | ~3.7 GB | Aggressive with imatrix |
| Q3_K_L/M/S | ~3.4/3.1/2.75 | K-quant | ~80–90% | ~3.0–3.4 GB | <8 GB VRAM, 13B+ |
| Q2_K | ~2.6 | K-quant | ~70–85% | ~2.7 GB | Emergency only |

Will-It-Run AI's 2026 verdict: "Q4_K_M saves 72% VRAM with minor quality loss. Q5_K_M is the sweet spot. Q8 is near-lossless but 2x larger… When in doubt, the guiding principle is: it's better to run a smaller model well than a larger model badly. A 7B model at Q6 will often outperform a 13B model at Q2."

#### 3.4 VRAM requirements table (weights only; add 25–40% for KV cache, batching)

| Model | FP16/BF16 | Q8_0 | Q5_K_M | Q4_K_M | Q3_K_M | Q2_K |
|---|---|---|---|---|---|---|
| 1B–3B | 2–6 GB | 1–3 GB | 0.7–2 GB | 0.6–1.7 GB | 0.5–1.4 GB | 0.4–1.1 GB |
| 7B–8B | 14–16 GB | 7–8.5 GB | 4.8–5.5 GB | 4.1–4.4 GB | 3.3–3.7 GB | 2.7–3.0 GB |
| 13B–14B | 26–28 GB | 13–14 GB | 9–10 GB | 7.5–8.1 GB | 6–6.5 GB | 5 GB |
| 27B–34B | 54–68 GB | 27–34 GB | 19–22 GB | 16–18 GB | 13–15 GB | 11 GB |
| 70B–72B | 140 GB | 74 GB | 50 GB | 39–43 GB | 32 GB | 26 GB |
| 405B | 810 GB | 405 GB | 270 GB | 230 GB | 175 GB | 145 GB |
| 671B MoE (DS-V3, R1) | ~1.3 TB | ~670 GB | ~450 GB | ~370 GB | ~290 GB | ~220 GB |

Per Will-It-Run AI: "Llama 3.1 8B needs approximately 4.3GB at Q4_K_M, 5.5GB at Q6_K, or 8.5GB at Q8. Llama 3.3 70B needs 39GB at Q4_K_M or 74GB at Q8… DeepSeek R1 [full 671B] needs ~370GB at Q4, requiring multiple GPUs or a high-memory Mac."

GPU sizing rule (Local AI Master decision tree): "<8GB → Q2_K or Q3_K_M. 8–12GB → Q4_K_M. 12–16GB → Q5_K_M or Q6_K. 16–24GB → Q8_0. 24GB+ → F16."

#### 3.5 `quantization_config` in `config.json`

**AWQ** (per Transformers docs: "Identify an AWQ-quantized model by checking the `quant_method` key in the models config.json file"):
```json
"quantization_config": {
  "quant_method": "awq",
  "bits": 4,
  "group_size": 128,
  "zero_point": true,
  "version": "gemm"
}
```
AWQ `version` enum: `gemm` (batch ≥ 8), `gemv` (batch < 8), `gemv_fast`, `llm-awq`.

**GPTQ** (`GPTQConfig`):
```json
"quantization_config": {
  "quant_method": "gptq",
  "bits": 4, "group_size": 128, "desc_act": false,
  "sym": true, "damp_percent": 0.1, "true_sequential": true
}
```
Per docs: "`desc_act`: Whether to quantize columns in order of decreasing activation size. Setting it to False can significantly speed up inference but the perplexity may become slightly worse."

**bitsandbytes**:
```json
"quantization_config": {
  "quant_method": "bitsandbytes",
  "load_in_4bit": true, "load_in_8bit": false,
  "bnb_4bit_quant_type": "nf4",
  "bnb_4bit_compute_dtype": "bfloat16",
  "bnb_4bit_use_double_quant": true
}
```
NF4 places quantization levels assuming normally-distributed weights; FP4 is the standard floating-point alternative.

#### 3.6 Toolchain quick map

| Tool | Role |
|---|---|
| `llama.cpp` (`convert_hf_to_gguf.py`, `llama-quantize`) | HF safetensors → GGUF; Q2..Q8 quantize |
| `GPTQModel` (replaces AutoGPTQ, archived 2025) | GPTQ quantize + serve |
| `AutoAWQ` | AWQ calibration |
| `bitsandbytes` | On-the-fly NF4/FP4/INT8 + QLoRA |
| `optimum` | HF → ONNX, OpenVINO, IPEX |
| `ctransformers` / `llama-cpp-python` | Python GGUF bindings |
| `exllamav2` | EXL2 + fast kernels |
| `vLLM`, `SGLang`, `TGI` | Serve GPTQ/AWQ/FP8/bitsandbytes |

---

### 4. Re-Packaging Existing Models into Custom-Tuned Docker Images

#### 4.1 Ollama Modelfiles

| Instruction | Purpose |
|---|---|
| `FROM` | Base — library tag, GGUF path, or safetensors directory |
| `PARAMETER <k> <v>` | `temperature`, `top_p`, `top_k`, `repeat_penalty`, `num_ctx`, `num_gpu`, `stop` |
| `TEMPLATE """..."""` | Go-template chat formatting (`{{ .System }}`, `{{ .Prompt }}`, `{{ .Response }}`) |
| `SYSTEM """..."""` | Default system prompt |
| `ADAPTER ./lora-dir` | LoRA adapter — Ollama docs list: "Currently supported Safetensor adapters: * Llama (including Llama 2, Llama 3, and Llama 3.1) * Mistral (including Mistral 1, Mistral 2, and Mixtral) * Gemma (including Gemma 1 and Gemma 2)" |
| `MESSAGE user/assistant "..."` | Few-shot primer |
| `LICENSE """..."""` | Embed license text |
| `REQUIRES <version>` | Min Ollama version |

Example (from Ollama docs):
```Modelfile
FROM llama3.2
PARAMETER temperature 1
PARAMETER num_ctx 4096
SYSTEM You are Mario from super mario bros, acting as an assistant.
```
```bash
ollama create mario -f ./Modelfile && ollama run mario
```

GGUF workflow:
```bash
python llama.cpp/convert_hf_to_gguf.py ./my-model-hf --outfile my-model-f16.gguf
llama.cpp/llama-quantize my-model-f16.gguf my-model-Q4_K_M.gguf Q4_K_M
cat > Modelfile <<EOF
FROM ./my-model-Q4_K_M.gguf
TEMPLATE """<|im_start|>system
{{ .System }}<|im_end|>
<|im_start|>user
{{ .Prompt }}<|im_end|>
<|im_start|>assistant
"""
PARAMETER stop "<|im_end|>"
SYSTEM "You are a helpful assistant."
EOF
ollama create my-model -f Modelfile
```

#### 4.2 vLLM Docker

Official image: `vllm/vllm-openai` on Docker Hub. Canonical run (per vLLM docs):
```bash
docker run --runtime nvidia --gpus all \
  -v ~/.cache/huggingface:/root/.cache/huggingface \
  --env "HF_TOKEN=$HF_TOKEN" \
  -p 8000:8000 --ipc=host \
  vllm/vllm-openai:latest \
  --model Qwen/Qwen3-0.6B
```
- `--ipc=host` or `--shm-size 1g` — required (PyTorch tensor-parallel uses /dev/shm).
- Engine args after image tag: `--tensor-parallel-size N`, `--max-model-len`, `--quantization awq|gptq|fp8`, `--gpu-memory-utilization 0.9`, `--enable-prefix-caching`, `--api-key sk-…`.

Production cold-start — bake weights in:
```Dockerfile
FROM vllm/vllm-openai:latest
RUN python -c "from huggingface_hub import snapshot_download; \
  snapshot_download('meta-llama/Llama-3.1-8B-Instruct', local_dir='/models/llama')"
ENV HF_HOME=/models
CMD ["--model", "/models/llama", "--served-model-name", "llama3.1-8b"]
```

#### 4.3 Text Generation Inference (TGI)

`ghcr.io/huggingface/text-generation-inference`:
```bash
docker run --gpus all --shm-size 1g -p 8080:80 \
  -e HF_TOKEN=$HF_TOKEN -v $PWD/data:/data \
  ghcr.io/huggingface/text-generation-inference:3.3.5 \
  --model-id meta-llama/Meta-Llama-3.1-8B-Instruct
```
**2025 status:** TGI's README states "text-generation-inference is now in maintenance mode." Treat as legacy; default to vLLM/SGLang for new work.

Env vars: `HF_TOKEN`, `MODEL_ID`, `NUM_SHARD`, `SHARDED=true`, `MAX_INPUT_LENGTH`, `MAX_TOTAL_TOKENS`, `QUANTIZE=bitsandbytes|gptq|awq|eetq|fp8`.

#### 4.4 NVIDIA NIM containers

NIM 2.0 ships as `nvcr.io/nim/<vendor>/<model>` (LLM-specific) or `nvcr.io/nim/nvidia/llm-nim:latest` (multi-LLM). Per NIM docs, NIM 2.0 follows a "one container, one backend philosophy."

Two modes:
- **Model-specific** (e.g., `nvcr.io/nim/meta/llama-3.1-8b-instruct:1.8.0`) — curated weights.
- **Model-free** — point at HF at runtime:
```bash
docker run --runtime=nvidia --gpus all -p 8000:8000 \
  -e NIM_MODEL_NAME="hf://Qwen/Qwen2.5-0.5B" \
  -e NIM_TENSOR_PARALLEL_SIZE=1 \
  nvcr.io/nim/nvidia/llm-nim:latest
```
OpenAI-compatible API at `/v1/chat/completions`, `/v1/completions`; readiness at `/v1/health/ready`. Prereqs: NVIDIA Driver ≥ 535, NVIDIA Container Toolkit, compute capability ≥ 7.0 (8.0+ for BF16).

#### 4.5 Triton Inference Server

Use Triton (`nvcr.io/nvidia/tritonserver`) for **multi-model serving** with heterogeneous backends (TensorRT-LLM, ONNX, PyTorch, Python). Each model lives in `model_repository/<name>/<version>/` with a `config.pbtxt`. Provides dynamic batching, multi-instance GPU sharing; transformer optimizations come from the TensorRT-LLM backend.

#### 4.6 Format conversion recipes

| From | To | Command |
|---|---|---|
| HF safetensors | GGUF | `python llama.cpp/convert_hf_to_gguf.py <dir> --outfile out.gguf` then `llama-quantize out.gguf out-Q4_K_M.gguf Q4_K_M` |
| HF safetensors | ONNX | `optimum-cli export onnx --model <repo> ./onnx-out` |
| HF safetensors | TensorRT-LLM | `trtllm-build --checkpoint_dir <conv> --output_dir ./engine --gemm_plugin float16` |
| HF safetensors | OpenVINO IR | `optimum-cli export openvino --model <repo> --weight-format int4 ./ov` |
| HF safetensors | MLX (Apple) | `python -m mlx_lm.convert --hf-path <repo> --mlx-path out --quantize` |
| LoRA + base | Merged HF | `peft.PeftModel.from_pretrained(base, adapter).merge_and_unload().save_pretrained(out)` |

#### 4.7 LoRA adapter merging

```python
from peft import PeftModel
from transformers import AutoModelForCausalLM, AutoTokenizer

base = AutoModelForCausalLM.from_pretrained("meta-llama/Llama-3.1-8B", torch_dtype="bfloat16")
merged = PeftModel.from_pretrained(base, "./my-lora").merge_and_unload()
merged.save_pretrained("./merged-model", safe_serialization=True)
AutoTokenizer.from_pretrained("meta-llama/Llama-3.1-8B").save_pretrained("./merged-model")
```
QLoRA caveat: if trained over 4-bit NF4, merge against the full-precision base for best quality. For Ollama, skip merging — use `ADAPTER` directly for supported families.

#### 4.8 Multi-stage Docker builds

```Dockerfile
FROM python:3.11-slim AS builder
RUN pip install --no-cache-dir huggingface_hub
RUN python -c "from huggingface_hub import snapshot_download; \
  snapshot_download('Qwen/Qwen2.5-7B-Instruct', local_dir='/weights', \
    allow_patterns=['*.json','*.safetensors','*.txt','tokenizer*'])"

FROM vllm/vllm-openai:latest
COPY --from=builder /weights /models/qwen
ENV HF_HOME=/models
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=10s --start-period=120s --retries=3 \
  CMD curl -f http://localhost:8000/health || exit 1
CMD ["--model", "/models/qwen", "--served-model-name", "qwen2.5-7b"]
```
Use `allow_patterns` to skip `*.bin`, `*.msgpack`, `consolidated.*` and save GBs.

#### 4.9 Docker Compose pattern

```yaml
services:
  vllm:
    image: vllm/vllm-openai:latest
    runtime: nvidia
    deploy: { resources: { reservations: { devices: [{ capabilities: [gpu] }] }}}
    ipc: host
    environment:
      HF_TOKEN: ${HF_TOKEN}
      HF_HOME: /root/.cache/huggingface
    volumes: [ "hf-cache:/root/.cache/huggingface" ]
    command: ["--model","meta-llama/Llama-3.1-8B-Instruct",
              "--gpu-memory-utilization","0.90","--max-model-len","8192"]
    healthcheck:
      test: ["CMD","curl","-f","http://localhost:8000/health"]
      interval: 30s
      start_period: 180s
      retries: 5

  litellm:
    image: ghcr.io/berriai/litellm:main-latest
    ports: ["4000:4000"]
    environment: { OPENAI_API_BASE: http://vllm:8000/v1 }
    depends_on: { vllm: { condition: service_healthy } }

volumes: { hf-cache: }
```

#### 4.10 GPU passthrough essentials

- Install **NVIDIA Container Toolkit** (`nvidia-container-toolkit`).
- `--gpus all`, `--gpus '"device=0,1"'`, or `--runtime=nvidia` with `NVIDIA_VISIBLE_DEVICES`.
- Always `--ipc=host` or `--shm-size=1g` for tensor-parallel (PyTorch /dev/shm).
- Podman: `--device nvidia.com/gpu=all` with CDI.
- AMD: `--device=/dev/kfd --device=/dev/dri --group-add=video` + ROCm-tagged images (`vllm/vllm-openai-rocm`, `ghcr.io/huggingface/text-generation-inference:*-rocm`).

#### 4.11 Hugging Face cache env vars

| Variable | Effect |
|---|---|
| `HF_HOME` | Root cache (default `~/.cache/huggingface`) |
| `HF_HUB_CACHE` | Just blob cache |
| `TRANSFORMERS_CACHE` | Legacy; superseded |
| `HF_TOKEN` | Auth (replaces `HUGGING_FACE_HUB_TOKEN`) |
| `HF_HUB_OFFLINE=1` | Disable network; fail fast |
| `HF_HUB_DISABLE_TELEMETRY=1` | Disable pings |
| `HF_HUB_ENABLE_HF_TRANSFER=1` | Rust `hf_transfer` (≥2× faster downloads) |
| `HF_HUB_DOWNLOAD_TIMEOUT=60` | Bump timeouts |

#### 4.12 Health checks & readiness probes

| Server | Endpoint | Notes |
|---|---|---|
| vLLM | `GET /health` | 200 once engine is warm |
| TGI | `GET /health` | Image lacks curl; per SaladCloud docs use `python -c "import requests,sys;sys.exit(0 if requests.get('http://localhost:80/health').status_code == 200 else -1)"` |
| NIM | `GET /v1/health/live`, `GET /v1/health/ready` | Standard |
| Triton | `GET /v2/health/live`, `GET /v2/health/ready` | KFServing-compatible |
| Ollama | `GET /api/tags` | No dedicated probe |

K8s: set `initialDelaySeconds` 120–600s — a 70B model can take 5+ min to load weights, especially over network storage.

---

## Recommendations

### Stage 1 — Choosing a model
1. **Identify your `pipeline_tag`** — pick the smallest model from §2 that hits your quality bar.
2. **For text generation, match VRAM to size:**
   - **≤8 GB:** Llama 3.2 3B, Phi-3-mini, Qwen 2.5 3B, Gemma 3 4B at Q4_K_M.
   - **12 GB:** Llama 3.1 8B, Qwen 2.5 7B, Mistral 7B at Q5_K_M.
   - **16–24 GB:** Llama 3.1 8B at Q8, Qwen 2.5 14B at Q5_K_M, Gemma 2 9B at Q8.
   - **24–48 GB:** Qwen 2.5 32B, Gemma 2 27B, CodeLlama 34B at Q4_K_M.
   - **48–80 GB:** Llama 3.3 70B / Qwen 2.5 72B at Q4_K_M.
   - **Multi-H100/H200:** DeepSeek-V3/R1 671B (MoE, 37B active); Llama 3.1 405B FP8.
3. **Tool/function calling?** Default to Llama 3.1+/3.3, Qwen 2.5/3, Mistral, Phi-4, DeepSeek-V3/R1. Gemma 3 needs workarounds.
4. **License:** Apache-2.0 (Mistral, Qwen 2/2.5/3, Falcon 7B/40B, Phi-3, InternLM, Yi 1.5) is safest for commercial use. Llama and Gemma are custom community licenses; Command R is non-commercial. DeepSeek-V3/R1 weights are commercial-use-permissive under DeepSeek's Model License.

### Stage 2 — Choosing a quantization
- **Default (any GPU ≥ 8 GB):** Q4_K_M GGUF or AWQ-INT4. Per the Jarvis Labs Qwen2.5-32B/H200 benchmarks, all four 4-bit methods stay within ~6% of FP16 perplexity.
- **VRAM to spare:** Q5_K_M, Q6_K, or Q8_0.
- **H100/H200/B200:** FP8 (W8A8) via vLLM or TensorRT-LLM.
- **Fine-tuning:** bitsandbytes 4-bit NF4 with QLoRA (only method with native adapter training).
- **Apple Silicon:** GGUF Q5_K_M or MLX 4-bit.

### Stage 3 — Choosing a serving stack

| Scenario | Stack |
|---|---|
| Laptop / single-user / offline | **Ollama** + GGUF Q4_K_M |
| Single-GPU server, simplest | **vLLM Docker** + AWQ or FP8 |
| Multi-GPU tensor-parallel | **vLLM** `--tensor-parallel-size N` or **SGLang** |
| Enterprise NVIDIA-supported | **NVIDIA NIM** |
| Mixed model zoo | **Triton** with per-model backends |
| Edge / browser / mobile | **transformers.js** (WebGPU), **MLX**, **ONNX Runtime Mobile** |
| Legacy HF stack | **TGI** — in maintenance mode; migrate for new work |

### Stage 4 — Containerization checklist
1. Pin image tags (`vllm/vllm-openai:v0.7.0`, not `:latest`).
2. Bake weights into the image for cold-starts < 60 s.
3. `--ipc=host` / `--shm-size 1g` for tensor parallelism.
4. Mount `HF_HOME` as a named volume if you're swapping models.
5. Set `HF_HUB_OFFLINE=1` in production.
6. Add `HEALTHCHECK`; K8s readiness probe `initialDelaySeconds: 300` for 70B+.
7. Merge LoRAs for production; reserve runtime adapters for dev/multi-tenant.
8. Convert to GGUF only when targeting llama.cpp/Ollama; keep safetensors as source of truth.

### Benchmarks that should change your decision
- **Perplexity uplift FP16 → 4-bit > 10%** → switch from GGUF Q4 to AWQ or smaller higher-precision model.
- **P95 latency < 100 ms at batch 1 required** → vLLM continuous batching or NIM TensorRT-LLM, not Ollama/TGI.
- **GPU utilization < 50%** → raise `--gpu-memory-utilization` and `--max-num-seqs` or move to H100 FP8.
- **Cold-start > 5 min on 70B** → bake weights in; use `HF_HUB_ENABLE_HF_TRANSFER=1` at build time.

---

## Caveats

- The HF ecosystem moves fast: `model_type`, `architectures`, and `quantization_config` keys change between major Transformers releases. Pin `transformers`, `vllm`, and quantization-library versions. The `max_shard_size` default itself has changed (5GB in v4.18.x → 10GB in mid-2024 → 50GB in current main/v5) — verify against the version you actually use.
- Quantization quality numbers aggregate public benchmarks (MMLU, HumanEval, GSM8K, perplexity) on Llama-family and Qwen-family models. **Reasoning-heavy tasks (math, multi-step planning) lose more at Q3/Q4**; code generation can drop 2–4 HumanEval points at Q4 vs BF16 on 7B. Re-benchmark on your own task.
- Licenses for "open" models often carry restrictions (Llama community licenses, Gemma terms, Mistral MRL on some models, Command R CC-BY-NC). On approximately January 22, 2025, the Free Software Foundation published "Llama 3.1 Community License is not a free software license" with the verdict: "This is not a free software license and you should not use it, nor any software released under it." The Open Source Initiative concurred: "We agree with the Free Software Foundation's recent evaluation that the Llama 3.1 Community Licence agreement fails in spectacular ways at granting basic rights." Read the license text before commercial deployment.
- `pickle`/`.bin` checkpoints from untrusted sources can execute arbitrary code on load — HF explicitly recommends safetensors over pickle. Treat `trust_remote_code=True` as a security review item.
- TGI is in maintenance mode. New optimizations land in vLLM/SGLang first.
- AutoGPTQ was archived in April 2025; the successor is GPTQModel. Older tutorials referencing `auto_gptq` may not work on current PyTorch/CUDA combos.
- VRAM tables assume small context and batch 1. KV cache scales linearly with both and dominates beyond ~16K tokens; budget 25–40% headroom or quantize KV (`OLLAMA_KV_CACHE_TYPE=q8_0` or vLLM `--kv-cache-dtype fp8`).
- Brand-new architectures (Llama 4, Qwen3-VL, Gemma 3n) may lag in llama.cpp by 1–4 weeks. Check llama.cpp PRs before committing a GGUF-only deployment path.