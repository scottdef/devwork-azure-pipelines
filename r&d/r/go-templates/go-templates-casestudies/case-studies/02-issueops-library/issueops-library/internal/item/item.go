// Package item renders and stores library items.
//
// An item lives at items/<source-id>/<template-id>/<issue>/ and holds
//
//	artifact.<ext>   the rendered template
//	next.tmpl        a Go template for the next phase ([[ ]] delimiters)
//	config.json      everything needed to re-render: inputs, hashes, provenance
//	bundle.tar.gz    the three files above, deterministic (fixed mtimes, sorted)
//	SHA256SUMS       sha256sum(1)-format lines for all four files
//
// Items are keyed by source, never by requester: the requester is recorded
// once, as config.json "requested_by", so a person's name never becomes
// part of a URL.
package item

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/event"
)

// Config is config.json.
type Config struct {
	ID          string         `json:"id"`
	SourceID    string         `json:"source_id"`
	TemplateID  string         `json:"template_id"`
	Phase       int            `json:"phase"`
	Issue       int            `json:"issue"`
	IssueURL    string         `json:"issue_url"`
	RequestedBy string         `json:"requested_by"`
	PublishedAt time.Time      `json:"published_at"`
	Artifact    string         `json:"artifact"`
	PayloadSHA  string         `json:"payload_sha256"`
	TemplateSHA string         `json:"template_sha256"`
	Payload     map[string]any `json:"payload"`
	Supersedes  string         `json:"supersedes,omitempty"`
}

// File is one entry in an item directory.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Item is a stored item as pages see it.
type Item struct {
	Config
	Files []File
}

// Path returns the item's directory relative to the items root.
func (it *Item) Path() string { return filepath.FromSlash(it.ID) }

// Renderer turns publish events into item files.
type Renderer struct {
	TemplateDir string // templates/items: <template-id>.tmpl
	MetaDir     string // templates/meta: next.tmpl.tmpl
	Now         func() time.Time
}

// Render produces the item's files in memory. supersedes, when set, is
// the id of the borrowed previous-phase item this one continues.
func (r *Renderer) Render(e *event.Event, supersedes string) (*Item, map[string][]byte, error) {
	if e.Action != event.Publish {
		return nil, nil, fmt.Errorf("item: render needs %s, got %s", event.Publish, e.Action)
	}
	tpath := filepath.Join(r.TemplateDir, e.TemplateID+".tmpl")
	src, err := os.ReadFile(tpath)
	if err != nil {
		return nil, nil, fmt.Errorf("item: template %s: %w", e.TemplateID, err)
	}
	t, h, err := ParseTemplate(e.TemplateID+".tmpl", string(src))
	if err != nil {
		return nil, nil, err
	}
	data := map[string]any{}
	for k, v := range e.Payload {
		data[k] = v
	}
	// Templates see the payload at the top level and the provenance under
	// "_", so a payload key can never shadow it.
	data["_"] = map[string]any{
		"source_id": e.SourceID, "template_id": e.TemplateID, "phase": e.Phase,
		"issue": e.Issue, "item_id": e.ItemID,
	}
	art, err := Execute(t, data)
	if err != nil {
		return nil, nil, err
	}
	tsum := sha256.Sum256(src)
	cfg := Config{
		ID: e.ItemID, SourceID: e.SourceID, TemplateID: e.TemplateID, Phase: e.Phase,
		Issue: e.Issue, IssueURL: fmt.Sprintf("https://github.com/%s/issues/%d", e.Repo, e.Issue),
		RequestedBy: e.Requester, PublishedAt: r.Now().UTC().Truncate(time.Second),
		Artifact: "artifact." + h.Ext, PayloadSHA: e.SHA256, TemplateSHA: hex.EncodeToString(tsum[:]),
		Payload: e.Payload, Supersedes: supersedes,
	}
	next, err := r.next(cfg)
	if err != nil {
		return nil, nil, err
	}
	cj, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	files := map[string][]byte{
		cfg.Artifact:  art,
		"next.tmpl":   next,
		"config.json": append(cj, '\n'),
	}
	bundle, err := Bundle(cfg.ID, files, cfg.PublishedAt)
	if err != nil {
		return nil, nil, err
	}
	files["bundle.tar.gz"] = bundle
	files["SHA256SUMS"] = Sums(files)
	it := &Item{Config: cfg, Files: fileList(files)}
	return it, files, nil
}

// next renders the meta-template that writes the next phase's template.
// The meta-template runs with [[ ]] delimiters, so every {{ }} in it is
// literal output: the result is itself a Go template, ready for tpl.
func (r *Renderer) next(cfg Config) ([]byte, error) {
	p := filepath.Join(r.MetaDir, "next.tmpl.tmpl")
	src, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("item: meta-template: %w", err)
	}
	t, _, err := ParseTemplate("next.tmpl.tmpl", "#!tmpl delims=[[,]] missingkey=error\n"+string(src))
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(cfg.Payload))
	for k := range cfg.Payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	nextPhase := cfg.Phase + 1
	return Execute(t, map[string]any{
		"Item": cfg, "Keys": keys, "NextPhase": nextPhase, "Final": nextPhase > 3,
	})
}

// Write stores files under root/<item id>/, replacing any previous item
// with the same id (an edited issue re-publishes in place).
func Write(root string, it *Item, files map[string][]byte) error {
	dir := filepath.Join(root, it.Path())
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Bundle builds a reproducible tar.gz: sorted names, fixed mtime and
// owner, no gzip timestamp. Same inputs, same bytes, same sha256.
func Bundle(prefix string, files map[string][]byte, mtime time.Time) ([]byte, error) {
	var buf bytes.Buffer
	gz, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	gz.Header.ModTime = time.Time{}
	gz.Header.OS = 255 // unknown: keeps output identical across GOOS
	tw := tar.NewWriter(gz)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	base := strings.ReplaceAll(prefix, "/", "_")
	for _, n := range names {
		hdr := &tar.Header{
			Name: base + "/" + n, Mode: 0o644, Size: int64(len(files[n])),
			ModTime: mtime.UTC(), Typeflag: tar.TypeReg, Format: tar.FormatPAX,
			Uname: "library", Gname: "library",
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Sums returns sha256sum(1) output ("<hex>  <name>\n") for files, sorted
// by name, excluding SHA256SUMS itself.
func Sums(files map[string][]byte) []byte {
	var b bytes.Buffer
	for _, f := range fileList(files) {
		if f.Name == "SHA256SUMS" {
			continue
		}
		fmt.Fprintf(&b, "%s  %s\n", f.SHA256, f.Name)
	}
	return b.Bytes()
}

// SHA256Hex returns the hex sha256 of b.
func SHA256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func fileList(files map[string][]byte) []File {
	out := make([]File, 0, len(files))
	for n, b := range files {
		s := sha256.Sum256(b)
		out = append(out, File{Name: n, Size: int64(len(b)), SHA256: hex.EncodeToString(s[:])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Load reads every item under root (root/<source>/<template>/<issue>/config.json).
// Directories that do not look like items are skipped, not fatal: the tree
// is git-managed and a stray file should not stop a deploy.
func Load(root string) ([]*Item, error) {
	cfgs, err := filepath.Glob(filepath.Join(root, "*", "*", "*", "config.json"))
	if err != nil {
		return nil, err
	}
	var out []*Item
	for _, p := range cfgs {
		dir := filepath.Dir(p)
		rel, _ := filepath.Rel(root, dir)
		id, err := event.CleanItemID(filepath.ToSlash(rel))
		if err != nil {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var it Item
		if err := json.Unmarshal(b, &it.Config); err != nil {
			return nil, fmt.Errorf("item: %s: %w", p, err)
		}
		if it.ID != id {
			return nil, fmt.Errorf("item: %s: id %q does not match its path", p, it.ID)
		}
		ents, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		files := map[string][]byte{}
		for _, e := range ents {
			if e.Type().IsRegular() {
				if files[e.Name()], err = os.ReadFile(filepath.Join(dir, e.Name())); err != nil {
					return nil, err
				}
			}
		}
		it.Files = fileList(files)
		out = append(out, &it)
	}
	Sort(out)
	return out, nil
}

// Sort orders items newest first, then by id, so pages are stable.
func Sort(items []*Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if !a.PublishedAt.Equal(b.PublishedAt) {
			return a.PublishedAt.After(b.PublishedAt)
		}
		return a.ID < b.ID
	})
}

// Exists reports whether the item id has a directory under root.
func Exists(root, id string) bool {
	clean, err := event.CleanItemID(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(root, filepath.FromSlash(clean), "config.json"))
	return err == nil
}

// PhaseOf reads the phase of a stored item.
func PhaseOf(root, id string) (int, error) {
	clean, err := event.CleanItemID(id)
	if err != nil {
		return 0, err
	}
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(clean), "config.json"))
	if err != nil {
		return 0, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return 0, fmt.Errorf("item: %s: %w", clean, err)
	}
	return c.Phase, nil
}

// Number formats an issue number for templates that want a string.
func Number(n int) string { return strconv.Itoa(n) }
