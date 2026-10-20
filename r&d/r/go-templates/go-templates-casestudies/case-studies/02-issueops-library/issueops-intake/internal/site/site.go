// Package site renders the static intake site from the issue forms and
// their rules files. Nothing on the page is written by hand: labels,
// options, required marks, patterns and length limits all come from
// .github/ISSUE_TEMPLATE/*.yml and schema/*.json, so the page and the
// form GitHub shows cannot drift apart.
package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/CoolGitOrg/issueops-intake/internal/form"
	"github.com/CoolGitOrg/issueops-intake/internal/schema"
)

//go:embed templates/*.html templates/pages/*.html
var tmplFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Config describes where the site points.
type Config struct {
	Repo      string // owner/name of the intake repo, e.g. CoolGitOrg/issueops-intake
	GitHub    string // web base, e.g. https://github.com
	Library   string // library Pages URL
	MaxURL    int    // prefill URL length guard
	Version   string // build version shown in the footer
	BuildTime string // fixed in tests so output is reproducible
}

// Field is a form field plus what the rules file says about it.
type Field struct {
	form.Field
	Slug      string // owning form, used to keep DOM ids unique per page
	Pattern   string
	MaxLength int
	MinLength int
	JSON      bool
}

// Page is one form as the template sees it.
type Page struct {
	Form        *form.Form
	Slug        string // file name without .yml
	Fields      []Field
	Markdown    []string // markdown elements; links rendered, everything else text
	TitleFields []string // field ids joined into the issue title
}

// Site is the data handed to every page template.
type Site struct {
	Config
	Pages []Page
	Flags map[string]any
}

// titleFields picks the fields whose values make a useful issue title.
var titleFields = map[string][]string{
	"request": {"source-id", "template-id"},
	"borrow":  {"item-id"},
	"return":  {"item-id"},
}

// Load reads forms from formDir and rules from schemaDir (<slug>.json,
// optional per form) and flags from flagsPath (optional).
func Load(cfg Config, formDir, schemaDir, flagsPath string) (*Site, error) {
	forms, err := form.LoadDir(formDir)
	if err != nil {
		return nil, err
	}
	if len(forms) == 0 {
		return nil, fmt.Errorf("site: no forms in %s", formDir)
	}
	s := &Site{Config: cfg, Flags: map[string]any{"submit": true}}
	if flagsPath != "" {
		b, err := os.ReadFile(flagsPath)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(b, &s.Flags); err != nil {
			return nil, fmt.Errorf("site: %s: %w", flagsPath, err)
		}
	}
	for _, f := range forms {
		slug := strings.TrimSuffix(f.File, ".yml")
		var rules *schema.Schema
		if schemaDir != "" {
			p := filepath.Join(schemaDir, slug+".json")
			if _, err := os.Stat(p); err == nil {
				if rules, err = schema.Load(p); err != nil {
					return nil, err
				}
			}
		}
		pg := Page{Form: f, Slug: slug, TitleFields: titleFields[slug]}
		for _, fd := range f.Fields {
			if fd.Type == "markdown" {
				pg.Markdown = append(pg.Markdown, strings.TrimSpace(fd.Value))
				continue
			}
			v := Field{Field: fd, Slug: slug, JSON: fd.Render == "json"}
			if rules != nil {
				if r, ok := rules.Fields[fd.ID]; ok {
					v.Pattern, v.MaxLength, v.MinLength = r.Pattern, r.MaxLength, r.MinLength
					v.JSON = v.JSON || r.Type == "json"
				}
			}
			pg.Fields = append(pg.Fields, v)
		}
		s.Pages = append(s.Pages, pg)
	}
	sort.SliceStable(s.Pages, func(i, j int) bool { return rank(s.Pages[i].Slug) < rank(s.Pages[j].Slug) })
	return s, nil
}

// rank puts the request form first; the rest keep file-name order.
func rank(slug string) int {
	switch slug {
	case "request":
		return 0
	case "borrow":
		return 1
	case "return":
		return 2
	}
	return 3
}

var mdLink = regexp.MustCompile(`\[([^\]]+)\]\((https://[^)\s]+)\)`)

// mdLinks renders a markdown element as HTML: [text](https://...) links
// become anchors, everything else is escaped text. That is all the form
// notes need, and it is small enough to trust.
func mdLinks(s string) template.HTML {
	var b strings.Builder
	last := 0
	for _, m := range mdLink.FindAllStringSubmatchIndex(s, -1) {
		b.WriteString(template.HTMLEscapeString(s[last:m[0]]))
		fmt.Fprintf(&b, `<a href="%s">%s</a>`, template.HTMLEscapeString(s[m[4]:m[5]]), template.HTMLEscapeString(s[m[2]:m[3]]))
		last = m[1]
	}
	b.WriteString(template.HTMLEscapeString(s[last:]))
	return template.HTML(b.String())
}

var funcs = template.FuncMap{
	"join":    strings.Join,
	"mdLinks": mdLinks,
	"htmlPattern": func(p string) string {
		// HTML pattern attributes are implicitly anchored; strip ^ $.
		return strings.TrimSuffix(strings.TrimPrefix(p, "^"), "$")
	},
	"isDefault": func(f Field, i int) bool { return f.Default == i },
}

// parse loads the layout and partials. Each page is parsed into its own
// Clone so that every page can define "content" without clobbering the
// others.
func parse() (*template.Template, error) {
	return template.New("").Funcs(funcs).ParseFS(tmplFS, "templates/*.html")
}

// Files renders the whole site as path -> content, so tests can compare
// without touching disk and Write can stay trivial.
func (s *Site) Files() (map[string][]byte, error) {
	base, err := parse()
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	pages := []struct{ path, tmpl, root string }{
		{"index.html", "pages/index.html", "./"},
		{"lab/index.html", "pages/lab.html", "../"},
	}
	for _, p := range pages {
		t, err := base.Clone()
		if err != nil {
			return nil, err
		}
		if t, err = t.ParseFS(tmplFS, "templates/"+p.tmpl); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		data := struct {
			*Site
			Root string
		}{s, p.root}
		if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
			return nil, fmt.Errorf("site: %s: %w", p.path, err)
		}
		out[p.path] = buf.Bytes()
	}
	err = fs.WalkDir(staticFS, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := staticFS.ReadFile(p)
		out[p] = b
		return err
	})
	if err != nil {
		return nil, err
	}
	flags, err := json.MarshalIndent(s.Flags, "", "  ")
	if err != nil {
		return nil, err
	}
	out["flags.json"] = append(flags, '\n')
	forms, err := json.MarshalIndent(s.Pages, "", "  ")
	if err != nil {
		return nil, err
	}
	out["forms.json"] = append(forms, '\n')
	out[".nojekyll"] = nil
	return out, nil
}

// Write renders the site into dir.
func (s *Site) Write(dir string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	for p, b := range files {
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
