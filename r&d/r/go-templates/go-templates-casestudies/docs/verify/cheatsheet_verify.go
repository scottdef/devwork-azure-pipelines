package main

import (
	"errors"
	"fmt"
	"html/template"
	"iter"
	"os"
	"strings"
	tt "text/template"
)

type U struct{ Name string }

func (u U) Hello(s string) string { return "hi " + s + " from " + u.Name }

func run(name, src string, data any) {
	t, err := tt.New(name).Funcs(tt.FuncMap{"add": func(a, b int) int { return a + b }}).Parse(src)
	if err != nil {
		fmt.Printf("%-28s PARSE ERR: %v\n", name, err)
		return
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		fmt.Printf("%-28s EXEC ERR: %v\n", name, err)
		return
	}
	fmt.Printf("%-28s %q\n", name, b.String())
}

func main() {
	seq := func(yield func(int) bool) {
		for i := 10; i < 13; i++ {
			if !yield(i) {
				return
			}
		}
	}
	seq2 := func(yield func(string, int) bool) {
		yield("a", 1)
		yield("b", 2)
	}
	_ = iter.Seq[int](seq)
	m := map[string]int{"b": 2, "a": 1}
	run("range int", `{{range 3}}{{.}}{{end}}`, nil)
	run("range int kv", `{{range $i, $v := 3}}{{$i}}:{{$v}} {{end}}`, nil)
	run("range seq", `{{range .}}{{.}} {{end}}`, seq)
	run("range seq2", `{{range $k, $v := .}}{{$k}}={{$v}} {{end}}`, seq2)
	run("range map sorted", `{{range $k, $v := .}}{{$k}}={{$v}} {{end}}`, m)
	run("range else", `{{range .}}x{{else}}none{{end}}`, []int{})
	run("break", `{{range .}}{{if eq . 2}}{{break}}{{end}}{{.}}{{end}}`, []int{0, 1, 2, 3})
	run("continue", `{{range .}}{{if eq . 1}}{{continue}}{{end}}{{.}}{{end}}`, []int{0, 1, 2})
	run("eq multi", `{{if eq . "dev" "stg"}}yes{{else}}no{{end}}`, "stg")
	run("lt mixed", `{{lt 1 1.5}}`, nil)
	run("trim", `a {{- 1 -}} b`, nil)
	run("reassign", `{{$x := 0}}{{range .}}{{$x = add $x .}}{{end}}{{$x}}`, []int{1, 2, 3})
	run("method", `{{.Hello "bob"}}`, U{"ann"})
	run("method ptr", `{{.Hello "bob"}}`, &U{"ann"})
	run("missing map key", `[{{.nope}}]`, map[string]int{})
	run("missing struct", `[{{.Nope}}]`, U{})
	run("index map", `{{index . "k-1"}}`, map[string]string{"k-1": "v"})
	run("slice", `{{slice . 1 3}}`, []int{0, 1, 2, 3})
	run("printf", `{{printf "%05d|%.2f|%q" 42 3.14159 "s"}}`, nil)
	run("html js urlquery", `{{"<a&b>" | html}}|{{"it's" | js}}|{{"a b&c" | urlquery}}`, nil)
	run("and short", `{{and false (index . 5)}}`, []int{})
	run("or", `{{or "" "fallback"}}`, nil)
	run("with else", `{{with .}}{{.}}{{else}}empty{{end}}`, "")
	run("define/template", `{{define "r"}}<{{.}}>{{end}}{{template "r" "x"}}`, nil)
	run("block", `{{block "c" .}}default {{.}}{{end}}`, "d")
	run("call", `{{call . 2 3}}`, func(a, b int) int { return a * b })
	run("comment", `a{{/* c */}}b`, nil)
	run("nested $", `{{range .L}}{{$.N}}/{{.}} {{end}}`, map[string]any{"N": "org", "L": []string{"a", "b"}})
	run("len", `{{len .}}`, "héllo")
	run("print", `{{print 1 "a" 2}}|{{println "x"}}`, nil)
	run("pipeline last arg", `{{"World" | printf "Hello, %s"}}`, nil)
	run("chained field", `{{.A.B}}`, map[string]any{"A": map[string]int{"B": 7}})
	run("not", `{{not true}}`, nil)
	run("ge", `{{ge 2 2}}`, nil)

	// missingkey option
	t := tt.Must(tt.New("mk").Option("missingkey=error").Parse(`[{{.nope}}]`))
	err := t.Execute(&strings.Builder{}, map[string]int{})
	var ee tt.ExecError
	fmt.Printf("%-28s %v  isExecError=%v\n", "missingkey=error", err, errors.As(err, &ee))
	t2 := tt.Must(tt.New("mk").Option("missingkey=zero").Parse(`[{{.nope}}]`))
	var b strings.Builder
	t2.Execute(&b, map[string]int{})
	fmt.Printf("%-28s %q\n", "missingkey=zero", b.String())

	// delims
	t3 := tt.Must(tt.New("d").Delims("[[", "]]").Parse(`{{ .Keep }} [[ .X ]]`))
	b.Reset()
	t3.Execute(&b, map[string]string{"X": "x"})
	fmt.Printf("%-28s %q\n", "delims", b.String())

	// html/template contexts
	h := template.Must(template.New("h").Parse(`<p>{{.}}</p><a href="{{.}}">x</a><script>var v = {{.}};</script><div title="{{.}}"></div>`))
	b.Reset()
	h.Execute(&b, `<b>"x" javascript:alert(1)</b>`)
	fmt.Printf("%-28s %s\n", "html contexts", b.String())
	h2 := template.Must(template.New("h").Parse(`<a href="{{.}}">`))
	b.Reset()
	h2.Execute(&b, "javascript:alert(1)")
	fmt.Printf("%-28s %s\n", "ZgotmplZ", b.String())
	h3 := template.Must(template.New("h").Parse(`{{.}}`))
	b.Reset()
	h3.Execute(&b, template.HTML("<b>trusted</b>"))
	fmt.Printf("%-28s %s\n", "template.HTML", b.String())

	// parse error line number
	_, perr := tt.New("lint").Parse("ok\n{{ if .X }}\nunclosed")
	fmt.Printf("%-28s %v\n", "parse error", perr)
	_, perr = tt.New("lint").Parse("{{ nosuchfunc 1 }}")
	fmt.Printf("%-28s %v\n", "unknown func", perr)

	// Clone + block override
	base := tt.Must(tt.New("layout").Parse(`[{{block "content" .}}base{{end}}]`))
	page, _ := tt.Must(base.Clone()).Parse(`{{define "content"}}page {{.}}{{end}}`)
	b.Reset()
	page.ExecuteTemplate(&b, "layout", "p1")
	fmt.Printf("%-28s %q  base-still=%q\n", "clone/block", b.String(), func() string { var s strings.Builder; base.Execute(&s, 1); return s.String() }())
	fmt.Println("DefinedTemplates:", page.DefinedTemplates())
	_ = os.Stdout
}
