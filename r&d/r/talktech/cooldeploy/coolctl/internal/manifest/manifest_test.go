package manifest

import (
	"strings"
	"testing"
)

const good = `{
  "team": "payments",
  "app": "checkout",
  "preset": "go-htmx",
  "port": 8080,
  "health": "/healthz",
  "scale": {"min": 2, "max": 10, "cpu": 70},
  "resources": {"cpu": "250m", "memory": "256Mi"},
  "env": {"LOG_LEVEL": "info"},
  "secrets": ["CRON_SECRET"],
  "routes": [
    {"prefix": "/api/", "timeout": "30s"},
    {"redirect": {"from": "/old", "to": "/new", "code": 308}}
  ],
  "crons": [{"path": "/internal/cleanup", "schedule": "0 3 * * *"}],
  "flags": {"new_cart": true},
  "api": {"openapi": "api/openapi.yaml", "product": "payments"},
  "environments": {"prd": {"scale": {"min": 3, "max": 12, "cpu": 60}}}
}`

func TestParseGood(t *testing.T) {
	m, err := Parse([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if m.Expose != "internal" {
		t.Errorf("expose = %q, want the default internal", m.Expose)
	}
}

func TestParseDefaults(t *testing.T) {
	m, err := Parse([]byte(`{"team":"web","app":"site","preset":"static"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Port != 8080 || m.Health != "/healthz" || m.Scale.Min != 2 || m.Resources.CPU != "250m" {
		t.Errorf("defaults not applied: %+v", m)
	}
}

func TestParseBad(t *testing.T) {
	tests := []struct{ name, json, want string }{
		{"unknown field", `{"team":"web","app":"site","preset":"static","prot":1}`, "unknown field"},
		{"long team", `{"team":"paymentsx","app":"site","preset":"static"}`, "team"},
		{"preview-like app", `{"team":"web","app":"site-pr-4","preset":"static"}`, "app"},
		{"preset", `{"team":"web","app":"site","preset":"rails"}`, "preset"},
		{"low port", `{"team":"web","app":"site","preset":"static","port":80}`, "port"},
		{"reserved env", `{"team":"web","app":"site","preset":"static","env":{"COOL_ENV":"x"}}`, "reserved"},
		{"cron secret", `{"team":"web","app":"site","preset":"static","crons":[{"path":"/x","schedule":"* * * * *"}]}`, "CRON_SECRET"},
		{"cron fields", `{"team":"web","app":"site","preset":"static","secrets":["CRON_SECRET"],"crons":[{"path":"/x","schedule":"hourly"}]}`, "five cron fields"},
		{"redirect code", `{"team":"web","app":"site","preset":"static","routes":[{"redirect":{"from":"/a","to":"/b","code":200}}]}`, "redirect.code"},
		{"scale", `{"team":"web","app":"site","preset":"static","scale":{"min":5,"max":2,"cpu":70}}`, "scale"},
		{"env override", `{"team":"web","app":"site","preset":"static","environments":{"qa":{}}}`, "environments.qa"},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.json))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want it to mention %q", tt.name, err, tt.want)
		}
	}
}

func TestValidKey(t *testing.T) {
	for k, want := range map[string]bool{"DATABASE_URL": true, "PORT": false, "COOL_APP": false, "lower": false, "A": true} {
		if got := ValidKey(k); got != want {
			t.Errorf("ValidKey(%q) = %v, want %v", k, got, want)
		}
	}
}
