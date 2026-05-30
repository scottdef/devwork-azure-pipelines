# Local Whisper-Large-v3 on Ubuntu 20.04: A Complete Operations Guide

OpenAI's `whisper-large-v3` runs efficiently on Ubuntu 20.04 today via two mature local runtimes — **whisper.cpp** (v1.8.4, C++/GGML, OpenAI-compatible HTTP server) and **faster-whisper** (v1.2.1, Python/CTranslate2). **Ollama does not support Whisper**; ignore tutorials that suggest otherwise. The two runtimes are complementary: whisper.cpp wins on portability, edge/CPU deployment, and dependency-free Docker images, while faster-whisper wins on GPU throughput with batched inference (~50× real-time on an RTX 3070 Ti). This guide covers building both, batching files, real-time microphone streaming, voice keyword detection, and sound-triggered automation, with executable commands tested against the May 2026 toolchain.

Everything below targets Ubuntu 20.04 LTS (now on ESM). Two host-level adjustments matter: install **CMake ≥ 3.27 from Kitware's repo** (20.04's default 3.16 is too old for CUDA builds), and install **CUDA 12 via NVIDIA's apt keyring** rather than the stale `cuda` package in `focal`. faster-whisper additionally avoids both by shipping cuBLAS and cuDNN as pip wheels.

## Installing whisper.cpp with CUDA, SDL2, and FFmpeg

The repo moved from `ggerganov/whisper.cpp` to **`ggml-org/whisper.cpp`** in early 2025 and the example binaries were renamed: `main → whisper-cli`, `server → whisper-server`, `stream → whisper-stream`. Many older blog posts still use the old names.

```bash
# System prerequisites
sudo apt update && sudo apt install -y build-essential git wget curl ffmpeg \
    libavcodec-dev libavformat-dev libavutil-dev libsdl2-dev gcc-10 g++-10

# Modern CMake from Kitware
wget -O - https://apt.kitware.com/keys/kitware-archive-latest.asc | gpg --dearmor - \
    | sudo tee /etc/apt/trusted.gpg.d/kitware.gpg >/dev/null
echo "deb https://apt.kitware.com/ubuntu/ focal main" | sudo tee /etc/apt/sources.list.d/kitware.list
sudo apt update && sudo apt install -y cmake     # >= 3.27

# CUDA 12.x via NVIDIA repo
wget https://developer.download.nvidia.com/compute/cuda/repos/ubuntu2004/x86_64/cuda-keyring_1.1-1_all.deb
sudo dpkg -i cuda-keyring_1.1-1_all.deb && sudo apt update
sudo apt install -y cuda-toolkit-12-6
export PATH=/usr/local/cuda/bin:$PATH
export LD_LIBRARY_PATH=/usr/local/cuda/lib64:$LD_LIBRARY_PATH

# Build with CUDA + SDL2 (for whisper-stream/-command) + FFmpeg input
git clone https://github.com/ggml-org/whisper.cpp.git && cd whisper.cpp && git checkout v1.8.4
CC=gcc-10 CXX=g++-10 cmake -B build \
    -DGGML_CUDA=1 -DWHISPER_SDL2=ON -DWHISPER_FFMPEG=yes \
    -DCMAKE_CUDA_ARCHITECTURES="86;89"          # Ampere + Ada Lovelace
cmake --build build -j --config Release

# Models
sh ./models/download-ggml-model.sh large-v3            # 3.1 GB F16
sh ./models/download-ggml-model.sh large-v3-turbo-q5_0 # 547 MB
sh ./models/download-vad-model.sh silero-v6.2.0        # Silero VAD
./build/bin/quantize models/ggml-large-v3.bin models/ggml-large-v3-q5_0.bin q5_0
```

Alternative backends use a single CMake flag each: `-DGGML_VULKAN=1` for cross-vendor GPUs, `-DGGML_BLAS=1` with `libopenblas-dev` for CPU acceleration, and `-DGGML_SYCL=ON` (with `icx/icpx`) for Intel GPUs. **OpenCL/CLBlast is deprecated**; use Vulkan instead. On Apple Silicon, Metal is on by default and Core ML is enabled via `-DWHISPER_COREML=1`.

The whisper.cpp **VRAM profile is excellent**: full F16 large-v3 with flash-attention (`-fa`) fits in **~6 GB**, q5_0 quantization needs only **~2 GB**, and `large-v3-turbo-q5_0` runs comfortably on a 4 GB GPU. The "turbo" variant has the same encoder as large-v3 but **4 decoder layers vs 32**, doubling speed at ~+0.3% WER on clean audio.

| Model file | Size | Runtime RAM | Notes |
|---|---|---|---|
| `ggml-large-v3.bin` (F16) | 3.1 GB | ~3.9 GB | best accuracy |
| `ggml-large-v3-q8_0.bin` | 1.66 GB | ~2.2 GB | ≈F16 quality |
| `ggml-large-v3-q5_0.bin` | **1.08 GB** | **~1.4 GB** | recommended default |
| `ggml-large-v3-q4_0.bin` | 0.9 GB | ~1.2 GB | noticeable WER drop |
| `ggml-large-v3-turbo.bin` (F16) | 1.62 GB | ~2.0 GB | 2× faster, multilingual |
| `ggml-large-v3-turbo-q5_0.bin` | **547 MB** | **~750 MB** | best speed/size tradeoff |

## Installing faster-whisper with pip-based CUDA

faster-whisper avoids system CUDA entirely by pulling cuBLAS and cuDNN as pip wheels — only the **NVIDIA driver (≥ 525)** needs to be on the host. **Pin compatibility carefully**: `ctranslate2 ≥ 4.5` needs CUDA 12 + cuDNN 9; pin `ctranslate2==4.4.0` for cuDNN 8, or `==3.24.0` for CUDA 11.

```bash
sudo add-apt-repository ppa:deadsnakes/ppa -y && sudo apt update
sudo apt install -y python3.10 python3.10-venv
python3.10 -m venv ~/fw && source ~/fw/bin/activate
pip install --upgrade pip
pip install faster-whisper==1.2.1
pip install nvidia-cublas-cu12 'nvidia-cudnn-cu12==9.*'

# Make CTranslate2 find the pip-installed CUDA libs at runtime
cat >> ~/fw/bin/activate <<'EOF'
export LD_LIBRARY_PATH=$(python3 -c "import os, nvidia.cublas.lib, nvidia.cudnn.lib; print(os.path.dirname(nvidia.cublas.lib.__file__)+':'+os.path.dirname(nvidia.cudnn.lib.__file__))"):$LD_LIBRARY_PATH
EOF
```

CTranslate2 is what makes faster-whisper fast: fused attention kernels, layer fusion, static graph compilation, weight pre-packing, native INT8/BF16 kernels, and (critically) **proper batched decoding** — Whisper's reference code processes one file at a time. The `compute_type` parameter picks a precision recipe. On Ampere or later, `int8_float16` and `int8_bfloat16` give the best speed/VRAM tradeoff; on a 4090, `float16` with `BatchedInferencePipeline(batch_size=16)` is the throughput sweet spot.

| large-v3 compute_type | VRAM (single stream) | Use case |
|---|---|---|
| `float32` | ~10 GB | reference quality, rare |
| `float16` / `bfloat16` | **~4.5 GB** | production GPU default |
| `int8_float16` | **~3.0 GB** | low-VRAM GPU, fastest |
| `int8` (CPU or GPU) | ~2.9 GB | CPU inference |

A minimal end-to-end transcription with word timestamps and VAD:

```python
from faster_whisper import WhisperModel, BatchedInferencePipeline

model = WhisperModel("large-v3", device="cuda", compute_type="float16")
batched = BatchedInferencePipeline(model=model)

segments, info = batched.transcribe(
    "meeting.mp3", batch_size=16, language="en",
    beam_size=5, word_timestamps=True, vad_filter=True,
)
print(f"{info.language}  duration={info.duration:.1f}s  after VAD={info.duration_after_vad:.1f}s")
for s in segments:
    print(f"[{s.start:7.2f}->{s.end:7.2f}] {s.text}")
```

**Performance head-to-head** (upstream README, 13-minute audio, RTX 3070 Ti 8 GB, large-v2):

| Implementation | Precision | Time | VRAM | RTF |
|---|---|---|---|---|
| openai/whisper | fp16 | 2m23s | 4708 MB | 5.4× |
| whisper.cpp v1.7.2 (Flash Attn) | fp16 | 1m05s | 4127 MB | 12× |
| faster-whisper | fp16 | 1m03s | 4525 MB | 12× |
| **faster-whisper, batch_size=8** | fp16 | **17s** | 6090 MB | **~46×** |
| **faster-whisper, batch_size=8** | int8 | **16s** | 4500 MB | **~49×** |

For **large-v3 on production GPUs**, plan on roughly **25× real-time on an RTX 4090** (faster-whisper fp16 batched), **35× on L40S**, and **60–70× on H100**. `insanely-fast-whisper` (Flash Attention 2) can hit 70–100× on a 4090 but uses much more VRAM. whisper.cpp+CUDA single-stream sits at ~25× on a 3090 — slower per stream than faster-whisper batched but simpler to deploy and quantize.

## Server modes and OpenAI-compatible APIs

`whisper-server` exposes a native `POST /inference` multipart endpoint. To make it look like OpenAI's API to existing SDKs, **remap the path with `--inference-path`**:

```bash
./build/bin/whisper-server \
    -m models/ggml-large-v3.bin \
    --host 0.0.0.0 --port 8080 \
    --inference-path /v1/audio/transcriptions \
    --convert -fa -t 8 \
    --vad --vad-model models/ggml-silero-v6.2.0.bin
```

`--convert` triggers server-side ffmpeg conversion of any uploaded format. `response_format` accepts `json`, `text`, `srt`, `vtt`, and `verbose_json`. The OpenAI Python SDK works unchanged against this endpoint:

```python
from openai import OpenAI
client = OpenAI(base_url="http://127.0.0.1:8080/v1", api_key="not-used")
with open("audio.wav", "rb") as f:
    print(client.audio.transcriptions.create(model="whisper-1", file=f,
                                              response_format="verbose_json").text)
```

For **production-grade scaling**, run multiple whisper-server containers on separate ports pinned to GPUs via `CUDA_VISIBLE_DEVICES` (or the new `-g N` flag from PR #3557) and front them with **LiteLLM** as a unified gateway. Several `model_name: whisper-1` entries pointing to different backends give LiteLLM automatic round-robin and retry. The official Docker images are `ghcr.io/ggml-org/whisper.cpp:main` (CPU) and `:main-cuda` (GPU); a compose snippet looks like this:

```yaml
services:
  whisper:
    image: ghcr.io/ggml-org/whisper.cpp:main-cuda
    restart: unless-stopped
    deploy: { resources: { reservations: { devices: [{ capabilities: [gpu] }] } } }
    ports: ["8080:8080"]
    volumes: ["./models:/models"]
    command: >
      whisper-server --model /models/ggml-large-v3.bin
      --host 0.0.0.0 --port 8080
      --inference-path /v1/audio/transcriptions --convert -fa -t 8
```

For faster-whisper, the best off-the-shelf wrapper is **`ahmetoner/whisper-asr-webservice` v1.9.1** (Docker, Swagger UI, supports faster_whisper/whisperX/openai_whisper engines). For OpenAI-API compatibility specifically, **`speaches-ai/speaches`** is the Docker-first choice. **WhisperLive** wraps faster-whisper as a WebSocket streaming server (covered below).

## Pipeline integration: bash, ffmpeg, Ollama, CI/CD, RAG

whisper-cli reads a finite-seekable WAV file; it does not accept unbounded stdin streams. The idiomatic pipeline pattern uses ffmpeg as a producer with `/dev/stdin` as the consumer's file path:

```bash
ffmpeg -hide_banner -loglevel error -i input.mp4 -ar 16000 -ac 1 -c:a pcm_s16le -f wav - \
  | whisper-cli -m models/ggml-large-v3.bin -f /dev/stdin -otxt -of out
```

Building whisper.cpp with `-DWHISPER_FFMPEG=yes` lets `whisper-cli` ingest mp3/m4a/opus directly. **faster-whisper handles all common formats natively** via PyAV (no host ffmpeg required). The typical ffmpeg preprocessing one-liners cover format conversion (`-ar 16000 -ac 1 -c:a pcm_s16le`), FFT denoise (`-af afftdn=nf=-25`), RNNoise (`arnndn=m=cb.rnnn`), EBU R128 normalization (`loudnorm=I=-16:TP=-1.5:LRA=11`), and silence removal (`silenceremove=...`).

For **transcript → LLM pipelines**, treat whisper output as a regular text stream and pipe it into Ollama via either the CLI or `/api/generate`:

```bash
TRANSCRIPT=$(whisper-cli -m models/ggml-large-v3.bin -f meeting.wav -otxt -of - 2>/dev/null)
ollama run llama3.1:8b "Summarize and list ACTION ITEMS as 'OWNER: TASK': $TRANSCRIPT" \
    > meeting.summary.md

# Structured JSON extraction
curl -s http://localhost:11434/api/generate -d "$(jq -n --arg p \
    "Extract action items from this transcript as JSON array of {owner,task,due}: $TRANSCRIPT" \
    '{model:"llama3.1:8b", prompt:$p, stream:false, format:"json"}')" | jq -r '.response'
```

The official **`whisper-talk-llama`** example wires this together for live mic chat using a shared ggml runtime. For **RAG**, transcribe with `vad_filter=True` and emit one JSON segment per utterance; LangChain has no native local-whisper loader (`OpenAIWhisperAudio` only calls the cloud API), so a 30-line custom loader feeding `RecursiveCharacterTextSplitter → OllamaEmbeddings → Chroma.from_texts` is the standard pattern.

For **CI/CD on GitHub Actions**, cache the HuggingFace model directory and use a self-hosted GPU runner for large jobs. **Note that `ubuntu-20.04` runners are retired**; use `ubuntu-22.04` or self-hosted 20.04. A minimal workflow caches `~/.cache/huggingface`, installs ffmpeg via `FedericoCarboni/setup-ffmpeg@v3`, runs faster-whisper on changed audio files, and commits transcripts. A **Makefile pattern** with `%.txt: %.wav` rules plus `make -j 2` parallelizes batch jobs at the file level (whisper.cpp issue #1408 confirms 2–3 instances at `--threads = nproc/instances` beats one instance with all threads).

## Batch transcription, subtitles, and speaker diarization

For one-off files, both runtimes emit standard formats — whisper.cpp via `-otxt -ovtt -osrt -oj -ocsv -of stem`, faster-whisper by serializing the segment generator. The non-obvious technique is the **`-ml 1` flag** for word-level timestamps in whisper-cli, equivalent to `word_timestamps=True` in faster-whisper. For video subtitles, generate `.vtt` then mux:

```bash
whisper-cli -m models/ggml-large-v3.bin -f movie.wav -ovtt -of subs
ffmpeg -i movie.mp4 -i subs.vtt -c copy -c:s mov_text out.mp4   # MP4
ffmpeg -i movie.mkv -i subs.vtt -c copy out.mkv                 # MKV native WebVTT
```

**Batch directory processing** has three viable patterns. For pure CPU, GNU parallel with 2 workers using 4 threads each beats one process with 8 threads. For GPU, **avoid GNU parallel entirely** — VRAM is the bottleneck. Instead use `BatchedInferencePipeline(batch_size=16)` in a single Python process, or Python `multiprocessing` with `initializer=` so each worker loads the model exactly once.

```python
import multiprocessing as mp
from faster_whisper import WhisperModel

MODEL = None
def init():
    global MODEL
    MODEL = WhisperModel("large-v3", device="cuda", compute_type="float16")

def worker(path):
    segs, info = MODEL.transcribe(path, beam_size=5, vad_filter=True)
    return path, info.language, list(segs)

if __name__ == "__main__":
    with mp.get_context("spawn").Pool(processes=2, initializer=init) as pool:
        for r in pool.imap_unordered(worker, file_list): print(r[0], r[1])
```

For **speaker diarization**, **whisperX** (m-bain/whisperX, v3.8.6) is the consensus tool: faster-whisper backend + wav2vec2 forced alignment for accurate word timestamps + pyannote-audio 3.1 for speaker turns, all in one pipeline. Set `HF_TOKEN` after accepting the pyannote license. The four-step flow is `load_model → transcribe → load_align_model + align → DiarizationPipeline + assign_word_speakers`, reporting ~70× real-time on large-v2 with batch_size=16 on an RTX 4090. whisper.cpp has built-in **tinydiarize** via the `-tdrz` flag, but it only ships for English `small.en` and emits `[SPEAKER_TURN]` markers without persistent speaker IDs — useful as a lightweight signal, not a diarization replacement.

Long audio files (multi-hour) are handled automatically by both runtimes' internal 30-second chunking, but **set `condition_on_previous_text=False` in faster-whisper** to prevent the well-known repetition-loop hallucination on long inputs. For very noisy long files, enable `vad_filter=True` with `min_silence_duration_ms=500` and `max_speech_duration_s=30`.

## Real-time microphone streaming with `whisper-stream` and faster-whisper

`whisper-stream` (only built when `-DWHISPER_SDL2=ON`) operates in two modes. **Sliding-window mode** uses `--step` (chunk interval, default 3000 ms), `--length` (window, default 10000 ms), and `--keep` (overlap, default 200 ms). **VAD-triggered mode** is activated with `--step 0` and waits for end-of-speech using `--vad-thold 0.6` before transcribing the last `--length` ms. The README is explicit that the stream binary is "naive" — production deployments should pair Silero VAD with LocalAgreement-style commit policies.

```bash
# Low-latency sliding window
./build/bin/whisper-stream -m models/ggml-large-v3-q5_0.bin -t 8 \
    --step 500 --length 5000 --keep 200 -c 0 --vad-thold 0.6 --freq-thold 100.0

# VAD-driven natural breakpoints
./build/bin/whisper-stream -m models/ggml-base.en.bin --step 0 --length 30000 -vth 0.6
```

For **faster-whisper streaming**, the right pattern is **external Silero VAD endpointing**: capture with `sounddevice` at 16 kHz, push 512-sample frames into `silero_vad.VADIterator`, accumulate audio until end-of-speech, then call `model.transcribe(numpy_array)` (faster-whisper accepts float32 arrays directly — no temp WAVs). Setting `condition_on_previous_text=False` and `beam_size=1` keeps live latency below 1 second post-EOS on CPU with the `base.en` model.

**Ubuntu 20.04 ships PulseAudio + ALSA**; PipeWire is optional. The capture toolchain is `arecord -l` / `pactl list sources short` to find devices, then either `arecord -D plughw:1,0 -f S16_LE -r 16000 -c 1 -t wav` (ALSA) or `parec` (PulseAudio). For an **unbounded pipe** to a Python consumer, use a named FIFO: `mkfifo /tmp/audio.fifo && arecord ... > /tmp/audio.fifo &`, then read 16-bit chunks in the consumer and convert to `float32 / 32768.0`.

For **WebSocket streaming servers**, three options dominate. **WhisperLive** (collabora) uses faster-whisper / TensorRT / OpenVINO backends, exposes a WebSocket plus iOS/Chrome clients, and supports hotwords for biasing rare terms. **ufal/whisper_streaming** implements the **LocalAgreement-2** policy from Macháček et al. 2023 (only commit text that agrees across two consecutive re-transcriptions of the growing buffer), achieving ~3.3 s average latency for large-v3 — the README itself notes the project is being superseded by **ufal/SimulStreaming**, which is roughly 5× faster. Both are TCP socket servers (port 43007 default).

| Stack | Chunk | First-token | Full utterance |
|---|---|---|---|
| whisper-stream `base.en` CPU, `--step 500` | 0.5 s | ~0.7 s | 1–2 s |
| WhisperLive `small` int8 GPU | continuous | ~300 ms | ~600 ms |
| ufal/whisper_streaming `large-v3` GPU | 1 s | ~1 s | ~3.3 s |
| Custom VAD + faster-whisper `base` CPU | endpointed | n/a | 0.5–1.5 s post-EOS |

The fundamental tradeoff is **chunk size vs context**: smaller `--step` cuts first-token latency but degrades accuracy because whisper sees less context per decode. Use sliding window with `--keep` overlap for streaming UX, and VAD-segmented mode whenever natural breakpoints are acceptable — accuracy is highest when the model sees a complete utterance.

## Voice keyword detection and desktop command execution

Whisper itself is a poor wake-word detector — too large, too slow, always-on CPU draw. The **production pattern is hybrid**: an always-on lightweight wake-word stage gates a heavier whisper transcription stage. **openWakeWord** (Apache-2.0, pre-trained "hey jarvis", "alexa", "hey mycroft") is the open-source default; **Picovoice Porcupine** is the lowest-CPU commercial option. Home Assistant's Assist pipeline uses exactly this architecture.

```python
from openwakeword.model import Model as OWWModel
from faster_whisper import WhisperModel
import sounddevice as sd, numpy as np

oww = OWWModel(wakeword_models=["hey_jarvis"], enable_speex_noise_suppression=True)
asr = WhisperModel("base.en", device="cpu", compute_type="int8")

with sd.InputStream(samplerate=16000, channels=1, dtype="int16", blocksize=1280) as s:
    while True:
        data, _ = s.read(1280)
        if max(oww.predict(data[:,0]).values()) > 0.5:
            # capture ~4 s of command audio and transcribe
            buf = [s.read(1280)[0][:,0] for _ in range(50)]
            audio = np.concatenate(buf).astype(np.float32) / 32768.0
            segs, _ = asr.transcribe(audio, language="en", beam_size=1,
                                     condition_on_previous_text=False)
            text = " ".join(seg.text for seg in segs)
            dispatch(text)   # see below
```

For **command matching**, `rapidfuzz.process.extractOne(transcript, COMMANDS.keys(), scorer=fuzz.WRatio)` with a threshold around 78 handles speech variability robustly. Always prefilter with a regex requiring a wake phrase, and normalize the transcript (lowercase, strip punctuation). For **constrained decoding**, whisper.cpp's `whisper-command` binary supports both command-list mode (`-cmd commands.txt`) and **GBNF grammar mode** (`--grammar smarthome.gbnf --grammar-penalty 100`). The maintainers warn that grammar is a *bias* not a hard filter, and counterintuitively works better with smaller models — `tiny`/`base`/`small` rather than `large-v3`.

**Desktop automation on Ubuntu 20.04 defaults to Xorg**, so `xdotool` is the right tool (`xdotool type`, `xdotool key ctrl+alt+t`, `xdotool search --class firefox windowactivate`). `wmctrl -a "Firefox"` switches windows, `xclip -selection clipboard` handles paste targets, and `playerctl play-pause` controls media. Switch to `ydotool` only if `echo $XDG_SESSION_TYPE` returns `wayland` — ydotool uses raw Linux input event codes rather than symbolic key names, which is a substantial UX regression.

**Security is non-negotiable** for voice-triggered shells:

- Whitelist a fixed command set; **never** `os.system`, `eval`, or `shell=True` on raw transcripts.
- Always use `subprocess.run([list, of, args])` with arguments hardcoded in your dispatch table, not constructed from the transcript.
- Require an explicit confirmation utterance ("yes confirm", rapidfuzz ≥ 90) with a 5-second timeout for destructive actions like `poweroff` or `rm`.
- Run the daemon as a **systemd user unit** with `NoNewPrivileges=true` — never as root. Add the user to the `audio` group instead.
- Reject low-confidence transcripts using whisper's `avg_logprob` (≥ -1.0) and `no_speech_prob` (≤ 0.4) thresholds.

## Sound-triggered automation with YAMNet + whisper hybrids

For **non-speech audio events** (doorbells, alarms, glass breaking, baby cries), pair an AudioSet classifier with whisper. **YAMNet** (TF Hub, 521 classes, mAP ~0.31, 0.96 s frames at 16 kHz) is the lightweight choice; **PANNs CNN14** (527 classes, mAP 0.44, 32 kHz) is more accurate but heavier; **AST** (HuggingFace `MIT/ast-finetuned-audioset-10-10-0.4593`) tops the leaderboard but is the slowest. The relevant AudioSet class IDs are `Doorbell` (354), `Smoke detector` (390), `Fire alarm` (388), `Glass` (446), `Baby cry` (20), `Dog/Bark` (74/75), and `Speech` (0).

The hybrid pipeline routes the audio stream based on the YAMNet top class: if Speech > 0.6, hand the audio to faster-whisper for transcription and keyword matching; otherwise, dispatch on the class label directly to MQTT, webhook, or shell triggers. A reference daemon runs the YAMNet classifier on rolling 1-second windows, only invoking whisper when speech is detected — keeping average CPU usage low while preserving full transcription quality when needed.

For **Home Assistant integration**, the **Wyoming protocol** (port 10300 for STT, 10200 for TTS, 10400 for wake-word) is the native pathway. Install `wyoming-faster-whisper` and add the Wyoming integration in HA Settings; the Assist pipeline now talks to your local model. For ad-hoc triggers, MQTT via `paho-mqtt` is the cleanest transport: publish `{"class":"Doorbell","conf":0.91}` to `home/audio/event` and use HA's MQTT trigger automations. **n8n self-hosted** is the right choice for visual workflow automation; **GitHub Actions `repository_dispatch`** events fire workflows from a `curl` POST signed with a PAT.

For **continuous monitoring**, run the daemon as a systemd user unit (`Restart=on-failure`, `loginctl enable-linger $USER` so it survives logout). **Avoid cron** — audio monitoring must be a long-lived process. Decouple audio I/O from ML inference with a named FIFO: `arecord ... > /tmp/audio.fifo &` and a Python consumer that reads 16-bit chunks. To monitor **RTSP camera audio**, use `ffmpeg -rtsp_transport tcp -i rtsp://CAM/Channels/101 -vn -ac 1 -ar 16000 -f s16le -` and pipe the output into the same consumer. FFmpeg ≥ 7.1 has a built-in `whisper` audio filter (`-af "whisper=model=...:format=json"`) that emits JSON lines directly — useful for one-shot transcription of live streams.

## Hardware sizing and recommendations

The right hardware depends on whether you optimize for **latency** (one stream as fast as possible) or **throughput** (many streams concurrently). Memory bandwidth dominates Whisper inference more than raw FLOPS — a 4090 at 1008 GB/s crushes a 3060 at 360 GB/s by 3–5× despite "only" 2× the cores.

| Use case | Runtime | Compute type | Min VRAM | Recommended GPU |
|---|---|---|---|---|
| Edge / single user offline | whisper.cpp q5_0 | — | 2 GB | Any GPU with 4+ GB, or CPU |
| Single live stream | faster-whisper | int8_float16 | 3 GB | RTX 3060 / 4060 Ti |
| Dev workstation, 2–3 streams | faster-whisper | float16 | 5 GB | RTX 3090 / 4070 Ti / 4080 |
| 10+ concurrent streams | faster-whisper + batched | float16 | 10–15 GB | **RTX 4090 24 GB** |
| 50+ concurrent | faster-whisper + batched | int8_float16 | 24–48 GB | **L40S 48 GB** |
| 200+ call-center scale | faster-whisper + batched | float16 | 80 GB | **H100 80 GB** |

On CPU, large-v3 is only practical for offline batch work — expect 2–5× slower than real-time on a 16-core Xeon at int8. AVX-512/VNNI on Ice Lake+ accelerates int8 by ~20–30%. For interactive use cases, **`large-v3-turbo-q8_0` (whisper.cpp) or `large-v3-turbo` with `compute_type="int8_float16"` (faster-whisper) is the recommended default** — within 0.3% WER of full large-v3 at half the latency.

## Choosing between the two runtimes

Pick **whisper.cpp** when you need a single dependency-free binary across CPU edge, Apple Silicon, and AMD-via-Vulkan; when you want an OpenAI-compatible HTTP server with minimal moving parts; when you're shipping Docker images and care about size; or when quantization down to q4/q5 matters for fitting on a constrained device. The renamed binaries (`whisper-cli`, `whisper-server`, `whisper-stream`) plus first-class Silero VAD, Flash Attention, and FFmpeg input make v1.8.4 a serious production target.

Pick **faster-whisper** when you have NVIDIA GPUs and need maximum throughput — `BatchedInferencePipeline(batch_size=16)` delivers ~3.7× speedup over sequential and is the foundation of WhisperX, WhisperLive, and whisper-asr-webservice. The Python API is richer (full word timestamps with dynamic time warping, hotword biasing, all decoder parameters exposed), the VAD integration is native, and the install path via pip-only CUDA wheels is dramatically simpler than building whisper.cpp with the right CUDA toolkit on a host. The fact that PyAV decodes any audio format without a system ffmpeg dependency is the cherry on top.

In practice, mature deployments **use both**: whisper.cpp as the lightweight HTTP backend behind LiteLLM for diverse clients including embedded devices, faster-whisper inside heavier Python pipelines that need batched inference, word timestamps, diarization (via whisperX), or streaming (via WhisperLive). The OpenAI `/v1/audio/transcriptions` contract gives you the freedom to swap one for the other without touching client code.