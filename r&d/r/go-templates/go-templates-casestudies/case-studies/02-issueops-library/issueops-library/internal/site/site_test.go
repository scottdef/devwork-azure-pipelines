package site

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/event"
	"github.com/CoolGitOrg/issueops-library/internal/golden"
	"github.com/CoolGitOrg/issueops-library/internal/item"
	"github.com/CoolGitOrg/issueops-library/internal/ledger"
)

var now = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// build publishes a few fixture events into a temp items dir, borrows one,
// and returns the rendered site.
func build(t *testing.T) (*Site, map[string][]byte) {
	t.Helper()
	root := t.TempDir()
	r := &item.Renderer{TemplateDir: "../../templates/items", MetaDir: "../../templates/meta", Now: func() time.Time { return now }}
	for i, n := range []string{"publish", "publish-readme", "publish-nomad"} {
		f, _ := os.Open("../../testdata/events/" + n + ".json")
		e, err := event.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		r.Now = func() time.Time { return now.Add(time.Duration(i) * time.Minute) }
		it, files, err := r.Render(e, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := item.Write(root, it, files); err != nil {
			t.Fatal(err)
		}
	}
	items, err := item.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	l := &ledger.Ledger{}
	if _, err := l.Borrow("payments-api/caddyfile/42", "bob", 14, 50, "", true, now.Add(-20*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Borrow("docs-portal/readme/47", "carol", 7, 51, "", true, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	cfg := Config{BaseURL: "https://coolgitorg.github.io/issueops-library", GitHub: "https://github.com",
		Repo: "CoolGitOrg/issueops-library", IntakeRepo: "CoolGitOrg/issueops-intake",
		IntakeURL: "https://coolgitorg.github.io/issueops-intake/", Version: "test", Now: now}
	s := New(cfg, root, items, l)
	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	return s, files
}

func TestGolden(t *testing.T) {
	_, files := build(t)
	for _, p := range []string{
		"index.html", "items/payments-api/caddyfile/42/index.html", "items/docs-portal/readme/47/status.html",
		"ledger/index.html", "ledger/open.html", "ledger/history.html", "ledger/open.json",
		"feed.xml", "sitemap.txt", "library.json", "SHA256SUMS", "404.html",
	} {
		b, ok := files[p]
		if !ok {
			t.Errorf("missing %s", p)
			continue
		}
		golden.Check(t, "../../testdata/golden/site/"+strings.ReplaceAll(p, "/", "_"), b)
	}
}

func TestStructure(t *testing.T) {
	s, files := build(t)
	if s.PathPrefix != "/issueops-library/" {
		t.Errorf("PathPrefix = %q", s.PathPrefix)
	}
	for _, p := range []string{"static/htmx/htmx.min.js", "static/htmx/LICENSE", "static/htmx/SHA256SUMS", "static/library.js", "static/site.css", ".nojekyll",
		"items/payments-api/caddyfile/42/bundle.tar.gz", "items/payments-api/caddyfile/42/artifact.caddyfile"} {
		if _, ok := files[p]; !ok {
			t.Errorf("missing %s", p)
		}
	}
	if !strings.Contains(string(files["static/htmx/SHA256SUMS"]), item.SHA256Hex(files["static/htmx/htmx.min.js"])) {
		t.Error("vendored htmx does not match its SHA256SUMS")
	}
	idx := string(files["index.html"])
	// Catalog rows alternate even, odd, even.
	cat := idx[strings.Index(idx, `<table id="catalog">`):]
	cat = cat[:strings.Index(cat, "</table>")]
	if strings.Count(cat, `<tr class="even"`) != 2 || strings.Count(cat, `<tr class="odd"`) != 1 {
		t.Errorf("catalog rows do not alternate:\n%s", cat)
	}
	if !strings.Contains(idx, `hx-get="./ledger/open.html"`) {
		t.Error("catalog does not load the ledger fragment")
	}
	if !strings.Contains(string(files["ledger/open.html"]), `href="/issueops-library/items/payments-api/caddyfile/42/"`) {
		t.Error("fragment links are not root-absolute")
	}
	if !strings.Contains(string(files["items/payments-api/caddyfile/42/status.html"]), "overdue") {
		t.Error("overdue loan not shown as overdue")
	}
	var ix Index
	if err := json.Unmarshal(files["library.json"], &ix); err != nil || ix.Count != 3 || !ix.Items[len(ix.Items)-1].Borrowed {
		t.Errorf("library.json: %v %+v", err, ix)
	}
	var feed struct {
		XMLName xml.Name `xml:"feed"`
		Entries []struct {
			ID string `xml:"id"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(files["feed.xml"], &feed); err != nil || len(feed.Entries) != 3 {
		t.Errorf("feed.xml: %v, %d entries", err, len(feed.Entries))
	}
	sums := string(files["SHA256SUMS"])
	if !strings.Contains(sums, "  index.html\n") || strings.Contains(sums, "  SHA256SUMS\n") {
		t.Error("site SHA256SUMS wrong")
	}
}

func TestEscaping(t *testing.T) {
	_, files := build(t)
	page := string(files["items/docs-portal/readme/47/index.html"])
	if strings.Contains(page, "<script>alert(1)") {
		t.Error("payload script tag reached the page unescaped")
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)") {
		t.Error("payload text missing from artifact preview")
	}
}

func TestWriteReplaces(t *testing.T) {
	s, _ := build(t)
	dir := t.TempDir() + "/site"
	os.MkdirAll(dir+"/stale", 0o755)
	os.WriteFile(dir+"/stale/old.html", []byte("x"), 0o644)
	if err := s.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/stale/old.html"); !os.IsNotExist(err) {
		t.Error("stale page survived a rebuild")
	}
	if _, err := os.Stat(dir + "/index.html"); err != nil {
		t.Error(err)
	}
}

func TestEmptyLibrary(t *testing.T) {
	s := New(Config{BaseURL: "https://x.test/", Now: now}, "", nil, &ledger.Ledger{})
	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["index.html"]), "The library is empty") || !strings.Contains(string(files["ledger/open.html"]), "Nothing is borrowed") {
		t.Error("empty states missing")
	}
	golden.Check(t, "../../testdata/golden/site/empty_library.json", files["library.json"])
}
