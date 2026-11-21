// p340sim is a first-order model of memory fit and turn time for local LLM
// lanes on one machine. It is arithmetic, not a benchmark: every constant is
// either a published figure, a calibration point, or a labelled assumption.
//
// Decode is treated as memory-bandwidth bound: a token costs the bytes of
// weights read for it, divided by the effective bandwidth of the pool they
// sit in. Prompt reading is treated as compute bound: 2 FLOP per active
// parameter per token. Replace the constants with your own measurements and
// rerun; that is the point of shipping the source.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Machine holds the resources and the effective rates of one computer.
type Machine struct {
	Name     string
	RAM      float64 // GB installed
	OS       float64 // GB held by OS, desktop, Studio idle
	VRAM     float64 // GB on the card
	VRAMRes  float64 // GB held by display server and CUDA context
	CPUBW    float64 // GB/s the CPU effectively reads weights at
	GPUBW    float64 // GB/s the GPU effectively reads weights at
	CPUFlops float64 // effective FLOP/s for prompt reading
	GPUFlops float64
	TokOvh   float64 // seconds of fixed overhead per decoded token
}

// Model describes one weight file in the terms the rate model needs.
type Model struct {
	Name     string
	File     float64 // GB on disk
	Dense    float64 // GB read on every token
	Expert   float64 // GB of routed experts in total (0 for dense models)
	Active   float64 // fraction of Expert read per token
	DenseP   float64 // billions of parameters in the dense part
	ExpertP  float64 // billions of active expert parameters per token
	KVKB     float64 // KB of GPU memory per context token
	EngineGB float64 // GB of fixed GPU reserve the engine asks for
	DraftGB  float64 // GB of GPU memory the draft (MTP) layer needs
}

// Lane is one model placed on one machine by one engine.
type Lane struct {
	M        Model
	DenseGPU float64 // fraction of Dense on the GPU
	ExpGPU   float64 // fraction of Expert cached on the GPU
	Engine   float64 // kernel efficiency against llama.cpp = 1.0
	Draft    float64 // decode multiplier from draft tokens, 1.0 = off
	Ctx      float64 // context tokens per slot
	Slots    float64
	MoEPen   float64 // CPU prompt penalty for routed experts, 1.0 = none
}

func (l Lane) vram() float64 {
	g := l.M.Dense*l.DenseGPU + l.M.Expert*l.ExpGPU
	if g == 0 {
		return 0
	}
	v := g + l.M.EngineGB + l.Slots*l.Ctx*l.M.KVKB/1e6
	if l.Draft > 1 {
		v += l.M.DraftGB
	}
	return v
}

func (l Lane) ram() float64 {
	return l.M.File - l.M.Dense*l.DenseGPU - l.M.Expert*l.ExpGPU
}

// decode returns tokens per second for one conversation.
func (l Lane) decode(mc Machine) float64 {
	dg := l.M.Dense * l.DenseGPU
	dc := l.M.Dense - dg
	e := l.M.Expert * l.M.Active
	eg := e * l.ExpGPU
	ec := e - eg
	t := (dg+eg)/mc.GPUBW + (dc+ec)/mc.CPUBW + mc.TokOvh
	return l.Engine * l.Draft / t
}

// prefill returns prompt tokens read per second.
func (l Lane) prefill(mc Machine) float64 {
	gp := l.M.DenseP*l.DenseGPU + l.M.ExpertP*l.ExpGPU
	cd := l.M.DenseP * (1 - l.DenseGPU)
	ce := l.M.ExpertP * (1 - l.ExpGPU)
	pen := l.MoEPen
	if pen == 0 {
		pen = 1
	}
	t := 2e9*gp/mc.GPUFlops + 2e9*cd/mc.CPUFlops + 2e9*ce/(mc.CPUFlops*pen)
	return l.Engine / t
}

// batch is the aggregate decode gain of n conversations in one engine with
// most experts off the card. Points are Strata's RTX 3060 hybrid table
// (QWEN36.md): 1.00, 1.33, 1.55 at 1, 2, 4 requests; 3 is interpolated.
func batch(n int) float64 {
	switch {
	case n <= 1:
		return 1
	case n == 2:
		return 1.33
	case n == 3:
		return 1.45
	}
	return 1.55
}

// Workload is one turn: prompt tokens that must be read, tokens written.
type Workload struct {
	Name   string
	Prompt float64
	Out    float64
}

// Result is one pattern under one workload.
type Result struct {
	TTFT  float64 `json:"ttft_s"`
	Total float64 `json:"total_s"`
	Over  bool    `json:"over_context,omitempty"` // the turn does not fit the lane's context
}

// Row is one simulated pattern.
type Row struct {
	ID      string            `json:"id"`
	Pattern string            `json:"pattern"`
	Machine string            `json:"machine"`
	RAMUse  float64           `json:"ram_gb"`
	RAMCap  float64           `json:"ram_cap_gb"`
	VRAMUse float64           `json:"vram_gb"`
	VRAMCap float64           `json:"vram_cap_gb"`
	Decode  float64           `json:"decode_tps"`
	Prefill float64           `json:"prefill_tps"`
	Turns   map[string]Result `json:"turns"`
	Fit     string            `json:"fit"`
	Note    string            `json:"note"`
}

func one(l Lane, mc Machine, w Workload) Result {
	if l.Ctx > 0 && w.Prompt+w.Out > l.Ctx {
		return Result{Over: true}
	}
	p := w.Prompt / l.prefill(mc)
	return Result{TTFT: p, Total: p + w.Out/l.decode(mc)}
}

func fit(ram, ramCap, vram, vramCap float64) string {
	switch {
	case ram > ramCap && vram > vramCap:
		return "no: RAM and VRAM"
	case ram > ramCap:
		return "no: RAM"
	case vram > vramCap:
		return "no: VRAM"
	case ramCap-ram < 3 || vramCap-vram < 0.5:
		return "tight"
	}
	return "fits"
}

func main() {
	format := flag.String("format", "md", "output format: md, csv or json")
	flag.Parse()

	// Calibration. Quadro P1000, Q4_K_M, tokens/s as quoted in the May 2026
	// guide (its source: a published Ollama P1000 benchmark). Sizes are the
	// GGUF file sizes. A least-squares line t = size/BW + c gives the card's
	// effective bandwidth and the fixed cost per token.
	cal := []struct {
		name    string
		gb, tps float64
	}{
		{"tinyllama 1.1B", 0.64, 62}, {"qwen2.5 1.5B", 0.99, 34},
		{"qwen2.5 3B", 1.93, 18}, {"llama3.2 3B", 2.02, 20}, {"phi3.5 3.8B", 2.20, 19},
	}
	var sx, sy, sxx, sxy float64
	for _, c := range cal {
		x, y := c.gb, 1/c.tps
		sx, sy, sxx, sxy = sx+x, sy+y, sxx+x*x, sxy+x*y
	}
	n := float64(len(cal))
	slope := (n*sxy - sx*sy) / (n*sxx - sx*sx)
	icpt := (sy - slope*sx) / n
	if icpt < 0 {
		icpt = 0
	}
	gpuBW := 1 / slope

	// CPU: dual-channel DDR4 moves 2 x 8 bytes x MT/s. llama.cpp reaches a
	// bit over half of that; 0.55 matches the May guide's 6 / 3 / 1.5
	// tokens/s rule of thumb for 7B / 13B / 27B Q4 files on this CPU.
	ddr := func(mts float64) float64 { return 2 * 8 * mts / 1000 * 0.55 }

	stock := Machine{
		Name: "P340 stock (32 GB DDR4-2933, P1000 4 GB)",
		RAM:  32, OS: 4, VRAM: 4, VRAMRes: 0.5,
		CPUBW: ddr(2933), GPUBW: gpuBW,
		CPUFlops: 0.42e12,        // assumption: about 30 prompt tokens/s on a 7B model
		GPUFlops: 0.5 * 1.894e12, // assumption: half of the card's FP32 peak
		TokOvh:   icpt,
	}
	stick := stock
	stick.Name = "P340 + 16 GB stick (48 GB at DDR4-2133)"
	stick.RAM, stick.CPUBW = 48, ddr(2133)

	// Models. File sizes and per-token figures are cited in the guide.
	q36 := Model{Name: "Qwen3.6-35B-A3B UD-Q4_K_XL", File: 22.9, Dense: 1.9,
		Expert: 19.5, Active: 8.0 / 256, DenseP: 1.9, ExpertP: 1.1,
		KVKB: 45, EngineGB: 0.83, DraftGB: 1.0}
	q4b := Model{Name: "Qwen3.5-4B Q4_K_M", File: 2.78, Dense: 2.78, DenseP: 4.0, KVKB: 75}
	q9b := Model{Name: "Qwen3.5-9B Q4_K_M", File: 5.63, Dense: 5.63, DenseP: 9.0, KVKB: 90}
	oss := Model{Name: "gpt-oss-20b MXFP4", File: 12.11, Dense: 0.7, Expert: 11.4,
		Active: 4.0 / 32, DenseP: 1.0, ExpertP: 2.6, KVKB: 60}
	fnx := Model{Name: "Qwen3.8-Flash-Next Q2_0", File: 37.6, Dense: 3.3, Expert: 34,
		Active: 0.66 / 34, DenseP: 2.0, ExpertP: 4.0, KVKB: 13, EngineGB: 0, DraftGB: 0.8}

	W := []Workload{
		{"chat turn (1,500 in, 300 out)", 1500, 300},
		{"agent step (500 new in, 150 out)", 500, 150},
		{"cold document (6,000 in, 500 out)", 6000, 500},
	}

	var rows []Row
	add := func(id, pat string, mc Machine, ram, vram, dec, pre float64, note string, f func(Workload) Result) {
		r := Row{ID: id, Pattern: pat, Machine: mc.Name, RAMUse: ram + mc.OS, RAMCap: mc.RAM,
			VRAMUse: vram + mc.VRAMRes, VRAMCap: mc.VRAM, Decode: dec, Prefill: pre,
			Turns: map[string]Result{}, Note: note}
		r.Fit = fit(r.RAMUse, r.RAMCap, r.VRAMUse, r.VRAMCap)
		for _, w := range W {
			r.Turns[w.Name] = f(w)
		}
		rows = append(rows, r)
	}

	// A. Strata, Qwen3.6, dense on the card, every expert on the CPU.
	// Engine 1.2: Strata 59.7 against llama.cpp 48-52 tokens/s without
	// drafts on an RTX 3060 in the same placement (QWEN36.md).
	a := Lane{M: q36, DenseGPU: 1, Engine: 1.2, Draft: 1, Ctx: 8192, Slots: 1, MoEPen: 0.7}
	add("A", "Strata, Qwen3.6-35B-A3B, dense on GPU, no drafts", stock, a.ram(), a.vram(),
		a.decode(stock), a.prefill(stock), "engine start on a 4 GB card is unreported",
		func(w Workload) Result { return one(a, stock, w) })
	ad := a
	ad.Draft = 1.42 // 59.7 -> 84.7 tokens/s with --mtp 2 on the same RTX 3060
	add("A+", "Strata, same, drafts on", stock, ad.ram(), ad.vram(),
		ad.decode(stock), ad.prefill(stock), "draft layer needs about 1 GB more VRAM",
		func(w Workload) Result { return one(ad, stock, w) })

	// B. llama-server, same file, same placement, and CPU only.
	b := a
	b.Engine = 1
	b.M.EngineGB = 0.3
	add("B", "llama-server, same file, dense on GPU, experts on CPU", stock, b.ram(), b.vram(),
		b.decode(stock), b.prefill(stock), "documented path (--n-cpu-moe)",
		func(w Workload) Result { return one(b, stock, w) })
	bc := b
	bc.DenseGPU = 0
	add("B-cpu", "llama-server, same file, CPU only", stock, bc.ram(), bc.vram(),
		bc.decode(stock), bc.prefill(stock), "GPU left free",
		func(w Workload) Result { return one(bc, stock, w) })

	// C. Router on the card, MoE on the CPU, 70 % of steps stay on the router.
	rt := Lane{M: q4b, DenseGPU: 1, Engine: 1, Draft: 1, Ctx: 4096, Slots: 1}
	add("C", "Qwen3.5-4B router on GPU + MoE on CPU, 70/30 split", stock,
		bc.ram(), rt.vram(), 0.7*rt.decode(stock)+0.3*bc.decode(stock),
		0.7*rt.prefill(stock)+0.3*bc.prefill(stock), "rates are the 70/30 blend",
		func(w Workload) Result {
			x, y := one(rt, stock, w), one(bc, stock, w)
			if x.Over { // too long for the router's context: the MoE takes it alone
				return y
			}
			return Result{TTFT: 0.7*x.TTFT + 0.3*y.TTFT, Total: 0.7*x.Total + 0.3*y.Total}
		})

	// D. Council: three different proposers on the CPU, aggregator on the card.
	props := []Lane{
		{M: q4b, Engine: 1, Draft: 1}, {M: q9b, Engine: 1, Draft: 1},
		{M: oss, Engine: 1, Draft: 1, MoEPen: 0.7},
	}
	var pram float64
	for _, p := range props {
		pram += p.ram()
	}
	add("D", "Council: 4B + 9B + gpt-oss-20b on CPU, 4B aggregator on GPU", stock,
		pram, rt.vram(), rt.decode(stock), rt.prefill(stock),
		"rates shown are the aggregator's",
		func(w Workload) Result {
			if w.Prompt+4*w.Out > rt.Ctx { // aggregator reads the prompt and three answers
				return Result{Over: true}
			}
			var t float64 // proposers share one memory bus: times add
			for _, p := range props {
				t += w.Prompt/p.prefill(stock) + w.Out/p.decode(stock)
			}
			t += (w.Prompt + 3*w.Out) / rt.prefill(stock)
			return Result{TTFT: t, Total: t + w.Out/rt.decode(stock)}
		})
	// D-self. Three samples of one model in three slots, majority vote.
	sc := b
	sc.Ctx, sc.Slots = 4096, 3
	add("D-self", "Self-consistency: 3 samples of the MoE in 3 slots, vote", stock,
		sc.ram(), sc.vram(), sc.decode(stock)*batch(3), sc.prefill(stock),
		"decode is the 3-slot aggregate",
		func(w Workload) Result {
			if w.Prompt+w.Out > sc.Ctx {
				return Result{Over: true}
			}
			p := 3 * w.Prompt / sc.prefill(stock)
			d := 3 * w.Out / (sc.decode(stock) * batch(3))
			return Result{TTFT: p, Total: p + d}
		})

	// E. The 16 GB DDR4-2133 stick: 48 GB, every DIMM drops to 2133 MT/s.
	e := Lane{M: fnx, DenseGPU: 1, Engine: 1.2, Draft: 1, Ctx: 8192, Slots: 1, MoEPen: 0.7}
	add("E", "Strata, Flash-Next Q2_0, dense on GPU, no drafts", stick, e.ram(), e.vram(),
		e.decode(stick), e.prefill(stick), "far below Strata's documented card sizes",
		func(w Workload) Result { return one(e, stick, w) })
	add("E-B", "Pattern B again on the 48 GB machine", stick, b.ram(), b.vram(),
		b.decode(stick), b.prefill(stick), "shows the cost of the downclock",
		func(w Workload) Result { return one(b, stick, w) })

	// ---- Upgrades hosted by the P340 (Appendix A of the guide) ----
	// A card's effective bandwidth is its rated bandwidth times the fraction
	// the P1000 calibration reached (41.5 of a rated 80 GB/s). Whatever VRAM
	// is left after the dense weights, the engine reserve, the conversation
	// and the draft layer caches experts; the rest are read by the CPU.
	gpuEff := gpuBW / 80
	withCard := func(name string, base Machine, ram, cpuBW, vram, rated, tflops float64) Machine {
		m := base
		m.Name, m.RAM, m.CPUBW = name, ram, cpuBW
		m.VRAM, m.GPUBW, m.GPUFlops = vram, rated*gpuEff, 0.5*tflops*1e12
		return m
	}
	place := func(l Lane, mc Machine) Lane {
		l.DenseGPU, l.ExpGPU = 1, 0
		free := mc.VRAM - mc.VRAMRes - l.vram()
		if l.M.Expert > 0 && free > 0 {
			l.ExpGPU = free / l.M.Expert
			if l.ExpGPU > 1 {
				l.ExpGPU = 1
			}
		}
		return l
	}
	type upRow struct {
		ID, What, Model string
		ExpGPU          float64
		Decode, Prefill float64
		Chat, Doc       float64
		Published       string
	}
	var ups, checks []upRow
	up := func(dst *[]upRow, id, what string, mc Machine, l Lane, published string) {
		l = place(l, mc)
		r := upRow{ID: id, What: what, Model: l.M.Name, ExpGPU: l.ExpGPU, Decode: l.decode(mc), Published: published}
		// Prompt reading is modelled only for Qwen3.6. For Flash-Next, Strata
		// streams experts to the card while it reads, which this model does
		// not describe; its published prompt speeds are far higher.
		if l.M.Name == q36.Name {
			c := one(l, mc, W[0])
			d := one(l, mc, Workload{"document", 6000, 500})
			r.Prefill, r.Chat, r.Doc = l.prefill(mc), c.Total, d.Total
		}
		*dst = append(*dst, r)
	}
	q36s := Lane{M: q36, Engine: 1.2, Draft: 1.42, Ctx: 32768, Slots: 1, MoEPen: 0.7}
	q36l := Lane{M: q36, Engine: 1.0, Draft: 1, Ctx: 32768, Slots: 1, MoEPen: 0.7}
	q36l.M.EngineGB = 0.3
	// Flash-Next: Strata against llama.cpp on an M5 Max is 108.2 against 56.6
	// tokens/s with drafts, a combined 1.9; split here as 1.2 x 1.6.
	fnxs := Lane{M: fnx, Engine: 1.2, Draft: 1.6, Ctx: 32768, Slots: 1, MoEPen: 0.7}
	p64 := func(name string, vram, rated, tflops float64) Machine {
		return withCard(name, stock, 64, ddr(2933), vram, rated, tflops)
	}
	a2000 := p64("P340, 64 GB, RTX A2000 12 GB in the slot", 12, 288, 8.0)
	ti16 := p64("P340, 64 GB, RTX 5060 Ti 16 GB on a dock", 16, 448, 23.7)
	r3090 := p64("P340, 64 GB, RTX 3090 24 GB on a dock", 24, 936, 35.6)
	up(&ups, "U1", a2000.Name, a2000, q36s, "")
	up(&ups, "U1-F", a2000.Name, a2000, fnxs, "")
	up(&ups, "U2", ti16.Name, ti16, q36s, "")
	up(&ups, "U2-L", ti16.Name+", llama-server", ti16, q36l, "about 42 estimated by Llama Builds")
	up(&ups, "U2-F", ti16.Name, ti16, fnxs, "")
	ada := p64("P340, 64 GB, RTX 2000 Ada 16 GB in the slot (256 GB/s assumed)", 16, 256, 12.0)
	up(&ups, "U2-A", ada.Name, ada, q36s, "")
	up(&ups, "U3", r3090.Name, r3090, q36s, "")
	up(&ups, "U3-F", r3090.Name, r3090, fnxs, "")

	// ---- The same model against figures others published ----
	// CPU memory speeds of those machines are assumptions (DDR5-5600/6000).
	nodraft := q36s
	nodraft.Draft, nodraft.Ctx = 1, 8192
	llama := q36l
	llama.Ctx = 8192
	r9700 := withCard("Radeon AI PRO R9700 32 GB", stock, 64, ddr(5600), 32, 640, 48)
	r3060 := withCard("RTX 3060 12 GB, DDR5-5600 assumed", stock, 64, ddr(5600), 12, 360, 12.7)
	r5070 := withCard("RTX 5070 12 GB, 64 GB DDR5-6000 assumed", stock, 64, ddr(6000), 12, 672, 31)
	up(&checks, "V1", r9700.Name+", Strata, no drafts", r9700, nodraft, "83.9 measured by Strata")
	up(&checks, "V2", r9700.Name+", llama.cpp", r9700, llama, "73.8 measured by Strata")
	up(&checks, "V3", r3060.Name+", Strata, no drafts", r3060, nodraft, "59.7 measured by Strata")
	up(&checks, "V4", r3060.Name+", llama.cpp", r3060, llama, "48 to 52 measured by Strata")
	fn1k := fnxs
	fn1k.Ctx = 8192
	up(&checks, "V5", r5070.Name+", Strata, drafts", r5070, fn1k, "87.3 measured by Strata")

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(struct {
			GPUBW, TokOvhMs, CPUBW2933, CPUBW2133 float64
			Rows                                  []Row
			Upgrades, Checks                      []upRow
		}{gpuBW, icpt * 1000, ddr(2933), ddr(2133), rows, ups, checks})
	case "csv":
		fmt.Println("id,pattern,ram_gb,ram_cap,vram_gb,vram_cap,decode_tps,prefill_tps,chat_ttft,chat_total,step_ttft,step_total,doc_ttft,doc_total,fit")
		for _, r := range rows {
			fmt.Printf("%s,%q,%.1f,%.0f,%.2f,%.0f,%.1f,%.0f", r.ID, r.Pattern, r.RAMUse, r.RAMCap, r.VRAMUse, r.VRAMCap, r.Decode, r.Prefill)
			for _, w := range W {
				if x := r.Turns[w.Name]; x.Over {
					fmt.Print(",,")
				} else {
					fmt.Printf(",%.0f,%.0f", x.TTFT, x.Total)
				}
			}
			fmt.Printf(",%s\n", r.Fit)
		}
	default:
		fmt.Printf("Calibration: P1000 effective bandwidth %.1f GB/s, fixed cost %.1f ms/token\n", gpuBW, icpt*1000)
		for _, c := range cal {
			fmt.Printf("  %-16s measured %4.0f tok/s, model %4.1f\n", c.name, c.tps, 1/(c.gb/gpuBW+icpt))
		}
		fmt.Printf("CPU effective bandwidth: %.1f GB/s at DDR4-2933, %.1f GB/s at DDR4-2133\n\n", ddr(2933), ddr(2133))
		fmt.Println("| ID | Pattern | RAM GB | VRAM GB | Decode tok/s | Prompt tok/s | Chat turn s | Agent step s | Cold document s | Fit |")
		fmt.Println("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |")
		for _, r := range rows {
			var t []string
			for _, w := range W {
				if x := r.Turns[w.Name]; x.Over {
					t = append(t, "over context")
				} else {
					t = append(t, fmt.Sprintf("%.0f", x.Total))
				}
			}
			fmt.Printf("| %s | %s | %.1f of %.0f | %.2f of %.0f | %.1f | %.0f | %s | %s |\n", r.ID, r.Pattern,
				r.RAMUse, r.RAMCap, r.VRAMUse, r.VRAMCap, r.Decode, r.Prefill, strings.Join(t, " | "), r.Fit)
		}
		fmt.Println("\nUpgrades hosted by the P340, Strata with drafts at 32K unless noted (modelled)")
		fmt.Println()
		fmt.Println("| ID | Machine | Model | Experts on card | Decode tok/s | Prompt tok/s | Chat turn s | Document s | Others report |")
		fmt.Println("| --- | --- | --- | --- | --- | --- | --- | --- | --- |")
		for _, u := range ups {
			rest := "not modelled | not modelled | not modelled"
			if u.Prefill > 0 {
				rest = fmt.Sprintf("%.0f | %.0f | %.0f", u.Prefill, u.Chat, u.Doc)
			}
			fmt.Printf("| %s | %s | %s | %.0f%% | %.0f | %s | %s |\n", u.ID, u.What, u.Model,
				100*u.ExpGPU, u.Decode, rest, u.Published)
		}
		fmt.Println("\nThe model against published figures (decode only)")
		fmt.Println()
		fmt.Println("| ID | Machine and engine | Model | Experts on card | Modelled tok/s | Published tok/s |")
		fmt.Println("| --- | --- | --- | --- | --- | --- |")
		for _, u := range checks {
			fmt.Printf("| %s | %s | %s | %.0f%% | %.0f | %s |\n", u.ID, u.What, u.Model, 100*u.ExpGPU, u.Decode, u.Published)
		}
	}
}
