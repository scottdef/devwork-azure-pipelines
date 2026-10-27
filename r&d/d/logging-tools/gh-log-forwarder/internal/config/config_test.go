package config

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDefaultsValidate(t *testing.T) {
	c := Defaults()
	if err := c.Validate(Need{}); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(Need{GitHub: true, Dynatrace: true}); err == nil {
		t.Fatal("missing tokens and DT url should fail")
	}
	if _, err := c.Redactors(); err != nil {
		t.Fatal(err)
	}
}

func TestPrecedence(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.json")
	os.WriteFile(p, []byte(`{"lookback":"2h","targets":["CoolGitOrg/file"],"dt":{"url":"https://file.live.dynatrace.com","batch_records":100}}`), 0o644)

	args := []string{"--config", p, "--lookback", "3h", "--repo", "CoolGitOrg/flag-a", "--repo", "CoolGitOrg/flag-b"}
	path, explicit := ConfigPath(args, envMap(nil))
	if path != p || !explicit {
		t.Fatalf("ConfigPath = %q %v", path, explicit)
	}
	c, err := Load(path, explicit)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.ApplyEnv(envMap(map[string]string{
		"GHLOG_LOOKBACK": "1h", "DT_URL": "https://env.live.dynatrace.com",
		"DT_LOGS_TOKEN": " dt0c01.x \n", "GITHUB_TOKEN": "fallback",
	})); err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	c.BindFlags(fs, ScopeGitHub|ScopeDynatrace|ScopeParse|ScopeState)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if c.Lookback.D() != 3*time.Hour {
		t.Errorf("flag should beat env and file: lookback %s", c.Lookback.D())
	}
	if c.DT.URL != "https://env.live.dynatrace.com" {
		t.Errorf("env should beat file: %s", c.DT.URL)
	}
	if c.DT.BatchRecords != 100 {
		t.Errorf("file should beat default: %d", c.DT.BatchRecords)
	}
	if !reflect.DeepEqual(c.Targets, []string{"CoolGitOrg/flag-a", "CoolGitOrg/flag-b"}) {
		t.Errorf("repeated --repo should replace file targets: %v", c.Targets)
	}
	if c.DT.Token != "dt0c01.x" || c.GitHubToken != "fallback" {
		t.Errorf("tokens: %q %q", c.DT.Token, c.GitHubToken)
	}
	if strings.Contains(c.String(), "dt0c01.x") || strings.Contains(c.String(), "fallback") {
		t.Error("String() leaks a secret")
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"lookbak":"2h"}`), 0o644)
	if _, err := Load(p, true); err == nil {
		t.Fatal("typo in config key accepted")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json"), false); err != nil {
		t.Fatal("implicit missing config should be fine")
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.json"), true); err == nil {
		t.Fatal("explicit missing config should fail")
	}
}

func TestValidateRules(t *testing.T) {
	bad := []func(*Config){
		func(c *Config) { c.MaxAge = Duration(25 * time.Hour) },
		func(c *Config) { c.Lookback = Duration(24 * time.Hour) },
		func(c *Config) { c.DT.BatchBytes = 11 << 20 },
		func(c *Config) { c.DT.BatchRecords = 50001 },
		func(c *Config) { c.DT.AuthScheme = "Basic" },
		func(c *Config) { c.Stale = "keep" },
		func(c *Config) { c.Targets = []string{"not-a-repo"} },
		func(c *Config) { c.Redact = []string{"("} },
		func(c *Config) { c.GitHubAPI = "http://api.github.com" },
		func(c *Config) { c.ShippedTTL = Duration(time.Hour) },
		func(c *Config) { c.Concurrency = 0 },
	}
	for i, mut := range bad {
		c := Defaults()
		mut(c)
		if err := c.Validate(Need{}); err == nil {
			t.Errorf("case %d: invalid config accepted", i)
		}
	}
	c := Defaults()
	c.GitHubAPI = "http://127.0.0.1:8080"
	if err := c.Validate(Need{}); err != nil {
		t.Errorf("loopback http should be allowed: %v", err)
	}
}

func TestSelection(t *testing.T) {
	c := Defaults()
	c.Include = []string{"coolgitorg/*"}
	c.Exclude = []string{"CoolGitOrg/*-archive", "CoolGitOrg/gh-log-forwarder"}
	for repo, want := range map[string]bool{
		"CoolGitOrg/app-a": true, "CoolGitOrg/old-archive": false,
		"CoolGitOrg/gh-log-forwarder": false, "Other/app": false,
	} {
		if got := c.Selected(repo); got != want {
			t.Errorf("Selected(%s)=%v", repo, got)
		}
	}
	c.Workflows = []string{"CI", ".github/workflows/release-*.yml"}
	if !c.WorkflowSelected("ci", "x") || !c.WorkflowSelected("x", ".github/workflows/release-prod.yml") || c.WorkflowSelected("Lint", ".github/workflows/lint.yml") {
		t.Error("workflow selection")
	}
}

func TestSplitList(t *testing.T) {
	got := SplitList("a/b, c/d\n# comment\n e/f,,")
	if !reflect.DeepEqual(got, []string{"a/b", "c/d", "e/f"}) {
		t.Fatalf("%v", got)
	}
}

func TestDefaultRedactPatterns(t *testing.T) {
	c := Defaults()
	res, _ := c.Redactors()
	secrets := []string{
		"ghp_" + strings.Repeat("a", 36),
		"ghs_" + strings.Repeat("B", 36),
		"github_pat_" + strings.Repeat("x", 82),
		"dt0c01." + strings.Repeat("A", 24) + "." + strings.Repeat("B", 64),
		"AKIA" + strings.Repeat("Z", 16),
		"-----BEGIN RSA PRIVATE KEY-----",
		"Authorization: Bearer abcdefghijklmnop1234",
		"xoxb-123456789012-abc",
	}
	for _, s := range secrets {
		hit := false
		for _, re := range res {
			if re.MatchString("prefix " + s + " suffix") {
				hit = true
			}
		}
		if !hit {
			t.Errorf("no default pattern catches %q", s)
		}
	}
	for _, re := range res {
		if re.MatchString("go build -o bin/app ./cmd/app") {
			t.Errorf("pattern %s matches an innocent line", re)
		}
	}
}
