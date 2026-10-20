package site

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func load(t *testing.T) *Site {
	t.Helper()
	cfg := Config{Repo: "CoolGitOrg/issueops-intake", GitHub: "https://github.com",
		Library: "https://coolgitorg.github.io/issueops-library/", MaxURL: 7000,
		Version: "test", BuildTime: "2026-10-19T00:00:00Z"}
	s, err := Load(cfg, "../../.github/ISSUE_TEMPLATE", "../../schema", "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestGolden(t *testing.T) {
	files, err := load(t).Files()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"index.html", "lab/index.html", "flags.json"} {
		golden.Check(t, "../../testdata/golden/site/"+strings.ReplaceAll(p, "/", "_"), files[p])
	}
	for _, p := range []string{"static/intake.js", "static/lab.js", "static/site.css", ".nojekyll", "forms.json"} {
		if _, ok := files[p]; !ok {
			t.Errorf("missing %s", p)
		}
	}
}

// The page must offer exactly the fields and options the issue forms do.
func TestNoDrift(t *testing.T) {
	s := load(t)
	files, _ := s.Files()
	page := string(files["index.html"])
	for _, pg := range s.Pages {
		if !strings.Contains(page, `data-template="`+pg.Form.File+`"`) {
			t.Errorf("form %s missing", pg.Form.File)
		}
		for _, f := range pg.Fields {
			if !strings.Contains(page, `id="f-`+pg.Slug+`-`+f.ID+`"`) && f.Type != "checkboxes" {
				t.Errorf("%s/%s missing", pg.Slug, f.ID)
			}
			for _, o := range f.Options {
				if !strings.Contains(page, `<option value="`+o+`"`) {
					t.Errorf("%s/%s option %q missing", pg.Slug, f.ID, o)
				}
			}
		}
	}
	var meta []map[string]any
	if err := json.Unmarshal(files["forms.json"], &meta); err != nil || len(meta) != 3 {
		t.Errorf("forms.json: %v (%d)", err, len(meta))
	}
}

func TestEscaping(t *testing.T) {
	s := load(t)
	s.Pages[0].Fields[0].Description = `<script>alert(1)</script>`
	s.Pages[0].Markdown = []string{`[x](javascript:alert(1)) <b>`}
	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	page := string(files["index.html"])
	if strings.Contains(page, "<script>alert(1)") || strings.Contains(page, `href="javascript:`) || strings.Contains(page, "<b>") {
		t.Error("unescaped user-controlled text in page")
	}
}

func TestFlags(t *testing.T) {
	if _, err := Load(Config{}, "../../.github/ISSUE_TEMPLATE", "", "../../testdata/flags-off.json"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(Config{}, "../../.github/ISSUE_TEMPLATE", "", "nope.json"); err == nil {
		t.Error("missing flags file: want error")
	}
	if _, err := Load(Config{}, "../../testdata", "", ""); err == nil {
		t.Error("no forms: want error")
	}
}
