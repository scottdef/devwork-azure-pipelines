package metrics

import (
	"strings"
	"testing"
)

func TestTextFormat(t *testing.T) {
	r := New()
	r.Inc("adoclone_items_total", "component", "repos", "side", "target")
	r.Add("adoclone_items_total", 2, "side", "target", "component", "repos")
	r.Set("adoclone_phase_status", Done, "component", `a"b`)
	var b strings.Builder
	r.WriteText(&b)
	out := b.String()
	for _, want := range []string{
		"# TYPE adoclone_items_total counter\n",
		`adoclone_items_total{component="repos",side="target"} 3` + "\n",
		`adoclone_phase_status{component="a\"b"} 2` + "\n",
		"# TYPE adoclone_phase_status gauge\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	var nilReg *Registry
	nilReg.Inc("adoclone_errors_total") // must not panic
}
