package issueform

import "testing"

const body = "### App\n\npayments/checkout\n\n### Target environment\n\nprd\n\n### Release\n\n_No response_\n\n" +
	"### Reason\n\nShip the new cart.\n### not a heading inside text? it is: headings are lines\n\nx\n\n" +
	"### Options\n\n- [x] Notify the team\n- [ ] Skip the gate\n- [X] Keep standby\n\n" +
	"### Config\n\n```json\n{\"a\": 1}\n### inside a fence\n```\n"

func TestParse(t *testing.T) {
	f := Parse(body)
	want := map[string]string{
		"app":                "payments/checkout",
		"target_environment": "prd",
		"release":            "",
		"reason":             "Ship the new cart.",
		"options":            "Notify the team,Keep standby",
		"config":             "{\"a\": 1}\n### inside a fence",
	}
	for k, v := range want {
		if f[k] != v {
			t.Errorf("%s = %q, want %q", k, f[k], v)
		}
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Target environment":   "target_environment",
		"  App (team/app) ":    "app_team_app",
		"OpenAPI path":         "openapi_path",
		"Soak minutes / stage": "soak_minutes_stage",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseWindowsNewlines(t *testing.T) {
	f := Parse("### App\r\n\r\nweb/site\r\n")
	if f["app"] != "web/site" {
		t.Errorf("app = %q", f["app"])
	}
}
