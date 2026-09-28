// Package tmpl renders the platform's Go text/templates: issue comments,
// Copilot cloud agent instructions, agent prompts, Kubernetes manifests and
// report summaries. Templates are embedded in the binary and can be
// overridden per deployment by pointing ISSUEOPS_TEMPLATES at a directory of
// same-named files (for example to re-brand comments) without rebuilding.
//
// Safety rules baked into the function map:
//   - user-supplied text goes through `safe` (neutralizes @mentions and HTML,
//     so a request can never spoof an IssueOps marker comment or ping teams);
//   - values embedded in YAML go through `yamlString` (JSON-quoted scalars);
//   - JSON payloads are produced with encoding/json, never with templates.
package tmpl

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

//go:embed templates/*.tmpl
var embedded embed.FS

// Set is a parsed template set.
type Set struct{ t *template.Template }

// Load parses embedded templates and applies overrides from dir (optional).
func Load(overrideDir string) (*Set, error) {
	t, err := template.New("issueops").Funcs(Funcs()).ParseFS(embedded, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("tmpl: parse embedded: %w", err)
	}
	if overrideDir == "" {
		overrideDir = os.Getenv("ISSUEOPS_TEMPLATES")
	}
	if overrideDir != "" {
		matches, _ := filepath.Glob(filepath.Join(overrideDir, "*.tmpl"))
		for _, m := range matches {
			b, err := os.ReadFile(m)
			if err != nil {
				return nil, err
			}
			if _, err := t.New(filepath.Base(m)).Parse(string(b)); err != nil {
				return nil, fmt.Errorf("tmpl: override %s: %w", m, err)
			}
		}
	}
	return &Set{t: t}, nil
}

// MustLoad panics on error (embedded templates are covered by tests).
func MustLoad() *Set {
	s, err := Load("")
	if err != nil {
		panic(err)
	}
	return s
}

// Render executes a template by file name, e.g. "comment.summary.md.tmpl".
func (s *Set) Render(name string, data any) (string, error) {
	var b bytes.Buffer
	if err := s.t.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("tmpl: %s: %w", name, err)
	}
	return collapseBlankLines(b.String()), nil
}

// Has reports whether a template exists.
func (s *Set) Has(name string) bool { return s.t.Lookup(name) != nil }

// Names lists template names.
func (s *Set) Names() []string {
	var out []string
	for _, t := range s.t.Templates() {
		if strings.HasSuffix(t.Name(), ".tmpl") {
			out = append(out, t.Name())
		}
	}
	sort.Strings(out)
	return out
}

func collapseBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	inFence := false
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
		}
		if !inFence && strings.TrimSpace(l) == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return strings.TrimSpace(strings.Join(out, "\n")) + "\n"
}

// Safe neutralizes user text for Markdown comments: HTML is escaped (so no
// raw HTML or <!-- markers -->), and @mentions / #refs get a zero-width
// joiner so quoting a request cannot notify teams or users.
func Safe(v any) string {
	s := fmt.Sprint(v)
	r := strings.NewReplacer("\r", "", "&", "&amp;", "<", "&lt;", ">", "&gt;", "@", "@‍")
	return r.Replace(s)
}

// MDSafe keeps Markdown (including code blocks) intact but neutralizes
// @mentions and HTML comments. Used for AI-generated deliverables.
func MDSafe(v any) string {
	r := strings.NewReplacer("\r", "", "<!--", "&lt;!--", "-->", "--&gt;", "@", "@\u200d")
	return r.Replace(fmt.Sprint(v))
}

// Cell makes text safe for a Markdown table cell.
func Cell(v any) string {
	s := Safe(v)
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", "<br>")
	return s
}

// Code renders an inline code span that tolerates backticks in the content.
func Code(v any) string {
	s := strings.ReplaceAll(fmt.Sprint(v), "\n", " ")
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		pad = " "
	}
	return fence + pad + s + pad + fence
}

// Fence renders a fenced code block whose fence is longer than any backtick
// run inside the content.
func Fence(lang string, v any) string {
	s := strings.TrimRight(fmt.Sprint(v), "\n")
	fence := "```"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	return fence + lang + "\n" + s + "\n" + fence
}

// YAMLString returns a JSON-quoted string, which is a valid YAML
// double-quoted scalar: safe for any user-controlled value.
func YAMLString(v any) string {
	b, _ := json.Marshal(fmt.Sprint(v))
	return string(b)
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case *float64:
		if x == nil {
			return math.NaN()
		}
		return *x
	case *int:
		if x == nil {
			return math.NaN()
		}
		return float64(*x)
	case string:
		f, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return math.NaN()
		}
		return f
	}
	return math.NaN()
}

func group(digits string) string {
	var out []byte
	for i, c := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// USD formats money with thousands separators ("n/a" for nil/NaN).
func USD(v any) string {
	f := toFloat(v)
	if math.IsNaN(f) {
		return "n/a"
	}
	neg := f < 0
	s := strconv.FormatFloat(math.Abs(f), 'f', 2, 64)
	ip, fp, _ := strings.Cut(s, ".")
	out := "$" + group(ip) + "." + fp
	if neg {
		return "-" + out
	}
	return out
}

// Num formats an integer-like number with separators.
func Num(v any) string {
	f := toFloat(v)
	if math.IsNaN(f) {
		return "n/a"
	}
	sign := ""
	if f < 0 {
		sign = "-"
	}
	if f == math.Trunc(f) {
		return sign + group(strconv.FormatInt(int64(math.Abs(f)), 10))
	}
	ip, fp, _ := strings.Cut(strconv.FormatFloat(math.Abs(f), 'f', 1, 64), ".")
	return sign + group(ip) + "." + fp
}

// Pct formats a ratio as a percentage.
func Pct(v any) string {
	f := toFloat(v)
	if math.IsNaN(f) {
		return "n/a"
	}
	return strconv.FormatFloat(f*100, 'f', 0, 64) + "%"
}

// Funcs is the shared function map.
func Funcs() template.FuncMap {
	return template.FuncMap{
		"safe":       Safe,
		"cell":       Cell,
		"mdsafe":     MDSafe,
		"code":       Code,
		"fence":      Fence,
		"yamlString": YAMLString,
		"usd":        USD,
		"num":        Num,
		"pct":        Pct,
		"hours": func(v any) string {
			f := toFloat(v)
			if math.IsNaN(f) {
				return "n/a"
			}
			return strconv.FormatFloat(f, 'f', 1, 64) + " h"
		},
		"json": func(v any) (string, error) {
			b, err := json.Marshal(v)
			return string(b), err
		},
		"jsonIndent": func(v any) (string, error) {
			b, err := json.MarshalIndent(v, "", "  ")
			return string(b), err
		},
		"mention": func(v any) string {
			s := strings.TrimPrefix(fmt.Sprint(v), "@")
			if s == "" {
				return ""
			}
			return "@" + s
		},
		"join":  func(sep string, v []string) string { return strings.Join(v, sep) },
		"lower": strings.ToLower,
		"upper": strings.ToUpper,
		"trim":  strings.TrimSpace,
		"lines": func(s string) []string {
			var out []string
			for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
				if l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-*•")); l != "" {
					out = append(out, l)
				}
			}
			return out
		},
		"default": func(def, v any) any {
			if v == nil || fmt.Sprint(v) == "" || fmt.Sprint(v) == "<nil>" {
				return def
			}
			return v
		},
		"trunc": func(n int, s string) string {
			r := []rune(s)
			if len(r) <= n {
				return s
			}
			return string(r[:n]) + "…"
		},
		"indent": func(n int, s string) string {
			pad := strings.Repeat(" ", n)
			return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"nindent": func(n int, s string) string {
			pad := strings.Repeat(" ", n)
			return "\n" + pad + strings.ReplaceAll(s, "\n", "\n"+pad)
		},
		"plural": func(n int, one, many string) string {
			if n == 1 {
				return one
			}
			return many
		},
		"add":  func(a, b int) int { return a + b },
		"sub":  func(a, b int) int { return a - b },
		"date": func(layout string, t time.Time) string { return t.UTC().Format(layout) },
		"now":  func() time.Time { return time.Now().UTC() },
		"short": func(d string) string {
			d = strings.TrimPrefix(d, "sha256:")
			if len(d) > 12 {
				return d[:12]
			}
			return d
		},
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, fmt.Errorf("dict needs key/value pairs")
			}
			m := map[string]any{}
			for i := 0; i < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m, nil
		},
		"list": func(v ...any) []any { return v },
		"deref": func(v any) any {
			switch x := v.(type) {
			case *int:
				if x == nil {
					return nil
				}
				return *x
			case *float64:
				if x == nil {
					return nil
				}
				return *x
			}
			return v
		},
		"isNil": func(v any) bool {
			switch x := v.(type) {
			case nil:
				return true
			case *int:
				return x == nil
			case *float64:
				return x == nil
			}
			return false
		},
		"dnsLabel": func(v any) string {
			s := strings.ToLower(fmt.Sprint(v))
			var b strings.Builder
			for _, r := range s {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
					b.WriteRune(r)
				} else {
					b.WriteRune('-')
				}
			}
			out := strings.Trim(b.String(), "-")
			if len(out) > 63 {
				out = strings.Trim(out[:63], "-")
			}
			return out
		},
	}
}
