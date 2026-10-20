package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func TestCases(t *testing.T) {
	s, err := Load("../../schema/request.json")
	if err != nil {
		t.Fatal(err)
	}
	good := map[string]any{
		"source-id": "payments-api", "template-id": "caddyfile", "phase": "1",
		"payload": `{"name":"x"}`,
	}
	if errs := s.Check(good); len(errs) != 0 {
		t.Fatalf("good doc: %v", errs)
	}
	cases := []struct {
		name string
		doc  map[string]any
		want string // field:rule, comma separated
	}{
		{"missing", map[string]any{}, "payload:required,phase:required,source-id:required,template-id:required"},
		{"pattern", with(good, "source-id", "Payments_API"), "source-id:pattern"},
		{"short", with(good, "source-id", "a"), "source-id:minLength,source-id:pattern"},
		{"enum", with(good, "phase", "9"), "phase:enum"},
		{"badjson", with(good, "payload", `{"a":`), "payload:json"},
		{"array", with(good, "payload", `[1]`), "payload:jsonType"},
		{"big", with(good, "payload", `{"a":"`+strings.Repeat("x", 32768)+`"}`), "payload:maxLength"},
		{"extra", with(good, "evil", "x"), "evil:additional"},
		{"type", with(good, "phase", []any{"1"}), "phase:type"},
	}
	for _, c := range cases {
		var got []string
		for _, e := range s.Check(c.doc) {
			got = append(got, e.Field+":"+e.Rule)
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("%s: got %v, want %s", c.name, got, c.want)
		}
	}
}

func with(m map[string]any, k string, v any) map[string]any {
	out := map[string]any{}
	for a, b := range m {
		out[a] = b
	}
	out[k] = v
	return out
}

func TestRulesFilesLoad(t *testing.T) {
	files, _ := filepath.Glob("../../schema/*.json")
	if len(files) != 3 {
		t.Fatalf("want 3 rules files, got %v", files)
	}
	for _, f := range files {
		if _, err := Load(f); err != nil {
			t.Error(err)
		}
	}
}

func TestBadRules(t *testing.T) {
	for _, src := range []string{`{}`, `{"fields":{"a":{"pattern":"("}}}`, `{"fields":{"a":{"type":"int"}}}`, `{"fields":{"a":null}}`, `nope`} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("Parse(%s): want error", src)
		}
	}
}

func TestGoldenInvalid(t *testing.T) {
	s, _ := Load("../../schema/request.json")
	b, _ := os.ReadFile("../../testdata/golden/issueparse/request-invalid.json")
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	out, _ := json.MarshalIndent(s.Check(doc), "", "  ")
	golden.Check(t, "../../testdata/golden/validate/request-invalid.errors.json", append(out, '\n'))
}
