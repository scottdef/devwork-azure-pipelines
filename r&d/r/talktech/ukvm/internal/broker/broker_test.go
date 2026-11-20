package broker

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func registry(t *testing.T, now time.Time) *Registry {
	t.Helper()
	r, err := New([]Node{
		{Name: "cos", Tier: Self, Caps: []string{"ocr", "pdf", "stt", "hands:server1"}},
		{Name: "pi3", Tier: Staff, Caps: []string{"ocr", "pdf", "hands:server3"}, Seen: now},
		{Name: "coral", Tier: Staff, Caps: []string{"ocr", "tpu"}, Seen: now},
		{Name: "bigbox", Tier: Lender, SSH: "ops@bigbox", Caps: []string{"ocr", "pdf", "exec:bigbox"}, Seen: now},
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func place(t *testing.T, r *Registry, need Need, now time.Time) string {
	t.Helper()
	n, err := r.Place(need, now)
	if err != nil {
		t.Fatalf("place %q: %v", need.Cap, err)
	}
	return n.Name
}

func TestLenderBeforeStaffBeforeSelf(t *testing.T) {
	now := time.Unix(1000, 0)
	r := registry(t, now)

	if got := place(t, r, Need{Cap: "pdf"}, now); got != "bigbox" {
		t.Errorf("pdf went to %s, want the lender", got)
	}
	// The lender drops off the network: staff boards take over.
	later := now.Add(2 * time.Minute)
	r.Touch("pi3", later)
	r.Touch("coral", later)
	if got := place(t, r, Need{Cap: "pdf"}, later); got != "pi3" {
		t.Errorf("pdf went to %s, want the staff board", got)
	}
	// Everything else is down: only now is the chief's own OS used.
	muchLater := later.Add(time.Hour)
	if got := place(t, r, Need{Cap: "pdf"}, muchLater); got != "cos" {
		t.Errorf("pdf went to %s, want self as the last resort", got)
	}
}

func TestLoadAndMemory(t *testing.T) {
	now := time.Unix(1000, 0)
	r := registry(t, now)
	if err := r.Beat("pi3", 0.2, 600, now); err != nil {
		t.Fatal(err)
	}
	if err := r.Beat("coral", 1.5, 900, now); err != nil {
		t.Fatal(err)
	}
	r.Touch("bigbox", now.Add(-time.Hour)) // lender is stale

	if got := place(t, r, Need{Cap: "ocr"}, now); got != "pi3" {
		t.Errorf("ocr went to %s, want the idler staff board", got)
	}
	if got := place(t, r, Need{Cap: "ocr", MemMB: 800}, now); got != "coral" {
		t.Errorf("800 MB job went to %s, want the board with room", got)
	}
	if err := r.Beat("pi3", 9, 600, now); err != nil { // overloaded
		t.Fatal(err)
	}
	if got := place(t, r, Need{Cap: "ocr"}, now); got != "coral" {
		t.Errorf("ocr went to %s, want the board that is not overloaded", got)
	}
}

func TestTargetBoundCapsAndMisses(t *testing.T) {
	now := time.Unix(1000, 0)
	r := registry(t, now)
	if got := place(t, r, Need{Cap: "hands:server3"}, now); got != "pi3" {
		t.Errorf("hands:server3 is on %s, want pi3", got)
	}
	if got := place(t, r, Need{Cap: "hands:server1"}, now); got != "cos" {
		t.Errorf("hands:server1 is on %s, want cos", got)
	}
	if _, err := r.Place(Need{Cap: "hands:server9"}, now); !errors.Is(err, ErrNoProvider) {
		t.Errorf("err = %v, want ErrNoProvider", err)
	}
	// A down satellite must not be handed its target's keyboard.
	if _, err := r.Place(Need{Cap: "hands:server3"}, now.Add(time.Hour)); !errors.Is(err, ErrNoProvider) {
		t.Errorf("stale satellite was still placed: %v", err)
	}
}

func TestNodeJSONAndDuplicates(t *testing.T) {
	var nodes []Node
	in := `[{"name":"a","tier":"lender","ssh":"u@a","caps":["pdf"]},{"name":"b","tier":"Self","caps":[]}]`
	if err := json.Unmarshal([]byte(in), &nodes); err != nil {
		t.Fatal(err)
	}
	if nodes[0].Tier != Lender || nodes[1].Tier != Self {
		t.Errorf("tiers = %v %v", nodes[0].Tier, nodes[1].Tier)
	}
	if err := json.Unmarshal([]byte(`[{"name":"a","tier":"boss"}]`), &nodes); err == nil {
		t.Error("unknown tier was accepted")
	}
	if _, err := New([]Node{{Name: "x"}, {Name: "x"}}); err == nil {
		t.Error("duplicate node names were accepted")
	}
}
