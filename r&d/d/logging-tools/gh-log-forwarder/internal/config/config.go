// Package config holds ghlog2dt's configuration.
//
// Precedence, lowest to highest: built-in defaults, the JSON config
// file, environment variables, command-line flags. Secrets (GitHub and
// Dynatrace tokens) are read from the environment only; they are never
// accepted as flags (they would leak through ps(1)) and never written by
// String or MarshalJSON.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration that reads and writes JSON as "90m".
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string like \"90m\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Dynatrace is the ingest side of the configuration.
type Dynatrace struct {
	URL          string   `json:"url"`
	AuthScheme   string   `json:"auth_scheme"`
	BatchBytes   int      `json:"batch_bytes"`
	BatchRecords int      `json:"batch_records"`
	Gzip         bool     `json:"gzip"`
	HeaderAttrs  bool     `json:"header_attrs"`
	Timeout      Duration `json:"timeout"`
	MaxRetries   int      `json:"max_retries"`

	Token string `json:"-"` // DT_LOGS_TOKEN, env only
}

// Config is the whole of ghlog2dt's configuration.
type Config struct {
	Owner        string            `json:"owner"`
	Targets      []string          `json:"targets"`
	All          bool              `json:"all"`
	Include      []string          `json:"include"`
	Exclude      []string          `json:"exclude"`
	Workflows    []string          `json:"workflows"`
	Lookback     Duration          `json:"lookback"`
	MaxAge       Duration          `json:"max_age"`
	PendingTTL   Duration          `json:"pending_ttl"`
	ShippedTTL   Duration          `json:"shipped_ttl"`
	StateDir     string            `json:"state_dir"`
	GitHubAPI    string            `json:"github_api"`
	APIVersion   string            `json:"github_api_version"`
	Concurrency  int               `json:"concurrency"`
	MaxLineBytes int               `json:"max_line_bytes"`
	MaxLogBytes  int64             `json:"max_log_bytes"`
	Stale        string            `json:"stale"`
	DropNoise    bool              `json:"drop_noise"`
	StripANSI    bool              `json:"strip_ansi"`
	SelfLog      bool              `json:"self_log"`
	Attributes   map[string]string `json:"attributes"`
	Redact       []string          `json:"redact"`
	DT           Dynatrace         `json:"dt"`

	GitHubToken string `json:"-"` // GH_TOKEN or GITHUB_TOKEN, env only
}

// Dynatrace's documented limits for POST /api/v2/logs/ingest.
const (
	DTMaxRequestBytes = 10 << 20 // 10 MB
	DTMaxRecords      = 50000
	DTMaxLogAge       = 24 * time.Hour
)

// DefaultRedact catches credentials GitHub's own masking can miss:
// tokens printed by tools, or values never registered as secrets.
var DefaultRedact = []string{
	`gh[pousr]_[A-Za-z0-9]{36,255}`,
	`github_pat_[A-Za-z0-9_]{22,255}`,
	`(?i)dt0[a-z][0-9]{2}\.[a-z0-9]{24}\.[a-z0-9]{64}`,
	`(?:AKIA|ASIA)[0-9A-Z]{16}`,
	`-----BEGIN [A-Z ]*PRIVATE KEY-----`,
	`(?i)(?:authorization|x-api-key)\s*[:=]\s*(?:bearer\s+|token\s+|api-token\s+)?[A-Za-z0-9._~+/=-]{16,}`,
	`xox[baprs]-[A-Za-z0-9-]{10,}`,
}

// Defaults returns the built-in configuration.
func Defaults() *Config {
	return &Config{
		Owner:        "CoolGitOrg",
		Lookback:     Duration(90 * time.Minute),
		MaxAge:       Duration(23*time.Hour + 45*time.Minute),
		PendingTTL:   Duration(12 * time.Hour),
		ShippedTTL:   Duration(48 * time.Hour),
		StateDir:     "state",
		GitHubAPI:    "https://api.github.com",
		APIVersion:   "2022-11-28",
		Concurrency:  4,
		MaxLineBytes: 64 << 10,
		MaxLogBytes:  32 << 20,
		Stale:        "drop",
		DropNoise:    true,
		StripANSI:    true,
		SelfLog:      true,
		Attributes:   map[string]string{"log.source": "github-actions"},
		Redact:       append([]string(nil), DefaultRedact...),
		DT: Dynatrace{
			AuthScheme:   "Api-Token",
			BatchBytes:   4 << 20,
			BatchRecords: 5000,
			Timeout:      Duration(60 * time.Second),
			MaxRetries:   5,
		},
	}
}

// Load reads a JSON config file over the defaults. A missing file is an
// error only when required is true (the user named it explicitly).
func Load(path string, required bool) (*Config, error) {
	c := Defaults()
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if c.Attributes == nil {
		c.Attributes = map[string]string{}
	}
	if _, ok := c.Attributes["log.source"]; !ok {
		c.Attributes["log.source"] = "github-actions"
	}
	return c, nil
}

// ApplyEnv overlays environment variables. getenv is os.Getenv in
// production and a map lookup in tests.
func (c *Config) ApplyEnv(getenv func(string) string) error {
	if v := getenv("GHLOG_TARGET_REPOS"); v != "" {
		c.Targets = SplitList(v)
	}
	if v := getenv("GHLOG_LOOKBACK"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("GHLOG_LOOKBACK: %w", err)
		}
		c.Lookback = Duration(d)
	}
	if v := getenv("GHLOG_STATE_DIR"); v != "" {
		c.StateDir = v
	}
	if v := getenv("GHLOG_CONCURRENCY"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return fmt.Errorf("GHLOG_CONCURRENCY: %w", err)
		}
		c.Concurrency = n
	}
	if v := getenv("GHLOG_GZIP"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return fmt.Errorf("GHLOG_GZIP: %w", err)
		}
		c.DT.Gzip = b
	}
	if v := getenv("GITHUB_API_URL"); v != "" {
		c.GitHubAPI = v
	}
	if v := getenv("DT_URL"); v != "" {
		c.DT.URL = v
	}
	if v := getenv("DT_AUTH_SCHEME"); v != "" {
		c.DT.AuthScheme = v
	}
	c.DT.Token = strings.TrimSpace(getenv("DT_LOGS_TOKEN"))
	c.GitHubToken = strings.TrimSpace(getenv("GH_TOKEN"))
	if c.GitHubToken == "" {
		c.GitHubToken = strings.TrimSpace(getenv("GITHUB_TOKEN"))
	}
	return nil
}

// SplitList splits a comma, space or newline separated list; '#'
// starts a comment that runs to the end of its line.
func SplitList(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		out = append(out, strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\r'
		})...)
	}
	return out
}

var repoRe = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)

// ValidRepo reports whether s looks like owner/name.
func ValidRepo(s string) bool { return repoRe.MatchString(s) && !strings.Contains(s, "..") }

// Need says which credentials a subcommand requires.
type Need struct {
	GitHub    bool
	Dynatrace bool
}

// Validate checks the configuration for a subcommand.
func (c *Config) Validate(n Need) error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	for _, r := range c.Targets {
		if !ValidRepo(r) {
			bad("target %q is not owner/name", r)
		}
	}
	if c.Lookback.D() <= 0 {
		bad("lookback must be > 0")
	}
	if c.MaxAge.D() <= 0 || c.MaxAge.D() > DTMaxLogAge {
		bad("max_age must be in (0, 24h]: Dynatrace rejects older logs")
	}
	if c.Lookback.D() > c.MaxAge.D() {
		bad("lookback %s exceeds max_age %s", c.Lookback.D(), c.MaxAge.D())
	}
	if c.PendingTTL.D() <= 0 || c.ShippedTTL.D() <= 0 {
		bad("pending_ttl and shipped_ttl must be > 0")
	}
	if c.ShippedTTL.D() < c.MaxAge.D() {
		bad("shipped_ttl %s must be >= max_age %s or jobs could ship twice", c.ShippedTTL.D(), c.MaxAge.D())
	}
	if c.Concurrency < 1 || c.Concurrency > 16 {
		bad("concurrency must be in [1,16] (GitHub secondary limits)")
	}
	if c.MaxLineBytes < 256 {
		bad("max_line_bytes must be >= 256")
	}
	if c.MaxLogBytes < 1<<20 {
		bad("max_log_bytes must be >= 1MiB")
	}
	if c.Stale != "drop" && c.Stale != "rewrite" {
		bad("stale must be drop or rewrite")
	}
	if c.DT.AuthScheme != "Api-Token" && c.DT.AuthScheme != "Bearer" {
		bad("dt.auth_scheme must be Api-Token or Bearer")
	}
	if c.DT.BatchBytes < 1<<10 || c.DT.BatchBytes > DTMaxRequestBytes {
		bad("dt.batch_bytes must be in [1KiB, 10MB]")
	}
	if c.DT.BatchRecords < 1 || c.DT.BatchRecords > DTMaxRecords {
		bad("dt.batch_records must be in [1, 50000]")
	}
	if c.DT.MaxRetries < 0 || c.DT.MaxRetries > 10 {
		bad("dt.max_retries must be in [0,10]")
	}
	if err := checkURL(c.GitHubAPI); err != nil {
		bad("github_api: %v", err)
	}
	if _, err := c.Redactors(); err != nil {
		errs = append(errs, err)
	}
	for _, p := range append(append([]string{}, c.Include...), c.Exclude...) {
		if _, err := matchGlob(p, "a/b"); err != nil {
			bad("bad glob %q: %v", p, err)
		}
	}
	if n.GitHub && c.GitHubToken == "" {
		bad("GH_TOKEN (or GITHUB_TOKEN) is not set")
	}
	if n.Dynatrace {
		if c.DT.URL == "" {
			bad("DT_URL / --dt-url is not set")
		} else if err := checkURL(c.DT.URL); err != nil {
			bad("dt url: %v", err)
		}
		if c.DT.Token == "" {
			bad("DT_LOGS_TOKEN is not set")
		}
	}
	return errors.Join(errs...)
}

// checkURL insists on https, except plain http to loopback (tests).
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Host == "" {
		return fmt.Errorf("%q has no host", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		h := u.Hostname()
		if h == "localhost" {
			return nil
		}
		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("%q: plain http is allowed only to loopback", raw)
	}
	return fmt.Errorf("%q: scheme must be https", raw)
}

// Redactors compiles the redaction patterns.
func (c *Config) Redactors() ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(c.Redact))
	for _, p := range c.Redact {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("redact pattern %q: %w", p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// String renders the effective configuration with secrets elided.
func (c *Config) String() string {
	b, _ := json.MarshalIndent(c, "", "  ")
	s := string(b)
	return s + fmt.Sprintf("\n# GH_TOKEN set: %t, DT_LOGS_TOKEN set: %t\n", c.GitHubToken != "", c.DT.Token != "")
}

// Selected filters repos through the include/exclude globs.
func (c *Config) Selected(repo string) bool {
	if len(c.Include) > 0 {
		ok := false
		for _, p := range c.Include {
			if m, _ := matchGlob(p, repo); m {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, p := range c.Exclude {
		if m, _ := matchGlob(p, repo); m {
			return false
		}
	}
	return true
}

// WorkflowSelected filters by workflow name or file path.
func (c *Config) WorkflowSelected(name, path string) bool {
	if len(c.Workflows) == 0 {
		return true
	}
	for _, p := range c.Workflows {
		if m, _ := matchGlob(p, name); m {
			return true
		}
		if m, _ := matchGlob(p, path); m {
			return true
		}
	}
	return false
}
