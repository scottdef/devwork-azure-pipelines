package order

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

var policy = Policy{
	Targets:  []string{"server1", "server3"},
	Commands: map[string]Mode{"uptime": Auto, "df -h": Auto, "sudo reboot": Confirm},
	Chords:   map[string]Mode{"ctrl+alt+t": Confirm, "ctrl+l": Auto},
}

func modeOf(c Call) (Mode, error) { return Validate(c, policy) }

func TestParseCompound(t *testing.T) {
	got, err := Parse(`switch to Server3 then press ctrl alt t; run df -h` + "\n" + `type "hello; then world"`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Call{
		{"select_target", map[string]any{"target": "server3"}},
		{"press_keys", map[string]any{"chord": "ctrl+alt+t"}},
		{"run_command", map[string]any{"command": "df -h"}},
		{"type_text", map[string]any{"text": "hello; then world"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %v\nwant %v", got, want)
	}
}

func TestParseForms(t *testing.T) {
	cases := map[string]Call{
		"click 512 300":            {"click", map[string]any{"x": 512, "y": 300}},
		`click "Log in"`:           {"click_text", map[string]any{"text": "Log in"}},
		"click on Save":            {"click_text", map[string]any{"text": "Save"}},
		"read screen":              {Name: "read_screen"},
		"read pdf manuals/bmc.pdf": {"read_pdf", map[string]any{"path": "manuals/bmc.pdf"}},
		"note bios.server3 = 2.19": {"note", map[string]any{"key": "bios.server3", "value": "2.19"}},
		"recall bios.server3":      {"recall", map[string]any{"key": "bios.server3"}},
		"runbook run nightly":      {"runbook_run", map[string]any{"name": "nightly"}},
		"please status":            {Name: "status"},
		"confirm":                  {Name: "confirm"},
	}
	for in, want := range cases {
		got, err := Parse(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
			t.Errorf("%q = %v, want %v", in, got, want)
		}
	}
	for _, bad := range []string{"", ";;", "frobnicate now", `type "unterminated`, "note k v", `"quoted" first`} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestValidate(t *testing.T) {
	ok := map[string]Mode{
		"target server1":   Auto,
		"run uptime":       Auto,
		"run sudo reboot":  Confirm,
		"press ctrl+l":     Auto,
		"press ctrl alt t": Confirm,
		`type "ls -la"`:    Confirm,
		"click 0 1000":     Confirm,
		"read screen":      Auto,
		"runbook nightly":  Confirm,
	}
	for in, want := range ok {
		calls, err := Parse(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if m, err := Validate(calls[0], policy); err != nil || m != want {
			t.Errorf("%q: mode %q err %v, want %q", in, m, err, want)
		}
	}
	bad := []string{
		"target server9",           // not a known target
		"run rm -rf /",             // not on the allowlist
		"press ctrl+alt+delete",    // chord not listed
		"click 5 1001",             // out of range
		`type "naïve"`,             // not typeable on the US layout
		"note Bad/Key = x",         // key shape
		"runbook ../../etc/passwd", // name shape
	}
	for _, in := range bad {
		calls, err := Parse(in)
		if err != nil {
			continue // rejected even earlier
		}
		if _, err := Validate(calls[0], policy); err == nil {
			t.Errorf("%q: expected the validator to reject it", in)
		}
	}
	if _, err := Validate(Call{Name: "format_disk"}, policy); err == nil {
		t.Error("an unknown tool was accepted")
	}
}

func TestGate(t *testing.T) {
	parse := func(s string) []Call {
		c, err := Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	cases := []struct {
		order string
		conf  float64
		want  Verdict
	}{
		{"target server3; run uptime", 1.0, Execute},
		{"target server3; run uptime", 0.80, NeedConfirm}, // heard, but not clearly
		{"target server3; run uptime", 0.40, Refuse},
		{"run sudo reboot", 1.0, NeedConfirm}, // mutating, however clearly heard
		{`type "hello"`, 0.99, NeedConfirm},
		{"run rm -rf /", 1.0, Refuse}, // not in the vocabulary at all
		{"read screen; status", 0.90, Execute},
	}
	for _, c := range cases {
		if got, why := Gate(parse(c.order), c.conf, modeOf); got != c.want {
			t.Errorf("%q @%.2f = %s (%s), want %s", c.order, c.conf, got, why, c.want)
		}
	}
	if v, _ := Gate(nil, 1, modeOf); v != Refuse {
		t.Error("an empty order was not refused")
	}
}

func TestToolsJSONMatchesPolicy(t *testing.T) {
	b, err := ToolsJSON(policy)
	if err != nil {
		t.Fatal(err)
	}
	var tools []struct {
		Name       string `json:"name"`
		Parameters struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		} `json:"parameters"`
	}
	if err := json.Unmarshal(b, &tools); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tl := range tools {
		seen[tl.Name] = true
		if tl.Name == "run_command" {
			got := strings.Join(tl.Parameters.Properties["command"].Enum, ",")
			if got != "df -h,sudo reboot,uptime" {
				t.Errorf("command enum = %q", got)
			}
		}
	}
	// Every tool offered to the model must be one the validator knows.
	for name := range seen {
		if name == "confirm" || name == "cancel" {
			continue
		}
		if _, err := Validate(Call{Name: name}, policy); err != nil && strings.HasPrefix(err.Error(), "unknown tool") {
			t.Errorf("tool %q is offered to the model but unknown to Validate", name)
		}
	}
}

func TestIntAcceptsModelSpellings(t *testing.T) {
	var c Call
	if err := json.Unmarshal([]byte(`{"name":"click","arguments":{"x":512,"y":"300"}}`), &c); err != nil {
		t.Fatal(err)
	}
	x, okx := c.Int("x")
	y, oky := c.Int("y")
	if !okx || !oky || x != 512 || y != 300 {
		t.Errorf("x=%d(%v) y=%d(%v)", x, okx, y, oky)
	}
}
