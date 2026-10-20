// Package templates_test renders every shipped template against
// testdata/kv and compares with testdata/golden. Run
//
//	go test ./templates -update
//
// to rewrite the golden files after an intended change, and review the
// diff like any other code change.
package templates_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/brain"
	"github.com/CoolGitOrg/tmplwatch/internal/config"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

var update = flag.Bool("update", false, "rewrite golden files")

// root is the repository root relative to this package.
const root = ".."

// fixedNow pins timestamp so golden files are stable.
func fixedNow() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }

func loadAll(t testing.TB) (*config.Config, []*brain.Template) {
	t.Helper()
	cfg, err := config.Load(filepath.Join(root, "config", "testdata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tps []*brain.Template
	for _, tc := range cfg.Templates {
		miss := "default"
		if tc.ErrorOnMissingKey {
			miss = "error"
		}
		tp, err := brain.ParseFile(tc.Name, filepath.Join(root, tc.Source),
			brain.TemplateOptions{Left: tc.LeftDelimiter, Right: tc.RightDelimiter, MissingKey: miss})
		if err != nil {
			t.Fatal(err)
		}
		tps = append(tps, tp)
	}
	return cfg, tps
}

func newBrain(data map[string]string) *brain.Brain {
	b := brain.New()
	b.Now = fixedNow
	b.Env = func(string) string { return "" }
	b.Update(source.Snapshot{Data: data, Version: source.Hash(data)})
	return b
}

func TestGolden(t *testing.T) {
	cfg, tps := loadAll(t)
	s, _, err := source.NewDir(filepath.Join(root, cfg.Source.Path), cfg.Source.Prefix).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b := newBrain(s.Data)
	shipped, _ := filepath.Glob(filepath.Join(root, "templates", "*.tmpl"))
	if len(shipped) != len(tps) {
		t.Errorf("%d templates shipped, %d in config/testdata.json", len(shipped), len(tps))
	}
	for i, tp := range tps {
		golden := filepath.Join(root, "testdata", "golden", filepath.Base(cfg.Templates[i].Destination))
		t.Run(tp.Name, func(t *testing.T) {
			r := b.Render(tp)
			if !r.Complete() {
				t.Fatalf("incomplete: err=%v missing=%v", r.Err, r.Missing)
			}
			if *update {
				if err := os.WriteFile(golden, r.Out, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./templates -update)", err)
			}
			if !bytes.Equal(r.Out, want) {
				t.Errorf("%s differs from golden; run go test ./templates -update and review.\n--- got\n%s", golden, r.Out)
			}
			checkHygiene(t, tp.Name, r.Out)
		})
	}
}

// checkHygiene enforces properties every rendered file must have.
func checkHygiene(t *testing.T, name string, out []byte) {
	t.Helper()
	s := string(out)
	if !strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "\n\n") {
		t.Errorf("%s: must end in exactly one newline", name)
	}
	for i, l := range strings.Split(s, "\n") {
		if strings.TrimRight(l, " \t") != l {
			t.Errorf("%s:%d: trailing whitespace", name, i+1)
		}
		if strings.Contains(l, "<no value>") {
			t.Errorf("%s:%d: <no value>", name, i+1)
		}
	}
	if strings.HasSuffix(name, "main.tf") || strings.HasSuffix(name, ".hcl") || name == "main.tf" || name == "job.nomad.hcl" {
		checkHCLInterpolation(t, name, s)
	}
}

// checkHCLInterpolation allows only the "${" sequences the template
// itself writes; any other unescaped "${" came from data.
func checkHCLInterpolation(t *testing.T, name, s string) {
	allowed := []string{"${local.env}", "${count.index}"}
	for i := 0; i < len(s); i++ {
		if !strings.HasPrefix(s[i:], "${") || (i > 0 && s[i-1] == '$') {
			continue
		}
		ok := false
		for _, a := range allowed {
			if strings.HasPrefix(s[i:], a) {
				ok = true
			}
		}
		if !ok {
			end := min(len(s), i+30)
			t.Errorf("%s: stray interpolation %q", name, s[i:end])
		}
	}
}

// TestMissingKeysIncomplete removes each key in turn: templates that use
// it must report incomplete (or fail), and none may panic.
func TestMissingKeysIncomplete(t *testing.T) {
	cfg, tps := loadAll(t)
	s, _, _ := source.NewDir(filepath.Join(root, cfg.Source.Path), "").Fetch(context.Background())
	for _, drop := range []string{"site/env", "wg/interface/address", "dns/zone", "tf/region"} {
		data := map[string]string{}
		for k, v := range s.Data {
			if k != drop {
				data[k] = v
			}
		}
		b := newBrain(data)
		n := 0
		for _, tp := range tps {
			r := b.Render(tp)
			if !r.Complete() {
				n++
			}
		}
		if n == 0 {
			t.Errorf("dropping %s left every template complete", drop)
		}
	}
}

// TestValidationFails checks that templates reject bad data with fail.
func TestValidationFails(t *testing.T) {
	cfg, tps := loadAll(t)
	s, _, _ := source.NewDir(filepath.Join(root, cfg.Source.Path), "").Fetch(context.Background())
	byName := map[string]*brain.Template{}
	for _, tp := range tps {
		byName[tp.Name] = tp
	}
	for _, tc := range []struct{ tmpl, key, val, want string }{
		{"wg0.conf", "wg/peers/mallory", `{"public_key":"not-a-key","allowed_ips":["0.0.0.0/0"]}`, "malformed public_key"},
		{"nginx.conf", "nginx/servers/x", `{"listen":443,"server_name":"x","upstream":"nope"}`, "unknown upstream"},
		{"Caddyfile", "caddy/sites/9", `{"host":"bare"}`, "needs upstreams or root"},
	} {
		data := map[string]string{tc.key: tc.val}
		for k, v := range s.Data {
			data[k] = v
		}
		r := newBrain(data).Render(byName[tc.tmpl])
		if r.Err == nil || !strings.Contains(r.Err.Error(), tc.want) {
			t.Errorf("%s with %s: err %v, want %q", tc.tmpl, tc.key, r.Err, tc.want)
		}
	}
}

// FuzzRender feeds arbitrary values into every key the shipped templates
// read. Renders may fail or be incomplete; they must never panic, and
// HCL outputs must never carry data-borne interpolation.
func FuzzRender(f *testing.F) {
	cfg, tps := loadAll(f)
	s, _, err := source.NewDir(filepath.Join(root, cfg.Source.Path), "").Fetch(context.Background())
	if err != nil {
		f.Fatal(err)
	}
	keys := make([]string, 0, len(s.Data))
	for k := range s.Data {
		keys = append(keys, k)
	}
	f.Add(uint16(0), "${evil}")
	f.Add(uint16(3), `{"host":"x","upstreams":["${a}"]}`)
	f.Add(uint16(7), "\x00\xff{{")
	f.Add(uint16(11), `["1.1.1.1", {"a": 1}]`)
	f.Add(uint16(20), `{"type":"%{x}","count":1,"tags":{"${k}":"${v}"}}`)
	f.Fuzz(func(t *testing.T, which uint16, val string) {
		data := map[string]string{}
		for k, v := range s.Data {
			data[k] = v
		}
		// Overwrite one key; sorted order keeps seeds stable.
		sortStrings(keys)
		data[keys[int(which)%len(keys)]] = val
		b := newBrain(data)
		for _, tp := range tps {
			r := b.Render(tp)
			if r.Complete() && (tp.Name == "main.tf" || tp.Name == "job.nomad.hcl") {
				checkHCLInterpolation(t, tp.Name, string(r.Out))
			}
		}
	})
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
