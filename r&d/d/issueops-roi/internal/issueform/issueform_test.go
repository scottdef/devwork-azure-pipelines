package issueform

import (
	"testing"

	"github.com/CoolEngOrg/issueops/internal/testutil"
)

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Requested monthly budget (USD)": "requested_monthly_budget_usd",
		"  Model format (publisher) ":    "model_format_publisher",
		"A -- B":                         "a_b",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderParseRoundTripAllForms(t *testing.T) {
	for _, name := range []string{"copilot-budget-request.yml", "copilot-usage-report.yml", "foundry-model-deployment.yml", "agentic-task-request.yml"} {
		form, err := LoadFormForTemplate(testutil.Path(t, "config", "forms"), name)
		if err != nil {
			t.Fatalf("%s: %v (run scripts/forms-to-json.sh)", name, err)
		}
		fields, err := form.Fields()
		if err != nil {
			t.Fatal(err)
		}
		in := Values{}
		for _, f := range fields {
			switch f.Type {
			case "dropdown":
				in[f.Key] = []string{f.Options[0]}
			case "checkboxes":
				in[f.Key] = map[string]any{"selected": []any{f.Checkbox[0].Label}}
			default:
				in[f.Key] = "value for " + f.Key + "\nsecond line"
			}
		}
		body, err := Render(form, in)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Parse(body, form)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fields {
			switch f.Type {
			case "dropdown":
				if out.String(f.Key) != f.Options[0] {
					t.Errorf("%s/%s: dropdown %q", name, f.Key, out.String(f.Key))
				}
			case "checkboxes":
				if !out.IsSelected(f.Key, f.Checkbox[0].Label) || len(out.Selected(f.Key)) != 1 {
					t.Errorf("%s/%s: checkboxes %v", name, f.Key, out.Selected(f.Key))
				}
			default:
				if out.String(f.Key) != in.String(f.Key) {
					t.Errorf("%s/%s: %q != %q", name, f.Key, out.String(f.Key), in.String(f.Key))
				}
			}
		}
	}
}

func TestNoResponseAndUnknownHeadings(t *testing.T) {
	form, err := LoadFormForTemplate(testutil.Path(t, "config", "forms"), "copilot-budget-request.yml")
	if err != nil {
		t.Fatal(err)
	}
	v, err := Parse("### Scope target\n\n_No response_\n\n### Injected heading\n\nignored\n\n### Metered product\n\nAI credits\n", form)
	if err != nil {
		t.Fatal(err)
	}
	if v.String("budget_target") != "" || v.String("budget_product") != "AI credits" {
		t.Fatalf("values = %#v", v)
	}
	if len(v) != 2 {
		t.Fatalf("unknown heading produced a value: %#v", v)
	}
}
