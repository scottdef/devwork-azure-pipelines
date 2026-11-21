# lm9 spike 0

Runnable code behind the snippets in the lm9 plan. It is not an inference
engine. It is the set of mechanisms the plan depends on, each small enough to
read in one sitting and each with a test that would fail if the claim were
false.

Standard library only. The one exception is `ffi/`, a separate module.

## Packages

| Package | What it shows | Status |
| --- | --- | --- |
| `kernels` | Float and Q8_0 dot products in portable Go, AVX2 and AVX-512 assembly; CPUID dispatch; off-heap arena; guard-page tests | Tested on amd64 |
| `kernels/dot_arm64.s` | The float kernel in NEON, opt-in with `-tags lm9_neon` | Assembles and passes vet; never executed |
| `quant` | Half-float conversion; Q8_0 and Q4_0 block formats; Q4_K dequantization | Tested. Q4_K is tested against a hand-built block, not against llama.cpp |
| `gguf` | Bounds-checked GGUF directory reader | Tested, including every truncation of a valid file, and fuzzed |
| `safetensors` | Bounds-checked safetensors directory reader | Tested, including every truncation of a valid file |
| `ops` | Reference RMSNorm, softmax, SiLU, rotary encoding and single-head attention | Tested on their defining properties, not against llama.cpp |
| `pool` | Pinned thread pool with a static row split | Tested: same bits for 1, 2, 3, 4 and 7 workers |
| `sample` | Top-k, temperature, top-p with a seeded ChaCha8 generator | Tested |
| `planner` | Layer placement across GPU and system memory budgets | Tested |
| `pretok` | GPT-2 pre-tokenizer as a state-function lexer, replacing a regex with lookahead; the BPE merge loop | Tested against hand-derived splits and fuzzed; not compared with a reference tokenizer |
| `ninep` | 9P2000 codec and a synthetic file server | Tested against hand-computed bytes and its own client, and fuzzed; not mounted by a real 9P client |
| `stream` | Per-token server-sent events | Tested |
| `probe` | GPUs, PCIe link speed and storage class from sysfs | Tested on the sandbox only |
| `cmd/fmacheck` | Multiply-add fusion differs by architecture | Checked from compiler output for amd64 and arm64 |
| `kernels/dot_simd_amd64.go` | The fast kernel in `simd/archsimd` intrinsics | Not compiled: needs Go 1.26 and `GOEXPERIMENT=simd` |
| `ffi` | A call into the system Vulkan loader with cgo off, through purego | Not built or run: the sandbox could not fetch purego, and building it from a local checkout with checksum verification off was not permitted |

## Run it

```bash
make preflight   # vet, tests, portable-tier tests, cross-compile, static check, fusion check
make bench       # kernels: portable Go against assembly
make fuzz        # 20 seconds each on the three parsers
make probe       # hardware report as JSON
make verify      # check every file against SHA256SUMS
```

## What was measured

Sandbox run on 21 Nov 2026: Go 1.24.7, linux/amd64, 2 vCPUs of an Intel Xeon
at 2.10 GHz. One run of `make bench`, medians of three. These numbers
describe that sandbox; rerun on your own hardware.

| Kernel | 4,096 weights, in cache | 16.7M floats, 64 MiB per vector |
| --- | --- | --- |
| Float dot, plain Go loop | 2,726 ns | 17.3 ms |
| Float dot, deterministic reference in Go | 3,295 ns | not run |
| Float dot, deterministic AVX2 assembly | 366 ns | not run |
| Float dot, AVX2 + FMA assembly | 222 ns | 7.2 ms |
| Float dot, AVX-512 assembly | 160 ns | 7.2 ms |
| Q8_0 dot, portable Go | 4,520 ns | not run |
| Q8_0 dot, AVX2 assembly reading the file layout | 370 ns | not run |

In cache, assembly is 12 to 17 times faster than the plain loop for floats
and about 12 times faster for Q8_0. Out of cache the gap falls to about 2.4
times, and AVX-512 is no faster than AVX2, because every version is waiting
on memory.

The fast and deterministic kernels returned different bits in 45 of 50 random
trials. Both are correct to rounding; they are different contracts.

Fuzzing: 1.2 million executions of the GGUF parser, 720 thousand of the 9P
decoder and 80 thousand of the pre-tokenizer, with no failure.

## Things the tests found

- The first version of the streaming handler did not send HTTP headers until
  the first token existed, so the client's request blocked. The test hung;
  the fix is one flush.
- The probe classes the sandbox's virtio disks as `hdd`, because the virtual
  machine reports them as rotational. This is why `--storage` is a flag and
  not only a probe.
- The Go compiler did not fuse `x*y+z` on amd64 at `GOAMD64=v3`, which I had
  expected it to. It does on arm64.
- `syscall.Madvise` exists only on Linux. The cross-compile step caught it
  for darwin and freebsd; `gguf/advise_other.go` is the workaround.

## Not tested

- Nothing ran on arm64, macOS, Windows or Plan 9. Those targets compile.
- `go.mod` says `go 1.24` because that is what the sandbox had. Change it to
  `go 1.26` when you build with a 1.26 toolchain.
- No GPU code ran anywhere.
