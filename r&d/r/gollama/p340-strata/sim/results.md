Calibration: P1000 effective bandwidth 41.5 GB/s, fixed cost 3.3 ms/token
  tinyllama 1.1B   measured   62 tok/s, model 53.5
  qwen2.5 1.5B     measured   34 tok/s, model 36.9
  qwen2.5 3B       measured   18 tok/s, model 20.1
  llama3.2 3B      measured   20 tok/s, model 19.3
  phi3.5 3.8B      measured   19 tok/s, model 17.8
CPU effective bandwidth: 25.8 GB/s at DDR4-2933, 18.8 GB/s at DDR4-2133

| ID | Pattern | RAM GB | VRAM GB | Decode tok/s | Prompt tok/s | Chat turn s | Agent step s | Cold document s | Fit |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| A | Strata, Qwen3.6-35B-A3B, dense on GPU, no drafts | 25.0 of 32 | 3.60 of 4 | 16.5 | 104 | 33 | 14 | 88 | tight |
| A+ | Strata, same, drafts on | 25.0 of 32 | 4.60 of 4 | 23.5 | 104 | 27 | 11 | 79 | no: VRAM |
| B | llama-server, same file, dense on GPU, experts on CPU | 25.0 of 32 | 3.07 of 4 | 13.8 | 87 | 39 | 17 | 105 | fits |
| B-cpu | llama-server, same file, CPU only | 26.9 of 32 | 0.50 of 4 | 9.9 | 60 | 55 | 23 | 149 | fits |
| C | Qwen3.5-4B router on GPU + MoE on CPU, 70/30 split | 26.9 of 32 | 3.59 of 4 | 13.0 | 101 | 40 | 17 | 149 | tight |
| D | Council: 4B + 9B + gpt-oss-20b on CPU, 4B aggregator on GPU | 24.5 of 32 | 3.59 of 4 | 14.2 | 118 | 293 | 123 | over context | tight |
| D-self | Self-consistency: 3 samples of the MoE in 3 slots, vote | 25.0 of 32 | 3.25 of 4 | 20.0 | 87 | 97 | 40 | over context | fits |
| E | Strata, Flash-Next Q2_0, dense on GPU, no drafts | 38.3 of 48 | 3.91 of 4 | 10.2 | 38 | 69 | 28 | 206 | tight |
| E-B | Pattern B again on the 48 GB machine | 25.0 of 48 | 3.07 of 4 | 12.3 | 87 | 42 | 18 | 110 | fits |

Upgrades hosted by the P340, Strata with drafts at 32K unless noted (modelled)

| ID | Machine | Model | Experts on card | Decode tok/s | Prompt tok/s | Chat turn s | Document s | Others report |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| U1 | P340, 64 GB, RTX A2000 12 GB in the slot | Qwen3.6-35B-A3B UD-Q4_K_XL | 32% | 51 | 194 | 14 | 41 |  |
| U1-F | P340, 64 GB, RTX A2000 12 GB in the slot | Qwen3.8-Flash-Next Q2_0 | 21% | 41 | not modelled | not modelled | not modelled |  |
| U2 | P340, 64 GB, RTX 5060 Ti 16 GB on a dock | Qwen3.6-35B-A3B UD-Q4_K_XL | 53% | 71 | 304 | 9 | 27 |  |
| U2-L | P340, 64 GB, RTX 5060 Ti 16 GB on a dock, llama-server | Qwen3.6-35B-A3B UD-Q4_K_XL | 61% | 45 | 296 | 12 | 31 | about 42 estimated by Llama Builds |
| U2-F | P340, 64 GB, RTX 5060 Ti 16 GB on a dock | Qwen3.8-Flash-Next Q2_0 | 32% | 54 | not modelled | not modelled | not modelled |  |
| U2-A | P340, 64 GB, RTX 2000 Ada 16 GB in the slot (256 GB/s assumed) | Qwen3.6-35B-A3B UD-Q4_K_XL | 53% | 55 | 275 | 11 | 31 |  |
| U3 | P340, 64 GB, RTX 3090 24 GB on a dock | Qwen3.6-35B-A3B UD-Q4_K_XL | 94% | 173 | 1516 | 3 | 7 |  |
| U3-F | P340, 64 GB, RTX 3090 24 GB on a dock | Qwen3.8-Flash-Next Q2_0 | 56% | 87 | not modelled | not modelled | not modelled |  |

The model against published figures (decode only)

| ID | Machine and engine | Model | Experts on card | Modelled tok/s | Published tok/s |
| --- | --- | --- | --- | --- | --- |
| V1 | Radeon AI PRO R9700 32 GB, Strata, no drafts | Qwen3.6-35B-A3B UD-Q4_K_XL | 100% | 111 | 83.9 measured by Strata |
| V2 | Radeon AI PRO R9700 32 GB, llama.cpp | Qwen3.6-35B-A3B UD-Q4_K_XL | 100% | 92 | 73.8 measured by Strata |
| V3 | RTX 3060 12 GB, DDR5-5600 assumed, Strata, no drafts | Qwen3.6-35B-A3B UD-Q4_K_XL | 43% | 55 | 59.7 measured by Strata |
| V4 | RTX 3060 12 GB, DDR5-5600 assumed, llama.cpp | Qwen3.6-35B-A3B UD-Q4_K_XL | 46% | 46 | 48 to 52 measured by Strata |
| V5 | RTX 5070 12 GB, 64 GB DDR5-6000 assumed, Strata, drafts | Qwen3.8-Flash-Next Q2_0 | 21% | 84 | 87.3 measured by Strata |
