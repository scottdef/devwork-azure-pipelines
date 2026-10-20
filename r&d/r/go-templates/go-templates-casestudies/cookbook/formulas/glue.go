package formulas

// This file is the Go that the cookbook formulas need beyond the FuncMap.
// It is not paraphrased in the document: Formula.GlueRef names declarations
// here and the generator copies their source, so what you read is what ran.
// Conventions: f.Template and f.Data are the Template and Data blocks shown
// with the formula, and Options() is the fixed clock, environment and file
// root described in the introduction.

import (
	_ "embed" // for glueSrc
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	htemplate "html/template"
	"io"
	"iter"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"

	"github.com/CoolGitOrg/go-template-cookbook/funcs"
)

//go:embed glue.go
var glueSrc string

// ppTabwriter aligns tab-separated cells into columns two spaces apart.
// Only cells that end in a tab take part; the last cell of a line is free.
func ppTabwriter(s string) (string, error) {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	if _, err := io.WriteString(w, s); err != nil {
		return "", err
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// ppGofmt formats generated Go source exactly as gofmt would, and fails if
// the source does not parse.
func ppGofmt(s string) (string, error) {
	out, err := format.Source([]byte(s))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// csvRow formats one CSV record, quoting as RFC 4180 requires.
func csvRow(fields ...any) (string, error) {
	rec := make([]string, len(fields))
	for i, f := range fields {
		rec[i] = fmt.Sprint(f)
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(rec); err != nil {
		return "", err
	}
	w.Flush()
	return b.String(), w.Error()
}

// glueFile renders the template, then tries three names that must not work.
func glueFile(f Formula) (string, error) {
	fm := funcs.New(Options()) // Options().FS is rooted at testdata/tmpl
	t, err := template.New("page").Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, f.Data); err != nil {
		return "", err
	}
	probe, err := template.New("probe").Funcs(fm).Parse(`{{ file . }}`)
	if err != nil {
		return "", err
	}
	for _, name := range []string{"../formulas.go", "/etc/passwd", "missing.txt"} {
		err := probe.Execute(io.Discard, name)
		fmt.Fprintf(&b, "file %q: %v\n", name, err)
	}
	return b.String(), nil
}

// allowed is the closed set of identifiers a SQL template may name.
var allowed = map[string]bool{"users": true, "id": true, "name": true, "email": true}

// glueSQL adds ident and bind to the FuncMap, renders the template, then
// renders it again with a table name that is not on the allowlist.
func glueSQL(f Formula) (string, error) {
	run := func(data any) (string, error) {
		var args []any
		fm := funcs.New(Options())
		fm["ident"] = func(s string) (string, error) {
			if !allowed[s] {
				return "", fmt.Errorf("ident: %q is not in the allowlist", s)
			}
			return `"` + s + `"`, nil
		}
		fm["bind"] = func(v any) string {
			args = append(args, v)
			return "$" + strconv.Itoa(len(args))
		}
		t, err := template.New("sql").Funcs(fm).Parse(f.Template)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		if err := t.Execute(&b, data); err != nil {
			return "", err
		}
		j, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "-- args: %s\n", j)
		return b.String(), nil
	}
	out, err := run(f.Data)
	if err != nil {
		return "", err
	}
	_, err = run(M{"Table": "users; DROP TABLE users", "Cols": L{"id"}, "Filter": "id", "Value": 1})
	if err == nil {
		return "", errors.New("identifier outside the allowlist was accepted")
	}
	return out + "-- rejected: " + err.Error() + "\n", nil
}

// emailText is the plain-text alternative. It is a text/template: nothing
// in it is escaped.
const emailText = `Hello {{ .Name }},

Order {{ .Order }}:
{{ range .Lines -}}
 * {{ .Qty }} x {{ .Item }} @ {{ printf "%.2f" .Price }}
{{ end }}
Total: {{ printf "%.2f" .Total }}. {{ .Note }}
`

// glueEmail renders the same data with both engines.
func glueEmail(f Formula) (string, error) {
	fm := funcs.New(Options())
	txt, err := template.New("text").Funcs(fm).Parse(emailText)
	if err != nil {
		return "", err
	}
	html, err := htemplate.New("html").Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("=== text/plain (text/template) ===\n")
	if err := txt.Execute(&b, f.Data); err != nil {
		return "", err
	}
	b.WriteString("\n=== text/html (html/template) ===\n")
	if err := html.Execute(&b, f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// spark scales vals into a w by h box and returns the points attribute of an
// SVG polyline. SVG's y axis points down.
func spark(vals []int, w, h int) string {
	if len(vals) == 0 {
		return ""
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = min(lo, v), max(hi, v)
	}
	pts := make([]string, len(vals))
	for i, v := range vals {
		x := 0.0
		if len(vals) > 1 {
			x = float64(i) * float64(w) / float64(len(vals)-1)
		}
		y := float64(h) / 2 // a flat series sits in the middle
		if hi > lo {
			y = float64(h-1) - float64(v-lo)*float64(h-2)/float64(hi-lo)
		}
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	return strings.Join(pts, " ")
}

// glueMeta runs the outer template with its own delimiters, then parses and
// executes what it produced, as proof that the output is a working template.
func glueMeta(f Formula) (string, error) {
	fm := funcs.New(Options())
	outer, err := template.New("outer").Delims(f.Delims[0], f.Delims[1]).Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var gen strings.Builder
	if err := outer.Execute(&gen, f.Data); err != nil {
		return "", err
	}
	inner, err := template.New("inner").Funcs(fm).Parse(gen.String())
	if err != nil {
		return "", fmt.Errorf("generated template does not parse: %w", err)
	}
	var run strings.Builder
	if err := inner.ExecuteTemplate(&run, "Contact", M{"Name": "rob", "Email": "rob@example.org"}); err != nil {
		return "", err
	}
	return gen.String() + "-- executed with Name=rob Email=rob@example.org --\n" + run.String() + "\n", nil
}

// flagSpec describes one flag; flagSet is a program's worth of them.
type flagSpec struct{ Name, Kind, Def, Usage string }

type flagSet struct {
	Prog  string
	Flags []flagSpec
}

// flagDoc is the flag set whose usage F38 prints.
var flagDoc = flagSet{Prog: "cookbook", Flags: []flagSpec{
	{"a", "string", ":8080", "listen on `addr`"},
	{"c", "string", "", "read configuration from `file`"},
	{"n", "int", "3", "retry `count` for failed requests"},
	{"v", "bool", "false", "print every formula as it runs"},
}}

// glueFlags defines the flags on a real flag.FlagSet and renders the usage
// from what VisitAll reports, not from the spec.
func glueFlags(f Formula) (string, error) {
	doc, ok := f.Data.(flagSet)
	if !ok {
		return "", fmt.Errorf("data is %T, want flagSet", f.Data)
	}
	set := flag.NewFlagSet(doc.Prog, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, s := range doc.Flags {
		switch s.Kind {
		case "string":
			set.String(s.Name, s.Def, s.Usage)
		case "bool":
			set.Bool(s.Name, s.Def == "true", s.Usage)
		case "int":
			n, err := strconv.Atoi(s.Def)
			if err != nil {
				return "", err
			}
			set.Int(s.Name, n, s.Usage)
		default:
			return "", fmt.Errorf("flag %s: unknown kind %q", s.Name, s.Kind)
		}
	}
	var rows []M
	set.VisitAll(func(fl *flag.Flag) {
		arg, usage := flag.UnquoteUsage(fl)
		rows = append(rows, M{"Name": fl.Name, "Arg": arg, "Usage": usage, "Def": fl.DefValue})
	})
	t, err := template.New("usage").Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, M{"Prog": doc.Prog, "Flags": rows}); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Item is one directory entry, shaped like the data Caddy's browse feeds its
// template. HumanSize has a value receiver so templates can call it on the
// elements of a []Item.
type Item struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// HumanSize formats Size with binary prefixes: 1234 is "1.2 KiB".
func (i Item) HumanSize() string {
	const unit = 1024
	if i.Size < unit {
		return fmt.Sprintf("%d B", i.Size)
	}
	div, exp := int64(unit), 0
	for n := i.Size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(i.Size)/float64(div), "KMGTPE"[exp])
}

// SeqData carries three iterators. A template ranges over them directly.
type SeqData struct {
	Seq   iter.Seq[int]
	Pairs iter.Seq2[string, int]
	Empty iter.Seq[int]
}

// newSeqData returns the squares 1, 4, 9, the pairs a=1 b=2, and nothing.
func newSeqData() SeqData {
	return SeqData{
		Seq: func(yield func(int) bool) {
			for i := 1; i <= 3; i++ {
				if !yield(i * i) {
					return
				}
			}
		},
		Pairs: func(yield func(string, int) bool) {
			for _, p := range []struct {
				k string
				v int
			}{{"a", 1}, {"b", 2}} {
				if !yield(p.k, p.v) {
					return
				}
			}
		},
		Empty: func(yield func(int) bool) {},
	}
}

// MarshalJSON runs the iterators so the document can show what they yield;
// encoding/json cannot encode a func.
func (d SeqData) MarshalJSON() ([]byte, error) {
	out := struct {
		Seq   []int
		Pairs [][2]any
		Empty []int
	}{Seq: []int{}, Pairs: [][2]any{}, Empty: []int{}}
	for v := range d.Seq {
		out.Seq = append(out.Seq, v)
	}
	for k, v := range d.Pairs {
		out.Pairs = append(out.Pairs, [2]any{k, v})
	}
	for v := range d.Empty {
		out.Empty = append(out.Empty, v)
	}
	return json.Marshal(out)
}

// wsVariant is one spelling of the same loop.
type wsVariant struct{ label, src string }

// wsVariants are the trimmed spellings; the untrimmed one is f.Template.
var wsVariants = []wsVariant{
	{"left-trim", "{{- range .Items }}\n  {{ . }}\n{{- end }}"},
	{"both-trim", "{{- range .Items -}}\n  {{ . }}\n{{- end -}}"},
	{"idiom", "{{ range .Items -}}\n  {{ . }}\n{{ end -}}"},
}

// glueWS renders every spelling and prints the output with %q.
func glueWS(f Formula) (string, error) {
	var b strings.Builder
	for _, v := range append([]wsVariant{{"untrimmed", f.Template}}, wsVariants...) {
		t, err := template.New(v.label).Parse(v.src)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		if err := t.Execute(&out, f.Data); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "== %s\n%s\n=> %q\n", v.label, v.src, out.String())
	}
	return b.String(), nil
}

// glueMissing runs one template under every missingkey setting, against a
// map[string]any, a map[string]int and a struct.
func glueMissing(f Formula) (string, error) {
	var b strings.Builder
	try := func(opt string, label string, data any) error {
		t, err := template.New("t").Option(opt).Funcs(funcs.New(Options())).Parse(f.Template)
		if err != nil {
			return err
		}
		var out strings.Builder
		if err := t.Execute(&out, data); err != nil {
			fmt.Fprintf(&b, "%-22s %-17s error: %v\n", opt, label, err)
			return nil
		}
		fmt.Fprintf(&b, "%-22s %-17s %s\n", opt, label, out.String())
		return nil
	}
	for _, opt := range []string{"missingkey=default", "missingkey=zero", "missingkey=error"} {
		for _, d := range []struct {
			label string
			data  any
		}{
			{"map[string]any", f.Data},
			{"map[string]int", map[string]int{"A": 1}},
		} {
			if err := try(opt, d.label, d.data); err != nil {
				return "", err
			}
		}
	}
	if err := try("missingkey=default", "struct", struct{ A int }{1}); err != nil {
		return "", err
	}
	return b.String(), nil
}

// pages are the per-page overrides for F51.
var pages = []struct{ name, src string }{
	{"home", `{{ define "title" }}Home{{ end }}{{ define "body" }}Hello, {{ .User }}{{ end }}`},
	{"about", `{{ define "title" }}About{{ end }}`},
}

// glueClone renders each page from its own Clone of the layout, then shows
// what happens when all pages are parsed into one set.
func glueClone(f Formula) (string, error) {
	base, err := htemplate.New("layout").Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("## one clone per page\n")
	for _, p := range pages {
		t, err := base.Clone()
		if err != nil {
			return "", err
		}
		if _, err := t.Parse(p.src); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "-- %s --\n", p.name)
		if err := t.Execute(&b, f.Data); err != nil {
			return "", err
		}
	}
	shared, err := base.Clone()
	if err != nil {
		return "", err
	}
	b.WriteString("## one shared set, home parsed first, then about\n")
	for _, p := range pages {
		if _, err := shared.Parse(p.src); err != nil {
			return "", err
		}
	}
	if err := shared.Execute(&b, f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// glueFS parses every *.tmpl file in the embedded directory into one set and
// executes one of them by name.
func glueFS(f Formula) (string, error) {
	t, err := template.New("").ParseFS(tmplFS, "testdata/tmpl/*.tmpl")
	if err != nil {
		return "", err
	}
	var names []string
	for _, x := range t.Templates() {
		if x.Name() != "" {
			names = append(names, x.Name())
		}
	}
	sort.Strings(names) // Templates() is in map order
	var b strings.Builder
	fmt.Fprintf(&b, "templates: %s\n", strings.Join(names, " "))
	if err := t.ExecuteTemplate(&b, "page.tmpl", f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// failWriter refuses every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// glueErr triggers four kinds of failure and reports what errors.As and
// errors.Is can recover from each.
func glueErr(f Formula) (string, error) {
	fm := funcs.New(Options())
	var b strings.Builder
	try := func(label, src string, data any, w io.Writer) {
		t, err := template.New("t").Funcs(fm).Parse(src)
		if err != nil {
			fmt.Fprintf(&b, "%-6s parse error: %v\n", label, err)
			return
		}
		err = t.Execute(w, data)
		var ee template.ExecError
		switch {
		case errors.As(err, &ee):
			fmt.Fprintf(&b, "%-6s ExecError in %q: %v\n", label, ee.Name, ee.Err)
			if errors.Is(err, funcs.ErrArgs) {
				fmt.Fprintf(&b, "%-6s errors.Is(err, funcs.ErrArgs) = true\n", "")
			}
		case err != nil:
			fmt.Fprintf(&b, "%-6s not an ExecError: %v\n", label, err)
		default:
			fmt.Fprintf(&b, "%-6s ok\n", label)
		}
	}
	try("parse", `{{ .user.name `, nil, io.Discard)
	try("nil", f.Template, f.Data, io.Discard)
	try("args", `{{ dict "a" }}`, nil, io.Discard)
	try("write", `hello`, nil, failWriter{})
	return b.String(), nil
}
