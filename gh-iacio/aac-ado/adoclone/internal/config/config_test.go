package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var known = []string{"project", "nodes", "repos", "workitems"}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestComponentsKeepRunOrder(t *testing.T) {
	c, err := Parse("clone", []string{"-source", "A", "-target", "B", "-components", "workitems,project"}, env(map[string]string{"ADO_PAT": "x"}), known)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(c.Components, ",") != "project,workitems" {
		t.Fatalf("got %v", c.Components)
	}
	c, _ = Parse("clone", []string{"-source", "A", "-target", "B", "-skip", "repos"}, env(map[string]string{"ADO_PAT": "x"}), known)
	if strings.Join(c.Components, ",") != "project,nodes,workitems" {
		t.Fatalf("skip: %v", c.Components)
	}
	if !c.DryRun || c.Visibility != "private" || c.Org != "CoolADO" {
		t.Fatalf("defaults: %+v", c)
	}
}

func TestValidation(t *testing.T) {
	e := env(map[string]string{"ADO_PAT": "x"})
	for _, args := range [][]string{
		{"-source", "A", "-target", "a"},
		{"-source", "A"},
		{"-source", "A", "-target", "B", "-components", "nope"},
		{"-source", "A", "-target", "B", "-repo-mode", "svn"},
	} {
		if _, err := Parse("clone", args, e, known); !errors.Is(err, ErrUsage) {
			t.Errorf("%v: want usage error, got %v", args, err)
		}
	}
	if _, err := Parse("plan", []string{"-source", "A"}, e, known); err != nil {
		t.Errorf("plan needs no target: %v", err)
	}
	if _, err := Parse("clone", []string{"-source", "A", "-target", "B"}, env(nil), known); !errors.Is(err, ErrUsage) {
		t.Error("missing PAT not rejected")
	}
}

func TestPATFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "pat")
	os.WriteFile(f, []byte("secret\n"), 0o600)
	c, err := Parse("clone", []string{"-source", "A", "-target", "B"}, env(map[string]string{"ADO_PAT_FILE": f}), known)
	if err != nil || c.PAT != "secret" {
		t.Fatalf("pat=%q err=%v", c.PAT, err)
	}
}
