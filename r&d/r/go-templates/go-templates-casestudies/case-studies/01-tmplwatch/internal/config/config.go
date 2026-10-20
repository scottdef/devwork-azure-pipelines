// Package config loads the tmplwatch JSON configuration.
//
// The format is JSON because the standard library parses it and nothing
// else; see docs/DESIGN.md for the trade-off against HCL.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Duration is a time.Duration that unmarshals from a Go duration string
// such as "15s" or "2m".
type Duration time.Duration

// UnmarshalJSON accepts a duration string or a number of nanoseconds.
func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		var n int64
		if err2 := json.Unmarshal(b, &n); err2 != nil {
			return fmt.Errorf("duration: want string like \"15s\": %w", err)
		}
		*d = Duration(n)
		return nil
	}
	if s == "" {
		*d = 0
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("duration: %w", err)
	}
	*d = Duration(v)
	return nil
}

// MarshalJSON writes the duration as a string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// D returns the value as a time.Duration.
func (d Duration) D() time.Duration { return time.Duration(d) }

// Source describes where key/value data comes from.
type Source struct {
	Type     string   `json:"type"`     // "github" or "dir"
	Repo     string   `json:"repo"`     // github: owner/name
	Ref      string   `json:"ref"`      // github: branch, tag or sha
	Prefix   string   `json:"prefix"`   // key prefix stripped from paths
	Path     string   `json:"path"`     // dir: root directory
	Interval Duration `json:"interval"` // poll interval
	APIURL   string   `json:"api_url"`  // github: API base, default https://api.github.com
	TokenEnv string   `json:"token_env"`

	// GitHub App authentication. When AppClientID is set the source mints
	// installation tokens instead of reading TokenEnv.
	AppClientID       string `json:"app_client_id"`
	AppClientIDEnv    string `json:"app_client_id_env"`
	AppPrivateKeyFile string `json:"app_private_key_file"`
	AppPrivateKeyEnv  string `json:"app_private_key_env"`
}

// Template is one template → destination mapping.
type Template struct {
	Name              string   `json:"name"`
	Source            string   `json:"source"`
	Destination       string   `json:"destination"`
	Perms             string   `json:"perms"`
	Backup            bool     `json:"backup"`
	Command           []string `json:"command"`
	CommandTimeout    Duration `json:"command_timeout"`
	Splay             Duration `json:"splay"`
	Wait              Duration `json:"wait"`
	LeftDelimiter     string   `json:"left_delimiter"`
	RightDelimiter    string   `json:"right_delimiter"`
	ErrorOnMissingKey bool     `json:"error_on_missing_key"`
}

// Exec describes an optional supervised child process.
type Exec struct {
	Command      []string `json:"command"`
	ChangeSignal string   `json:"change_signal"`
	KillSignal   string   `json:"kill_signal"`
	KillTimeout  Duration `json:"kill_timeout"`
}

// Config is the whole file.
type Config struct {
	Source    Source     `json:"source"`
	Templates []Template `json:"templates"`
	Exec      Exec       `json:"exec"`
	Listen    string     `json:"listen"`
	Dry       bool       `json:"dry"`
	Once      bool       `json:"once"`
}

// Load reads and validates the file at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes and validates a configuration, filling defaults.
// Unknown fields are errors: a typo should not silently disable a feature.
func Parse(b []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, err
	}
	if err := c.fill(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) fill() error {
	s := &c.Source
	switch s.Type {
	case "dir":
		if s.Path == "" {
			return errors.New("source.path required for dir source")
		}
	case "github":
		if strings.Count(s.Repo, "/") != 1 {
			return fmt.Errorf("source.repo %q: want owner/name", s.Repo)
		}
		if s.Ref == "" {
			s.Ref = "main"
		}
		if s.APIURL == "" {
			s.APIURL = "https://api.github.com"
		}
		if s.TokenEnv == "" && s.AppClientID == "" && s.AppClientIDEnv == "" {
			s.TokenEnv = "GITHUB_TOKEN"
		}
	default:
		return fmt.Errorf("source.type %q: want github or dir", s.Type)
	}
	if s.Prefix != "" && !strings.HasSuffix(s.Prefix, "/") {
		s.Prefix += "/"
	}
	if s.Interval == 0 {
		s.Interval = Duration(15 * time.Second)
	}
	if s.Interval.D() < time.Second {
		return fmt.Errorf("source.interval %v: minimum is 1s", s.Interval.D())
	}
	if len(c.Templates) == 0 {
		return errors.New("no templates")
	}
	seen := map[string]bool{}
	for i := range c.Templates {
		t := &c.Templates[i]
		if t.Source == "" || t.Destination == "" {
			return fmt.Errorf("templates[%d]: source and destination required", i)
		}
		if t.Name == "" {
			t.Name = strings.TrimSuffix(filepath.Base(t.Source), ".tmpl")
		}
		if seen[t.Name] {
			return fmt.Errorf("templates[%d]: duplicate name %q (set \"name\")", i, t.Name)
		}
		seen[t.Name] = true
		if t.Perms == "" {
			t.Perms = "0644"
		}
		if _, err := t.Mode(); err != nil {
			return fmt.Errorf("templates[%d]: %w", i, err)
		}
		if (t.LeftDelimiter == "") != (t.RightDelimiter == "") {
			return fmt.Errorf("templates[%d]: set both delimiters or neither", i)
		}
		if t.CommandTimeout == 0 {
			t.CommandTimeout = Duration(30 * time.Second)
		}
	}
	if c.Exec.KillTimeout == 0 {
		c.Exec.KillTimeout = Duration(5 * time.Second)
	}
	if c.Exec.KillSignal == "" {
		c.Exec.KillSignal = "SIGTERM"
	}
	if c.Listen == "" {
		c.Listen = "127.0.0.1:9180"
	}
	return nil
}

// Mode parses Perms as an octal file mode.
func (t *Template) Mode() (os.FileMode, error) {
	n, err := strconv.ParseUint(t.Perms, 8, 32)
	if err != nil || n > 0o777 {
		return 0, fmt.Errorf("perms %q: want octal like 0644", t.Perms)
	}
	return os.FileMode(n), nil
}
