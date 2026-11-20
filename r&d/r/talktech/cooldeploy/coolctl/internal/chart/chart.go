// Package chart renders the platform's Helm charts without Helm.
//
// It exists for two reasons: `coolctl render chart` lets a developer see what
// a values file turns into on a machine with no Helm, and the tests can check
// the charts with the standard library alone. Clusters always get real Helm,
// through Argo CD.
//
// It is not a Helm replacement. It implements the template functions the cool
// charts use and nothing more, and it reads values only as JSON, which is why
// those charts keep their values.yaml in JSON syntax. A chart that needs
// toYaml, tpl, lookup or subcharts needs Helm.
package chart

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

// Chart is a parsed chart directory.
type Chart struct {
	Name, Version string
	Defaults      map[string]any
	files         []string // template names that produce output, sorted
	tmpl          *template.Template
}

// Load parses the chart in dir.
func Load(dir string) (*Chart, error) {
	c := &Chart{}
	meta, err := os.ReadFile(filepath.Join(dir, "Chart.yaml"))
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(meta), "\n") {
		k, v, ok := strings.Cut(line, ":")
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch {
		case !ok:
		case k == "name":
			c.Name = v
		case k == "version":
			c.Version = v
		}
	}
	if c.Name == "" {
		return nil, fmt.Errorf("%s/Chart.yaml has no name", dir)
	}
	if c.Defaults, err = LoadValues(filepath.Join(dir, "values.yaml")); err != nil {
		return nil, err
	}

	c.tmpl = template.New(c.Name).Option("missingkey=zero")
	c.tmpl.Funcs(funcs(c.tmpl))
	paths, err := filepath.Glob(filepath.Join(dir, "templates", "*"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	for _, p := range paths {
		base := filepath.Base(p)
		if base == "NOTES.txt" || !(strings.HasSuffix(base, ".yaml") || strings.HasSuffix(base, ".tpl")) {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		name := c.Name + "/templates/" + base
		if _, err := c.tmpl.New(name).Parse(string(src)); err != nil {
			return nil, err
		}
		if !strings.HasPrefix(base, "_") {
			c.files = append(c.files, name)
		}
	}
	return c, nil
}

// LoadValues reads a values file written in JSON. Lines that start with #
// are skipped, so a values.yaml in JSON syntax may carry a comment header.
func LoadValues(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var kept []string
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			kept = append(kept, line)
		}
	}
	v := map[string]any{}
	if err := json.Unmarshal([]byte(strings.Join(kept, "\n")), &v); err != nil {
		return nil, fmt.Errorf("%s: coolctl reads values as JSON: %w", path, err)
	}
	return v, nil
}

// Merge copies src over dst the way Helm merges values: maps merge key by
// key, everything else is replaced. It returns dst.
func Merge(dst, src map[string]any) map[string]any {
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				dst[k] = Merge(dm, sm)
				continue
			}
			dst[k] = Merge(map[string]any{}, sm)
			continue
		}
		dst[k] = v
	}
	return dst
}

// Render executes every template with the chart's defaults overlaid by the
// given values, in order, and returns one multi-document YAML stream.
func (c *Chart) Render(release, namespace string, values ...map[string]any) (string, error) {
	vals := Merge(map[string]any{}, c.Defaults)
	for _, v := range values {
		Merge(vals, v)
	}
	data := map[string]any{
		"Values":  vals,
		"Release": map[string]any{"Name": release, "Namespace": namespace, "Service": "Helm"},
		"Chart":   map[string]any{"Name": c.Name, "Version": c.Version},
	}
	var out strings.Builder
	for _, name := range c.files {
		var b bytes.Buffer
		if err := c.tmpl.ExecuteTemplate(&b, name, data); err != nil {
			return "", err
		}
		doc := strings.TrimSpace(strings.ReplaceAll(b.String(), "<no value>", ""))
		if doc == "" {
			continue
		}
		doc = strings.TrimPrefix(doc, "---")
		fmt.Fprintf(&out, "---\n# Source: %s\n%s\n", name, strings.TrimSpace(doc))
	}
	return out.String(), nil
}

// funcs returns the template functions the cool charts use, with the
// argument order and behaviour of their Helm and Sprig namesakes.
func funcs(t *template.Template) template.FuncMap {
	return template.FuncMap{
		"include": func(name string, data any) (string, error) {
			var b bytes.Buffer
			err := t.ExecuteTemplate(&b, name, data)
			return b.String(), err
		},
		"toJson": func(v any) (string, error) {
			var b bytes.Buffer
			e := json.NewEncoder(&b)
			e.SetEscapeHTML(false)
			if err := e.Encode(v); err != nil {
				return "", err
			}
			return strings.TrimSuffix(b.String(), "\n"), nil
		},
		"required": func(msg string, v any) (any, error) {
			if empty(v) {
				return nil, errors.New(msg)
			}
			return v, nil
		},
		"fail":     func(msg string) (string, error) { return "", errors.New(msg) },
		"default":  func(d, v any) any { return ternary(v, d, !empty(v)) },
		"empty":    empty,
		"ternary":  ternary,
		"quote":    func(v any) string { return strconv.Quote(toString(v)) },
		"toString": toString,
		"int":      toInt,
		"add": func(xs ...any) int {
			n := 0
			for _, x := range xs {
				n += toInt(x)
			}
			return n
		},
		"sub":        func(a, b any) int { return toInt(a) - toInt(b) },
		"lower":      strings.ToLower,
		"upper":      strings.ToUpper,
		"trim":       strings.TrimSpace,
		"replace":    func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"trimSuffix": func(suffix, s string) string { return strings.TrimSuffix(s, suffix) },
		"trimPrefix": func(prefix, s string) string { return strings.TrimPrefix(s, prefix) },
		"hasPrefix":  func(prefix, s string) bool { return strings.HasPrefix(s, prefix) },
		"hasSuffix":  func(suffix, s string) bool { return strings.HasSuffix(s, suffix) },
		"contains":   func(sub, s string) bool { return strings.Contains(s, sub) },
		"trunc": func(n int, s string) string {
			if n >= 0 && len(s) > n {
				return s[:n]
			}
			return s
		},
		"join": func(sep string, v any) string {
			var parts []string
			switch xs := v.(type) {
			case []string:
				parts = xs
			case []any:
				for _, x := range xs {
					parts = append(parts, toString(x))
				}
			}
			return strings.Join(parts, sep)
		},
		"list": func(xs ...any) []any { return xs },
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict: odd number of arguments")
			}
			d := map[string]any{}
			for i := 0; i < len(kv); i += 2 {
				d[toString(kv[i])] = kv[i+1]
			}
			return d, nil
		},
		"set":       func(d map[string]any, k string, v any) map[string]any { d[k] = v; return d },
		"hasKey":    func(d map[string]any, k string) bool { _, ok := d[k]; return ok },
		"sha256sum": func(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) },
		"b64enc":    func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"indent":    indent,
		"nindent":   func(n int, s string) string { return "\n" + indent(n, s) },
	}
}

func indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
}

func ternary(yes, no any, cond bool) any {
	if cond {
		return yes
	}
	return no
}

// empty follows Sprig: nil, zero numbers, false, and empty strings, lists
// and maps are empty.
func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case float64:
		return x == 0
	case int:
		return x == 0
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	}
	return false
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// toInt converts the number types a JSON document or a template can yield.
func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(x)
		return n
	case bool:
		if x {
			return 1
		}
	}
	return 0
}
