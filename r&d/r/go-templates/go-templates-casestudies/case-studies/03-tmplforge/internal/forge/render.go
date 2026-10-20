// Package forge is tmplforge's engine: render, lint, variable discovery,
// line diff, bundle export, the catalog and the re-exec sandbox. The TUI,
// the CLI and the web GUI are thin shells over it.
package forge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

// Formats lists the output formats in display order. Only html changes the
// engine (html/template, contextual auto-escaping); json additionally gets
// an output validity check. The rest are labels that pick a file extension.
var Formats = []string{"html", "md", "json", "txt", "yaml"}

// Ext maps a format to the file extension used by w, x and export.
var Ext = map[string]string{"html": ".html", "md": ".md", "json": ".json", "txt": ".txt", "yaml": ".yml"}

// Hard limits for untrusted input. The CLI applies them only under -sandbox.
const (
	MaxTemplate = 64 << 10 // template source, bytes
	MaxOutput   = 1 << 20  // rendered output, bytes
	MaxFixture  = 1 << 20  // fixture JSON, bytes
)

// Errors callers branch on.
var (
	ErrOutputLimit   = errors.New("output limit exceeded")
	ErrTemplateLimit = errors.New("template too large")
	ErrFormat        = errors.New("unknown format")
	ErrInvalidOutput = errors.New("rendered output is not valid for its format")
)

// Options control one render.
type Options struct {
	Name        string           // template name used in error messages; default "tmpl"
	Format      string           // one of Formats; default txt
	Left, Right string           // action delimiters; default {{ }}
	Funcs       template.FuncMap // default funcs.All()
	MaxTemplate int              // 0 means no cap
	MaxOutput   int64            // 0 means no cap
}

func (o Options) name() string {
	if o.Name == "" {
		return "tmpl"
	}
	return o.Name
}

func (o Options) funcs() template.FuncMap {
	if o.Funcs == nil {
		return funcs.All()
	}
	return o.Funcs
}

// ValidFormat reports whether f is a known format.
func ValidFormat(f string) bool {
	for _, x := range Formats {
		if x == f {
			return true
		}
	}
	return false
}

// ParseDelims splits "L,R" into its two halves. Empty means the default.
func ParseDelims(s string) (l, r string, err error) {
	if s == "" {
		return "", "", nil
	}
	l, r, ok := strings.Cut(s, ",")
	if !ok || l == "" || r == "" || strings.ContainsAny(s, " \t\n") {
		return "", "", fmt.Errorf("delims %q: want L,R such as [[,]]", s)
	}
	return l, r, nil
}

// executor is the method set text/template and html/template share.
type executor interface {
	Execute(io.Writer, any) error
}

// Compile parses src for o.Format: html/template for html, else
// text/template. It enforces MaxTemplate.
func Compile(src string, o Options) (executor, error) {
	if o.MaxTemplate > 0 && len(src) > o.MaxTemplate {
		return nil, fmt.Errorf("%w: %d bytes > %d", ErrTemplateLimit, len(src), o.MaxTemplate)
	}
	f := o.Format
	if f == "" {
		f = "txt"
	}
	if !ValidFormat(f) {
		return nil, fmt.Errorf("%w %q (want %s)", ErrFormat, f, strings.Join(Formats, "|"))
	}
	if f == "html" {
		// html/template, not text/template: the GUI previews HTML that an
		// untrusted author wrote against data someone else supplied.
		// Contextual escaping is the difference between a preview and XSS.
		t, err := htmltemplate.New(o.name()).Delims(o.Left, o.Right).Funcs(htmltemplate.FuncMap(o.funcs())).Parse(src)
		if err != nil {
			return nil, err
		}
		return t, nil
	}
	t, err := template.New(o.name()).Delims(o.Left, o.Right).Funcs(o.funcs()).Parse(src)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Render compiles src and executes it against data.
func Render(src string, data any, o Options) ([]byte, error) {
	t, err := Compile(src, o)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	var w io.Writer = &b
	if o.MaxOutput > 0 {
		w = &LimitWriter{W: &b, N: o.MaxOutput}
	}
	if err := t.Execute(w, data); err != nil {
		if errors.Is(err, ErrOutputLimit) {
			return b.Bytes(), fmt.Errorf("%w (%d bytes)", ErrOutputLimit, o.MaxOutput)
		}
		return b.Bytes(), err
	}
	return b.Bytes(), nil
}

// CheckOutput validates out for formats with a cheap stdlib validator.
func CheckOutput(format string, out []byte) error {
	if format == "json" && !json.Valid(out) {
		return fmt.Errorf("%w: json", ErrInvalidOutput)
	}
	return nil
}

// LimitWriter passes at most N bytes to W, then fails every write with
// ErrOutputLimit. Unlike io.LimitReader's silent truncation, the error
// stops template execution at the first byte over.
type LimitWriter struct {
	W io.Writer
	N int64 // bytes remaining
}

func (l *LimitWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > l.N {
		n, _ := l.W.Write(p[:l.N])
		l.N = 0
		return n, ErrOutputLimit
	}
	n, err := l.W.Write(p)
	l.N -= int64(n)
	return n, err
}

// LoadFixture decodes a JSON object into map[string]any. Fixture data is
// deliberately plain: maps, slices, strings, numbers, bools. No Go value
// with methods ever reaches a template, so a template cannot call into the
// host program beyond the FuncMap.
func LoadFixture(r io.Reader) (map[string]any, error) {
	d := json.NewDecoder(io.LimitReader(r, MaxFixture+1))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("fixture: %w", err)
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("fixture: trailing data after JSON object")
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("fixture: top level is %T, want JSON object", v)
	}
	funcs.Normalize(m)
	return m, nil
}

// errLineRE matches the location prefix text/template and html/template
// put on parse and exec errors: "template: NAME:LINE:" and
// "html/template:NAME:LINE:".
var errLineRE = regexp.MustCompile(`template: ?[^:\s]+:(\d+)`)

// errPrefixRE matches the whole location prefix, for StripErrorPrefix.
var errPrefixRE = regexp.MustCompile(`^(html/)?template: ?[^:\s]+:\d+(:\d+)?: `)

// StripErrorPrefix returns err's message without the "template: name:line:"
// location, for displays that show the line separately.
func StripErrorPrefix(err error) string {
	return errPrefixRE.ReplaceAllString(err.Error(), "")
}

// ErrorLine extracts the 1-based template line from a template error, or 0.
func ErrorLine(err error) int {
	if err == nil {
		return 0
	}
	m := errLineRE.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
