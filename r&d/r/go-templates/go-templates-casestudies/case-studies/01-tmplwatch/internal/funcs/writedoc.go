package funcs

import (
	_ "embed"
	"io"
	"strings"
	"text/template"
)

//go:embed funcs.md.tmpl
var docTmpl string

// WriteDoc renders the FUNCS.md reference from Docs.
func WriteDoc(w io.Writer) error {
	t, err := template.New("funcs.md").Funcs(template.FuncMap{
		"lower":      strings.ToLower,
		"sortedKeys": sortedKeys,
	}).Parse(docTmpl)
	if err != nil {
		return err
	}
	n := 0
	for _, d := range Docs {
		if d.Consul == "yes" {
			n++
		}
	}
	data := map[string]string{}
	for k, v := range ExampleData {
		data[k] = v
	}
	return t.Execute(w, map[string]any{
		"Docs": Docs, "Groups": Groups, "Data": data, "ConsulCount": n,
	})
}
