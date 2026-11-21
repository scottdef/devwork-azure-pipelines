// Package planner decides which device holds each part of a model, given
// memory budgets. It is pure arithmetic: no device is touched, so the same
// code backs `lm9 plan`, the TUI's "fits here" column and the real loader.
package planner

import (
	"errors"
	"fmt"
)

// Part is one unit the planner can place: a transformer layer, the embedding
// table, the output head, or a group of mixture-of-experts weights.
type Part struct {
	Name    string
	Weights uint64 // bytes of weights
	State   uint64 // bytes of KV cache and scratch that must sit with it
	Expert  bool   // mixture-of-experts weights, read sparsely per token
}

// Device is a memory pool with a budget.
type Device struct {
	Name    string
	Budget  uint64 // --vram or --ram
	Reserve uint64 // headroom never handed out
}

// Request is everything the planner needs.
type Request struct {
	Parts        []Part   // in model order, input side first
	GPUs         []Device // in preference order
	CPU          Device
	MaxGPUParts  int  // --gpu-layers: -1 for auto
	ExpertsOnCPU bool // --moe-experts cpu
}

// Placement is one row of the plan.
type Placement struct {
	Part   string
	Device string
	Bytes  uint64
}

// Plan is the result: one placement per part, plus what each device ends up using.
type Plan struct {
	Rows []Placement
	Used map[string]uint64
}

// ErrNoFit means the model does not fit in the budgets given.
var ErrNoFit = errors.New("planner: model does not fit")

// Make places parts on GPUs starting from the output end of the model, as
// llama.cpp does when given a layer count, and puts the rest in system RAM.
// It fails if what is left over exceeds the CPU budget.
func Make(req Request) (*Plan, error) {
	plan := &Plan{Rows: make([]Placement, len(req.Parts)), Used: map[string]uint64{}}
	free := func(d Device) uint64 {
		if d.Reserve >= d.Budget {
			return 0
		}
		return d.Budget - d.Reserve - plan.Used[d.Name]
	}
	placed, gpu := 0, 0
	for i := len(req.Parts) - 1; i >= 0; i-- {
		p := req.Parts[i]
		need := p.Weights + p.State
		dev := req.CPU
		if !(p.Expert && req.ExpertsOnCPU) && (req.MaxGPUParts < 0 || placed < req.MaxGPUParts) {
			for gpu < len(req.GPUs) && free(req.GPUs[gpu]) < need {
				gpu++ // this card is full; later parts go to the next one
			}
			if gpu < len(req.GPUs) {
				dev = req.GPUs[gpu]
				placed++
			}
		}
		plan.Rows[i] = Placement{Part: p.Name, Device: dev.Name, Bytes: need}
		plan.Used[dev.Name] += need
	}
	if cpuFree := req.CPU.Budget - min(req.CPU.Budget, req.CPU.Reserve); plan.Used[req.CPU.Name] > cpuFree {
		return plan, fmt.Errorf("%w: needs %d bytes of system memory, budget allows %d",
			ErrNoFit, plan.Used[req.CPU.Name], cpuFree)
	}
	return plan, nil
}
