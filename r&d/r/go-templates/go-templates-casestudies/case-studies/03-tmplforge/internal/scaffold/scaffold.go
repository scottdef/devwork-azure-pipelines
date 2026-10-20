// Package scaffold generates GitHub automation from small specs: IssueOps
// kits (issue form + parse workflow + Go parser) and CI/CD workflows.
//
// Every generator is a text/template parsed with [[ ]] delimiters, so the
// GitHub Actions expressions ${{ ... }} in the output are plain text to the
// template engine and survive untouched. User-supplied strings reach YAML
// only through toJSON (a JSON string is a valid YAML flow scalar), and
// reach shell only through env: blocks, never through run: interpolation.
package scaffold

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

//go:embed tmpl/*.tmpl
var tmplFS embed.FS

var tmpls = template.Must(template.New("scaffold").Delims("[[", "]]").Funcs(funcs.Safe()).ParseFS(tmplFS, "tmpl/*.tmpl"))

// GitHub's limits: workflow_dispatch inputs per workflow, and top-level
// client_payload keys per repository_dispatch event.
const (
	MaxDispatchInputs = 25
	MaxPayloadKeys    = 10
)

var (
	idRE   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)
	nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
)

// Field is one issue-form field.
type Field struct {
	ID       string
	Label    string
	Type     string // string, text, int, bool, dropdown
	Options  []string
	Required bool
}

// FormType is the GitHub issue-form element type for f.
func (f Field) FormType() string {
	switch f.Type {
	case "text":
		return "textarea"
	case "dropdown":
		return "dropdown"
	case "bool":
		return "checkboxes"
	}
	return "input"
}

// FieldTypes lists the accepted Field.Type values.
var FieldTypes = []string{"string", "text", "int", "bool", "dropdown"}

// ParseFields parses "a:string,b:dropdown=x|y,c:int!" into fields. A
// trailing ! on the type marks the field required. Labels are derived from
// IDs ("repo_name" becomes "Repo name").
func ParseFields(spec string) ([]Field, error) {
	var out []Field
	seen := map[string]bool{}
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		id, typ, ok := strings.Cut(part, ":")
		if !ok {
			typ = "string"
		}
		f := Field{ID: id}
		typ, opts, hasOpts, req := splitType(typ)
		f.Type, f.Required = typ, req
		if !idRE.MatchString(id) {
			return nil, fmt.Errorf("field %q: id must match %s", id, idRE)
		}
		if seen[id] {
			return nil, fmt.Errorf("field %q: duplicate", id)
		}
		seen[id] = true
		switch typ {
		case "string", "text", "int", "bool":
			if hasOpts {
				return nil, fmt.Errorf("field %q: only dropdown takes =options", id)
			}
		case "dropdown":
			if !hasOpts || opts == "" {
				return nil, fmt.Errorf("field %q: dropdown needs =opt1|opt2", id)
			}
			f.Options = strings.Split(opts, "|")
		default:
			return nil, fmt.Errorf("field %q: unknown type %q (want %s)", id, typ, strings.Join(FieldTypes, "|"))
		}
		f.Label = Label(id)
		out = append(out, f)
	}
	return out, nil
}

// splitType splits "type[!][=opts][!]" into its parts. The required mark
// may follow the type name or end the spec: "dropdown!=a|b" and
// "dropdown=a|b!" mean the same.
func splitType(spec string) (typ, opts string, hasOpts, required bool) {
	typ, opts, hasOpts = strings.Cut(spec, "=")
	if strings.HasSuffix(typ, "!") {
		typ, required = strings.TrimSuffix(typ, "!"), true
	}
	if strings.HasSuffix(opts, "!") {
		opts, required = strings.TrimSuffix(opts, "!"), true
	}
	return typ, opts, hasOpts, required
}

// Label turns an identifier into a human label: "repo_name" -> "Repo name".
func Label(id string) string {
	s := strings.NewReplacer("_", " ", "-", " ").Replace(id)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// IssueOps generates an IssueOps kit called name:
//
//	.github/ISSUE_TEMPLATE/NAME.yml   issue form, labelled NAME
//	.github/workflows/NAME-parse.yml  on issue open/edit: parse, validate, comment
//	cmd/NAME-parse/main.go            stdlib Go parser: issue body -> JSON
func IssueOps(name string, fields []Field) ([]forge.File, error) {
	if !nameRE.MatchString(name) {
		return nil, fmt.Errorf("issueops name %q: want lower-case letters, digits and dashes", name)
	}
	if len(fields) == 0 {
		return nil, errors.New("issueops: at least one field required")
	}
	// The parse workflow sends the fields as repository_dispatch
	// client_payload, which GitHub caps at 10 top-level keys.
	if len(fields) > MaxPayloadKeys {
		return nil, fmt.Errorf("issueops: %d fields; client_payload allows at most %d", len(fields), MaxPayloadKeys)
	}
	p := map[string]any{"Name": name, "Title": Label(name), "Fields": fields}
	files := []forge.File{
		{Name: ".github/ISSUE_TEMPLATE/" + name + ".yml"},
		{Name: ".github/workflows/" + name + "-parse.yml"},
		{Name: "cmd/" + name + "-parse/main.go"},
	}
	for i, t := range []string{"issue-form.yml.tmpl", "issueops-parse.yml.tmpl", "parser.go.tmpl"} {
		b, err := execute(t, p)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(files[i].Name, ".go") {
			if b, err = format.Source(b); err != nil {
				return nil, fmt.Errorf("issueops: generated Go does not format: %w", err)
			}
		}
		files[i].Data, files[i].Mode = b, 0o644
	}
	return files, nil
}

// Input is one workflow_dispatch input or repository_dispatch payload key.
type Input struct {
	Name        string
	Type        string // string, boolean, number, choice, environment
	Description string
	Default     string
	Options     []string
	Required    bool
}

// InputTypes lists the accepted Input.Type values (GitHub's set).
var InputTypes = []string{"string", "boolean", "number", "choice", "environment"}

// ParseInputs parses "env:choice=beta|prod,dry_run:boolean" like
// ParseFields, for -inputs.
func ParseInputs(spec string) ([]Input, error) {
	var out []Input
	if strings.TrimSpace(spec) == "" {
		return nil, nil
	}
	for _, part := range strings.Split(spec, ",") {
		id, typ, ok := strings.Cut(strings.TrimSpace(part), ":")
		if !ok {
			typ = "string"
		}
		in := Input{Name: id, Description: Label(id)}
		typ, opts, has, req := splitType(typ)
		in.Type, in.Required = typ, req
		if typ == "choice" {
			if !has || opts == "" {
				return nil, fmt.Errorf("input %q: choice needs =a|b", id)
			}
			in.Options = strings.Split(opts, "|")
			in.Default = in.Options[0]
		} else if has {
			in.Default = opts
		}
		out = append(out, in)
	}
	return out, validInputs(out)
}

func validInputs(in []Input) error {
	if len(in) > MaxDispatchInputs {
		return fmt.Errorf("workflow_dispatch allows at most %d inputs, got %d", MaxDispatchInputs, len(in))
	}
	seen := map[string]bool{}
	for _, i := range in {
		if !idRE.MatchString(i.Name) {
			return fmt.Errorf("input %q: name must match %s", i.Name, idRE)
		}
		if seen[i.Name] {
			return fmt.Errorf("input %q: duplicate", i.Name)
		}
		seen[i.Name] = true
		ok := false
		for _, t := range InputTypes {
			ok = ok || t == i.Type
		}
		if !ok {
			return fmt.Errorf("input %q: unknown type %q (want %s)", i.Name, i.Type, strings.Join(InputTypes, "|"))
		}
	}
	return nil
}

// DefaultYAML renders the default as a YAML scalar of the input's type:
// a bare boolean or number where GitHub expects one, else a JSON string.
func (i Input) DefaultYAML() string {
	switch i.Type {
	case "boolean":
		if b, err := strconv.ParseBool(i.Default); err == nil {
			return strconv.FormatBool(b)
		}
		return "false"
	case "number":
		if _, err := strconv.ParseFloat(i.Default, 64); err == nil {
			return i.Default
		}
	}
	b, _ := json.Marshal(i.Default)
	return string(b)
}

// EnvName is the environment variable an input is exposed as: INPUT_NAME.
func (i Input) EnvName() string {
	return "INPUT_" + strings.ToUpper(strings.ReplaceAll(i.Name, "-", "_"))
}

// CICD describes a workflow to generate.
type CICD struct {
	Kind    string   // one of Kinds
	Name    string   // workflow name; default Kind
	Binary  string   // Go command under cmd/; default "app"
	Go      string   // Go version fallback when vars.GO_VERSION is unset
	Targets []string // GOOS/GOARCH pairs
	Inputs  []Input  // dispatch inputs / repository_dispatch payload keys
	Types   []string // repository_dispatch event types
}

// Kinds lists the CI/CD generators.
var Kinds = []string{"build", "dispatch", "release", "repository_dispatch", "tag"}

// DefaultTargets are the platforms every case study ships.
var DefaultTargets = []string{"linux/amd64", "linux/arm64", "darwin/arm64", "plan9/amd64"}

// Workflow generates .github/workflows/<kind>.yml for c.
func Workflow(c CICD) (forge.File, error) {
	ok := false
	for _, k := range Kinds {
		ok = ok || k == c.Kind
	}
	if !ok {
		return forge.File{}, fmt.Errorf("cicd: unknown kind %q (want %s)", c.Kind, strings.Join(Kinds, "|"))
	}
	if c.Name == "" {
		c.Name = strings.ReplaceAll(c.Kind, "_", "-")
	}
	if c.Binary == "" {
		c.Binary = "app"
	}
	if c.Go == "" {
		c.Go = "1.24.x"
	}
	if len(c.Targets) == 0 {
		c.Targets = DefaultTargets
	}
	if c.Kind == "repository_dispatch" && len(c.Types) == 0 {
		c.Types = []string{c.Name}
	}
	if !nameRE.MatchString(c.Name) {
		return forge.File{}, fmt.Errorf("cicd: name %q: want lower-case letters, digits and dashes", c.Name)
	}
	if !nameRE.MatchString(c.Binary) {
		return forge.File{}, fmt.Errorf("cicd: binary %q: want lower-case letters, digits and dashes", c.Binary)
	}
	if c.Kind == "tag" && !hasInput(c.Inputs, "version") {
		c.Inputs = append([]Input{{Name: "version", Type: "string", Description: "Version to tag, e.g. v1.2.3", Required: true}}, c.Inputs...)
	}
	if err := validInputs(c.Inputs); err != nil {
		return forge.File{}, err
	}
	if c.Kind == "repository_dispatch" && len(c.Inputs) > MaxPayloadKeys {
		return forge.File{}, fmt.Errorf("repository_dispatch client_payload allows at most %d top-level keys, got %d", MaxPayloadKeys, len(c.Inputs))
	}
	type target struct{ OS, Arch string }
	var ts []target
	for _, t := range c.Targets {
		os, arch, ok := strings.Cut(t, "/")
		if !ok {
			return forge.File{}, fmt.Errorf("cicd: target %q: want GOOS/GOARCH", t)
		}
		ts = append(ts, target{os, arch})
	}
	sort.SliceStable(ts, func(i, j int) bool { return ts[i].OS+ts[i].Arch < ts[j].OS+ts[j].Arch })
	b, err := execute("cicd-"+strings.ReplaceAll(c.Kind, "_", "-")+".yml.tmpl", map[string]any{"C": c, "Targets": ts})
	if err != nil {
		return forge.File{}, err
	}
	return forge.File{Name: ".github/workflows/" + c.Name + ".yml", Data: b, Mode: 0o644}, nil
}

func hasInput(in []Input, name string) bool {
	for _, i := range in {
		if i.Name == name {
			return true
		}
	}
	return false
}

func execute(name string, data any) ([]byte, error) {
	var b bytes.Buffer
	if err := tmpls.ExecuteTemplate(&b, name, data); err != nil {
		return nil, fmt.Errorf("scaffold: %w", err)
	}
	return b.Bytes(), nil
}

// InputsFromVars maps TUI/GUI variable declarations to dispatch inputs:
// int becomes number, bool becomes boolean, list/map become string (JSON).
func InputsFromVars(vars []Var) []Input {
	var out []Input
	for _, v := range vars {
		in := Input{Name: v.Name, Description: Label(v.Name), Default: v.Default, Type: "string"}
		switch v.Type {
		case "int":
			in.Type = "number"
		case "bool":
			in.Type = "boolean"
		}
		out = append(out, in)
	}
	return out
}

// FieldsFromVars maps variable declarations to issue-form fields.
func FieldsFromVars(vars []Var) []Field {
	var out []Field
	for _, v := range vars {
		f := Field{ID: v.Name, Label: Label(v.Name), Type: "string"}
		switch v.Type {
		case "int", "bool":
			f.Type = v.Type
		case "list", "map":
			f.Type = "text"
		}
		out = append(out, f)
	}
	return out
}

// Var is a declared template variable, shared by the TUI and GUI.
type Var struct {
	Name    string
	Type    string // string, int, bool, list, map
	Default string
}

// VarTypes lists the accepted Var.Type values.
var VarTypes = []string{"string", "int", "bool", "list", "map"}

// Value converts the declared default to its type's Go value: int, bool,
// []any (JSON array or comma list), map[string]any (JSON object) or
// string. An empty default is the type's zero value.
func (v Var) Value() (any, error) {
	switch v.Type {
	case "int":
		if v.Default == "" {
			return 0, nil
		}
		return strconv.Atoi(v.Default)
	case "bool":
		if v.Default == "" {
			return false, nil
		}
		return strconv.ParseBool(v.Default)
	case "list":
		if v.Default == "" {
			return []any{}, nil
		}
		if strings.HasPrefix(v.Default, "[") {
			return decodeJSON[[]any](v.Default)
		}
		var l []any
		for _, e := range strings.Split(v.Default, ",") {
			l = append(l, strings.TrimSpace(e))
		}
		return l, nil
	case "map":
		if v.Default == "" {
			return map[string]any{}, nil
		}
		return decodeJSON[map[string]any](v.Default)
	}
	return v.Default, nil
}

func decodeJSON[T any](s string) (T, error) {
	var v T
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return v, err
	}
	funcs.Normalize(any(v))
	return v, nil
}

// ValidVar checks a declaration: a template field name and a known type
// whose default converts.
func ValidVar(v Var) error {
	if !validIdent(v.Name) {
		return fmt.Errorf("variable %q: want a Go-template field name", v.Name)
	}
	ok := false
	for _, t := range VarTypes {
		ok = ok || t == v.Type
	}
	if !ok {
		return fmt.Errorf("type %q: want %s", v.Type, strings.Join(VarTypes, "|"))
	}
	if _, err := v.Value(); err != nil {
		return fmt.Errorf("default for %s %s: %w", v.Name, v.Type, err)
	}
	return nil
}

func validIdent(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	for i, r := range s {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

// Merge returns the declared defaults overlaid by fixture: the data a
// template sees in the TUI's p and the GUI's preview.
func Merge(vars []Var, fixture map[string]any) map[string]any {
	d := make(map[string]any, len(vars)+len(fixture))
	for _, v := range vars {
		if val, err := v.Value(); err == nil {
			d[v.Name] = val
		}
	}
	for k, v := range fixture {
		d[k] = v
	}
	return d
}
