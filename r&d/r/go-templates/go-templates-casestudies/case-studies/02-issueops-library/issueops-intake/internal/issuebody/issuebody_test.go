package issuebody

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/form"
	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func load(t testing.TB, path string) *form.Form {
	t.Helper()
	f, err := form.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// formFor picks the form a fixture was written against from its name.
func formFor(t testing.TB, name string) *form.Form {
	switch {
	case strings.HasPrefix(name, "checks-"):
		return load(t, "../../testdata/forms/checks.yml")
	default:
		kind := name[:strings.Index(name, "-")]
		return load(t, "../../.github/ISSUE_TEMPLATE/"+kind+".yml")
	}
}

func TestGolden(t *testing.T) {
	bodies, _ := filepath.Glob("../../testdata/bodies/*.md")
	if len(bodies) < 6 {
		t.Fatalf("only %d fixtures", len(bodies))
	}
	for _, p := range bodies {
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			v, err := Parse(formFor(t, name), string(body))
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.MarshalIndent(v, "", "  ")
			golden.Check(t, "../../testdata/golden/issueparse/"+name+".json", append(b, '\n'))
		})
	}
}

func TestCRLFMatchesLF(t *testing.T) {
	f := load(t, "../../.github/ISSUE_TEMPLATE/request.yml")
	crlf, _ := os.ReadFile("../../testdata/bodies/request-crlf.md")
	lf := strings.ReplaceAll(string(crlf), "\r\n", "\n")
	a, _ := Parse(f, string(crlf))
	b, _ := Parse(f, lf)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("CRLF %s != LF %s", ja, jb)
	}
	if strings.Contains(string(ja), `\r`) {
		t.Errorf("carriage return leaked: %s", ja)
	}
}

func TestFenceProtectsHeadings(t *testing.T) {
	f := load(t, "../../.github/ISSUE_TEMPLATE/request.yml")
	body, _ := os.ReadFile("../../testdata/bodies/request-fence-heading.md")
	v, err := Parse(f, string(body))
	if err != nil {
		t.Fatal(err)
	}
	if v["phase"] != "1" {
		t.Errorf("phase = %q; a heading inside the payload fence leaked", v["phase"])
	}
	var x map[string]any
	if err := json.Unmarshal([]byte(v["payload"].(string)), &x); err != nil {
		t.Errorf("payload no longer JSON: %v", err)
	}
}

func TestNoFields(t *testing.T) {
	f := load(t, "../../.github/ISSUE_TEMPLATE/request.yml")
	for _, body := range []string{"", "hello", "### Unknown\n\nx\n"} {
		if _, err := Parse(f, body); err != ErrNoFields {
			t.Errorf("Parse(%q) err = %v", body, err)
		}
	}
}

func TestUnfence(t *testing.T) {
	for in, want := range map[string]string{
		"```json\n{}\n```":              "{}",
		"~~~\na\nb\n~~~":                "a\nb",
		"plain":                         "plain",
		"```\na\n```\ntext after":       "```\na\n```\ntext after",
		"```\na\n```\n```\nb\n```":      "```\na\n```\n```\nb\n```",
		"````md\n```\ninner\n```\n````": "```\ninner\n```",
	} {
		if got := unfence(in); got != want {
			t.Errorf("unfence(%q) = %q, want %q", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	frm, err := form.Load("../../.github/ISSUE_TEMPLATE/request.yml")
	if err != nil {
		f.Fatal(err)
	}
	bodies, _ := filepath.Glob("../../testdata/bodies/request-*.md")
	for _, p := range bodies {
		b, _ := os.ReadFile(p)
		f.Add(string(b))
	}
	f.Add("### Payload\n\n````\n```\n### Phase\n")
	f.Add("###\n\n### \r\n\r\r")
	f.Fuzz(func(t *testing.T, body string) {
		v, err := Parse(frm, body)
		if err != nil {
			return
		}
		for k, x := range v {
			if _, ok := frm.Field(k); !ok {
				t.Fatalf("unknown key %q", k)
			}
			if s, ok := x.(string); ok && strings.ContainsRune(s, '\r') {
				t.Fatalf("CR in %q", k)
			}
		}
		if _, err := json.Marshal(v); err != nil {
			t.Fatal(err)
		}
	})
}
