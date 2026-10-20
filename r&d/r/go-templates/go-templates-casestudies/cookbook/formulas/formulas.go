// Package formulas holds the cookbook: every formula is a template, its data,
// and the exact output it must produce. The tests render all of them, and
// cmd/cookbook generates docs/COOKBOOK.md from the same values, so the
// document cannot say what the code does not do.
package formulas

import (
	"embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	htemplate "html/template"
	"io/fs"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/CoolGitOrg/go-template-cookbook/funcs"
)

// M and L are the two literals templates cannot write themselves.
type (
	// M is a string-keyed map, the usual shape of template data.
	M = map[string]any
	// L is a list of anything.
	L = []any
)

// Formula is one recipe: a problem, the template that solves it, the data it
// runs on and the exact text it must produce.
type Formula struct {
	// ID is "F01", "F02", ... and is stable: docs link to it.
	ID string
	// Name is the one-line title.
	Name string
	// Problem says what goes wrong without the formula.
	Problem string
	// Template is the template source. For formulas that need Go glue it is
	// still the template the glue parses.
	Template string
	// Data is passed to Execute. It must marshal with encoding/json so the
	// document can print it.
	Data any
	// Delims overrides the action delimiters; the zero value means {{ }}.
	Delims [2]string
	// Glue is the Go source shown next to the template. It is not typed by
	// hand: it is extracted from glue.go by GlueRef, so the text in the
	// document is the code that ran.
	Glue string
	// GlueRef names the declarations of glue.go that make up Glue, comma
	// separated: "name", "Type" or "Type.Method".
	GlueRef string
	// Want is the exact output.
	Want string
	// Notes explains the trap the formula avoids.
	Notes string
	// Engine is "text" (the default) or "html".
	Engine string
	// PostProcess, if set, transforms the rendered text (tabwriter, gofmt).
	PostProcess func(string) (string, error)
	// Funcs adds functions to the shared FuncMap for this formula only.
	Funcs map[string]any
	// Run replaces the default parse-and-execute step. Formulas whose point
	// is the Go around the template (Option, Clone, errors) use it.
	Run func(Formula) (string, error)
}

// FixedNow is the clock every formula sees.
var FixedNow = time.Date(2024, 3, 5, 14, 30, 0, 0, time.UTC)

// FixedEnv is the environment every formula sees.
var FixedEnv = map[string]string{"APP_ENV": "staging", "HOME": "/home/rob", "USER": "rob"}

//go:embed testdata/tmpl
var tmplFS embed.FS

// tmplRoot is testdata/tmpl as a rooted filesystem: the root for "file".
var tmplRoot = func() fs.FS {
	sub, err := fs.Sub(tmplFS, "testdata/tmpl")
	if err != nil { // the directory is embedded; this cannot happen
		return tmplFS
	}
	return sub
}()

// Options is the fixed world formulas run in: a pinned clock, a fake
// environment and testdata/tmpl as the file root. Output is therefore
// reproducible on any machine.
func Options() funcs.Options {
	return funcs.Options{
		FS:  tmplRoot,
		Env: func(k string) string { return FixedEnv[k] },
		Now: func() time.Time { return FixedNow },
	}
}

// All is the cookbook in order.
var All = collect(basics(), emitters(), generators(), language())

func collect(groups ...[]Formula) []Formula {
	var all []Formula
	for _, g := range groups {
		all = append(all, g...)
	}
	return all
}

// Lookup finds a formula by ID.
func Lookup(id string) (Formula, bool) {
	for _, f := range All {
		if f.ID == id {
			return f, true
		}
	}
	return Formula{}, false
}

// Render executes f and returns its output.
func Render(f Formula) (string, error) {
	var out string
	var err error
	if f.Run != nil {
		out, err = f.Run(f)
	} else {
		out, err = render(f)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", f.ID, err)
	}
	if f.PostProcess != nil {
		if out, err = f.PostProcess(out); err != nil {
			return "", fmt.Errorf("%s: post-process: %w", f.ID, err)
		}
	}
	return out, nil
}

func render(f Formula) (string, error) {
	fm := funcs.New(Options())
	for k, v := range f.Funcs {
		fm[k] = v
	}
	var b strings.Builder
	if f.Engine == "html" {
		t, err := htemplate.New(f.ID).Delims(f.Delims[0], f.Delims[1]).Funcs(fm).Parse(f.Template)
		if err != nil {
			return "", err
		}
		if err := t.Execute(&b, f.Data); err != nil {
			return "", err
		}
		return b.String(), nil
	}
	t, err := template.New(f.ID).Delims(f.Delims[0], f.Delims[1]).Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	if err := t.Execute(&b, f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// DataJSON returns Data as indented JSON, or "" if Data is nil.
func (f Formula) DataJSON() (string, error) {
	if f.Data == nil {
		return "", nil
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(f.Data); err != nil {
		return "", fmt.Errorf("%s: data: %w", f.ID, err)
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// init resolves every GlueRef against the embedded source of glue.go.
func init() {
	for i := range All {
		if All[i].GlueRef == "" {
			continue
		}
		src, err := glueSource(All[i].GlueRef)
		if err != nil {
			src = "// " + err.Error()
		}
		All[i].Glue = src
	}
}

// glueSource cuts the named declarations out of glue.go, comments and all.
func glueSource(refs string) (string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "glue.go", glueSrc, parser.ParseComments)
	if err != nil {
		return "", err
	}
	decls := map[string]ast.Decl{}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) == 1 {
				t := d.Recv.List[0].Type
				if s, ok := t.(*ast.StarExpr); ok {
					t = s.X
				}
				if id, ok := t.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
			decls[name] = d
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.TypeSpec:
					decls[s.Name.Name] = d
				case *ast.ValueSpec:
					for _, n := range s.Names {
						decls[n.Name] = d
					}
				}
			}
		}
	}
	var parts []string
	for _, ref := range strings.Split(refs, ",") {
		ref = strings.TrimSpace(ref)
		d, ok := decls[ref]
		if !ok {
			return "", fmt.Errorf("glue.go has no declaration %q", ref)
		}
		start, end := d.Pos(), d.End()
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				start = d.Doc.Pos()
			}
		}
		parts = append(parts, glueSrc[fset.Position(start).Offset:fset.Position(end).Offset])
	}
	return strings.Join(parts, "\n\n"), nil
}

// IDs returns the formula IDs in order.
func IDs() []string {
	ids := make([]string, len(All))
	for i, f := range All {
		ids[i] = f.ID
	}
	sort.Strings(ids)
	return ids
}
