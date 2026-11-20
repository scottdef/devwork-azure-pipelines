// Package manifest defines cool.json, the file in which an app describes itself.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Manifest is the content of cool.json.
type Manifest struct {
	Team         string              `json:"team"`
	App          string              `json:"app"`
	Preset       string              `json:"preset"`
	Expose       string              `json:"expose,omitempty"` // internal or public
	Port         int                 `json:"port,omitempty"`
	Health       string              `json:"health,omitempty"`
	Scale        Scale               `json:"scale"`
	Resources    Resources           `json:"resources"`
	Env          map[string]string   `json:"env,omitempty"`
	Secrets      []string            `json:"secrets,omitempty"`
	Routes       []Route             `json:"routes,omitempty"`
	Crons        []Cron              `json:"crons,omitempty"`
	Flags        map[string]any      `json:"flags,omitempty"`
	API          *API                `json:"api,omitempty"`
	AI           *AI                 `json:"ai,omitempty"`
	Environments map[string]Override `json:"environments,omitempty"`
}

// Scale bounds the replica count and names the CPU target of the autoscaler.
type Scale struct {
	Min int `json:"min"`
	Max int `json:"max"`
	CPU int `json:"cpu"`
}

// Resources is what one replica requests.
type Resources struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
}

// Route is one routing rule: a prefix with a timeout or headers, or a redirect.
type Route struct {
	Prefix   string            `json:"prefix,omitempty"`
	Timeout  string            `json:"timeout,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Redirect *Redirect         `json:"redirect,omitempty"`
}

// Redirect sends an exact path elsewhere.
type Redirect struct {
	From string `json:"from"`
	To   string `json:"to"`
	Code int    `json:"code"`
}

// Cron calls a path of the app on a schedule.
type Cron struct {
	Path     string `json:"path"`
	Schedule string `json:"schedule"`
}

// API names the OpenAPI file that the APIM publish flow reads.
type API struct {
	OpenAPI string `json:"openapi"`
	Product string `json:"product"`
	Auth    string `json:"auth,omitempty"` // none, subscription or entra-jwt
}

// AI opts the app into the cool-ai gateway with a named token budget.
type AI struct {
	Budget string `json:"budget"` // small, medium or large
}

// Override adjusts a manifest for one environment.
type Override struct {
	Scale     *Scale            `json:"scale,omitempty"`
	Resources *Resources        `json:"resources,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

// Presets lists the build presets the platform supplies.
var Presets = []string{"go-server", "go-htmx", "static", "dockerfile"}

// Environments lists the deployment environments, in promotion order.
var Environments = []string{"sbx", "dev", "tst", "prd"}

var (
	reTeam    = regexp.MustCompile(`^[a-z][a-z0-9]{1,7}$`)
	reApp     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,18}[a-z0-9]$`)
	rePreview = regexp.MustCompile(`-pr-[0-9]+$`)
	reKey     = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
	reCPU     = regexp.MustCompile(`^[1-9][0-9]{1,4}m$`)
	reMemory  = regexp.MustCompile(`^[1-9][0-9]{1,4}(Mi|Gi)$`)
	reCron    = regexp.MustCompile(`^\S+(\s+\S+){4}$`)
	rePath    = regexp.MustCompile(`^/[A-Za-z0-9/_.~%-]*$`)
	reHeader  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,63}$`)
	reHost    = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$`)
)

// ValidTeam reports whether s is a team slug.
func ValidTeam(s string) bool { return reTeam.MatchString(s) }

// ValidApp reports whether s is an app slug. A name that ends in -pr-<n> is
// refused because preview namespaces use that suffix.
func ValidApp(s string) bool { return reApp.MatchString(s) && !rePreview.MatchString(s) }

// ValidEnv reports whether s names an environment.
func ValidEnv(s string) bool { return slices.Contains(Environments, s) }

// ValidKey reports whether s can name an environment variable or secret.
func ValidKey(s string) bool {
	return reKey.MatchString(s) && s != "PORT" && !strings.HasPrefix(s, "COOL_")
}

// ValidHost reports whether s is a lower-case DNS name.
func ValidHost(s string) bool { return len(s) <= 253 && reHost.MatchString(s) }

// Default returns a manifest with the platform defaults filled in.
func Default(team, app, preset string) Manifest {
	m := Manifest{Team: team, App: app, Preset: preset}
	m.Defaults()
	return m
}

// Defaults fills unset fields with the platform defaults.
func (m *Manifest) Defaults() {
	if m.Expose == "" {
		m.Expose = "internal"
	}
	if m.Port == 0 {
		m.Port = 8080
	}
	if m.Health == "" {
		m.Health = "/healthz"
	}
	if m.Scale == (Scale{}) {
		m.Scale = Scale{Min: 2, Max: 4, CPU: 70}
	}
	if m.Resources == (Resources{}) {
		m.Resources = Resources{CPU: "250m", Memory: "256Mi"}
	}
}

// Load reads, defaults and validates the manifest at path.
func Load(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	return Parse(b)
}

// Parse decodes, defaults and validates a manifest. Unknown fields are errors,
// so a typo fails here instead of being ignored in production.
func Parse(b []byte) (Manifest, error) {
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("cool.json: %w", err)
	}
	m.Defaults()
	return m, m.Validate()
}

// Validate reports every problem in m, joined into one error.
func (m Manifest) Validate() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !ValidTeam(m.Team) {
		bad("team %q: want 2 to 8 lower-case letters or digits, starting with a letter", m.Team)
	}
	if !ValidApp(m.App) {
		bad("app %q: want 2 to 20 of a-z, 0-9 and -, starting with a letter", m.App)
	}
	if !slices.Contains(Presets, m.Preset) {
		bad("preset %q: want one of %s", m.Preset, strings.Join(Presets, ", "))
	}
	if m.Expose != "internal" && m.Expose != "public" {
		bad("expose %q: want internal or public", m.Expose)
	}
	if m.Port < 1024 || m.Port > 65535 {
		bad("port %d: want 1024 to 65535; the container runs as non-root", m.Port)
	}
	if !rePath.MatchString(m.Health) {
		bad("health %q: want a path such as /healthz", m.Health)
	}
	errs = append(errs, checkScale("scale", m.Scale), checkResources("resources", m.Resources))
	for k := range m.Env {
		if !ValidKey(k) {
			bad("env %q: want A-Z, 0-9 and _; PORT and COOL_* are reserved", k)
		}
	}
	for _, k := range m.Secrets {
		if !ValidKey(k) {
			bad("secret %q: want A-Z, 0-9 and _", k)
		}
	}
	for i, r := range m.Routes {
		errs = append(errs, checkRoute(i, r))
	}
	if len(m.Crons) > 10 {
		bad("crons: at most 10, got %d", len(m.Crons))
	}
	for i, c := range m.Crons {
		if !rePath.MatchString(c.Path) {
			bad("crons[%d].path %q: want a path", i, c.Path)
		}
		if !reCron.MatchString(c.Schedule) {
			bad("crons[%d].schedule %q: want five cron fields", i, c.Schedule)
		}
	}
	if len(m.Crons) > 0 && !slices.Contains(m.Secrets, "CRON_SECRET") {
		bad("crons need CRON_SECRET in secrets; cron calls carry it as a bearer token")
	}
	for k, v := range m.Flags {
		switch v.(type) {
		case string, bool, float64:
		default:
			bad("flags.%s: want a string, number or boolean", k)
		}
	}
	if a := m.API; a != nil {
		if a.OpenAPI == "" || strings.Contains(a.OpenAPI, "..") || strings.HasPrefix(a.OpenAPI, "/") {
			bad("api.openapi %q: want a path inside the repository", a.OpenAPI)
		}
		if !reApp.MatchString(a.Product) {
			bad("api.product %q: want a product slug", a.Product)
		}
		if !slices.Contains([]string{"", "none", "subscription", "entra-jwt"}, a.Auth) {
			bad("api.auth %q: want none, subscription or entra-jwt", a.Auth)
		}
	}
	if m.AI != nil && !slices.Contains([]string{"small", "medium", "large"}, m.AI.Budget) {
		bad("ai.budget %q: want small, medium or large", m.AI.Budget)
	}
	for env, o := range m.Environments {
		if !ValidEnv(env) {
			bad("environments.%s: want one of %s", env, strings.Join(Environments, ", "))
		}
		if o.Scale != nil {
			errs = append(errs, checkScale("environments."+env+".scale", *o.Scale))
		}
		if o.Resources != nil {
			errs = append(errs, checkResources("environments."+env+".resources", *o.Resources))
		}
		for k := range o.Env {
			if !ValidKey(k) {
				bad("environments.%s.env %q: want A-Z, 0-9 and _", env, k)
			}
		}
	}
	return errors.Join(errs...)
}

func checkScale(at string, s Scale) error {
	switch {
	case s.Min < 1 || s.Max > 50 || s.Min > s.Max:
		return fmt.Errorf("%s: want 1 <= min <= max <= 50, got min %d max %d", at, s.Min, s.Max)
	case s.CPU < 10 || s.CPU > 95:
		return fmt.Errorf("%s.cpu %d: want 10 to 95 percent", at, s.CPU)
	}
	return nil
}

func checkResources(at string, r Resources) error {
	switch {
	case !reCPU.MatchString(r.CPU):
		return fmt.Errorf("%s.cpu %q: want millicores such as 250m", at, r.CPU)
	case !reMemory.MatchString(r.Memory):
		return fmt.Errorf("%s.memory %q: want Mi or Gi such as 256Mi", at, r.Memory)
	}
	return nil
}

func checkRoute(i int, r Route) error {
	at := fmt.Sprintf("routes[%d]", i)
	if d := r.Redirect; d != nil {
		switch {
		case r.Prefix != "" || r.Timeout != "" || len(r.Headers) > 0:
			return fmt.Errorf("%s: a redirect carries no prefix, timeout or headers", at)
		case !rePath.MatchString(d.From):
			return fmt.Errorf("%s.redirect.from %q: want a path", at, d.From)
		case !rePath.MatchString(d.To):
			return fmt.Errorf("%s.redirect.to %q: want a path on the same host", at, d.To)
		case !slices.Contains([]int{301, 302, 307, 308}, d.Code):
			return fmt.Errorf("%s.redirect.code %d: want 301, 302, 307 or 308", at, d.Code)
		}
		return nil
	}
	if !rePath.MatchString(r.Prefix) {
		return fmt.Errorf("%s.prefix %q: want a path", at, r.Prefix)
	}
	if r.Timeout != "" {
		if d, err := time.ParseDuration(r.Timeout); err != nil || d <= 0 || d > time.Hour {
			return fmt.Errorf("%s.timeout %q: want a duration up to 1h such as 30s", at, r.Timeout)
		}
	}
	for k := range r.Headers {
		if !reHeader.MatchString(k) {
			return fmt.Errorf("%s.headers %q: not a header name", at, k)
		}
	}
	return nil
}
