// Package config reads one JSON file and a few environment variables.
// Flags win over environment, environment wins over the file, the file
// wins over defaults. There is nothing else to know.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Config is everything the program needs to find Foundry and GitHub.
type Config struct {
	Subscription  string `json:"subscription,omitempty"` // empty: az default
	ResourceGroup string `json:"resource_group"`
	Account       string `json:"account"`
	Location      string `json:"location"`

	Repo      string    `json:"repo"` // OWNER/NAME holding the workflows
	Ref       string    `json:"ref"`
	GitHubAPI string    `json:"github_api"` // https://api.github.com or https://api.SUBDOMAIN.ghe.com
	Workflows Workflows `json:"workflows"`

	TokenResource  string   `json:"token_resource"` // audience for data-plane tokens
	StateDir       string   `json:"state_dir"`      // audit log and probe history
	ReportDir      string   `json:"report_dir"`
	TemplateDir    string   `json:"template_dir,omitempty"` // overrides embedded templates
	RefreshSeconds int      `json:"refresh_seconds"`
	Metrics        []string `json:"metrics"` // only read when the role allows it
	Copilot        Copilot  `json:"copilot"`
}

// Workflows names the workflow files that perform writes.
type Workflows struct {
	Deploy string `json:"deploy"`
	Report string `json:"report"`
	Test   string `json:"test"`
}

// Copilot configures the Copilot CLI probe.
type Copilot struct {
	Bin            string   `json:"bin"`
	ProviderType   string   `json:"provider_type"` // openai | azure | anthropic; anthropic deployments force anthropic
	WireAPI        string   `json:"wire_api"`      // completions | responses
	Args           []string `json:"args"`          // appended after: -p PROMPT
	Offline        bool     `json:"offline"`       // COPILOT_OFFLINE=true: never call GitHub
	TimeoutSeconds int      `json:"timeout_seconds"`
}

// Default matches the standing assumptions of this project.
func Default() Config {
	return Config{
		ResourceGroup: "rg-coolgit-copilot",
		Account:       "ais-coolgit-copilot-prod",
		Location:      "eastus2",
		Repo:          "CoolGitOrg/foundry-ops",
		Ref:           "main",
		GitHubAPI:     "https://api.github.com",
		Workflows: Workflows{
			Deploy: "foundry-deploy.yml",
			Report: "foundry-report.yml",
			Test:   "foundry-model-test.yml",
		},
		TokenResource:  "https://cognitiveservices.azure.com",
		StateDir:       defaultStateDir(),
		ReportDir:      "reports",
		RefreshSeconds: 120,
		Metrics:        []string{"ModelRequests", "InputTokens", "OutputTokens"},
		Copilot: Copilot{
			Bin:            "copilot",
			ProviderType:   "openai",
			WireAPI:        "completions",
			Args:           []string{"-s", "--no-ask-user"},
			Offline:        true,
			TimeoutSeconds: 120,
		},
	}
}

func defaultStateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "foundry-tui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".foundry-tui"
	}
	return filepath.Join(home, ".local", "state", "foundry-tui")
}

// Path returns the config file to read: the argument, $FOUNDRY_TUI_CONFIG,
// ./foundry-tui.json, or the user config directory, in that order.
func Path(flag string) string {
	if flag != "" {
		return flag
	}
	if p := os.Getenv("FOUNDRY_TUI_CONFIG"); p != "" {
		return p
	}
	if _, err := os.Stat("foundry-tui.json"); err == nil {
		return "foundry-tui.json"
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "foundry-tui", "config.json")
	}
	return "foundry-tui.json"
}

// Load reads path over the defaults. A missing file is not an error.
func Load(path string) (Config, error) {
	c := Default()
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return c, err
	default:
		dec := json.NewDecoder(strings.NewReader(string(b)))
		dec.DisallowUnknownFields() // a typo in a key should be loud
		if err := dec.Decode(&c); err != nil {
			return c, fmt.Errorf("%s: %w", path, err)
		}
	}
	env := map[string]*string{
		"AZURE_SUBSCRIPTION_ID":  &c.Subscription,
		"FOUNDRY_SUBSCRIPTION":   &c.Subscription,
		"FOUNDRY_RESOURCE_GROUP": &c.ResourceGroup,
		"FOUNDRY_ACCOUNT":        &c.Account,
		"FOUNDRY_LOCATION":       &c.Location,
		"FOUNDRY_REPO":           &c.Repo,
		"FOUNDRY_REF":            &c.Ref,
		"FOUNDRY_STATE_DIR":      &c.StateDir,
		"FOUNDRY_REPORT_DIR":     &c.ReportDir,
	}
	for k, p := range env {
		if v := os.Getenv(k); v != "" {
			*p = v
		}
	}
	c.StateDir = expand(c.StateDir)
	c.ReportDir = expand(c.ReportDir)
	return c, c.Validate()
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

var (
	reName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,89}$`)
	reRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)
	reFile = regexp.MustCompile(`^[A-Za-z0-9._-]+\.ya?ml$`)
)

// Validate rejects values that would later be pasted into a command line.
func (c Config) Validate() error {
	checks := []struct {
		name, val string
		re        *regexp.Regexp
	}{
		{"resource_group", c.ResourceGroup, reName},
		{"account", c.Account, reName},
		{"location", c.Location, reName},
		{"repo", c.Repo, reRepo},
		{"workflows.deploy", c.Workflows.Deploy, reFile},
		{"workflows.report", c.Workflows.Report, reFile},
		{"workflows.test", c.Workflows.Test, reFile},
	}
	for _, k := range checks {
		if !k.re.MatchString(k.val) {
			return fmt.Errorf("config: %s=%q is not valid", k.name, k.val)
		}
	}
	if c.Ref == "" || strings.HasPrefix(c.Ref, "-") {
		return fmt.Errorf("config: ref=%q is not valid", c.Ref)
	}
	if !strings.HasPrefix(c.GitHubAPI, "https://") {
		return fmt.Errorf("config: github_api must be https")
	}
	return nil
}
