// Package form reads a GitHub issue form (.github/ISSUE_TEMPLATE/*.yml)
// into a small typed structure. The same structure drives both the issue
// body parser and the static intake page, so the two cannot drift.
package form

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/CoolGitOrg/issueops-intake/internal/yamlite"
)

// Form is one issue form.
type Form struct {
	File        string   // base name, e.g. "request.yml"; used as ?template=
	Name        string   // name:
	Description string   // description:
	Title       string   // title: (prefix put in the issue title)
	Labels      []string // labels:
	Fields      []Field  // body: elements with an id or a label
}

// Field is one body element. Markdown elements carry only Value.
type Field struct {
	Type        string   // input, textarea, dropdown, checkboxes, markdown
	ID          string   // id: (may be empty for markdown)
	Label       string   // attributes.label
	Description string   // attributes.description
	Placeholder string   // attributes.placeholder
	Value       string   // attributes.value (markdown text, or prefilled value)
	Render      string   // attributes.render (textarea only)
	Options     []string // dropdown / checkboxes option labels
	Multiple    bool     // dropdown multiple:
	Default     int      // dropdown default: index, -1 when unset
	Required    bool     // validations.required
}

// Load reads and parses one form file.
func Load(path string) (*Form, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.File = filepath.Base(path)
	return f, nil
}

// LoadDir loads every *.yml form in dir except config.yml, sorted by file name.
func LoadDir(dir string) ([]*Form, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out []*Form
	for _, n := range names {
		if filepath.Base(n) == "config.yml" {
			continue
		}
		f, err := Load(n)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Parse parses form YAML.
func Parse(src []byte) (*Form, error) {
	root, err := yamlite.Parse(src)
	if err != nil {
		return nil, err
	}
	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("form: top level is not a mapping")
	}
	f := &Form{
		Name:        str(m["name"]),
		Description: str(m["description"]),
		Title:       str(m["title"]),
		Labels:      strs(m["labels"]),
	}
	if f.Name == "" {
		return nil, fmt.Errorf("form: missing name")
	}
	body, ok := m["body"].([]any)
	if !ok || len(body) == 0 {
		return nil, fmt.Errorf("form: missing body")
	}
	seen := map[string]bool{}
	for i, el := range body {
		em, ok := el.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("form: body[%d] is not a mapping", i)
		}
		attr, _ := em["attributes"].(map[string]any)
		val, _ := em["validations"].(map[string]any)
		fd := Field{
			Type:        str(em["type"]),
			ID:          str(em["id"]),
			Label:       str(attr["label"]),
			Description: str(attr["description"]),
			Placeholder: str(attr["placeholder"]),
			Value:       str(attr["value"]),
			Render:      str(attr["render"]),
			Multiple:    attr["multiple"] == true,
			Default:     -1,
			Required:    val["required"] == true,
		}
		if d, ok := attr["default"].(string); ok {
			fmt.Sscanf(d, "%d", &fd.Default)
		}
		switch fd.Type {
		case "markdown":
		case "input", "textarea":
		case "dropdown":
			fd.Options = strs(attr["options"])
			if len(fd.Options) == 0 {
				return nil, fmt.Errorf("form: dropdown %q has no options", fd.ID)
			}
		case "checkboxes":
			for _, o := range asList(attr["options"]) {
				om, _ := o.(map[string]any)
				fd.Options = append(fd.Options, str(om["label"]))
			}
		default:
			return nil, fmt.Errorf("form: body[%d]: unknown type %q", i, fd.Type)
		}
		if fd.Type != "markdown" {
			if fd.Label == "" {
				return nil, fmt.Errorf("form: body[%d] (%s): missing label", i, fd.Type)
			}
			if fd.ID == "" {
				return nil, fmt.Errorf("form: field %q: missing id (ids key the parsed JSON)", fd.Label)
			}
			if seen[fd.ID] {
				return nil, fmt.Errorf("form: duplicate id %q", fd.ID)
			}
			seen[fd.ID] = true
		}
		f.Fields = append(f.Fields, fd)
	}
	return f, nil
}

// Inputs returns the fields that produce output (everything but markdown).
func (f *Form) Inputs() []Field {
	var out []Field
	for _, fd := range f.Fields {
		if fd.Type != "markdown" {
			out = append(out, fd)
		}
	}
	return out
}

// Field returns the field with the given id.
func (f *Form) Field(id string) (Field, bool) {
	for _, fd := range f.Fields {
		if fd.ID == id && fd.Type != "markdown" {
			return fd, true
		}
	}
	return Field{}, false
}

// Prefillable reports whether GitHub honours a ?<id>=value query parameter
// for this field. GitHub documents prefill for input and textarea; dropdown
// and checkbox prefill is not reliable, so the page treats them as
// "choose on GitHub" and only uses them for client-side checks.
func (fd Field) Prefillable() bool { return fd.Type == "input" || fd.Type == "textarea" }

func str(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	}
	return ""
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func strs(v any) []string {
	switch t := v.(type) {
	case string:
		var out []string
		for _, s := range strings.Split(t, ",") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		var out []string
		for _, e := range t {
			out = append(out, str(e))
		}
		return out
	}
	return nil
}
