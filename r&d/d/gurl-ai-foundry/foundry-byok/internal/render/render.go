// Package render turns a resolved inventory Bundle into files using Go
// text/template templates.
//
// Rendering is pure and deterministic: the same inventory always yields
// byte-identical output (no timestamps, sorted input), so generated files can
// be committed and a CI job can detect drift with Diff.
//
// The tool version is recorded in manifest.json only. Stamping it into every
// file would turn a tool upgrade into a diff of the entire output tree.
package render

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
	"github.com/CoolGitOrg/foundry-byok/internal/inventory"
)

//go:embed templates/*.tmpl
var embedded embed.FS

const templateDir = "templates"

// Kind classifies an output file so callers can route subsets elsewhere
// (for example, copy only workflows into .github/workflows).
type Kind string

const (
	KindData     Kind = "data"
	KindDoc      Kind = "doc"
	KindScript   Kind = "script"
	KindCode     Kind = "code"
	KindWorkflow Kind = "workflow"
	KindSnippet  Kind = "snippet"
)

// File is one rendered output, with a slash-separated path relative to the
// output root.
type File struct {
	Path string
	Kind Kind
	Mode fs.FileMode
	Data []byte
}

// Renderer renders bundles. The zero value uses the embedded templates.
type Renderer struct {
	// OverrideDir, when set, is searched first for each template file name.
	// Files that are absent fall back to the embedded copy, so a team can
	// customize one template without forking all of them.
	OverrideDir string
}

// TemplateNames lists the embedded template file names, sorted.
func TemplateNames() []string {
	entries, err := embedded.ReadDir(templateDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// ExportTemplates writes the embedded templates into dir as a starting point
// for customization. Existing files are never overwritten.
func ExportTemplates(dir string) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var written []string
	for _, name := range TemplateNames() {
		data, err := embedded.ReadFile(path.Join(templateDir, name))
		if err != nil {
			return written, err
		}
		dst := filepath.Join(dir, name)
		f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return written, err
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if werr != nil {
			return written, werr
		}
		if cerr != nil {
			return written, cerr
		}
		written = append(written, dst)
	}
	return written, nil
}

func (r Renderer) load(name string) (*template.Template, error) {
	var (
		data []byte
		err  error
		src  = "embedded:" + name
	)
	if r.OverrideDir != "" {
		p := filepath.Join(r.OverrideDir, name)
		if data, err = os.ReadFile(p); err == nil {
			src = p
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("template %s: %w", p, err)
		}
	}
	if data == nil {
		if data, err = embedded.ReadFile(path.Join(templateDir, name)); err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
	}
	t, err := template.New(name).Option("missingkey=error").Funcs(funcs).Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", src, err)
	}
	return t, nil
}

func (r Renderer) exec(name string, data any) ([]byte, error) {
	t, err := r.load(name)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buf.Bytes(), nil
}

// itemData is the template context for per-deployment templates.
type itemData struct {
	foundry.Resolved
	ToolVersion string
	// KeyEnv is the local environment variable that holds the API key. It
	// reuses the GitHub secret name so one name works everywhere.
	KeyEnv string
	// EntraScope is the OAuth2 scope for bearer tokens.
	EntraScope string
	// ClientKind selects the request shape in client.go:
	// "chat", "responses" or "anthropic".
	ClientKind string
	// ClientEndpoint is the full URL client.go posts to.
	ClientEndpoint string
	// ModelsURLHint is where to look up valid engine.model values.
	ModelsURLHint string
}

func newItemData(version string, it foundry.Resolved) itemData {
	d := itemData{
		Resolved: it, ToolVersion: version, EntraScope: foundry.EntraScope,
		KeyEnv: it.Spec.APIKeySecret,
	}
	if d.KeyEnv == "" { // auth=entra: still offer a key variable for local use
		d.KeyEnv = foundry.DefaultAPIKeySecret
	}
	switch {
	case it.Surface == foundry.SurfaceAnthropic:
		d.ClientKind, d.ClientEndpoint = "anthropic", it.URLs.AnthropicMessages
		d.ModelsURLHint = "(n/a for the Anthropic surface: use the Claude model name shown in the Foundry deployment)"
	case it.WireAPI == foundry.WireResponses:
		d.ClientKind, d.ClientEndpoint = "responses", it.URLs.V1Responses
		d.ModelsURLHint = it.URLs.V1Models
	default:
		d.ClientKind, d.ClientEndpoint = "chat", it.URLs.V1ChatCompletions
		d.ModelsURLHint = it.URLs.V1Models
	}
	return d
}

// ItemDir returns the output directory for one deployment.
func ItemDir(it foundry.Resolved) string {
	env := it.Spec.Environment
	if env == "" {
		env = "default"
	}
	res := it.Spec.Resource
	if res == "" {
		res = it.Host
	}
	return path.Join(safeSegment(env), safeSegment(res), safeSegment(it.Spec.Deployment))
}

var reUnsafeSegment = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeSegment makes s safe as a single path element. Inputs are already
// validated; this is defence in depth against path traversal.
func safeSegment(s string) string {
	s = reUnsafeSegment.ReplaceAllString(s, "-")
	s = strings.Trim(s, ".-")
	if s == "" {
		return "_"
	}
	return s
}

// Render produces every output file for the bundle, in memory.
func (r Renderer) Render(b inventory.Bundle) ([]File, error) {
	var files []File
	add := func(p string, k Kind, mode fs.FileMode, data []byte) {
		files = append(files, File{Path: p, Kind: k, Mode: mode, Data: data})
	}
	addJSON := func(p string, v any) error {
		data, err := marshalJSON(v)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		add(p, KindData, 0o644, data)
		return nil
	}

	if err := addJSON("manifest.json", b); err != nil {
		return nil, err
	}
	summary, err := r.exec("SUMMARY.md.tmpl", b)
	if err != nil {
		return nil, err
	}
	add("SUMMARY.md", KindDoc, 0o644, summary)

	runbook, err := r.exec("copilot-enterprise-runbook.md.tmpl", b)
	if err != nil {
		return nil, err
	}
	add("copilot-enterprise/custom-models-runbook.md", KindDoc, 0o644, runbook)
	if err := addJSON("copilot-enterprise/custom-models.json", struct {
		Provider string                      `json:"provider"`
		Keys     []inventory.EnterpriseGroup `json:"keys"`
		Skipped  []inventory.Skipped         `json:"skipped,omitempty"`
	}{"Microsoft Foundry", b.EnterpriseGroups, b.EnterpriseSkipped}); err != nil {
		return nil, err
	}

	perItem := []struct {
		tmpl string
		out  func(foundry.Resolved) string
		kind Kind
		mode fs.FileMode
	}{
		{"curl.sh.tmpl", func(foundry.Resolved) string { return "curl.sh" }, KindScript, 0o755},
		{"client.go.tmpl", func(foundry.Resolved) string { return "client.go" }, KindCode, 0o644},
		{"copilot-cli.env.tmpl", func(foundry.Resolved) string { return "copilot-cli.env" }, KindSnippet, 0o644},
		{"gh-aw-frontmatter.yml.tmpl", func(foundry.Resolved) string { return "gh-aw/frontmatter.yml" }, KindSnippet, 0o644},
		{"gh-aw-smoke.md.tmpl", func(it foundry.Resolved) string { return "gh-aw/" + it.GhAw.WorkflowSlug + ".md" }, KindWorkflow, 0o644},
	}
	for _, it := range b.Items {
		dir := ItemDir(it)
		if err := addJSON(path.Join(dir, "urls.json"), it); err != nil {
			return nil, err
		}
		data := newItemData(b.ToolVersion, it)
		for _, p := range perItem {
			out, err := r.exec(p.tmpl, data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", dir, err)
			}
			add(path.Join(dir, p.out(it)), p.kind, p.mode, out)
		}
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for i := 1; i < len(files); i++ {
		if files[i].Path == files[i-1].Path {
			return nil, fmt.Errorf("two outputs map to %s", files[i].Path)
		}
	}
	return files, nil
}

// marshalJSON encodes v indented, without HTML escaping (URLs contain '&'),
// with a trailing newline.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Write stores files under root atomically (temp file + rename) so a failed
// run never leaves a half-written config behind.
func Write(root string, files []File) error {
	for _, f := range files {
		dst, err := resolveUnder(root, f.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp, err := os.CreateTemp(filepath.Dir(dst), ".foundry-byok-*")
		if err != nil {
			return err
		}
		tmpName := tmp.Name()
		_, werr := tmp.Write(f.Data)
		cerr := tmp.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(tmpName, f.Mode)
		}
		if werr == nil {
			werr = os.Rename(tmpName, dst)
		}
		if werr != nil {
			os.Remove(tmpName)
			return fmt.Errorf("write %s: %w", dst, werr)
		}
	}
	return nil
}

// Diff compares rendered files with what is on disk under root and returns the
// relative paths that are missing or differ. Extra files on disk are ignored.
func Diff(root string, files []File) ([]string, error) {
	var changed []string
	for _, f := range files {
		dst, err := resolveUnder(root, f.Path)
		if err != nil {
			return nil, err
		}
		have, err := os.ReadFile(dst)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			changed = append(changed, f.Path+" (missing)")
		case err != nil:
			return nil, err
		case !bytes.Equal(have, f.Data):
			changed = append(changed, f.Path)
		}
	}
	return changed, nil
}

// resolveUnder joins rel onto root and refuses results that escape root.
func resolveUnder(root, rel string) (string, error) {
	dst := filepath.Join(root, filepath.FromSlash(rel))
	back, err := filepath.Rel(root, dst)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("refusing to write outside %s: %s", root, rel)
	}
	return dst, nil
}

var reGHExpr = regexp.MustCompile(`^\$\{\{\s*[A-Za-z_][A-Za-z0-9_.]*\s*\}\}$`)

// funcs is the template function map. Each quoting helper targets exactly one
// output syntax; use the one that matches the file being rendered.
var funcs = template.FuncMap{
	// shq single-quotes a value for POSIX shells.
	"shq": func(v any) string {
		return "'" + strings.ReplaceAll(fmt.Sprint(v), "'", `'\''`) + "'"
	},
	// shvar emits a shell parameter expansion: shvar "X" ":-" -> ${X:-}.
	"shvar": func(name, op string) string { return "${" + name + op + "}" },
	// yamlq emits a double-quoted YAML scalar, so values such as "5.4",
	// "true" or "null" can never be re-typed by a YAML parser.
	"yamlq": func(v any) string { return strconv.Quote(fmt.Sprint(v)) },
	// yamlv is yamlq, except a lone ${{ ... }} Actions expression is emitted
	// bare, the way GitHub's documentation writes them.
	"yamlv": func(v any) string {
		s := fmt.Sprint(v)
		if reGHExpr.MatchString(s) {
			return s
		}
		return strconv.Quote(s)
	},
	// jsonq emits a JSON string literal.
	"jsonq": func(v any) (string, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(fmt.Sprint(v)); err != nil {
			return "", err
		}
		return strings.TrimSuffix(buf.String(), "\n"), nil
	},
	// goq emits a Go string literal.
	"goq": func(v any) string { return strconv.Quote(fmt.Sprint(v)) },
	// runsOn renders comma-separated runner labels as a YAML scalar or flow
	// sequence: "a" -> "a"; "a, b" -> ["a", "b"].
	"runsOn": func(v string) string {
		labels := foundry.RunsOnLabels(v)
		if len(labels) == 1 {
			return strconv.Quote(labels[0])
		}
		quoted := make([]string, len(labels))
		for i, l := range labels {
			quoted[i] = strconv.Quote(l)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	},
	"inc":  func(i int) int { return i + 1 },
	"join": strings.Join,
}
