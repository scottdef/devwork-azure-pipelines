// Package issueform models GitHub issue forms and parses submitted issue
// bodies into the same JSON shape produced by issue-ops/parser
// (@github/issue-parser with a template and slugify enabled):
//
//   - input / textarea -> string ("" when empty)
//   - dropdown         -> []string (split on ", ")
//   - checkboxes       -> {"selected": [...], "unselected": [...]}
//
// Issue form YAML is converted to JSON with `yq -o=json` (see `make forms`)
// so this package needs no YAML dependency.
package issueform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Form is an issue form template.
type Form struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Title       string    `json:"title"`
	Labels      []string  `json:"labels"`
	Body        []Element `json:"body"`
}

// Element is one body element of an issue form.
type Element struct {
	Type        string      `json:"type"`
	ID          string      `json:"id,omitempty"`
	Attributes  Attributes  `json:"attributes"`
	Validations Validations `json:"validations"`
}

// Attributes of a form element. Options is raw because dropdown options are
// strings while checkbox options are objects.
type Attributes struct {
	Label       string          `json:"label"`
	Description string          `json:"description,omitempty"`
	Placeholder string          `json:"placeholder,omitempty"`
	Value       string          `json:"value,omitempty"`
	Options     json.RawMessage `json:"options,omitempty"`
	Multiple    bool            `json:"multiple,omitempty"`
	Default     *int            `json:"default,omitempty"`
	Render      string          `json:"render,omitempty"`
}

// Validations of a form element.
type Validations struct {
	Required bool `json:"required"`
}

// CheckboxOption is one checkbox.
type CheckboxOption struct {
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

// Field is the normalized view of an input element.
type Field struct {
	Key       string
	Label     string
	Type      string
	Required  bool
	Options   []string
	Checkbox  []CheckboxOption
	Multiple  bool
	HasRender bool
}

// Fields returns the non-markdown elements keyed as the parser keys them.
func (f *Form) Fields() ([]Field, error) {
	var out []Field
	for _, e := range f.Body {
		if e.Type == "markdown" {
			continue
		}
		fd := Field{Key: e.ID, Label: strings.TrimSpace(e.Attributes.Label), Type: e.Type, Required: e.Validations.Required, Multiple: e.Attributes.Multiple, HasRender: e.Attributes.Render != ""}
		if fd.Key == "" {
			fd.Key = Slugify(fd.Label)
		}
		switch e.Type {
		case "dropdown":
			if err := json.Unmarshal(e.Attributes.Options, &fd.Options); err != nil {
				return nil, fmt.Errorf("issueform: dropdown %s options: %w", fd.Key, err)
			}
		case "checkboxes":
			if err := json.Unmarshal(e.Attributes.Options, &fd.Checkbox); err != nil {
				return nil, fmt.Errorf("issueform: checkboxes %s options: %w", fd.Key, err)
			}
		case "input", "textarea":
		default:
			return nil, fmt.Errorf("issueform: unsupported element type %q", e.Type)
		}
		out = append(out, fd)
	}
	return out, nil
}

// LoadForm reads a template that was converted to JSON.
func LoadForm(path string) (*Form, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f Form
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("issueform: %s: %w", path, err)
	}
	return &f, nil
}

// LoadFormForTemplate maps "copilot-budget-request.yml" to <dir>/copilot-budget-request.json.
func LoadFormForTemplate(formsDir, template string) (*Form, error) {
	name := strings.TrimSuffix(template, filepath.Ext(template)) + ".json"
	return LoadForm(filepath.Join(formsDir, name))
}

var (
	slugNonWord = regexp.MustCompile(`[^a-z0-9_]`)
	slugMulti   = regexp.MustCompile(`_+`)
)

// Slugify mirrors @github/issue-parser: trim, lowercase, spaces -> "_",
// strip other symbols, collapse and trim underscores.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = slugNonWord.ReplaceAllString(s, "")
	s = slugMulti.ReplaceAllString(s, "_")
	return strings.Trim(s, "_")
}

// Checkboxes is the parsed value of a checkboxes element.
type Checkboxes struct {
	Selected   []string `json:"selected"`
	Unselected []string `json:"unselected"`
}

var checkboxLine = regexp.MustCompile(`^\s*[-*]\s+\[([ xX])\]\s+(.*)$`)

func isEmptyResponse(v string) bool {
	switch strings.TrimSpace(v) {
	case "", "_No response_", "None":
		return true
	}
	return false
}

// Parse converts an issue-form body into Values keyed by element id.
func Parse(body string, form *Form) (Values, error) {
	fields, err := form.Fields()
	if err != nil {
		return nil, err
	}
	byLabel := map[string]Field{}
	for _, f := range fields {
		byLabel[f.Label] = f
	}
	sections := splitSections(body)
	out := Values{}
	for _, sec := range sections {
		f, ok := byLabel[sec.heading]
		if !ok {
			continue // unknown heading: ignored, like the upstream parser with a template
		}
		raw := strings.TrimSpace(sec.content)
		switch f.Type {
		case "input", "textarea":
			if isEmptyResponse(raw) {
				out[f.Key] = ""
			} else {
				out[f.Key] = raw
			}
		case "dropdown":
			if isEmptyResponse(raw) {
				out[f.Key] = []string{}
				continue
			}
			parts := strings.Split(raw, ", ")
			vals := make([]string, 0, len(parts))
			for _, p := range parts {
				if p = strings.TrimSpace(p); p != "" {
					vals = append(vals, p)
				}
			}
			out[f.Key] = vals
		case "checkboxes":
			cb := Checkboxes{Selected: []string{}, Unselected: []string{}}
			if !isEmptyResponse(raw) {
				for _, line := range strings.Split(raw, "\n") {
					m := checkboxLine.FindStringSubmatch(line)
					if m == nil {
						continue
					}
					label := strings.TrimSpace(m[2])
					if m[1] == " " {
						cb.Unselected = append(cb.Unselected, label)
					} else {
						cb.Selected = append(cb.Selected, label)
					}
				}
			}
			out[f.Key] = cb
		}
	}
	return out, nil
}

type section struct{ heading, content string }

func splitSections(body string) []section {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var out []section
	var cur *section
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "### ") {
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &section{heading: strings.TrimSpace(strings.TrimPrefix(line, "### "))}
			continue
		}
		if cur != nil {
			cur.content += line + "\n"
		}
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

// Render produces the Markdown body GitHub would create for the given values.
// Used to build examples and tests.
func Render(form *Form, v Values) (string, error) {
	fields, err := form.Fields()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for i, f := range fields {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "### %s\n\n", f.Label)
		switch f.Type {
		case "checkboxes":
			sel := map[string]bool{}
			for _, s := range v.Selected(f.Key) {
				sel[s] = true
			}
			for _, o := range f.Checkbox {
				mark := " "
				if sel[o.Label] {
					mark = "X"
				}
				fmt.Fprintf(&b, "- [%s] %s\n", mark, o.Label)
			}
		case "dropdown":
			l := v.List(f.Key)
			if len(l) == 0 {
				b.WriteString("_No response_\n")
			} else {
				b.WriteString(strings.Join(l, ", ") + "\n")
			}
		default:
			s := v.String(f.Key)
			if s == "" {
				s = "_No response_"
			}
			b.WriteString(s + "\n")
		}
	}
	return b.String(), nil
}

// CheckDrift compares a form against a list of expected keys; used by CI to
// guarantee the registry's field references exist in the template.
func CheckDrift(form *Form, requiredKeys []string) error {
	fields, err := form.Fields()
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, f := range fields {
		have[f.Key] = true
	}
	var missing []string
	for _, k := range requiredKeys {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("issueform: template %q lacks fields %v", form.Name, missing)
	}
	return nil
}
