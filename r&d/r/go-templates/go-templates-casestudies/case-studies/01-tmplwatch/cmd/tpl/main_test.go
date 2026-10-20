package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTpl(t *testing.T) {
	dir := t.TempDir()
	w := func(name, s string) string {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	tf := w("a.tmpl", `[[ .name | toUpper ]] [[ .port ]] {{ literal }}`)
	df := w("d.json", `{"name":"web","port":8080}`)
	var out, errb bytes.Buffer
	if code := run([]string{"-t", tf, "-d", df, "-delims", "[[,]]"}, nil, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if out.String() != "WEB 8080 {{ literal }}" {
		t.Errorf("got %q", out.String())
	}

	// stdin data, missingkey=error.
	tf2 := w("b.tmpl", `{{ .nope }}`)
	out.Reset()
	errb.Reset()
	if code := run([]string{"-t", tf2, "-d", "-", "-missingkey", "error"}, strings.NewReader(`{"a":1}`), &out, &errb); code != 1 || !strings.Contains(errb.String(), "nope") {
		t.Errorf("missingkey=error: %d %q", code, errb.String())
	}

	// -kv store, missing key fails, -o writes.
	w("kv/motd/text", "hello\n")
	tf3 := w("c.tmpl", `{{ key "motd/text" }}`)
	o := filepath.Join(dir, "out", "motd")
	if code := run([]string{"-t", tf3, "-kv", filepath.Join(dir, "kv"), "-o", o}, nil, &out, &errb); code != 0 {
		t.Fatalf("-kv: %s", errb.String())
	}
	if b, _ := os.ReadFile(o); string(b) != "hello" {
		t.Errorf("-o wrote %q", b)
	}
	tf4 := w("d.tmpl", `{{ key "absent" }}`)
	errb.Reset()
	if code := run([]string{"-t", tf4}, nil, &out, &errb); code != 1 || !strings.Contains(errb.String(), "missing keys: absent") {
		t.Errorf("missing: %d %q", code, errb.String())
	}
	if code := run([]string{"-t", tf, "-delims", "[["}, nil, &out, &errb); code != 1 {
		t.Errorf("bad delims accepted")
	}
	if code := run(nil, nil, &out, &errb); code != 2 {
		t.Errorf("no -t: %d", code)
	}
}
