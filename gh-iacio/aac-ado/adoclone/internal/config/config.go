// Package config parses adoclone's flags and environment.
package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config is everything a run needs.
type Config struct {
	Command        string
	Org            string
	Source, Target string
	PAT            string
	Components     []string
	DryRun         bool
	BypassRules    bool
	AdoptExisting  bool
	StatePath      string
	Out            string
	Visibility     string
	RepoMode       string
	VarGroupMode   string
	SecureFilesDir string
	Git            string
	WorkDir        string
	MetricsAddr    string
	MetricsHold    time.Duration
	SamplePct      float64
	LogLevel       string
}

// Usage is printed for bad invocations.
const Usage = `usage: adoclone <command> [flags]

commands:
  plan        inventory the source project (read-only)
  clone       copy the source project into the target project
  verify      compare source and target; exits 4 on any mismatch
  report      write the HTML parity report
  cleanup     unshare resources from the target and delete it (needs CONFIRM=<target>)
  components  list components in run order
  version     print the version

environment: ADO_PAT or ADO_PAT_FILE (required), ADO_ORG, LOG_LEVEL`

// ErrUsage marks invocation errors (exit code 2).
var ErrUsage = errors.New("usage")

// Parse reads flags for cmd. known is the ordered list of component names.
func Parse(cmd string, args []string, getenv func(string) string, known []string) (*Config, error) {
	c := &Config{Command: cmd}
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&c.Org, "org", or(getenv("ADO_ORG"), "CoolADO"), "Azure DevOps organization")
	fs.StringVar(&c.Source, "source", "", "source project name")
	fs.StringVar(&c.Target, "target", "", "target project name")
	comps := fs.String("components", "all", `components to run, comma-separated, or "all"`)
	skip := fs.String("skip", "", "components to leave out, comma-separated")
	fs.BoolVar(&c.DryRun, "dry-run", true, "log writes instead of sending them")
	fs.BoolVar(&c.AdoptExisting, "adopt-existing", false, "let clone/cleanup use a target project this checkpoint didn't create")
	fs.BoolVar(&c.BypassRules, "bypass-rules", true, "keep CreatedDate/ChangedBy etc. on work items (needs the Bypass rules permission)")
	fs.StringVar(&c.StatePath, "state", "state.json", "checkpoint file; todo.json is written next to it")
	fs.StringVar(&c.Out, "out", "report.html", "report path")
	fs.StringVar(&c.Visibility, "visibility", "private", `target visibility: private, public, or "source" to copy it`)
	fs.StringVar(&c.RepoMode, "repo-mode", "mirror", "mirror (git clone/push, needs git) or import (server-side import requests, no LFS)")
	fs.StringVar(&c.VarGroupMode, "vargroup-mode", "copy", "copy (new groups; secrets from SECRET_<GROUP>_<VAR> or todo.json) or share")
	fs.StringVar(&c.SecureFilesDir, "securefiles-dir", "", "directory with secure files to upload by name")
	fs.StringVar(&c.Git, "git", "git", "git binary")
	fs.StringVar(&c.WorkDir, "work-dir", os.TempDir(), "scratch space for repo mirrors and large attachments")
	fs.StringVar(&c.MetricsAddr, "metrics-addr", "", `serve Prometheus metrics on this address, e.g. ":9090"`)
	fs.DurationVar(&c.MetricsHold, "metrics-hold", 0, "keep serving metrics this long after the run so the last values get scraped")
	fs.Float64Var(&c.SamplePct, "sample", 5, "percent of mapped work items to field-check in verify/report")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUsage, err)
	}
	if fs.NArg() > 0 {
		return nil, fmt.Errorf("%w: unexpected argument %q", ErrUsage, fs.Arg(0))
	}
	c.LogLevel = getenv("LOG_LEVEL")

	pat := getenv("ADO_PAT")
	if pat == "" {
		if f := getenv("ADO_PAT_FILE"); f != "" {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("%w: ADO_PAT_FILE: %v", ErrUsage, err)
			}
			pat = strings.TrimSpace(string(b))
		}
	}
	c.PAT = pat

	var err error
	if c.Components, err = selectComponents(*comps, *skip, known); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrUsage}, a...)...)
	}
	if c.PAT == "" {
		return bad("set ADO_PAT or ADO_PAT_FILE")
	}
	if c.Source == "" {
		return bad("-source is required")
	}
	if c.Command != "plan" && c.Target == "" {
		return bad("-target is required")
	}
	if c.Target != "" && strings.EqualFold(c.Source, c.Target) {
		return bad("-source and -target must differ")
	}
	if !in(c.Visibility, "private", "public", "source") {
		return bad("-visibility must be private, public or source")
	}
	if !in(c.RepoMode, "mirror", "import") {
		return bad("-repo-mode must be mirror or import")
	}
	if !in(c.VarGroupMode, "copy", "share") {
		return bad("-vargroup-mode must be copy or share")
	}
	if c.SamplePct < 0 || c.SamplePct > 100 {
		return bad("-sample must be between 0 and 100")
	}
	return nil
}

func selectComponents(list, skip string, known []string) ([]string, error) {
	isKnown := map[string]bool{}
	for _, k := range known {
		isKnown[k] = true
	}
	want := map[string]bool{}
	if strings.TrimSpace(list) == "all" {
		for _, k := range known {
			want[k] = true
		}
	} else {
		for _, n := range split(list) {
			if !isKnown[n] {
				return nil, fmt.Errorf("%w: unknown component %q (run: adoclone components)", ErrUsage, n)
			}
			want[n] = true
		}
	}
	for _, n := range split(skip) {
		if !isKnown[n] {
			return nil, fmt.Errorf("%w: unknown component %q in -skip", ErrUsage, n)
		}
		delete(want, n)
	}
	var out []string
	for _, k := range known { // keep run order
		if want[k] {
			out = append(out, k)
		}
	}
	return out, nil
}

func split(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func in(s string, opts ...string) bool {
	for _, o := range opts {
		if s == o {
			return true
		}
	}
	return false
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
