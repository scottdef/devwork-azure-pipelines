package form

import (
	"strings"
	"testing"
)

func TestLoadDir(t *testing.T) {
	forms, err := LoadDir("../../.github/ISSUE_TEMPLATE")
	if err != nil {
		t.Fatal(err)
	}
	if len(forms) != 3 {
		t.Fatalf("got %d forms, want 3 (config.yml skipped)", len(forms))
	}
	byFile := map[string]*Form{}
	for _, f := range forms {
		byFile[f.File] = f
	}
	r := byFile["request.yml"]
	if r == nil {
		t.Fatal("request.yml missing")
	}
	if r.Title != "[request] " || strings.Join(r.Labels, ",") != "issueops,request" {
		t.Errorf("request header: %q %v", r.Title, r.Labels)
	}
	var ids []string
	for _, f := range r.Inputs() {
		ids = append(ids, f.ID)
	}
	if got := strings.Join(ids, " "); got != "source-id template-id phase payload notes" {
		t.Errorf("ids = %s", got)
	}
	tmpl, _ := r.Field("template-id")
	if len(tmpl.Options) != 6 || tmpl.Options[5] != "readme" || tmpl.Default != 0 {
		t.Errorf("template-id = %+v", tmpl)
	}
	ph, _ := r.Field("phase")
	if strings.Join(ph.Options, "") != "123" || !ph.Required {
		t.Errorf("phase = %+v", ph)
	}
	pl, _ := r.Field("payload")
	if pl.Render != "json" || !pl.Required || !pl.Prefillable() {
		t.Errorf("payload = %+v", pl)
	}
	if n, _ := r.Field("notes"); n.Required {
		t.Error("notes must be optional")
	}
	if d, _ := byFile["borrow.yml"].Field("days"); strings.Join(d.Options, ",") != "7,14" || d.Prefillable() {
		t.Errorf("days = %+v", d)
	}
}

func TestCheckboxesForm(t *testing.T) {
	f, err := Load("../../testdata/forms/checks.yml")
	if err != nil {
		t.Fatal(err)
	}
	env, _ := f.Field("environments")
	if strings.Join(env.Options, ",") != "staging,production,sandbox" {
		t.Errorf("environments = %+v", env)
	}
	reg, _ := f.Field("regions")
	if !reg.Multiple || len(reg.Options) != 3 {
		t.Errorf("regions = %+v", reg)
	}
}

func TestParseErrors(t *testing.T) {
	for name, src := range map[string]string{
		"no name":   "body:\n  - type: input\n    id: a\n    attributes:\n      label: A\n",
		"no body":   "name: x\n",
		"no id":     "name: x\nbody:\n  - type: input\n    attributes:\n      label: A\n",
		"dup id":    "name: x\nbody:\n  - type: input\n    id: a\n    attributes:\n      label: A\n  - type: input\n    id: a\n    attributes:\n      label: B\n",
		"bad type":  "name: x\nbody:\n  - type: slider\n    id: a\n    attributes:\n      label: A\n",
		"no option": "name: x\nbody:\n  - type: dropdown\n    id: a\n    attributes:\n      label: A\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
