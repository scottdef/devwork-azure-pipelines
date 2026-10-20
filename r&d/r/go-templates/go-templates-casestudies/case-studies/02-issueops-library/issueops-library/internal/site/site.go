// Package site renders the library's GitHub Pages site from items/ and
// the ledger. The site is rebuilt in full on every event: it is small, a
// full rebuild cannot leave a stale page behind, and the output is a pure
// function of (items, ledger, config, time) so it is golden-testable.
//
// HTML goes through html/template (contextual escaping of everything that
// came from an issue). The Atom feed, sitemap and SHA256SUMS are not HTML
// and go through text/template with an explicit xml escaper.
package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	ttemplate "text/template"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/funcs"
	"github.com/CoolGitOrg/issueops-library/internal/item"
	"github.com/CoolGitOrg/issueops-library/internal/ledger"
)

//go:embed templates/*.html templates/pages/*.html templates/text/*.tmpl
var tmplFS embed.FS

//go:embed static
var staticFS embed.FS

// Config is where the site lives and what it links to.
type Config struct {
	BaseURL    string // https://coolgitorg.github.io/issueops-library/ (trailing slash)
	PathPrefix string // URL path of the site root, e.g. /issueops-library/; derived from BaseURL
	GitHub     string // https://github.com
	Repo       string // CoolGitOrg/issueops-library
	IntakeRepo string // CoolGitOrg/issueops-intake
	IntakeURL  string // https://coolgitorg.github.io/issueops-intake/
	Version    string
	Now        time.Time // build time; fixed in tests
}

// Entry is an item plus its loan state, as templates see it.
type Entry struct {
	*item.Item
	Loan         *ledger.Loan // open loan or nil
	URL          string       // absolute
	Href         string       // relative to site root, with trailing slash
	BorrowURL    string       // prefilled borrow issue
	ReturnURL    string       // prefilled return issue
	ArtifactFile *item.File   // the Files entry for Config.Artifact
	SupersededBy *Entry       // the next-phase item that continued this one
}

// Site is the data for all pages.
type Site struct {
	Config
	Entries []*Entry
	Open    []*ledger.Loan
	History []*ledger.Loan
	Ledger  *ledger.Ledger
	Items   string // items root on disk, for copying files
}

// New builds the site model.
func New(cfg Config, itemsDir string, items []*item.Item, l *ledger.Ledger) *Site {
	if !strings.HasSuffix(cfg.BaseURL, "/") {
		cfg.BaseURL += "/"
	}
	if cfg.PathPrefix == "" {
		cfg.PathPrefix = "/"
		if u, err := url.Parse(cfg.BaseURL); err == nil && u.Path != "" {
			cfg.PathPrefix = u.Path
		}
	}
	s := &Site{Config: cfg, Ledger: l, Items: itemsDir, Open: l.OpenLoans(), History: l.History()}
	for _, it := range items {
		e := &Entry{Item: it, Loan: l.OpenLoan(it.ID)}
		e.Href = "items/" + it.ID + "/"
		e.URL = cfg.BaseURL + e.Href
		e.BorrowURL = s.issueURL("borrow.yml", "[borrow] "+it.ID, it.ID)
		e.ReturnURL = s.issueURL("return.yml", "[return] "+it.ID, it.ID)
		for i := range it.Files {
			if it.Files[i].Name == it.Artifact {
				e.ArtifactFile = &it.Files[i]
			}
		}
		s.Entries = append(s.Entries, e)
	}
	byID := map[string]*Entry{}
	for _, e := range s.Entries {
		byID[e.ID] = e
	}
	for _, e := range s.Entries {
		if prev := byID[e.Supersedes]; prev != nil {
			prev.SupersededBy = e
		}
	}
	return s
}

// issueURL builds a prefilled new-issue link. Only the input field is
// prefilled: GitHub honours ?<id>= for inputs and textareas.
func (s *Site) issueURL(tmpl, title, id string) string {
	return fmt.Sprintf("%s/%s/issues/new?template=%s&title=%s&item-id=%s",
		s.GitHub, s.IntakeRepo, tmpl, queryEscape(title), queryEscape(id))
}

func queryEscape(s string) string {
	r := strings.NewReplacer("%", "%25", " ", "%20", "&", "%26", "#", "%23", "+", "%2B", "[", "%5B", "]", "%5D", "=", "%3D", "?", "%3F")
	return r.Replace(s)
}

// page is the per-page wrapper: the site, the page's own data, and the
// relative path back to the site root.
type page struct {
	*Site
	Root     string
	Title    string
	Entry    *Entry
	Artifact string // item pages: artifact text, shown escaped
	Next     string // item pages: next.tmpl text
}

func htmlFuncs() template.FuncMap {
	m := template.FuncMap{}
	for k, v := range funcs.Map() {
		m[k] = v
	}
	m["overdue"] = func(l *ledger.Loan, now time.Time) bool { return l != nil && now.After(l.Due) }
	m["odd"] = func(i int) bool { return i%2 == 1 }
	return m
}

func textFuncs() ttemplate.FuncMap {
	m := ttemplate.FuncMap{}
	for k, v := range funcs.Map() {
		m[k] = v
	}
	m["xml"] = func(s string) (string, error) {
		var b bytes.Buffer
		err := xml.EscapeText(&b, []byte(s))
		return b.String(), err
	}
	return m
}

// Files renders the whole site as path -> bytes.
func (s *Site) Files() (map[string][]byte, error) {
	base, err := template.New("").Funcs(htmlFuncs()).ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}

	// render clones the base set and adds one page's "content" to it.
	// Every page, including each item page, gets its own Clone so no page
	// can see another page's definitions.
	render := func(dst, pageFile, layout string, p page) error {
		t, err := base.Clone()
		if err != nil {
			return err
		}
		if pageFile != "" {
			if t, err = t.ParseFS(tmplFS, "templates/pages/"+pageFile); err != nil {
				return err
			}
		}
		var buf bytes.Buffer
		if err := t.ExecuteTemplate(&buf, layout, p); err != nil {
			return fmt.Errorf("site: %s: %w", dst, err)
		}
		out[dst] = buf.Bytes()
		return nil
	}

	if err := render("index.html", "index.html", "layout", page{Site: s, Root: "./", Title: "Library catalog"}); err != nil {
		return nil, err
	}
	if err := render("ledger/index.html", "ledger.html", "layout", page{Site: s, Root: "../", Title: "Ledger"}); err != nil {
		return nil, err
	}
	if err := render("404.html", "404.html", "layout", page{Site: s, Root: s.PathPrefix, Title: "Not found"}); err != nil {
		return nil, err
	}
	// Fragments: bare partials, fetched by htmx with GET (static hosting
	// serves GET; nothing here needs a server). A fragment is swapped into
	// pages at different depths, so its own file links from the site's
	// absolute path; the copy inlined into each page at build time uses
	// that page's relative root and so also works from a local checkout.
	if err := render("ledger/open.html", "", "ledger-open", page{Site: s, Root: s.PathPrefix}); err != nil {
		return nil, err
	}
	if err := render("ledger/history.html", "", "ledger-history", page{Site: s, Root: s.PathPrefix}); err != nil {
		return nil, err
	}
	for _, e := range s.Entries {
		if e.ArtifactFile == nil {
			return nil, fmt.Errorf("site: item %s: artifact %q missing from its directory", e.ID, e.Item.Artifact)
		}
		root := strings.Repeat("../", strings.Count(e.Href, "/"))
		if err := s.copyItem(e, out); err != nil {
			return nil, err
		}
		p := page{Site: s, Root: root, Title: e.ID, Entry: e,
			Artifact: preview(out[e.Href+e.ArtifactFile.Name]), Next: preview(out[e.Href+"next.tmpl"])}
		if err := render(e.Href+"index.html", "item.html", "layout", p); err != nil {
			return nil, err
		}
		frag := p
		frag.Root = s.PathPrefix
		if err := render(e.Href+"status.html", "", "item-status", frag); err != nil {
			return nil, err
		}
	}

	// Text outputs.
	tt, err := ttemplate.New("").Funcs(textFuncs()).ParseFS(tmplFS, "templates/text/*.tmpl")
	if err != nil {
		return nil, err
	}
	for name, dst := range map[string]string{"feed.xml.tmpl": "feed.xml", "sitemap.txt.tmpl": "sitemap.txt"} {
		var buf bytes.Buffer
		if err := tt.ExecuteTemplate(&buf, name, s); err != nil {
			return nil, fmt.Errorf("site: %s: %w", dst, err)
		}
		out[dst] = buf.Bytes()
	}

	if out["ledger/open.json"], err = jsonBytes(s.Open); err != nil {
		return nil, err
	}
	if out["library.json"], err = jsonBytes(s.index()); err != nil {
		return nil, err
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
	out[".nojekyll"] = []byte{}
	out["SHA256SUMS"] = item.Sums(out)
	return out, nil
}

// preview returns at most 64 KiB of b for display on the item page.
func preview(b []byte) string {
	const max = 64 << 10
	if len(b) > max {
		return string(b[:max]) + "\n... (truncated; download the file)"
	}
	return string(b)
}

// copyItem publishes the item's stored files next to its page.
func (s *Site) copyItem(e *Entry, out map[string][]byte) error {
	if s.Items == "" {
		return nil
	}
	for _, f := range e.Files {
		b, err := os.ReadFile(filepath.Join(s.Items, e.Path(), f.Name))
		if err != nil {
			return err
		}
		out[e.Href+f.Name] = b
	}
	return nil
}

// Index is library.json: the machine-readable catalog.
type Index struct {
	Generated time.Time      `json:"generated"`
	Base      string         `json:"base"`
	Count     int            `json:"count"`
	Items     []IndexItem    `json:"items"`
	Open      []*ledger.Loan `json:"open_loans"`
}

// IndexItem is one catalog row in library.json.
type IndexItem struct {
	ID          string      `json:"id"`
	SourceID    string      `json:"source_id"`
	TemplateID  string      `json:"template_id"`
	Phase       int         `json:"phase"`
	Issue       int         `json:"issue"`
	PublishedAt time.Time   `json:"published_at"`
	URL         string      `json:"url"`
	Artifact    string      `json:"artifact"`
	Borrowed    bool        `json:"borrowed"`
	Due         *time.Time  `json:"due,omitempty"`
	Files       []item.File `json:"files"`
}

func (s *Site) index() Index {
	ix := Index{Generated: s.Now.UTC(), Base: s.BaseURL, Count: len(s.Entries), Items: []IndexItem{}, Open: s.Open}
	if ix.Open == nil {
		ix.Open = []*ledger.Loan{}
	}
	for _, e := range s.Entries {
		r := IndexItem{ID: e.ID, SourceID: e.SourceID, TemplateID: e.TemplateID, Phase: e.Phase,
			Issue: e.Issue, PublishedAt: e.PublishedAt, URL: e.URL, Artifact: e.URL + e.ArtifactFile.Name, Files: e.Files}
		if e.Loan != nil {
			r.Borrowed, r.Due = true, &e.Loan.Due
		}
		ix.Items = append(ix.Items, r)
	}
	return ix
}

func jsonBytes(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	return b.Bytes(), err
}

// Write replaces dir with the rendered site. It writes to a sibling temp
// dir and renames, so a failed render leaves the previous site intact.
func (s *Site) Write(dir string) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	dir = filepath.Clean(dir)
	tmp := dir + ".tmp"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for p := range files {
		names = append(names, p)
	}
	sort.Strings(names)
	for _, p := range names {
		if path.IsAbs(p) || strings.Contains(p, "..") {
			return fmt.Errorf("site: refusing path %q", p)
		}
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, files[p], 0o644); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(tmp, dir)
}
