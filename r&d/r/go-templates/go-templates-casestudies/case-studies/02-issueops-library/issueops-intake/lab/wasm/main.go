//go:build js && wasm

// Command wasm is the /lab/ showcase: Go's text/template compiled to
// WebAssembly. It registers renderTemplate(tmpl, json) on the JS global
// object and then blocks forever so the function stays callable.
//
// Build:
//
//	GOOS=js GOARCH=wasm go build -trimpath -o render.wasm ./lab/wasm
//
// It is documentation, not a dependency of the intake form.
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"syscall/js"
	"text/template"
)

var funcs = template.FuncMap{
	"default": func(def, v any) any {
		if v == nil || v == "" {
			return def
		}
		return v
	},
	"upper": strings.ToUpper,
	"lower": strings.ToLower,
	"join": func(sep string, l []any) string {
		s := make([]string, len(l))
		for i, v := range l {
			b, _ := json.Marshal(v)
			s[i] = strings.Trim(string(b), `"`)
		}
		return strings.Join(s, sep)
	},
	"toJSON": func(v any) (string, error) {
		b, err := json.Marshal(v)
		return string(b), err
	},
}

// render is the pure part, kept separate from syscall/js plumbing.
func render(src, data string) (string, error) {
	var d any
	if strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return "", err
		}
	}
	t, err := template.New("lab").Funcs(funcs).Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func main() {
	js.Global().Set("renderTemplate", js.FuncOf(func(this js.Value, args []js.Value) any {
		res := map[string]any{"out": "", "err": ""}
		if len(args) != 2 {
			res["err"] = "renderTemplate(template, json)"
			return js.ValueOf(res)
		}
		out, err := render(args[0].String(), args[1].String())
		res["out"] = out
		if err != nil {
			res["err"] = err.Error()
		}
		return js.ValueOf(res)
	}))
	select {}
}
