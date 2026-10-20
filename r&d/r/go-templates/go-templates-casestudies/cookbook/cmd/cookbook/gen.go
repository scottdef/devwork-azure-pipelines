package main

import (
	"fmt"
	"sort"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/go-template-cookbook/formulas"
	"github.com/CoolGitOrg/go-template-cookbook/funcs"
)

// fview is a formula plus everything the Markdown template needs that
// cannot be a plain field: rendered output, JSON data, a one-line summary of
// the engine and delimiters.
type fview struct {
	formulas.Formula
	DataJSON  string
	Output    string // as rendered, minus one trailing newline for display
	NoFinalNL bool   // the rendered output does not end in a newline
	Empty     bool
	Meta      string
}

func newView(f formulas.Formula) (fview, error) {
	out, err := formulas.Render(f)
	if err != nil {
		return fview{}, err
	}
	dj, err := f.DataJSON()
	if err != nil {
		return fview{}, err
	}
	engine := "text"
	if f.Engine == "html" {
		engine = "html"
	}
	d0, d1 := f.Delims[0], f.Delims[1]
	if d0 == "" {
		d0, d1 = "{{", "}}"
	}
	return fview{
		Formula:   f,
		DataJSON:  dj,
		Output:    strings.TrimSuffix(out, "\n"),
		NoFinalNL: out != "" && !strings.HasSuffix(out, "\n"),
		Empty:     out == "",
		Meta:      fmt.Sprintf("Engine: `%s/template`. Delimiters: `%s` `%s`.", engine, d0, d1),
	}, nil
}

type funcRow struct {
	Name, Sig, Example, Output, Notes string
}

// mdFuncs are the helpers of cookbook.md.tmpl.
var mdFuncs = template.FuncMap{
	"lower": strings.ToLower,
	"fence": fence,
	"link":  func(id string) string { return "[" + id + "](#" + strings.ToLower(id) + ")" },
	"cell":  cell,
	"code":  code,
	"chomp": func(s string) string { return strings.TrimSuffix(s, "\n") },
}

// longestRun is the length of the longest run of backticks in s.
func longestRun(s string) int {
	longest, run := 0, 0
	for _, r := range s {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

// fence returns a code fence longer than any run of backticks in s.
func fence(s string) string {
	return strings.Repeat("`", max(3, longestRun(s)+1))
}

// cell makes s safe inside a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", "<br>")
}

// code renders s as an inline code span inside a table cell, showing
// newlines and tabs as \n and \t.
func code(s string) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "|", `\|`)
	if s == "" {
		return "_(empty)_"
	}
	tick := strings.Repeat("`", longestRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return tick + s + tick
}

func execMD(name string, data any) (string, error) {
	t, err := template.New("md").Delims("[[", "]]").Funcs(mdFuncs).Parse(mdTmpl)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.ExecuteTemplate(&b, name, data); err != nil {
		return "", err
	}
	return b.String(), nil
}

// genCookbook renders docs/COOKBOOK.md from formulas.All. Nothing in it
// depends on the clock, the machine or map order.
func genCookbook() (string, error) {
	views := make([]fview, 0, len(formulas.All))
	for _, f := range formulas.All {
		v, err := newView(f)
		if err != nil {
			return "", err
		}
		views = append(views, v)
	}
	var env []string
	for k, v := range formulas.FixedEnv {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return execMD("cookbook", struct {
		Formulas []fview
		Count    int
		Now      string
		Env      string
	}{views, len(views), formulas.FixedNow.Format("2006-01-02T15:04:05Z07:00"), strings.Join(env, " ")})
}

// genFuncs renders docs/FUNCS-SHARED.md from funcs.Docs.
func genFuncs() (string, error) {
	rows := make([]funcRow, 0, len(funcs.Docs))
	for _, d := range funcs.Docs {
		out, err := funcs.RenderDoc(d)
		if err != nil {
			return "", err
		}
		rows = append(rows, funcRow{d.Name, d.Sig, d.Example, out, d.Notes})
	}
	return execMD("funcs", struct {
		Funcs []funcRow
		Count int
	}{rows, len(rows)})
}
