package runner

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/brain"
	"github.com/CoolGitOrg/tmplwatch/internal/config"
	"github.com/CoolGitOrg/tmplwatch/internal/metrics"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

func setup(t *testing.T, tmpls map[string]string, mod func(*config.Config)) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := &config.Config{Source: config.Source{Type: "dir", Path: dir}}
	for name, text := range tmpls {
		src := filepath.Join(dir, name+".tmpl")
		os.WriteFile(src, []byte(text), 0o644)
		cfg.Templates = append(cfg.Templates, config.Template{
			Name: name, Source: src, Destination: filepath.Join(dir, "out", name), Perms: "0644",
			CommandTimeout: config.Duration(10 * time.Second),
		})
	}
	if mod != nil {
		mod(cfg)
	}
	r, err := New(context.Background(), cfg, brain.New(), metrics.New("test"), log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return r, dir
}

func snap(kv ...string) source.Snapshot {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return source.Snapshot{Data: m, Version: source.Hash(m)}
}

func TestUpdateWritesOnlyAffected(t *testing.T) {
	r, dir := setup(t, map[string]string{
		"a": `a={{ key "x" }}`,
		"b": `b={{ key "y" }}`,
	}, nil)
	o := r.Update(snap("x", "1", "y", "2"))
	if len(o.Written) != 2 || o.Err() != nil {
		t.Fatalf("first: %+v", o)
	}
	o = r.Update(snap("x", "1", "y", "3", "z", "0"))
	if len(o.Rendered) != 1 || o.Rendered[0] != "b" {
		t.Errorf("rendered %v, want [b]", o.Rendered)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "out", "b")); string(b) != "b=3" {
		t.Errorf("b = %q", b)
	}
	if !r.Status().Healthy {
		t.Error("unhealthy")
	}
	if c, ok := r.Current("a"); !ok || string(c) != "a=1" {
		t.Errorf("current %q", c)
	}
}

func TestIncompleteNotWritten(t *testing.T) {
	r, dir := setup(t, map[string]string{"a": `{{ key "x" }}{{ key "later" }}`}, nil)
	o := r.Update(snap("x", "1"))
	if len(o.Incomplete) != 1 || len(o.Written) != 0 {
		t.Fatalf("%+v", o)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "a")); !os.IsNotExist(err) {
		t.Error("incomplete render was written")
	}
	if st := r.Status(); st.Healthy || st.Templates[0].Missing[0] != "later" {
		t.Errorf("status %+v", st)
	}
	o = r.Update(snap("x", "1", "later", "2"))
	if len(o.Written) != 1 || !r.Status().Healthy {
		t.Errorf("after key arrives: %+v", o)
	}
}

func TestDry(t *testing.T) {
	r, dir := setup(t, map[string]string{"a": "line1\n{{ key \"x\" }}\n"}, func(c *config.Config) {
		c.Dry = true
		c.Templates[0].Command = []string{"false"}
	})
	var out bytes.Buffer
	r.DryOut = &out
	o := r.Update(snap("x", "hello"))
	if len(o.Written) != 1 || o.Err() != nil {
		t.Fatalf("%+v", o)
	}
	if _, err := os.Stat(filepath.Join(dir, "out", "a")); !os.IsNotExist(err) {
		t.Error("-dry wrote a file")
	}
	for _, want := range []string{"# tmplwatch -dry: a", "+line1", "+hello", "# would run: false"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("dry output missing %q:\n%s", want, out.String())
		}
	}
}

func TestOnceCommandAndReload(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	var marker string
	r, dir := setup(t, map[string]string{"a": `{{ key "x" }}`}, func(c *config.Config) {
		c.Once = true
		marker = filepath.Join(filepath.Dir(c.Templates[0].Source), "ran")
		// The test's own command may use a shell; tmplwatch never adds one.
		c.Templates[0].Command = []string{"/bin/sh", "-c", `cat "$1" >> "$2"`, "sh", c.Templates[0].Destination, marker}
	})
	if o := r.Update(snap("x", "v1")); o.Err() != nil {
		t.Fatal(o.Err())
	}
	if b, _ := os.ReadFile(marker); string(b) != "v1" {
		t.Errorf("command saw %q", b)
	}
	// Reload re-reads the template from disk.
	os.WriteFile(filepath.Join(dir, "a.tmpl"), []byte(`new {{ key "x" }}`), 0o644)
	if o := r.Reload(); o.Err() != nil || len(o.Written) != 1 {
		t.Fatalf("reload %+v", o)
	}
	// A broken template on reload keeps the old one.
	os.WriteFile(filepath.Join(dir, "a.tmpl"), []byte(`{{ broken`), 0o644)
	if o := r.Reload(); o.Err() == nil {
		t.Error("broken reload not reported")
	}
	if c, _ := r.Current("a"); string(c) != "new v1" {
		t.Errorf("current after bad reload %q", c)
	}
}

func TestCommandDebounce(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("no /bin/sh")
	}
	r, dir := setup(t, map[string]string{"a": `{{ key "x" }}`}, func(c *config.Config) {
		c.Templates[0].Wait = config.Duration(150 * time.Millisecond)
		c.Templates[0].Command = []string{"/bin/sh", "-c", "echo x >> " + filepath.Join(filepath.Dir(c.Templates[0].Source), "count")}
	})
	for i := range 5 {
		r.Update(snap("x", string(rune('a'+i))))
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(600 * time.Millisecond)
	b, _ := os.ReadFile(filepath.Join(dir, "count"))
	if n := strings.Count(string(b), "x"); n != 1 {
		t.Errorf("command ran %d times for a burst of 5 writes, want 1", n)
	}
	r.Stop()
}

func TestParseSignal(t *testing.T) {
	for _, n := range []string{"SIGHUP", "hup", " sigterm "} {
		if _, err := ParseSignal(n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
}

func TestShellWords(t *testing.T) {
	if got := shellWords([]string{"caddy", "validate", "--config", "a b", "it's"}); got != `caddy validate --config 'a b' 'it'\''s'` {
		t.Errorf("%s", got)
	}
}
