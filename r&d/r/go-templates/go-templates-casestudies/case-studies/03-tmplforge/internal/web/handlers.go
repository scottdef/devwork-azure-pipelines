package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
	"github.com/CoolGitOrg/tmplforge/internal/funcs"
	"github.com/CoolGitOrg/tmplforge/internal/scaffold"
)

// page renders a full page: layout.html around the page's "content".
func (s *Server) page(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["Version"] = s.cfg.Version
	var b bytes.Buffer
	if err := s.pages[name].ExecuteTemplate(&b, "layout.html", data); err != nil {
		s.cfg.Log.Printf("page %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}

// part renders a named fragment from items.html, for htmx swaps.
func (s *Server) part(w http.ResponseWriter, code int, name string, data any) {
	var b bytes.Buffer
	if err := s.parts.ExecuteTemplate(&b, name, data); err != nil {
		s.cfg.Log.Printf("part %s: %v", name, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write(b.Bytes())
}

// formError reports a bad request as a fragment. htmx does not swap 4xx
// responses by default, so form errors are 200 with an error fragment.
func (s *Server) formError(w http.ResponseWriter, err error) {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	s.part(w, http.StatusOK, "error", err.Error())
}

func (s *Server) parseForm(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "bad form: "+err.Error(), http.StatusBadRequest)
		}
		return false
	}
	return true
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.page(w, "index.html", map[string]any{"Title": "Catalog", "Entries": s.catalog.Entries})
}

// builderState is what the builder page and its fragments are drawn from.
type builderState struct {
	Name     string
	Format   string
	Delims   string
	Template string
	Fixture  string
	Vars     []scaffold.Var
	Formats  []string
	Types    []string
	Desc     string
}

func (s *Server) handleBuilder(w http.ResponseWriter, r *http.Request) {
	st := builderState{Name: "untitled", Format: "txt", Fixture: "{}", Formats: forge.Formats, Types: scaffold.VarTypes}
	if f := r.URL.Query().Get("format"); forge.ValidFormat(f) {
		st.Format = f
	}
	if name := r.URL.Query().Get("name"); name != "" {
		e, ok := s.catalog.Lookup(name)
		if !ok {
			http.Error(w, "no such catalog entry", http.StatusNotFound)
			return
		}
		src, err := s.catalog.Source(e)
		if err != nil {
			http.Error(w, "catalog read failed", http.StatusInternalServerError)
			return
		}
		fx, err := s.catalog.FixtureRaw(e)
		if err != nil {
			http.Error(w, "catalog read failed", http.StatusInternalServerError)
			return
		}
		data, _ := s.catalog.Fixture(e)
		st.Name, st.Format, st.Delims, st.Template, st.Fixture, st.Desc = e.Name, e.Format, e.Delims, src, string(fx), e.Description
		st.Vars = varsOf(data)
	}
	s.page(w, "builder.html", map[string]any{"Title": "Builder: " + st.Name, "S": st})
}

func varsOf(data map[string]any) []scaffold.Var {
	var vs []scaffold.Var
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t := "string"
		switch data[k].(type) {
		case int, float64:
			t = "int"
		case bool:
			t = "bool"
		case []any:
			t = "list"
		case map[string]any:
			t = "map"
		}
		vs = append(vs, scaffold.Var{Name: k, Type: t})
	}
	return vs
}

// formVars reads the builder's repeated var rows: var_name, var_type,
// var_default, aligned by position. Blank names are skipped.
func formVars(r *http.Request) ([]scaffold.Var, error) {
	names, types, defs := r.PostForm["var_name"], r.PostForm["var_type"], r.PostForm["var_default"]
	if len(names) > 256 {
		return nil, errors.New("too many variables")
	}
	var out []scaffold.Var
	for i, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		v := scaffold.Var{Name: n, Type: "string"}
		if i < len(types) {
			v.Type = types[i]
		}
		if i < len(defs) {
			v.Default = defs[i]
		}
		if err := scaffold.ValidVar(v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// handleVar returns one new variable row for the wizard's var table
// (hx-post="/builder/var" hx-target="#vars" hx-swap="beforeend").
func (s *Server) handleVar(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	v := scaffold.Var{Name: strings.TrimSpace(r.PostForm.Get("new_name")), Type: r.PostForm.Get("new_type"), Default: r.PostForm.Get("new_default")}
	if v.Type == "" {
		v.Type = "string"
	}
	if err := scaffold.ValidVar(v); err != nil {
		s.part(w, http.StatusOK, "var-error", err.Error())
		return
	}
	s.part(w, http.StatusOK, "var-row", map[string]any{"V": v, "Types": scaffold.VarTypes})
}

// request is the common form of render/lint/vars/test/export posts.
type request struct {
	Name     string
	Template string
	Fixture  string
	Opts     forge.Options
	Vars     []scaffold.Var
}

func (s *Server) readRequest(w http.ResponseWriter, r *http.Request) (*request, bool) {
	if !s.parseForm(w, r) {
		return nil, false
	}
	f := r.PostForm
	q := &request{Name: f.Get("name"), Template: f.Get("template"), Fixture: f.Get("fixture")}
	if q.Name == "" {
		q.Name = "untitled"
	}
	if len(q.Template) > forge.MaxTemplate {
		s.formError(w, fmt.Errorf("%w: %d bytes > %d", forge.ErrTemplateLimit, len(q.Template), forge.MaxTemplate))
		return nil, false
	}
	format := f.Get("format")
	if format == "" {
		format = "txt"
	}
	if !forge.ValidFormat(format) {
		s.formError(w, fmt.Errorf("unknown format %q", format))
		return nil, false
	}
	l, rr, err := forge.ParseDelims(f.Get("delims"))
	if err != nil {
		s.formError(w, err)
		return nil, false
	}
	q.Opts = forge.Options{Name: forge.SafeName(q.Name), Format: format, Left: l, Right: rr, Funcs: funcs.Safe(), MaxTemplate: forge.MaxTemplate}
	if q.Vars, err = formVars(r); err != nil {
		s.formError(w, err)
		return nil, false
	}
	return q, true
}

// fixtureJSON merges declared defaults and the posted fixture into the
// JSON document handed to the sandbox.
func fixtureJSON(vars []scaffold.Var, raw string) ([]byte, map[string]any, error) {
	fx := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		var err error
		if fx, err = forge.LoadFixture(strings.NewReader(raw)); err != nil {
			return nil, nil, err
		}
	}
	d := scaffold.Merge(vars, fx)
	b, err := json.Marshal(d)
	return b, d, err
}

// renderResult is the preview fragment's data.
type renderResult struct {
	Out     string
	Lines   []srcLine
	Err     string
	ErrLine int
	Bytes   int
	Took    string
}

type srcLine struct {
	N    int
	Text string
	Bad  bool
}

// handleRender is the live preview: sandboxed render, output escaped by
// html/template inside <pre>, and on error the template source with the
// failing line highlighted.
func (s *Server) handleRender(w http.ResponseWriter, r *http.Request) {
	q, ok := s.readRequest(w, r)
	if !ok {
		return
	}
	fx, _, err := fixtureJSON(q.Vars, q.Fixture)
	if err != nil {
		s.part(w, http.StatusOK, "preview", renderResult{Err: err.Error()})
		return
	}
	start := time.Now()
	out, err := s.render(r.Context(), q.Template, fx, q.Opts)
	res := renderResult{Out: string(out), Bytes: len(out), Took: time.Since(start).Round(100 * time.Microsecond).String()}
	if err != nil {
		res.Err = err.Error()
		var re *forge.RenderError
		if errors.As(err, &re) {
			res.Err = re.Msg
			res.ErrLine = re.Line
		}
		if res.ErrLine > 0 {
			for i, l := range forge.Lines(q.Template) {
				res.Lines = append(res.Lines, srcLine{N: i + 1, Text: l, Bad: i+1 == res.ErrLine})
			}
		}
		if errors.Is(err, errBusy) {
			w.Header().Set("Retry-After", "1")
			s.part(w, http.StatusServiceUnavailable, "preview", res)
			return
		}
	}
	s.part(w, http.StatusOK, "preview", res)
}

func (s *Server) handleLint(w http.ResponseWriter, r *http.Request) {
	q, ok := s.readRequest(w, r)
	if !ok {
		return
	}
	var declared []string
	if len(q.Vars) > 0 {
		for _, v := range q.Vars {
			declared = append(declared, v.Name)
		}
	}
	var fixture map[string]any
	if strings.TrimSpace(q.Fixture) != "" {
		fixture, _ = forge.LoadFixture(strings.NewReader(q.Fixture))
	}
	fs := forge.Lint(q.Template, forge.LintOptions{Options: q.Opts, Declared: declared, Fixture: fixture})
	type row struct {
		Text  string
		Error bool
	}
	var rows []row
	for _, f := range fs {
		rows = append(rows, row{f.Format(q.Opts.Name), f.Sev == forge.SevError})
	}
	s.part(w, http.StatusOK, "lint", rows)
}

// handleVars returns the JSON skeleton fixture for the posted template,
// as a fragment that replaces the fixture textarea.
func (s *Server) handleVars(w http.ResponseWriter, r *http.Request) {
	q, ok := s.readRequest(w, r)
	if !ok {
		return
	}
	v, err := forge.Vars(q.Template, q.Opts.Left, q.Opts.Right)
	if err != nil {
		s.formError(w, err)
		return
	}
	b, err := json.MarshalIndent(v.Skeleton, "", "  ")
	if err != nil {
		s.formError(w, err)
		return
	}
	if r.URL.Query().Get("raw") == "1" {
		w.Header().Set("Content-Type", "application/json")
		w.Write(append(b, '\n'))
		return
	}
	s.part(w, http.StatusOK, "fixture", map[string]any{"JSON": string(b), "Refs": v.Refs})
}

func (s *Server) handleTestPage(w http.ResponseWriter, r *http.Request) {
	st := map[string]any{"Title": "Test", "Formats": forge.Formats, "Template": "", "Format": "txt", "A": "{}", "B": "{}", "Delims": ""}
	if name := r.URL.Query().Get("name"); name != "" {
		if e, ok := s.catalog.Lookup(name); ok {
			src, _ := s.catalog.Source(e)
			fx, _ := s.catalog.FixtureRaw(e)
			st["Template"], st["Format"], st["A"], st["B"], st["Delims"], st["Name"] = src, e.Format, string(fx), string(fx), e.Delims, e.Name
		}
	}
	s.page(w, "test.html", st)
}

// handleTest renders against fixture_a and fixture_b and returns the line
// diff of the two outputs.
func (s *Server) handleTest(w http.ResponseWriter, r *http.Request) {
	q, ok := s.readRequest(w, r)
	if !ok {
		return
	}
	var outs [2]string
	for i, k := range []string{"fixture_a", "fixture_b"} {
		fx, _, err := fixtureJSON(q.Vars, r.PostForm.Get(k))
		if err != nil {
			s.formError(w, fmt.Errorf("%s: %w", k, err))
			return
		}
		out, err := s.render(r.Context(), q.Template, fx, q.Opts)
		if err != nil {
			s.formError(w, fmt.Errorf("%s: %w", k, err))
			return
		}
		outs[i] = string(out)
	}
	ops := forge.Diff(forge.Lines(outs[0]), forge.Lines(outs[1]))
	type row struct {
		Kind string
		Text string
	}
	var rows []row
	adds, dels := 0, 0
	for _, o := range ops {
		k := "same"
		switch o.Kind {
		case '+':
			k = "add"
			adds++
		case '-':
			k = "del"
			dels++
		}
		rows = append(rows, row{k, string(o.Kind) + " " + o.Text})
	}
	s.part(w, http.StatusOK, "diff", map[string]any{"Rows": rows, "Adds": adds, "Dels": dels, "Same": !forge.Changed(ops)})
}

// handleExport returns the bundle as a reproducible tar.gz download. The
// output inside is rendered by the sandbox, never in-process.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	q, ok := s.readRequest(w, r)
	if !ok {
		return
	}
	fx, _, err := fixtureJSON(q.Vars, q.Fixture)
	if err != nil {
		s.formError(w, err)
		return
	}
	out, err := s.render(r.Context(), q.Template, fx, q.Opts)
	if err != nil {
		s.formError(w, err)
		return
	}
	pretty := &bytes.Buffer{}
	json.Indent(pretty, fx, "", "  ")
	pretty.WriteByte('\n')
	files, err := forge.ExportFiles(forge.Bundle{Name: q.Name, Template: q.Template, Fixture: pretty.Bytes(), Output: out, Options: q.Opts})
	if err != nil {
		s.formError(w, err)
		return
	}
	name := forge.SafeName(q.Name)
	var b bytes.Buffer
	if err := forge.WriteTarGz(&b, name, files); err != nil {
		http.Error(w, "export failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tar.gz"`, name))
	w.Header().Set("Content-Length", strconv.Itoa(b.Len()))
	w.Write(b.Bytes())
}

func (s *Server) handleNewPage(w http.ResponseWriter, r *http.Request) {
	s.page(w, "new.html", map[string]any{"Title": "Generate", "Kinds": scaffold.Kinds, "FieldTypes": scaffold.FieldTypes, "InputTypes": scaffold.InputTypes})
}

type genFile struct {
	Name string
	Data string
}

func genFiles(fs []forge.File) []genFile {
	out := make([]genFile, len(fs))
	for i, f := range fs {
		out[i] = genFile{f.Name, string(f.Data)}
	}
	return out
}

func (s *Server) handleNewIssueOps(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	fields, err := scaffold.ParseFields(r.PostForm.Get("fields"))
	if err != nil {
		s.formError(w, err)
		return
	}
	files, err := scaffold.IssueOps(strings.TrimSpace(r.PostForm.Get("name")), fields)
	if err != nil {
		s.formError(w, err)
		return
	}
	s.part(w, http.StatusOK, "files", genFiles(files))
}

func (s *Server) handleNewCICD(w http.ResponseWriter, r *http.Request) {
	if !s.parseForm(w, r) {
		return
	}
	inputs, err := scaffold.ParseInputs(r.PostForm.Get("inputs"))
	if err != nil {
		s.formError(w, err)
		return
	}
	c := scaffold.CICD{
		Kind:   r.PostForm.Get("kind"),
		Name:   strings.TrimSpace(r.PostForm.Get("name")),
		Binary: strings.TrimSpace(r.PostForm.Get("binary")),
		Inputs: inputs,
	}
	if t := strings.TrimSpace(r.PostForm.Get("types")); t != "" {
		c.Types = strings.Fields(strings.ReplaceAll(t, ",", " "))
	}
	f, err := scaffold.Workflow(c)
	if err != nil {
		s.formError(w, err)
		return
	}
	s.part(w, http.StatusOK, "files", genFiles([]forge.File{f}))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.metrics.WriteProm(w, s.cfg.Version, runtime.Version())
}

// chartWindow picks a bucket width from ?w= (Go duration), default 10s.
func chartWindow(r *http.Request) time.Duration {
	if d, err := time.ParseDuration(r.URL.Query().Get("w")); err == nil && d >= time.Second && d <= time.Hour {
		return d
	}
	return 10 * time.Second
}

// handleChart is the latency page: the go-echarts chart element, the
// chart's script tags (echarts.min.js from /static/echarts/, then
// /chart.js), and the same numbers as a table, which is what a browser
// without echarts.min.js, or a reader of the page source, gets.
func (s *Server) handleChart(w http.ResponseWriter, r *http.Request) {
	win := chartWindow(r)
	pts := Series(s.metrics.Samples(), win)
	line := latencyChart(pts)
	assets := chartAssets(line)
	snip := line.RenderSnippet()
	s.page(w, "chart.html", map[string]any{
		"Title":   "Latency",
		"Element": template.HTML(snip.Element), // go-echarts' own html/template output
		"Assets":  assets,
		"Points":  pts,
		"Window":  win.String(),
	})
}

func (s *Server) handleChartJS(w http.ResponseWriter, r *http.Request) {
	pts := Series(s.metrics.Samples(), chartWindow(r))
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, chartJS(latencyChart(pts)))
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, "ok")
}
