# Cactus Compute (Whistle, Needle, Mirage) for Voice-, Vision- and PDF-Driven Computer Use, KVM and Automation: A Field Guide (November 2026)

Cactus 4.0.0 is a small CPU engine (under 1 MB per platform) with one C API.\[1\] It loads three tiny open models: **Whistle** for speech-to-text, **Needle 3** for tool calling and structured extraction, and **Mirage** for vision, OCR and detection.\[2\] It is a good fit for a Unix-style voice/vision → command → HID pipeline on a Raspberry Pi 5 or Ubuntu server. It is not a drop-in "agent brain": Needle only emits calls to tools you declare, Mirage's OCR is Latin-only and weak on some text, and most published speed figures were measured on phones or an Apple M4 Pro, not on a Pi.

## TL;DR

- **Which model does what:** Whistle is speech-to-text (16.9 MB, 7 languages, Apache-2.0, released 2 Oct 2026). Needle 3 is tool calling, extraction and embeddings (8–29 MB, 29–121M parameters, Apache-2.0, released 17 Sep 2026). Mirage is vision: description, detection, OCR, masks and forms (51.3M parameters, 21.6 MB, Apache-2.0, released 6 Nov 2026). All three load into the same Cactus engine, which ships prebuilt for linux-arm64, linux-armv7 and linux-x86_64, with a CLI runner, `libcactus.a` and `cactus.h`.
- **Recommended architecture:** Run Cactus on a Pi 5 (or an Ubuntu box) as a set of small single-purpose processes. Drive them from a stdlib-only Go 1.24 orchestrator, using the CLI runner as a subprocess or its localhost `--serve` mode, and inject actions through `/dev/hidg0`. Use the ESP32-S3/P4 as a microphone, wake-word and VAD front end that streams PCM to the edge node. Running Whistle on the ESP32 itself is possible only through community ports, not official builds. Treat the Coral Dev Board as legacy: Mendel Linux is frozen on Debian 10, and Google archived the Edge TPU repo in April 2026. Give it a front-end role only, such as a status display, a wake-word or change detector, or a spare HID gadget.
- **Safety:** Voice-triggered keystroke injection is a remote-code-execution surface. Ship it only with closed tool enums, Needle's confidence gating plus your own threshold, spoken or touchscreen confirmation for anything destructive, an append-only keystroke audit log, and a hardware kill switch. Never let OCR/PDF text flow into the channel that selects commands.

## Key Findings

### What "Cactus" is in November 2026 (and what changed)

- **Two generations share the name.** The older general engine was "Energy-efficient inference engine for running AI on mobile devices", with "OpenAI compatible APIs for C/C++, Swift, Kotlin, Flutter & React-Native" and "cloud handoff for complex tasks" (Cactus docs v1.8).\[3\] It now lives in the `cactus-compute/engine` repo ("2bit quants, kernels and runtime for consumer devices").\[4\] The current `cactus-compute/cactus` repo, under Apache-2.0, is "Sub-50mb agentic, voice and vision intelligence for consumer devices".\[5\] Its 4.0.0 runtime is built around Needle 3, Whistle and Mirage.\[2\] Older docs show a different, handle-based C API (`cactus_init(path, rag_path, bool)` returning `cactus_model_t`).\[6\] **Do not mix the two.**
- **Runtime 4.0.0** has 17 target folders. Native runners load `.cact` weights at runtime. The Linux folders (`linux-x86_64`, `linux-arm64`, `linux-armv7`, `linux-riscv64`, `linux-mipsel`) each ship `cactus`, `libcactus.a` and `cactus.h`. Release artifacts carry SHA-256 checksums in a manifest, pinned by revision `1a028f5a3972b67dd8d8752f882113d475e7ec4c` or tag `v4.0.0` (Cactus-Compute/cactus on Hugging Face).\[2\]
- **The engine is CPU-only on Linux SBCs.** Cactus describes Whistle as running "on the CPU with no dependencies".\[7\] Its kernels are SIMD CPU kernels.\[8\] The only NPU figure is Mirage on a Samsung Galaxy S26.\[9\] I found nothing showing Hailo, Edge TPU or VideoCore acceleration.
- **C API (verified names, unverified prototypes):** `cactus.h` exposes `cactus_load`, `cactus_init`, `cactus_complete`, `cactus_transcribe` and `cactus_embed`, "returning non-negative counts on success and negative values on failure".\[1\] The speech docs add `cactus_set_audio` and `cactus_models`, and the Mirage docs add `cactus_describe` (which takes RGB pixels).\[1\]\[9\] "The native API owns one process-global model and conversation."\[1\] The exact argument lists are not printed in the public docs, so read the shipped `cactus.h` before writing cgo.
- **CLI and server:** `./cactus --model needle3.cact --tools tools.json --prompt "..."` answers one query and exits. Adding `--serve` serves HTTP on localhost (`POST /complete {"input": "..."}`).\[1\] The port, the bind address, and whether `--serve` accepts audio or images are **not documented**. The native engine "reads no environment variables".\[7\] The Python package collects anonymous usage counts, which `CACTUS_TELEMETRY=0` or `DO_NOT_TRACK=1` disables.\[10\]
- **Speech plus tools in one call:** `cactus --model needle3.cact --model whistle.cact --tools tools.json --audio clip.wav` returns one JSON object containing `function_calls`, `confidence`, `audio_text` and `audio_language`.\[7\] This single call is the core of a voice-command pipeline.

### The three models

| | **Whistle** | **Needle 3** | **Mirage** |
|---|---|---|---|
| Task | ASR, word timestamps, speech embeddings (one row per 80 ms) | Tool calls, structured extraction, text embeddings | Scene description, detection, OCR, masks, relations, enum/count forms |
| Released / license | 2 Oct 2026 / Apache-2.0 | 17 Sep 2026 / Apache-2.0 | 6 Nov 2026 / Apache-2.0 |
| Size on disk | 16.9 MB (2–4-bit) | 8–29 MB (CQ2; 20-layer = 29 MB) | 21.6 MB (mostly 2-bit) |
| Parameters | 55,151,433, counted from the checkpoint header by reviewer Satyajit Ghana (6 Oct 2026); Cactus states "55 million total parameters, 36 million active" | 29M (4 layers) to 121M (20 layers) | 51.3M (7.8M encoder, 40.8M decoder, 2.6M heads) |
| Input | 16 kHz mono; 30 s per pass, windowed beyond | Text plus tools/schema | Image (512 px stem); boxes on a 0–1000 scale |
| Languages | en, de, fr, es, it, nl, pl | Not stated | Descriptions in the same 7 languages; OCR Latin script only |
| RAM | ~124 MB peak RSS for a whole Python process (MarkTechPost, hardware unstated) | Needle 2: ~28 MB native session; Needle 3 reports `peak_ram_mb` per call | ~134 MB peak RSS on an M-series CPU, with ~80–150 ms per image (lawkitt/mdoc issue #911, checked 17 Nov 2026) |
| Pi 5 speed | **Not published** | 400–4,000 tok/s decode, 1–10k tok/s prefill (vendor) | **Not published** |
| Other published speed | 11.1 ms time to first token and 1,319 tok/s decode on a 10 s clip, Apple M4 Pro (Cactus; Whisper base 73.2 ms and 266 tok/s) | Needle 2 on a Pi 5 8GB (CPU only, cactus-needle 2.0.7): 76–149 ms per call, per a Raspberry Pi blog post written by Cactus | 74 ms (S26 NPU), 107 ms (S26 CPU); 100–150 ms on one user's CPU |
| HF ID | `Cactus-Compute/whistle` | `Cactus-Compute/needle3` | `Cactus-Compute/mirage` |

**Naming caveats.** "Needle" covers three generations:
- Needle 1: 26M parameters, distilled from Gemini 3.1 Flash Lite.\[11\] Its original Show HN described it as MIT-licensed, while Cactus's blog said Apache 2.0.\[12\]\[13\]
- Needle 2: 45M parameters, 14 MB, with a "28MB of RAM" session.\[14\]
- Needle 3: 29–121M parameters.\[15\]

Cactus 4 runs only Needle 3. Needle 2 needs `cactus-compute<4`.\[10\] Some third-party write-ups call the engine itself "Needle", because Whistle "loads into the same C++ engine as Needle".\[7\]\[16\]

**Whistle details worth knowing:**
- Decoding uses 5 beams.\[7\]\[17\]
- Keyword biasing uses an Aho-Corasick automaton.\[7\] Pass your hostnames and command words as keywords.
- `--audio-depth` selects how many decoder layers run, from 2 up to 8.\[7\]
- Silent clips return an empty transcript.\[7\]
- Independent caveats:
  - Steady noise is filtered, but "other sounds, music especially, can still come out as text" (Cactus docs).\[10\]
  - One ESP32 porter reports that Whistle, "the official engine included, transcribes ten seconds of microphone noise as 'Thank you.'"\[18\]
  - Reviewer Satyajit Ghana (6 Oct 2026) disputes Cactus's FLEURS comparison: "the seven-language mean of Whisper's Table 13 is 21.0, ahead of Whistle's 21.4." He read the weights but did not run the engine.
- Cactus itself concedes that Whisper base is ahead on TED-LIUM, AMI and MLS.\[7\]

**Needle 3 details:**
- A byte-level grammar guarantees well-formed calls.\[10\]
- An off-topic request yields an empty call list, and "no free text is generated".\[10\]
- A calibrated `confidence` comes with each call. The engine applies a floor of 0.1, and anything below it goes to `suppressed_calls`.\[19\]
- `triggers` regexes force routing to a named tool.\[19\]
- Mobile Actions score is 86.0% at 20 layers (vendor-reported).\[20\]\[21\]
- LoRA fine-tuning with `cactus finetune` lifts DroidCall accuracy by 18–36 points per depth.\[22\]

**Mirage details:**
- `describe()` returns JSON sections (objects, text, relations), and `extract()` fills enum or count forms.\[9\]
- Issue #190 on cactus-compute/cactus (tested with cactus-compute 4.1.0 on an Apple M3 Pro) reports:
  - At the default `text_conf`, Mirage returned no text for 23 of 30 large-text phrases.\[23\]
  - Overlapping text regions produce repeated words.\[23\]
  - Cyrillic and Greek come back as Latin look-alikes: 0/4 exact matches for Cyrillic, 0/2 for Greek, and 0/18 for clean Latin text with diacritics.
- **Implication: Mirage is a perception front end, not a document OCR engine.**

### Cactus vs alternatives, by role

| Role | Cactus option | Alternatives | Recommendation |
|---|---|---|---|
| STT | Whistle (16.9 MB) | whisper.cpp / faster-whisper (Whisper base 145.3 MB), Moonshine tiny v2 (41.9 MB), Vosk (grammar mode) | Whistle for short commands. Whisper for long dictation and noisy meetings, where Whistle lost on AMI and TED-LIUM. |
| Command intent | Needle 3 (2–20 layers) | llama.cpp/Ollama with a 0.5–3B model plus JSON grammar | Needle for closed tool sets. A local LLM via an OpenAI-compatible API (or the cloud) for open-ended planning. |
| Image → text | Mirage | Tesseract, Florence-2, SmolVLM, moondream | Mirage for UI and scene structure plus box grounding. Tesseract for dense or large document text. |
| PDF → text | none (Mirage only for scans) | poppler `pdftotext`, MuPDF `mutool` | Native extraction first. Rasterize only pages that have no text layer. |
| Wake/KWS on an MCU | none official | ESP-SR WakeNet9/9s/10, MultiNet7 | ESP-SR on the ESP32; Whistle on the edge node. |

### Hardware platforms

| Platform | Cactus status | Role in this guide |
|---|---|---|
| ESP32-S3 (8–16 MB PSRAM) | No official build. A community C99 Needle 2 engine exists, and a CircuitDigest port runs Whistle in ~6 s per short command | Mic, WakeNet/VAD, PCM streaming, display and relays |
| ESP32-P4 (32 MB PSRAM) | No official build. "Cactus ships no ESP32 build" (Crazor/whistle-esp32); community Whistle ports at depth 4 report 11.7% WER | Optional on-chip Whistle for offline fallback\[18\] |
| Raspberry Pi 4 (1–8 GB) | `linux-arm64`; the models total under 70 MB of weights | Works; expect slower than a Pi 5 (unmeasured) |
| Raspberry Pi 5 | `linux-arm64`; Needle numbers published | **Primary edge node and KVM gadget** |
| Coral Dev Board (i.MX 8M, 4× A53, 1/4 GB) | Untested. Mendel 4.0 "Day" = Debian 10, kernel 4.14, Python 3.7; "Mendel development has been frozen" | Legacy coprocessor: TFLite KWS/vision, status UI, second HID gadget |
| Ubuntu server x86_64/arm64 | `linux-x86_64` / `linux-arm64` | Orchestrator, LLM escalation, PDF batch work |

## Details

### 1. Pipeline design (all three models)

```
mic ─▶ [VAD/wake] ─▶ PCM ─▶ whistle+needle (one cactus call) ─▶ JSON calls ─▶ gate ─▶ actuator (hidg0 / exec / MCP)
HDMI ─▶ [frame diff] ─▶ PNG ─▶ mirage.describe ─▶ observation JSON ──▶ (context only, never commands)
PDF ─▶ pdftotext | (no text layer) pdftoppm ─▶ tesseract/mirage ─▶ chunks ─▶ needle extract / LLM
```

**Design rules:**

1. **Each stage is one process speaking JSON lines on stdout.** Text is the interface, so every stage stays testable with `jq` and `cat`.
2. **Concurrent residency, not swapping.** All three weight files together are about 68 MB, so they fit in a 1 GB Pi 4. The native API is process-global, so run one process per role for fault isolation.\[1\]
3. **Batch per utterance, not streaming.** Whistle runs one pass per 30 s or less, and commands are short.\[7\] If you need live captions, one independent test re-transcribed 1 s chunks and committed a word after two passes agreed, with a median 1.68 s lag.\[24\]
4. **Latency budget, Pi 5 voice command:**
   - VAD tail: 300–500 ms.
   - Whistle: unmeasured on a Pi. Benchmark it with `make bench`.
   - Needle: about 80–150 ms, based on the Needle 2 Pi 5 numbers.\[25\]
   - Gate, then HID: under 10 ms.
   - Target: under 1 s from end of speech to keystroke.
5. **Gates:**
   - `confidence ≥ 0.85` with a read-only tool: execute.
   - `0.5–0.85`, or a mutating tool: speak or display the call, and require "confirm" or a touch.
   - Below 0.5, or empty: refuse.
   - Tuned weights return `confidence=None`, so treat them as needing confirmation.\[10\]
6. **Errors:**
   - Non-zero exit, `success:false`, or an `error_code`: log the error and drop the request. Never retry a mutating call.
   - Whistle returning "Thank you." or a one-word transcript under 0.6 s of audio: discard it.

**Action schema** (`tools.json`, Needle compact form; closed enums keep the decode grammar tight):

```json
[
 {"name":"kvm_select","description":"Switch the KVM to a target computer","parameters":{"type":"object",
  "properties":{"target":{"type":"string","enum":["server1","server2","server3","server4"]}},"required":["target"]}},
 {"name":"open_terminal","description":"Open a terminal on the selected computer","parameters":{"type":"object","properties":{}}},
 {"name":"run_command","description":"Type an allowlisted shell command and press Enter","parameters":{"type":"object",
  "properties":{"command":{"type":"string","enum":["uptime","df -h","free -m","systemctl --failed","journalctl -p err -n 50"]}},"required":["command"]}},
 {"name":"press_keys","description":"Press a key chord","parameters":{"type":"object",
  "properties":{"chord":{"type":"string","enum":["ctrl+alt+t","ctrl+c","ctrl+l","alt+tab","enter","escape"]}},"required":["chord"]}}
]
```

"Switch to server 3, open a terminal, run uptime" should yield three ordered calls, since Needle 3 returns multiple calls in order.\[19\] Validate this with your own test set before trusting it.

### 2. Speech → commands → UI operation

- **Closed vocabulary beats open language.** Use enums plus Whistle keyword biasing (`--keywords "server3\nuptime"` in Python, or `cactus_set_audio` natively).\[1\]\[10\] Free-form `run_command` with a string argument should exist only behind a typed-confirmation gate, or not at all.
- **Actuators:**
  - `/dev/hidg0`: an 8-byte boot keyboard report `[mods, 0, k1..k6]`.
  - `/dev/hidg1`: an absolute mouse.
  - Linux `uinput`, for local automation.
  - RFB/VNC, when the target exposes one.
- **Grounding:** Mirage boxes use a 0–1000 scale.\[9\] Convert with `hidX = x*32767/1000` for a 0–32767 absolute pointer, using the box centre.

### 3. Image to text

- **Capture sources:**
  - USB UVC HDMI dongle (`/dev/video0`, MJPEG).
  - TC358743 CSI-2 bridge (`dtoverlay=tc358743`), which PiKVM rates as the lowest-latency option.\[26\]
  - `/dev/fb0` or a VNC framebuffer for local screens.
- **Frame policy:** Grab one frame per second. Downscale to 512 px wide to match Mirage's stem.\[9\] Run Mirage only when a perceptual diff exceeds a threshold. Request only `sections=["objects","text"]` to cut decode work.\[9\]
- **Tesseract vs Mirage:** For terminal text, BIOS screens and logs, Tesseract on a 2× upscaled crop is more reliable, because issue #190 reports Mirage missing large text. Use Mirage for "where is the OK button", "is a login prompt visible" (`extract` with an enum), and object boxes.

### 4. Converting PDFs to text

```bash
#!/usr/bin/env bash
set -euo pipefail
pdf=$1; out=${2:-out}; mkdir -p "$out"
n=$(pdfinfo "$pdf" | awk '/^Pages:/{print $2}')
for p in $(seq 1 "$n"); do
  pdftotext -layout -f "$p" -l "$p" "$pdf" "$out/p$p.txt"
  if [ "$(tr -d '[:space:]' < "$out/p$p.txt" | wc -c)" -lt 20 ]; then   # no text layer
    pdftoppm -r 200 -f "$p" -l "$p" -png "$pdf" "$out/p$p"
    tesseract "$out/p$p-$p.png" "$out/p$p" --psm 1 >/dev/null 2>&1 || true
  fi
done
cat "$out"/p*.txt > "$out/all.txt"; sha256sum "$pdf" "$out/all.txt" > "$out/SHA256SUMS"
```

**Notes:**
- `mutool draw -F txt` is a faster MuPDF alternative.
- Go has no stdlib PDF parser, so shell out with `os/exec`.
- Chunk on page and paragraph boundaries into chunks of about 1–2k characters, with the page number in metadata.
- Use Needle `extract` for fixed schemas such as invoice, ticket or IP/hostname tables, and escalate summarisation to an LLM.
- Use Mirage only for figure classification or form checkboxes on rasterised pages.

## Case Study A: ESP32 speech-to-text system

**Honest status.** Cactus publishes no ESP32 build. Its own Needle 2 post says only that Needle "runs on newer microcontrollers like ESP32-S3", citing a 28 MB RAM ceiling that fits an "ESP32-P4 with 32MB of PSRAM".\[14\] Community projects do run Whistle on-chip:
- Crazor/whistle-esp32 on an ESP32-P4NRW32, with a 14.5 MB depth-4 model.\[17\]\[18\]
- Xorlent/ESP32P4-Voice.\[27\]
- Hahihula/cactus-whistle-rs, a `no_std` Rust engine.\[17\]
- CircuitDigest, on an ESP32-S3 at "roughly 6 seconds for a short command".\[28\]

Those ports are push-to-talk, multi-second and unofficial. **Recommendation:** split the work. The ESP32 handles capture, wake word and VAD; a Pi 5 or server runs Whistle and Needle.

**Bill of materials:**
- ESP32-S3-DevKitC-1 N16R8 (or ESP32-P4 for an on-chip fallback).
- INMP441 I2S microphone (pins: SCK→GPIO4, WS→GPIO5, SD→GPIO6, L/R→GND).
- WS2812 LED or SSD1306 display.
- Relay module.
- Raspberry Pi 5 (4 GB or more).

**ESP-only alternative.** ESP-SR WakeNet (WakeNet9/9s/10, with WakeNet10 released 17 Aug 2026) plus MultiNet7 recognises "up to 200" offline commands on the ESP32-S3/P4.\[29\]\[30\] For a fixed set such as "lights on" or "relay two off", that is cheaper, lower-latency and needs no network. Use the Cactus split only when you need open phrasing, seven languages or downstream tool calls.

**Firmware** (ESP-IDF v5.x, `driver/i2s_std.h`). It uses energy-VAD push-to-talk. Swap in the AFE/WakeNet pipeline from `esp-skainet` once the path works, and check the AFE API against your ESP-SR version, because V2.0 changed it.

```c
// main/voice_node.c — capture 16 kHz s16 mono, VAD-gate, POST to edge, act on reply
#include <string.h>
#include "freertos/FreeRTOS.h"
#include "driver/i2s_std.h"
#include "driver/gpio.h"
#include "esp_http_client.h"
#include "esp_log.h"
#define RATE 16000
#define MAX_S (RATE * 6)            // 6 s max utterance (192 KB in PSRAM)
#define RELAY GPIO_NUM_10
static const char *T = "voice";
static i2s_chan_handle_t rx;
static int16_t *buf;

static void mic_init(void) {
  i2s_chan_config_t cc = I2S_CHANNEL_DEFAULT_CONFIG(I2S_NUM_0, I2S_ROLE_MASTER);
  ESP_ERROR_CHECK(i2s_new_channel(&cc, NULL, &rx));
  i2s_std_config_t sc = {
    .clk_cfg = I2S_STD_CLK_DEFAULT_CONFIG(RATE),
    .slot_cfg = I2S_STD_PHILIPS_SLOT_DEFAULT_CONFIG(I2S_DATA_BIT_WIDTH_32BIT, I2S_SLOT_MODE_MONO),
    .gpio_cfg = { .mclk = I2S_GPIO_UNUSED, .bclk = GPIO_NUM_4, .ws = GPIO_NUM_5,
                  .dout = I2S_GPIO_UNUSED, .din = GPIO_NUM_6 } };
  sc.slot_cfg.slot_mask = I2S_STD_SLOT_LEFT;
  ESP_ERROR_CHECK(i2s_channel_init_std_mode(rx, &sc));
  ESP_ERROR_CHECK(i2s_channel_enable(rx));
}

static int32_t frame_energy(const int16_t *s, int n) {
  int64_t e = 0; for (int i = 0; i < n; i++) e += (int32_t)s[i] * s[i];
  return (int32_t)(e / n);
}

static void post_utterance(int nsamp) {
  char resp[512] = {0};
  esp_http_client_config_t c = { .url = CONFIG_EDGE_URL, .method = HTTP_METHOD_POST, .timeout_ms = 5000 };
  esp_http_client_handle_t h = esp_http_client_init(&c);
  esp_http_client_set_header(h, "Content-Type", "audio/L16;rate=16000;channels=1");
  esp_http_client_set_header(h, "Authorization", "Bearer " CONFIG_NODE_TOKEN);
  if (esp_http_client_open(h, nsamp * 2) == ESP_OK) {
    esp_http_client_write(h, (const char *)buf, nsamp * 2);
    esp_http_client_fetch_headers(h);
    int r = esp_http_client_read(h, resp, sizeof resp - 1);
    if (r > 0) {
      ESP_LOGI(T, "edge: %s", resp);
      if (strstr(resp, "\"relay\":\"on\""))  gpio_set_level(RELAY, 1);
      if (strstr(resp, "\"relay\":\"off\"")) gpio_set_level(RELAY, 0);
    }
  }
  esp_http_client_cleanup(h);
}

void voice_task(void *arg) {
  int32_t raw[320]; int16_t pcm[320]; size_t got; int n = 0, quiet = 0, noise = 2000;
  for (;;) {
    i2s_channel_read(rx, raw, sizeof raw, &got, portMAX_DELAY);   // 20 ms
    int k = got / 4;
    for (int i = 0; i < k; i++) pcm[i] = (int16_t)(raw[i] >> 14);  // INMP441 24-bit in 32
    int32_t e = frame_energy(pcm, k);
    if (n == 0) { noise = (noise * 31 + e) / 32; if (e < noise * 4) continue; }
    if (n + k < MAX_S) { memcpy(buf + n, pcm, k * 2); n += k; }
    quiet = (e < noise * 2) ? quiet + 1 : 0;
    if (quiet > 25 || n + k >= MAX_S) {                             // 500 ms tail
      if (n > RATE / 2) post_utterance(n);                          // drop <0.5 s blips
      n = 0; quiet = 0;
    }
  }
}

void app_main(void) {
  buf = heap_caps_malloc(MAX_S * 2, MALLOC_CAP_SPIRAM);
  gpio_set_direction(RELAY, GPIO_MODE_OUTPUT);
  mic_init();  /* wifi_init_sta() omitted: use the esp-idf wifi station example */
  xTaskCreatePinnedToCore(voice_task, "voice", 8192, NULL, 5, NULL, 1);
}
```

**Edge receiver.** This is the `/v1/utterance` handler of the Go orchestrator below. It wraps the PCM in a WAV header, makes one `cactus` call with both models, gates the result, and returns a compact action JSON.

**Latency (expected; Whistle-on-Pi not yet measured):**
- VAD tail: 500 ms.
- Upload of 2 s of PCM (64 KB) over 2.4 GHz Wi-Fi: about 50–150 ms.
- Whistle plus Needle on a Pi 5: benchmark it. Needle alone took 76–149 ms per call for Needle 2 on a Pi 5 8GB, per the Cactus-written Raspberry Pi blog post.
- Return path: under 20 ms.

**Power:**
- Keep the ESP32-S3 in modem-sleep between utterances, or use Wi-Fi power save.
- Continuous I2S capture prevents deep sleep. For battery builds, use a hardware wake button or a P4/S3 with WakeNet gating the radio.

**Security:**
- Per-node bearer token.
- The edge listens only on a dedicated VLAN.
- Firmware maps only the reply keys it knows (`relay`, `led`) and ignores everything else.

## Case Study B: Controlling LLMs/agents with the pipelines

**Topology:**
- **Pi 5 (8 GB):** "ears and hands". Runs Whistle, Needle and Mirage, plus the HID gadget.
- **Ubuntu server:** "brain". Runs the orchestrator, PDF batch work, an optional llama.cpp or Ollama OpenAI-compatible `/v1/chat/completions`, and cloud escalation.
- **Coral Dev Board:** "front end".

The orchestrator routes on `function_calls[0].name`:
- Local tools execute directly.
- An `ask_agent(task)` tool forwards the transcript to the LLM tier.
- LLM-proposed actions come back through the same allowlist and gate.

**Coral Dev Board reality check:**
- Mendel 4.0 is Debian 10 with kernel 4.14 and Python 3.7, and "Mendel development has been frozen".\[31\]\[32\]
- `google-coral/edgetpu` "was archived by the owner on Apr 19, 2026".
- PyCoral packages target Python 3.6–3.9.\[33\]
- Cactus's `linux-arm64` runner may or may not run on Debian 10's glibc. The Python package can fetch a `manylinux2014_aarch64` engine, which hints at an old glibc baseline, but this is **untested**, so run `make coral-probe`.\[10\]
- Realistic roles for the board:
  1. An Edge TPU TFLite keyword spotter or person/screen-change detector, which posts events to the Pi.
  2. A local status display.
  3. A spare USB gadget on its USB-C OTG port (unverified on Mendel kernels).
- Do not design new work around it.

**MCP:**
- The official `modelcontextprotocol/go-sdk` is at v1.8.0 (14 Sep 2026), and "New releases of the SDK target only supported versions of Go".\[34\]\[35\] Under Go's two-release support policy, that means a toolchain newer than 1.24, which **violates your ≤1.24 constraint**.
- **Options:**
  - (a) Implement a minimal MCP stdio server by hand: newline-delimited JSON-RPC 2.0 with `initialize`, `tools/list` and `tools/call`, using `encoding/json`.
  - (b) Pin an old go-sdk tag whose `go.mod` declares ≤1.24. Which tag does this is unverified, so check its `go` directive.
- Expose `kvm_select`, `run_command` and the rest as MCP tools, so a desktop agent can drive the same gated actuators.

**Go orchestrator** (Go 1.24, stdlib only; `cmd/voxd/main.go`):

```go
package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Call struct {
	Name string         `json:"name"`
	Args map[string]any `json:"arguments"`
}
type Result struct {
	Success   bool    `json:"success"`
	Calls     []Call  `json:"function_calls"`
	Held      []Call  `json:"suppressed_calls"`
	Conf      *float64 `json:"confidence"`
	AudioText string  `json:"audio_text"`
	ErrorCode any     `json:"error_code"`
}

var (
	bin    = flag.String("cactus", "/opt/cactus/linux-arm64/cactus", "runner")
	models = flag.String("models", "/opt/cactus/models", "dir with needle3.cact, whistle.cact")
	tools  = flag.String("tools", "/etc/voxd/tools.json", "tool schema")
	addr   = flag.String("listen", "127.0.0.1:8088", "listen addr")
	audit  = flag.String("audit", "/var/log/voxd/audit.jsonl", "append-only audit log")
	token  = os.Getenv("VOXD_TOKEN")
	safe   = map[string]bool{"kvm_select": true, "run_command": true} // closed enums
)

func wav(pcm []byte) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF"); w(uint32(36 + len(pcm))); b.WriteString("WAVEfmt ")
	w(uint32(16)); w(uint16(1)); w(uint16(1)); w(uint32(16000)); w(uint32(32000)); w(uint16(2)); w(uint16(16))
	b.WriteString("data"); w(uint32(len(pcm))); b.Write(pcm)
	return b.Bytes()
}

func infer(ctx context.Context, clip string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *bin,
		"--model", filepath.Join(*models, "needle3.cact"),
		"--model", filepath.Join(*models, "whistle.cact"),
		"--tools", *tools, "--audio", clip)
	out, err := cmd.Output()
	var r Result
	if err != nil {
		return r, fmt.Errorf("cactus: %w", err)
	}
	return r, json.Unmarshal(out, &r)
}

func gate(r Result) (string, []Call) {
	t := strings.TrimSpace(strings.ToLower(r.AudioText))
	if t == "" || t == "thank you." || !r.Success {
		return "drop", nil
	}
	if len(r.Calls) == 0 {
		if len(r.Held) > 0 {
			return "confirm", r.Held
		}
		return "refuse", nil
	}
	if r.Conf == nil || *r.Conf < 0.85 {
		return "confirm", r.Calls
	}
	for _, c := range r.Calls {
		if !safe[c.Name] {
			return "confirm", r.Calls
		}
	}
	return "execute", r.Calls
}

func logAudit(v any) {
	f, err := os.OpenFile(*audit, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		log.Print(err)
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(v)
}

func utterance(w http.ResponseWriter, req *http.Request) {
	if subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	pcm, err := io.ReadAll(io.LimitReader(req.Body, 16000*2*30))
	if err != nil || len(pcm) < 16000 {
		http.Error(w, "bad audio", http.StatusBadRequest)
		return
	}
	f, _ := os.CreateTemp("", "utt-*.wav")
	defer os.Remove(f.Name())
	f.Write(wav(pcm)); f.Close()
	r, err := infer(req.Context(), f.Name())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	verdict, calls := gate(r)
	logAudit(map[string]any{"ts": time.Now().UTC(), "src": req.RemoteAddr, "text": r.AudioText,
		"conf": r.Conf, "verdict": verdict, "calls": calls})
	if verdict == "execute" {
		for _, c := range calls {
			if err := dispatch(c); err != nil {
				verdict = "error"
				break
			}
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"verdict": verdict, "text": r.AudioText, "calls": calls})
}

func main() {
	flag.Parse()
	if len(token) < 32 {
		log.Fatal("VOXD_TOKEN missing or short")
	}
	http.HandleFunc("POST /v1/utterance", utterance)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
```

`dispatch` maps calls to actuators:
- `kvm_select`: the USB/HDMI switch or the per-target gadget.
- `run_command`: types an enum value via HID.
- `ask_agent`: POSTs to the LLM tier.

**Plan 9 angle.** Expose stages as files:
- `/run/vox/text`: a FIFO of transcripts.
- `/run/vox/calls`: a FIFO of gated JSON calls.
- `/run/kvm/kbd`: write text and it gets typed.

A true 9P2000 server (`/mnt/voice`, `/mnt/kvm`) is a few hundred lines of `encoding/binary` in Go, with no third-party dependency. Mount it with `9pfuse` or Linux v9fs. That is a design suggestion; it is not shipped by Cactus.

**Makefile (unified interface):**

```make
SHELL := /bin/bash -euo pipefail
GO ?= go
REV := 1a028f5a3972b67dd8d8752f882113d475e7ec4c
PLAT ?= linux-arm64
DIST := dist
.PHONY: preflight fetch verify build cross test bench install coral-probe
preflight:
	@$(GO) version | grep -Eq 'go1\.2[0-4]' || { echo "need Go <=1.24"; exit 1; }
	@command -v uv pdftotext tesseract sha256sum >/dev/null
	@test -e /sys/kernel/config/usb_gadget || echo "WARN: configfs gadget not mounted"
fetch:
	uv tool run --from cactus-compute cactus download $(PLAT) --out /opt/cactus
	for m in needle3 whistle mirage; do uv tool run --from cactus-compute cactus download $$m --out /opt/cactus/models; done
verify:
	cd /opt/cactus && sha256sum -c SHA256SUMS   # generated from the v4.0.0 manifest.json
build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags='-s -w' -o $(DIST)/voxd ./cmd/voxd
cross:
	for t in linux/arm64 linux/amd64 linux/arm; do o=$${t%/*}; a=$${t#*/}; \
	  GOOS=$$o GOARCH=$$a GOARM=7 CGO_ENABLED=0 $(GO) build -trimpath -o $(DIST)/voxd-$$o-$$a ./cmd/voxd; done
	cd $(DIST) && sha256sum voxd-* > SHA256SUMS
test:
	$(GO) vet ./... && $(GO) test ./...
bench:
	for f in testdata/*.wav; do /usr/bin/time -f "%e s %M KB" /opt/cactus/$(PLAT)/cactus \
	  --model /opt/cactus/models/whistle.cact --audio $$f >/dev/null; done
coral-probe:
	ssh mendel@coral 'ldd --version | head -1; uname -m; ./cactus --model whistle.cact --audio t.wav' || true
install: build
	install -m0755 $(DIST)/voxd /usr/local/bin/voxd && install -m0644 deploy/voxd.service /etc/systemd/system/
```

`cactus download` has no documented `--revision` flag, so pin by verifying against the v4.0.0 manifest checksums.

**systemd unit:**

```ini
[Unit]
Description=voxd voice->command orchestrator
After=network-online.target
[Service]
User=voxd
Group=hidgadget
EnvironmentFile=/etc/voxd/env
ExecStart=/usr/local/bin/voxd -listen 0.0.0.0:8088
Restart=on-failure
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
ReadWritePaths=/var/log/voxd
DeviceAllow=/dev/hidg0 w
DeviceAllow=/dev/hidg1 w
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
MemoryMax=512M
[Install]
WantedBy=multi-user.target
```

**GitHub Actions** (`CoolGitOrg/voxd` on the CoolGitEnterprise GHES instance; the release token comes from `issueops-autoadmin-app`):

```yaml
name: release
on: { push: { tags: ["v*"] } }
permissions: { contents: write }
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.24.x" }
      - run: make test cross
      - id: app
        uses: actions/create-github-app-token@v1
        with:
          app-id: ${{ vars.ISSUEOPS_AUTOADMIN_APP_ID }}
          private-key: ${{ secrets.ISSUEOPS_AUTOADMIN_APP_KEY }}
          owner: CoolGitOrg
      - env: { GH_TOKEN: "${{ steps.app.outputs.token }}" }
        run: gh release create "$GITHUB_REF_NAME" dist/voxd-* dist/SHA256SUMS --repo CoolGitOrg/voxd --generate-notes
```

## Case Study C: Universal voice-and-vision KVM

**Hardware:**
- **Raspberry Pi 5 (8 GB):**
  - Its USB-C port acts as the device gadget (`dtoverlay=dwc2,dr_mode=peripheral`, `modules-load=dwc2,libcomposite`). Power it from GPIO 5 V or a PoE HAT, so that the target's USB port does not brown it out.\[36\]\[37\]
  - HDMI capture comes from a USB UVC dongle or a TC358743 CSI-2 bridge.
- **5-inch 800×480 DSI or HDMI touch panel:** shows a local status/confirm UI. A Go `net/http` page runs in a kiosk browser, or you draw frames to `/dev/fb0` with `image/draw`.
- **Coral Dev Board (optional):** a frame-change or "screen-locked" TFLite classifier on the Edge TPU, posting events. Treat it as legacy hardware.
- **Multiplexing:**
  - (a) A 4-port USB+HDMI KVM switch, driven by a GPIO relay or its hotkey, which is cheapest. Hotkeys pass through as HID, so emit the switch's chord.
  - (b) One Pi Zero 2 W gadget per target, controlled over the LAN, which gives the cleanest isolation.

**Gadget setup** (boot keyboard plus absolute mouse; extends your existing configfs notes):

```bash
#!/usr/bin/env bash
set -euo pipefail
G=/sys/kernel/config/usb_gadget/vkvm
modprobe libcomposite
mkdir -p $G && cd $G
echo 0x1d6b > idVendor; echo 0x0104 > idProduct; echo 0x0200 > bcdUSB
mkdir -p strings/0x409 configs/c.1/strings/0x409
echo "vkvm-$(cat /etc/machine-id | cut -c1-8)" > strings/0x409/serialnumber
echo "vkvm" > strings/0x409/product; echo "kbd+abs" > configs/c.1/strings/0x409/configuration
mkdir -p functions/hid.kbd functions/hid.abs
echo 1 > functions/hid.kbd/protocol; echo 1 > functions/hid.kbd/subclass; echo 8 > functions/hid.kbd/report_length
printf '\x05\x01\x09\x06\xa1\x01\x05\x07\x19\xe0\x29\xe7\x15\x00\x25\x01\x75\x01\x95\x08\x81\x02\x95\x01\x75\x08\x81\x03\x95\x05\x75\x01\x05\x08\x19\x01\x29\x05\x91\x02\x95\x01\x75\x03\x91\x03\x95\x06\x75\x08\x15\x00\x25\x65\x05\x07\x19\x00\x29\x65\x81\x00\xc0' > functions/hid.kbd/report_desc
echo 0 > functions/hid.abs/protocol; echo 0 > functions/hid.abs/subclass; echo 5 > functions/hid.abs/report_length
printf '\x05\x01\x09\x02\xa1\x01\x09\x01\xa1\x00\x05\x09\x19\x01\x29\x03\x15\x00\x25\x01\x95\x03\x75\x01\x81\x02\x95\x01\x75\x05\x81\x03\x05\x01\x09\x30\x09\x31\x15\x00\x26\xff\x7f\x75\x10\x95\x02\x81\x02\xc0\xc0' > functions/hid.abs/report_desc
ln -sf $G/functions/hid.kbd configs/c.1/; ln -sf $G/functions/hid.abs configs/c.1/
ls /sys/class/udc | head -1 > UDC
```

The absolute-mouse descriptor is a common 3-button, 15-bit X/Y layout. Validate it on each target OS and BIOS, because some firmware only accepts boot-protocol mice (your existing caveats apply).

**HID typing in Go** (stdlib; US layout; writes are logged before injection):

```go
var usage = func() map[rune][2]byte { // rune -> {modifier, keycode}
	m := map[rune][2]byte{' ': {0, 0x2c}, '\n': {0, 0x28}, '-': {0, 0x2d}, '.': {0, 0x37}, '/': {0, 0x38}}
	for i, r := range "abcdefghijklmnopqrstuvwxyz" {
		m[r] = [2]byte{0, byte(0x04 + i)}
		m[r-32] = [2]byte{0x02, byte(0x04 + i)} // shift
	}
	for i, r := range "123456789" {
		m[r] = [2]byte{0, byte(0x1e + i)}
	}
	m['0'] = [2]byte{0, 0x27}
	return m
}()

func typeString(dev *os.File, s string) error {
	for _, r := range s {
		k, ok := usage[r]
		if !ok {
			return fmt.Errorf("unmapped rune %q", r)
		}
		if _, err := dev.Write([]byte{k[0], 0, k[1], 0, 0, 0, 0, 0}); err != nil {
			return err
		}
		if _, err := dev.Write(make([]byte, 8)); err != nil { // key up
			return err
		}
		time.Sleep(8 * time.Millisecond)
	}
	return nil
}

func click(dev *os.File, x1000, y1000 int) error { // Mirage 0..1000 box centre -> 0..32767
	x, y := uint16(x1000*32767/1000), uint16(y1000*32767/1000)
	b := []byte{1, byte(x), byte(x >> 8), byte(y), byte(y >> 8)}
	if _, err := dev.Write(b); err != nil {
		return err
	}
	b[0] = 0
	_, err := dev.Write(b)
	return err
}
```

**Voice flow, "switch to server 3, open a terminal, run uptime":**

1. Whistle and Needle return `[kvm_select{server3}, open_terminal{}, run_command{uptime}]`.
2. The gate puts it on the LCD: "server3 → terminal → `uptime`? [OK]".
3. On a touch or a spoken "confirm", the switch hotkey fires and the KVM waits 1.5 s for the HDMI to re-lock.
4. Mirage `extract(frame, {"screen":{"enum":["desktop","login","terminal","bios","locked"]}})` checks the expected state before typing. If the state is `login` or `locked`, abort.
5. `typeString(hidg0, "uptime\n")`.
6. Grab a frame, run Tesseract on the bottom 20% for the result, and display it or speak it.

**Remote access:**
- Serve the web UI and WebSocket from the Go binary behind TLS 1.3 with mTLS client certificates (`crypto/tls`, `ClientAuth: tls.RequireAndVerifyClientCert`).
- Or reach it only via WireGuard or Tailscale.
- Never forward the port.

**Audit:** Each injected keystroke batch is written to the JSONL log before `Write`, with source (voice, web or MCP), transcript, confidence, target and SHA-256 of the frame. Ship the log off-box with `journald` or `rsyslog`.

**Off-the-shelf comparison (2026):**

| Device | Price | Latency (WarpKVM test) | Notes |
|---|---|---|---|
| PiKVM V4 Mini / Plus | ~$275 / ~$385–400 | 57.3 ms (V4 Plus, H.264) | Most mature and fully GPL. "No notable 2026 CVEs" per one review |
| JetKVM | $103 ($119 PoE) | 59.8 ms | Touchscreen; fixes for the March 2026 coordinated disclosure in firmware 0.5.4 |
| NanoKVM Full / Pro | ~$55–87 / ~$155 | 144.8 ms / 53.3 ms | Fixed in app 2.3.1; reviewers advise LAN isolation |
| BliKVM v4 | ~$200 (2025 price) | n/a | Allwinner H616, 4K@30 capture\[38\] |
| **This build** | ~$150–250 | Capture-bound; adds a voice/vision layer | The only one with local STT, intent and vision; less polished |

**Verdict:** For plain remote console access, buy a PiKVM or JetKVM. Build this when you need voice and vision automation, and consider running it *alongside* a PiKVM, using PiKVM's HID for injection.

## Recommendations

1. **Start on a Pi 5 with the CLI runner as a subprocess.** It is the most stable surface, because the cgo prototypes and `--serve` details are undocumented. Move to `--serve` for Needle once you have confirmed its port and bind address.
2. **Benchmark before you commit.** Run `make bench` on your Pi 5 and Pi 4 with your microphone and your phrases. No vendor publishes Whistle or Mirage numbers for a Pi.
3. **Fine-tune Needle on your tool set.** Generate 500–2,000 rows with `cactus generate-data`, run `cactus finetune`, and build a 4- or 8-layer `.cact` with `cactus build --layers 8 --platform linux-arm64`. Gate on confirmation, because tuned weights return `confidence=None`.\[10\]
4. **Use ESP-SR MultiNet for fixed MCU commands,** and the ESP32 → Pi split for open phrasing. Treat on-chip Whistle ports as experiments.
5. **Retire the Coral from critical paths.** Repurpose it only where TFLite on the Edge TPU and a Debian 10 base are acceptable.
6. **Use Tesseract for document and terminal text, and Mirage for UI state and grounding.** Use poppler before any vision model for PDFs.

## Caveats

- **Vendor numbers:**
  - Whistle's word error rates and time to first token are vendor-measured on an M4 Pro,\[7\] and the FLEURS comparison is disputed.
  - Mirage's latency figures come from a Galaxy S26.\[9\]
  - Needle's Pi 5 speed range is vendor-reported.\[1\]
  - The Raspberry Pi Needle 2 post was written by Cactus Compute, so it is not independent. The independent chungddong/NeedleTest benchmark of Needle 3 on a Pi 5 16GB (C runtime) measured p50 361 ms, p90 627 ms, worst 4.7 s and 74 MB peak memory. It also found that "the advertised ~4,000 tok/s on a Pi 5 matches prefill at --depth 2, where accuracy is near zero". These figures come from a search snippet, because the README did not render when fetched.
- **Unverified:**
  - The exact `cactus.h` prototypes.
  - The `--serve` port, bind address and media support.
  - Whether the CLI accepts images for Mirage. Until confirmed, call Mirage through a tiny `uv run` Python shim that prints JSON.
  - Whether Cactus runs on Mendel Linux.
  - The absolute-mouse descriptor's behaviour on your targets.
- **Security, safety and ethics:**
  - Use this only on machines you own or are authorised to administer.
  - HID injection bypasses endpoint controls, so treat the device as privileged hardware.
  - Accidental activation (TV audio, "Thank you." hallucinations) and prompt injection are the main risks. Screen and PDF text must never become tool arguments for mutating tools; only closed enums may.
- **Constraint conflicts:**
  - The official Go MCP SDK's current releases target Go versions newer than 1.24.\[35\]
  - `cactus download` has no documented revision pin, so rely on manifest checksums.

## Sources

1. [What Devices Are Supported on Needle](https://cactuscompute.com/blog/needle-supported-devices)
2. [Cactus-Compute/cactus · Hugging Face](https://huggingface.co/Cactus-Compute/cactus)
3. [Cactus Docs - Cactus Compute](https://docs.cactuscompute.com/v1.8/)
4. [Cactus Compute, Inc. · GitHub](https://github.com/cactus-compute)
5. [GitHub - cactus-compute/cactus: Sub-50mb agentic, voice and vision intelligence for consumer devices.](https://github.com/cactus-compute/cactus)
6. [Cactus - Cactus Docs](https://docs.cactuscompute.com/v1.14/)
7. [Whistle: Speech to Text in 16.9 MB](https://cactuscompute.com/blog/whistle)
8. [whistle by Cactus-Compute](https://mixpeek.com/model/Cactus-Compute/whistle)
9. [Mirage: VLM + OCR + Detection + Segment + Decide in 22 MB](https://cactuscompute.com/blog/mirage)
10. [Needle Python Docs](https://cactuscompute.com/blog/needle-python-docs)
11. [Needle: Cactus Compute's 26M Tool-Calling Model Explained](https://www.devpik.com/blog/needle-26m-tool-calling-model-complete-guide)
12. [Cactus Releases Needle: A 26M Distilled Model for On-Device Tool Calling](https://rits.shanghai.nyu.edu/ai/cactus-releases-needle-a-26m-distilled-model-for-on-device-tool-calling/)
13. [We Distilled Gemini Tool Calling into a 26M Model](https://cactuscompute.com/blog/needle)
14. [Needle 2: The 14 MB Agentic LLM for Tiny Devices](https://cactuscompute.com/blog/needle-2)
15. [Needle 3 — an 8-29 MB model that picks tools on…](https://ai-tldr.dev/releases/cactus-needle-3/)
16. [Whistle ASR: 16.9 MB On-Device Speech Model](https://artrealmai.com/article/whistle-asr-16mb-on-device-needle)
17. [GitHub - Hahihula/cactus-whistle-rs: no\_std inference engine for Cactus Whistle, a multilingual speech recognition model: desktop or bare-metal · GitHub](https://github.com/Hahihula/cactus-whistle-rs)
18. [GitHub - Crazor/whistle-esp32: Whistle speech-to-text running on the ESP32-P4 · GitHub](https://github.com/Crazor/whistle-esp32)
19. [Needle 3 - 8-29 MB foundation model for tiny devices | Cactus](https://cactuscompute.com/needle)
20. [Needle 3: Fine-Tuning an 8-29MB Tool-Calling Model for Edge Devices](https://betterstack.com/community/guides/ai/needle3-edge/)
21. [Needle 3 — Specs, Benchmarks & On-Device Tool Calling](https://ai-tldr.dev/models/needle-3/)
22. [Fine-tuning Needle](https://cactuscompute.com/blog/finetuning-needle)
23. [Mirage OCR drops or swaps diacritics, returns Latin look-alikes for Cyrillic/Greek, misses large text; are there OCR numbers? · Issue #190 · cactus-compute/cactus · GitHub](https://github.com/cactus-compute/cactus/issues/190)
24. [Cactus Whistle vs Whisper Base: WER, 33 ms, streaming - HAIQ](https://haiq.agency/en/cactus-whistle-vs-whisper-base-16-9-mb-cpu-wer-33-ms-streaming)
25. [Turn text input into actions with Needle, a 14MB function-calling LLM - Raspberry Pi](https://www.raspberrypi.com/news/turn-text-input-into-actions-with-needle-a-14mb-function-calling-llm/)
26. [DIY PiKVM V1 quickstart guide - PiKVM Handbook](https://docs.pikvm.org/v1/)
27. [GitHub - Xorlent/ESP32P4-Voice: On-device speech-to-text optimized for ESP32-P4 boards. Speak a command, and it is transcribed using Cactus Whistle. · GitHub](https://github.com/Xorlent/ESP32P4-Voice)
28. [ESP32 Offline Speech Recognition: Whistle Guide + GitHub](https://circuitdigest.com/tutorial/esp32-offline-speech-recognition)
29. [ESP32-S3 ESP-SR User Guide Release master Espressif Systems Oct 30, 2026](https://docs.espressif.com/projects/esp-sr/en/latest/esp32s3/esp-sr-en-master-esp32s3.pdf)
30. [GitHub - espressif/esp-sr: Speech recognition · GitHub](https://github.com/espressif/esp-sr)
31. [Updates from Coral: Mendel Linux 4.0 and much more! - Google Developers Blog](https://developers.googleblog.com/updates-from-coral-mendel-linux-40-and-much-more/)
32. [Gentoo Forums :: View topic - Gentoo on Google's Coral Dev Board?](https://forums.gentoo.org/viewtopic-p-8870354.html?sid=5f584b278c75f0935f7a15e1764f4db1)
33. [Software](https://www.coral.withgoogle.com/software/)
34. [\[go-fan\] Go Module Review: modelcontextprotocol/go-sdk · Issue #15320 · github/gh-aw-mcpg · GitHub](https://github.com/github/gh-aw-mcpg/issues/15320)
35. [go-sdk/README.md at main · modelcontextprotocol/go-sdk · GitHub](https://github.com/modelcontextprotocol/go-sdk/blob/main/README.md)
36. [RPi 5 USB gadget mode possible? - Raspberry Pi Forums](https://forums.raspberrypi.com/viewtopic.php?t=358573)
37. [Raspberry Pi USB Gadget Mode: RNDIS Ethernet Driver Setup on Windows](https://www.techeia.com/blog/raspberry-pi-usb-gadget-rndis-windows-setup)
38. [Which KVM over IP in 2025? — IT Pro Expert](https://itproexpert.com/which-kvm-over-ip-in-2025/)
