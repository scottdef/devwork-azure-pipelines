// Package tmpl holds the Go templates the platform renders: Dockerfiles for
// the build presets, the caller workflow, a Grafana dashboard per app, and
// the HTML fragments of the portal.
//
// Text templates use [[ and ]] as delimiters, because their output is full of
// the {{ }} that GitHub Actions and Grafana use themselves.
package tmpl

import (
	"embed"
	htmltemplate "html/template"
	"io"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates
var files embed.FS

var funcs = map[string]any{
	"upper": strings.ToUpper,
	"short": func(s string) string { // a digest or commit, shortened for display
		s = strings.TrimPrefix(s, "sha256:")
		if len(s) > 12 {
			return s[:12]
		}
		return s
	},
	"day": func(s string) string { // the date part of an RFC 3339 time
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Format("2006-01-02 15:04")
		}
		return s
	},
}

var (
	text = texttemplate.Must(texttemplate.New("").Delims("[[", "]]").Funcs(funcs).ParseFS(files, "templates/*.tmpl"))
	html = htmltemplate.Must(htmltemplate.New("").Funcs(funcs).ParseFS(files, "templates/*.html"))
)

// Text renders the named text template.
func Text(w io.Writer, name string, data any) error { return text.ExecuteTemplate(w, name, data) }

// HTML renders the named HTML template, escaping every value it inserts.
func HTML(w io.Writer, name string, data any) error { return html.ExecuteTemplate(w, name, data) }

// Docker is the data of a Dockerfile template.
type Docker struct {
	Preset   string
	Registry string // the platform registry that mirrors base images
	Port     int
	Main     string // the Go package to build, such as "." or "./cmd/server"
}

// Dashboard is the data of the Grafana dashboard template.
type Dashboard struct {
	Team, App, Environment, Namespace string
}
