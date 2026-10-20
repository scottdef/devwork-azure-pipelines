package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-library/internal/golden"
)

func TestNextTemplateRoundTrip(t *testing.T) {
	// The golden next.tmpl a borrower downloads, rendered with their changes.
	var out bytes.Buffer
	o := options{tmpl: "../../testdata/golden/items/caddyfile/next.tmpl", data: "../../testdata/tpl/phase2-mine.json"}
	if err := run(o, nil, &out); err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/tpl/phase2-payload.json", out.Bytes())
}

func TestDelimsAndMissingKey(t *testing.T) {
	dir := t.TempDir()
	tm := filepath.Join(dir, "x.tmpl")
	os.WriteFile(tm, []byte("run: ${{ github.sha }} [[.name]]\n"), 0o644)
	var out bytes.Buffer
	if err := run(options{tmpl: tm, delims: "[[,]]", data: "-"}, strings.NewReader(`{"name":"edge"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "run: ${{ github.sha }} edge\n" {
		t.Errorf("got %q", out.String())
	}
	if err := run(options{tmpl: tm, delims: "[[,]]", missing: "error"}, nil, &bytes.Buffer{}); err == nil {
		t.Error("missingkey=error: want error")
	}
	if err := run(options{tmpl: tm, delims: "[[,]]", missing: "default"}, nil, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "<no value>") {
		t.Errorf("missingkey=default should be refused by the <no value> guard: %v", err)
	}
	if err := run(options{tmpl: tm, delims: "[["}, nil, &bytes.Buffer{}); err == nil {
		t.Error("bad -delims: want error")
	}
}

func TestOutFile(t *testing.T) {
	dir := t.TempDir()
	tm := filepath.Join(dir, "x.tmpl")
	os.WriteFile(tm, []byte("#!tmpl ext=txt\n{{add 1 2}} {{.n | default \"none\"}}"), 0o644)
	dst := filepath.Join(dir, "out.txt")
	if err := run(options{tmpl: tm, out: dst}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dst); string(b) != "3 none" {
		t.Errorf("got %q", b)
	}
	if _, err := os.Stat(dst + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind")
	}
}

// Every comment template renders its fixture to a golden file.
func TestCommentTemplates(t *testing.T) {
	for _, n := range []string{"published", "refused", "borrowed", "returned"} {
		var out bytes.Buffer
		o := options{tmpl: "../../templates/comments/" + n + ".md.tmpl", data: "../../testdata/tpl/" + n + ".json", missing: "error"}
		if err := run(o, nil, &out); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		golden.Check(t, "../../testdata/golden/comments/"+n+".md", out.Bytes())
	}
}
