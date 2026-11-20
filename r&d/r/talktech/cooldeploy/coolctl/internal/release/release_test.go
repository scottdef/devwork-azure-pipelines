package release

import (
	"fmt"
	"strings"
	"testing"

	"github.com/CoolEngOrg/coolctl/internal/manifest"
)

func rel(id string, weight int) Release {
	return Release{ID: id, Digest: "sha256:" + strings.Repeat(id[1:2], 64), Weight: weight, Standby: weight == 0}
}

// show renders a release list as "id:weight id:sweight", s marking a standby,
// so a test can compare a whole list with one string.
func show(rs []Release) string {
	var parts []string
	for _, r := range rs {
		s := ""
		if r.Standby {
			s = "s"
		}
		parts = append(parts, fmt.Sprintf("%s:%s%d", r.ID, s, r.Weight))
	}
	return strings.Join(parts, " ")
}

func TestID(t *testing.T) {
	id, err := ID("3F9C2AB81d04e7")
	if err != nil || id != "r3f9c2ab" {
		t.Fatalf("ID = %q, %v", id, err)
	}
	if _, err := ID("xyz"); err == nil {
		t.Error("ID accepted a non-hex commit")
	}
}

func TestDeploy(t *testing.T) {
	a, b, c := rel("raaaaaaa", 100), rel("rbbbbbbb", 100), rel("rccccccc", 100)

	rs := Deploy(nil, a)
	if got := show(rs); got != "raaaaaaa:100" {
		t.Fatalf("first deploy: %s", got)
	}
	rs = Deploy(rs, b)
	if got := show(rs); got != "rbbbbbbb:100 raaaaaaa:s0" {
		t.Fatalf("second deploy: %s", got)
	}
	rs = Deploy(rs, c)
	if got := show(rs); got != "rccccccc:100 rbbbbbbb:s0" {
		t.Fatalf("third deploy drops the oldest: %s", got)
	}
	rs = Deploy(rs, c)
	if got := show(rs); got != "rccccccc:100 rbbbbbbb:s0" {
		t.Fatalf("redeploy keeps the standby: %s", got)
	}
	if err := Validate(rs); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackIsItsOwnInverse(t *testing.T) {
	rs := Deploy(Deploy(nil, rel("raaaaaaa", 100)), rel("rbbbbbbb", 100))
	back, err := Rollback(rs)
	if err != nil {
		t.Fatal(err)
	}
	if got := show(back); got != "rbbbbbbb:s0 raaaaaaa:100" {
		t.Fatalf("rollback: %s", got)
	}
	again, err := Rollback(back)
	if err != nil || show(again) != show(rs) {
		t.Fatalf("second rollback: %s, %v; want %s", show(again), err, show(rs))
	}
	if _, err := Rollback(rs[:1]); err != ErrNoStandby {
		t.Errorf("rollback with one release: err = %v, want ErrNoStandby", err)
	}
}

func TestStage(t *testing.T) {
	rs := Deploy(nil, rel("raaaaaaa", 100))
	n := rel("rbbbbbbb", 0)

	steps := []struct {
		weight int
		want   string
	}{
		{10, "rbbbbbbb:10 raaaaaaa:90"},
		{50, "rbbbbbbb:50 raaaaaaa:50"},
		{100, "rbbbbbbb:100 raaaaaaa:s0"},
	}
	var err error
	for _, s := range steps {
		// After the first stage the caller names the release by ID alone.
		rs, err = Stage(rs, Release{ID: n.ID, Digest: n.Digest}, s.weight)
		if err != nil {
			t.Fatal(err)
		}
		if got := show(rs); got != s.want {
			t.Fatalf("stage %d: %s, want %s", s.weight, got, s.want)
		}
		if err := Validate(rs); err != nil {
			t.Fatalf("stage %d: %v", s.weight, err)
		}
		n.Digest = ""
	}
}

func TestStageHalt(t *testing.T) {
	rs, _ := Stage(Deploy(nil, rel("raaaaaaa", 100)), rel("rbbbbbbb", 0), 10)
	rs, err := Stage(rs, Release{ID: "rbbbbbbb"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := show(rs); got != "rbbbbbbb:s0 raaaaaaa:100" {
		t.Fatalf("halt: %s", got)
	}
	if rs[0].Digest == "" {
		t.Error("halt lost the digest of the halted release")
	}
}

func TestStageErrors(t *testing.T) {
	if _, err := Stage(nil, rel("raaaaaaa", 0), 10); err == nil {
		t.Error("a first release at 10 percent was accepted")
	}
	if _, err := Stage(nil, rel("raaaaaaa", 0), 101); err == nil {
		t.Error("weight 101 was accepted")
	}
}

func TestValidate(t *testing.T) {
	ok := []Release{rel("raaaaaaa", 100), rel("rbbbbbbb", 0)}
	if err := Validate(ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string][]Release{
		"empty":     nil,
		"sum":       {rel("raaaaaaa", 60), rel("rbbbbbbb", 30)},
		"duplicate": {rel("raaaaaaa", 50), rel("raaaaaaa", 50)},
		"id":        {{ID: "v1", Digest: ok[0].Digest, Weight: 100}},
		"digest":    {{ID: "raaaaaaa", Digest: "latest", Weight: 100}},
		"standby":   {{ID: "raaaaaaa", Digest: ok[0].Digest, Weight: 100, Standby: true}},
		"four":      {rel("raaaaaaa", 100), rel("rbbbbbbb", 0), rel("rccccccc", 0), rel("rddddddd", 0)},
	}
	for name, rs := range bad {
		if Validate(rs) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRetire(t *testing.T) {
	rs := Retire([]Release{rel("raaaaaaa", 100), rel("rbbbbbbb", 0)})
	if got := show(rs); got != "raaaaaaa:100" {
		t.Fatalf("retire: %s", got)
	}
}

func TestApplyEnvironmentOverride(t *testing.T) {
	m := manifest.Default("payments", "checkout", "go-htmx")
	m.Env = map[string]string{"LOG_LEVEL": "debug"}
	m.Environments = map[string]manifest.Override{
		"prd": {Scale: &manifest.Scale{Min: 3, Max: 12, CPU: 60}, Env: map[string]string{"LOG_LEVEL": "info"}},
	}
	dev, prd := New("payments", "checkout", "dev", "cool-app-1.0.0"), New("payments", "checkout", "prd", "cool-app-1.0.0")
	dev.Apply(m)
	prd.Apply(m)
	if dev.Scale.Min != 2 || dev.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("dev took the prd override: %+v %v", dev.Scale, dev.Env)
	}
	if prd.Scale.Min != 3 || prd.Env["LOG_LEVEL"] != "info" {
		t.Errorf("prd missed its override: %+v %v", prd.Scale, prd.Env)
	}
	if m.Env["LOG_LEVEL"] != "debug" {
		t.Error("Apply changed the manifest it was given")
	}
}
