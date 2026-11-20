package main

import (
	"path/filepath"
	"testing"

	"ukvm/internal/broker"
	"ukvm/internal/order"
)

// The example configurations ship in the repository and get copied to real
// nodes, so they are tested like code: they must load, every allowlisted
// command and chord must be something the keyboard can actually produce,
// and every tether must belong to a declared target and a declared node.
func TestShippedConfigsAreUsable(t *testing.T) {
	files, _ := filepath.Glob("../../deploy/*.json")
	if len(files) < 2 {
		t.Fatalf("expected the example configs in deploy/, found %v", files)
	}
	for _, f := range files {
		cfg, err := loadConfig(f)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		for cmd := range cfg.Policy.Commands {
			if _, err := order.Validate(order.Call{Name: "run_command", Args: map[string]any{"command": cmd}}, cfg.Policy); err != nil {
				t.Errorf("%s: command %q: %v", f, cmd, err)
			}
		}
		for chord := range cfg.Policy.Chords {
			if _, err := order.Validate(order.Call{Name: "press_keys", Args: map[string]any{"chord": chord}}, cfg.Policy); err != nil {
				t.Errorf("%s: chord %q: %v", f, chord, err)
			}
		}
		reg, err := broker.New(cfg.Nodes)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		var self *broker.Node
		for _, n := range reg.Snapshot() {
			if n.Name == cfg.Self {
				n := n
				self = &n
			}
		}
		if self == nil || self.Tier != broker.Self {
			t.Errorf("%s: self %q must be a node with tier \"self\"", f, cfg.Self)
			continue
		}
		targets := map[string]bool{}
		for _, tg := range cfg.Policy.Targets {
			targets[tg] = true
		}
		for tg, th := range cfg.Tethers {
			if !targets[tg] {
				t.Errorf("%s: tether for %q, which is not a policy target", f, tg)
			}
			if th.Hands != "" && !self.Has("hands:"+tg) {
				t.Errorf("%s: %s has hands for %q but does not advertise hands:%s", f, cfg.Self, tg, tg)
			}
			if th.Eyes != "" && !self.Has("eyes:"+tg) {
				t.Errorf("%s: %s has eyes for %q but does not advertise eyes:%s", f, cfg.Self, tg, tg)
			}
		}
		if _, err := order.ToolsJSON(cfg.Policy); err != nil {
			t.Errorf("%s: tool schema: %v", f, err)
		}
	}
}
