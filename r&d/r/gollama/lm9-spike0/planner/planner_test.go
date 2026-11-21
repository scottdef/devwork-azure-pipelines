package planner

import (
	"errors"
	"fmt"
	"testing"
)

const gib = 1 << 30

// model returns n dense layers of 500 MiB weights with 50 MiB of state each,
// plus one expert group per layer when experts is true.
func model(n int, experts bool) []Part {
	var parts []Part
	for i := 0; i < n; i++ {
		parts = append(parts, Part{Name: fmt.Sprintf("blk.%d", i), Weights: 500 << 20, State: 50 << 20})
		if experts {
			parts = append(parts, Part{Name: fmt.Sprintf("blk.%d.experts", i), Weights: 2 * gib, Expert: true})
		}
	}
	return parts
}

func count(p *Plan, dev string) (n int) {
	for _, r := range p.Rows {
		if r.Device == dev {
			n++
		}
	}
	return n
}

func TestEverythingFitsOnTheGPU(t *testing.T) {
	p, err := Make(Request{Parts: model(8, false), MaxGPUParts: -1,
		GPUs: []Device{{Name: "vulkan:0", Budget: 8 * gib}}, CPU: Device{Name: "cpu", Budget: 16 * gib}})
	if err != nil || count(p, "vulkan:0") != 8 {
		t.Fatalf("err=%v, on GPU=%d", err, count(p, "vulkan:0"))
	}
}

// A 4 GiB card with 0.5 GiB held back takes six 550 MiB layers (3.22 GiB);
// the seventh would need 3.76 GiB, so it and the rest stay in RAM. The layers
// on the card are the ones nearest the output.
func TestSmallCardTakesTheLastLayers(t *testing.T) {
	p, err := Make(Request{Parts: model(12, false), MaxGPUParts: -1,
		GPUs: []Device{{Name: "vulkan:0", Budget: 4 * gib, Reserve: gib / 2}},
		CPU:  Device{Name: "cpu", Budget: 32 * gib}})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(p, "vulkan:0"); n != 6 {
		t.Fatalf("%d layers on the GPU, want 6", n)
	}
	if p.Rows[11].Device != "vulkan:0" || p.Rows[0].Device != "cpu" {
		t.Fatalf("wrong end of the model was offloaded: first=%s last=%s", p.Rows[0].Device, p.Rows[11].Device)
	}
	if p.Used["vulkan:0"] > 4*gib-gib/2 {
		t.Fatalf("GPU budget exceeded: %d", p.Used["vulkan:0"])
	}
}

func TestExpertsStayInRAM(t *testing.T) {
	p, err := Make(Request{Parts: model(4, true), MaxGPUParts: -1, ExpertsOnCPU: true,
		GPUs: []Device{{Name: "vulkan:0", Budget: 4 * gib, Reserve: gib / 2}},
		CPU:  Device{Name: "cpu", Budget: 32 * gib}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range p.Rows {
		expert := len(r.Part) > 8 && r.Part[len(r.Part)-8:] == ".experts"
		if expert && r.Device != "cpu" {
			t.Fatalf("%s placed on %s", r.Part, r.Device)
		}
		if !expert && r.Device != "vulkan:0" {
			t.Fatalf("dense layer %s placed on %s", r.Part, r.Device)
		}
	}
}

func TestLayerLimitAndSecondCard(t *testing.T) {
	p, _ := Make(Request{Parts: model(12, false), MaxGPUParts: 3,
		GPUs: []Device{{Name: "vulkan:0", Budget: 8 * gib}}, CPU: Device{Name: "cpu", Budget: 32 * gib}})
	if n := count(p, "vulkan:0"); n != 3 {
		t.Fatalf("--gpu-layers 3 placed %d", n)
	}
	p, err := Make(Request{Parts: model(12, false), MaxGPUParts: -1,
		GPUs: []Device{{Name: "vulkan:0", Budget: 2 * gib}, {Name: "vulkan:1", Budget: 2 * gib}},
		CPU:  Device{Name: "cpu", Budget: 32 * gib}})
	if err != nil || count(p, "vulkan:0") != 3 || count(p, "vulkan:1") != 3 || count(p, "cpu") != 6 {
		t.Fatalf("two cards: err=%v plan=%v", err, p.Used)
	}
}

func TestDoesNotFit(t *testing.T) {
	_, err := Make(Request{Parts: model(12, false), MaxGPUParts: -1,
		CPU: Device{Name: "cpu", Budget: 4 * gib, Reserve: gib}})
	if !errors.Is(err, ErrNoFit) {
		t.Fatalf("err = %v, want ErrNoFit", err)
	}
}
