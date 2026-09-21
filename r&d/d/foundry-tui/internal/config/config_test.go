package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The shipped example must load, validate, and say what the defaults say.
func TestExampleMatchesDefaults(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "examples", "foundry-tui.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if want := Default(); !reflect.DeepEqual(c, want) {
		t.Errorf("example drifted from Default():\n got %+v\nwant %+v", c, want)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	t.Setenv("FOUNDRY_ACCOUNT", "ais-other")
	c, err := Load(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil || c.Account != "ais-other" {
		t.Fatalf("account=%q err=%v", c.Account, err)
	}
}

func TestUnknownKeyIsLoud(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"acount":"typo"}`), 0o600)
	if _, err := Load(p); err == nil {
		t.Fatal("a misspelt key was accepted")
	}
}

func TestValidateRejectsFlagLookalikes(t *testing.T) {
	c := Default()
	c.Ref = "--repo=evil/repo"
	if c.Validate() == nil {
		t.Fatal("ref beginning with - was accepted")
	}
}
