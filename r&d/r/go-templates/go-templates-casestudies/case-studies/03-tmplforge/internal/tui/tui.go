// Package tui implements `tmplforge sh`, a line-command language for
// building templates, in the manner of ed(1) and sam(1): one command per
// line, a letter and its arguments, "?" for anything it does not like.
//
// It reads with bufio and writes plain text. There is no raw mode, no
// cursor addressing and no termios, so it runs unchanged on Plan 9, in an
// acme win, over ssh, and from a script: `tmplforge sh < session.tf`.
package tui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
	"github.com/CoolGitOrg/tmplforge/internal/funcs"
	"github.com/CoolGitOrg/tmplforge/internal/scaffold"
)

// Prompt is printed before each command when standard input is a terminal.
const Prompt = "tf> "

// Session is one REPL: the template being edited and everything around it.
type Session struct {
	In     *bufio.Reader
	Out    io.Writer
	Prompt bool                // print Prompt before each command
	Color  bool                // ANSI colour on diagnostics
	Dir    string              // base for relative paths; default "."
	Getenv func(string) string // default os.Getenv
	Shell  []string            // argv prefix for !cmd; default shellArgv

	// Editor I/O for `e` with $EDITOR: the real terminal, not In/Out.
	EdIn    io.Reader
	EdOut   io.Writer
	Catalog *forge.Catalog

	name     string
	format   string
	left     string
	right    string
	lines    []string
	vars     []scaffold.Var
	fixture  map[string]any
	fixName  string
	dirty    bool
	paint    painter
	quitWarn bool
}

// New returns a session reading commands from in and writing to out.
func New(in io.Reader, out io.Writer) *Session {
	return &Session{In: bufio.NewReader(in), Out: out, format: "txt", name: "untitled", fixture: map[string]any{}}
}

// Run executes commands until q or end of input. It returns nil at end of
// input; errors from individual commands are reported as "?" and do not
// stop the session, except in a script (non-interactive) where the first
// error makes Run return it, so `tmplforge sh < x.tf` exits non-zero.
func (s *Session) Run() error {
	if s.Getenv == nil {
		s.Getenv = os.Getenv
	}
	if s.Dir == "" {
		s.Dir = "."
	}
	if s.Shell == nil {
		s.Shell = shellArgv
	}
	s.paint = painter(s.Color)
	var firstErr error
	for {
		if s.Prompt {
			fmt.Fprint(s.Out, s.paint.paint(cyan, Prompt))
		}
		line, ok := s.readLine()
		if !ok {
			return firstErr
		}
		quit, err := s.Exec(line)
		if err != nil {
			s.report(err)
			if firstErr == nil && !s.Prompt {
				firstErr = err
			}
		}
		if quit {
			return firstErr
		}
	}
}

// readLine returns the next input line without its newline.
func (s *Session) readLine() (string, bool) {
	l, err := s.In.ReadString('\n')
	if err != nil && l == "" {
		return "", false
	}
	return strings.TrimRight(l, "\r\n"), true
}

// errUnknown is ed's famous diagnostic.
var errUnknown = errors.New("")

func (s *Session) report(err error) {
	if errors.Is(err, errUnknown) {
		fmt.Fprintln(s.Out, "?")
		return
	}
	fmt.Fprintln(s.Out, s.paint.paint(red, "?"+" "+err.Error()))
}

func (s *Session) path(p string) string {
	if filepath.IsAbs(p) || s.Dir == "" {
		return p
	}
	return filepath.Join(s.Dir, p)
}

// Exec runs one command line. It reports whether the session should end.
func (s *Session) Exec(line string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return false, nil
	}
	if line[0] == '!' {
		return false, s.shell(line[1:])
	}
	cmd, arg := line[:1], strings.TrimSpace(line[1:])
	args := strings.Fields(arg)
	need := func(n int, usage string) error {
		if len(args) < n {
			return fmt.Errorf("usage: %s", usage)
		}
		return nil
	}
	switch cmd {
	case "q":
		if s.dirty && !s.quitWarn && s.Prompt {
			// ed's rule: warn once about unsaved work, quit on the second q.
			s.quitWarn = true
			return false, errors.New("template modified since last W; q again to quit")
		}
		return true, nil
	case "h":
		s.help()
	case "c":
		if err := need(1, "c NAME"); err != nil {
			return false, err
		}
		return false, s.load(args[0])
	case "f":
		if arg == "" {
			fmt.Fprintln(s.Out, s.format)
			return false, nil
		}
		if !forge.ValidFormat(arg) {
			return false, fmt.Errorf("format %q: want %s", arg, strings.Join(forge.Formats, "|"))
		}
		s.format = arg
	case "v":
		if err := need(2, "v NAME TYPE [DEFAULT]"); err != nil {
			return false, err
		}
		return false, s.declare(args[0], args[1], rest(arg, 2))
	case "V":
		s.listVars()
	case "e":
		return false, s.edit()
	case "a":
		s.lines = append(s.lines, s.readBlock()...)
		s.dirty = true
	case "i":
		// i N: insert before line N, ed style
		n := 1
		if arg != "" {
			var err error
			if n, err = strconv.Atoi(arg); err != nil || n < 1 || n > len(s.lines)+1 {
				return false, fmt.Errorf("line %q out of range 1..%d", arg, len(s.lines)+1)
			}
		}
		block := s.readBlock()
		s.lines = append(s.lines[:n-1], append(block, s.lines[n-1:]...)...)
		s.dirty = true
	case "d":
		return false, s.delete(arg)
	case "n":
		for i, l := range s.lines {
			fmt.Fprintf(s.Out, "%s\t%s\n", s.paint.paint(dim, strconv.Itoa(i+1)), l)
		}
	case "p":
		out, err := s.render(s.data())
		s.Out.Write(out)
		if len(out) > 0 && out[len(out)-1] != '\n' {
			fmt.Fprintln(s.Out)
		}
		return false, err
	case "D":
		if err := need(1, "D FILE"); err != nil {
			return false, err
		}
		fx, err := s.readFixture(arg)
		if err != nil {
			return false, err
		}
		s.fixture, s.fixName = fx, arg
		fmt.Fprintf(s.Out, "%s: %d %s\n", arg, len(fx), plural(len(fx), "key", "keys"))
	case "l":
		return false, s.lint()
	case "t":
		if err := need(2, "t FIX1 FIX2 [...]"); err != nil {
			return false, err
		}
		return false, s.test(args)
	case "w":
		if err := need(1, "w FILE"); err != nil {
			return false, err
		}
		out, err := s.render(s.data())
		if err != nil {
			return false, err
		}
		return false, s.write(arg, out)
	case "W":
		if err := need(1, "W FILE"); err != nil {
			return false, err
		}
		if err := s.write(arg, []byte(s.source())); err != nil {
			return false, err
		}
		s.dirty = false
	case "r":
		if err := need(1, "r FILE"); err != nil {
			return false, err
		}
		b, err := os.ReadFile(s.path(arg))
		if err != nil {
			return false, err
		}
		s.lines = append(s.lines, forge.Lines(string(b))...)
		s.dirty = true
		fmt.Fprintln(s.Out, len(b))
	case "x":
		if err := need(1, "x DIR"); err != nil {
			return false, err
		}
		return false, s.export(arg)
	case "g":
		if err := need(2, "g issueops NAME [DIR] | g cicd KIND [DIR]"); err != nil {
			return false, err
		}
		return false, s.generate(args)
	case "s":
		s.status()
	case "=":
		fmt.Fprintln(s.Out, len(s.lines))
	default:
		return false, errUnknown
	}
	return false, nil
}

// readBlock reads input lines until a line holding a single ".", as ed's
// a, i and c commands do. End of input also ends the block.
func (s *Session) readBlock() []string {
	var block []string
	for {
		l, ok := s.readLine()
		if !ok || l == "." {
			return block
		}
		block = append(block, l)
	}
}

func (s *Session) source() string {
	if len(s.lines) == 0 {
		return ""
	}
	return strings.Join(s.lines, "\n") + "\n"
}

func (s *Session) options() forge.Options {
	return forge.Options{Name: s.name, Format: s.format, Left: s.left, Right: s.right, Funcs: funcs.All()}
}

// render runs in-process with the full FuncMap: the TUI user is the
// operator and is trusted (see docs/SECURITY.md). A runaway template is
// stopped the way any runaway command is, with an interrupt.
func (s *Session) render(data map[string]any) ([]byte, error) {
	out, err := forge.Render(s.source(), data, s.options())
	if err == nil {
		err = forge.CheckOutput(s.format, out)
	}
	return out, err
}

// data is the fixture with declared-variable defaults filling any gaps.
func (s *Session) data() map[string]any { return scaffold.Merge(s.vars, s.fixture) }

func (s *Session) declare(name, typ, def string) error {
	v := scaffold.Var{Name: name, Type: typ, Default: def}
	if err := scaffold.ValidVar(v); err != nil {
		return err
	}
	for i := range s.vars {
		if s.vars[i].Name == name {
			s.vars[i] = v
			return nil
		}
	}
	s.vars = append(s.vars, v)
	return nil
}

func (s *Session) listVars() {
	for _, v := range s.vars {
		if v.Default != "" {
			fmt.Fprintf(s.Out, "%s\t%s\t%s\n", v.Name, v.Type, v.Default)
		} else {
			fmt.Fprintf(s.Out, "%s\t%s\n", v.Name, v.Type)
		}
	}
}

func (s *Session) load(name string) error {
	if s.Catalog == nil {
		c, err := forge.LoadCatalog()
		if err != nil {
			return err
		}
		s.Catalog = c
	}
	e, ok := s.Catalog.Lookup(name)
	if !ok {
		return fmt.Errorf("no catalog entry %q (try: ! tmplforge catalog)", name)
	}
	src, err := s.Catalog.Source(e)
	if err != nil {
		return err
	}
	fx, err := s.Catalog.Fixture(e)
	if err != nil {
		return err
	}
	o, err := e.Options()
	if err != nil {
		return err
	}
	s.name, s.format, s.left, s.right = e.Name, e.Format, o.Left, o.Right
	s.lines = forge.Lines(src)
	s.fixture, s.fixName = fx, e.Fixture
	s.dirty = false
	// Declare the fixture's top-level keys, with the catalog values as
	// defaults: l can cross-check, and a later D FILE overrides only the
	// keys it sets, the rest falling back to the catalog's example data.
	s.vars = nil
	keys := make([]string, 0, len(fx))
	for k := range fx {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.vars = append(s.vars, varOf(k, fx[k]))
	}
	d := ""
	if e.Delims != "" {
		d = ", delims " + e.Delims
	}
	fmt.Fprintf(s.Out, "%s: %d lines, %s%s, %d vars\n", e.Name, len(s.lines), e.Format, d, len(s.vars))
	return nil
}

// varOf declares name with v's type and v as its default.
func varOf(name string, v any) scaffold.Var {
	switch x := v.(type) {
	case int:
		return scaffold.Var{Name: name, Type: "int", Default: strconv.Itoa(x)}
	case bool:
		return scaffold.Var{Name: name, Type: "bool", Default: strconv.FormatBool(x)}
	case string:
		return scaffold.Var{Name: name, Type: "string", Default: x}
	case []any, map[string]any:
		t := "list"
		if _, ok := x.(map[string]any); ok {
			t = "map"
		}
		b, _ := json.Marshal(x)
		return scaffold.Var{Name: name, Type: t, Default: string(b)}
	}
	// float or null: keep the text form
	if v == nil {
		return scaffold.Var{Name: name, Type: "string"}
	}
	return scaffold.Var{Name: name, Type: "string", Default: fmt.Sprint(v)}
}

func (s *Session) edit() error {
	ed := s.Getenv("EDITOR")
	if ed == "" {
		// ed style: the new text replaces the template, ended by "."
		s.lines = s.readBlock()
		s.dirty = true
		return nil
	}
	f, err := os.CreateTemp("", "tmplforge-*"+forge.Ext[s.format]+".tmpl")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(s.source()); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	argv := append(strings.Fields(ed), f.Name())
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s.EdIn, s.EdOut, s.EdOut
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	if cmd.Stdout == nil {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", argv[0], err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		return err
	}
	s.lines = forge.Lines(string(b))
	s.dirty = true
	fmt.Fprintln(s.Out, len(b))
	return nil
}

func (s *Session) delete(arg string) error {
	if arg == "" {
		return errors.New("usage: d N[,M]")
	}
	a, b, isRange := strings.Cut(arg, ",")
	n, err := strconv.Atoi(strings.TrimSpace(a))
	m := n
	if err == nil && isRange {
		if strings.TrimSpace(b) == "$" {
			m = len(s.lines)
		} else {
			m, err = strconv.Atoi(strings.TrimSpace(b))
		}
	}
	if err != nil || n < 1 || m < n || m > len(s.lines) {
		return fmt.Errorf("line range %q out of 1..%d", arg, len(s.lines))
	}
	s.lines = append(s.lines[:n-1], s.lines[m:]...)
	s.dirty = true
	return nil
}

func (s *Session) readFixture(name string) (map[string]any, error) {
	f, err := os.Open(s.path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return forge.LoadFixture(f)
}

func (s *Session) lint() error {
	var declared []string
	for _, v := range s.vars {
		declared = append(declared, v.Name)
	}
	if declared == nil {
		declared = []string{}
	}
	fs := forge.Lint(s.source(), forge.LintOptions{Options: s.options(), Declared: declared})
	for _, f := range fs {
		msg := f.Format(s.name)
		if f.Sev == forge.SevError {
			msg = s.paint.paint(red, msg)
		} else {
			msg = s.paint.paint(yellow, msg)
		}
		fmt.Fprintln(s.Out, msg)
	}
	if forge.HasError(fs) {
		return errors.New("lint failed")
	}
	if len(fs) == 0 {
		fmt.Fprintln(s.Out, s.paint.paint(green, "ok"))
	}
	return nil
}

func (s *Session) test(fixtures []string) error {
	outs := make([]string, len(fixtures))
	for i, name := range fixtures {
		fx, err := s.readFixture(name)
		if err != nil {
			return err
		}
		d := s.data()
		for k, v := range fx {
			d[k] = v
		}
		b, err := s.render(d)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		outs[i] = string(b)
	}
	for i := 1; i < len(outs); i++ {
		ops := forge.Diff(forge.Lines(outs[i-1]), forge.Lines(outs[i]))
		if !forge.Changed(ops) {
			fmt.Fprintf(s.Out, "%s == %s\n", fixtures[i-1], fixtures[i])
			continue
		}
		for _, l := range forge.Lines(forge.Unified(fixtures[i-1], fixtures[i], ops, 2)) {
			switch {
			case strings.HasPrefix(l, "@@"):
				l = s.paint.paint(cyan, l)
			case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"):
				l = s.paint.paint(bold, l)
			case strings.HasPrefix(l, "-"):
				l = s.paint.paint(red, l)
			case strings.HasPrefix(l, "+"):
				l = s.paint.paint(green, l)
			}
			fmt.Fprintln(s.Out, l)
		}
	}
	return nil
}

func (s *Session) write(name string, b []byte) error {
	p := s.path(name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return err
	}
	fmt.Fprintln(s.Out, len(b)) // ed prints the byte count
	return nil
}

func (s *Session) export(dir string) error {
	fx, err := json.MarshalIndent(s.data(), "", "  ")
	if err != nil {
		return err
	}
	files, err := forge.Export(s.path(dir), forge.Bundle{
		Name: s.name, Template: s.source(), Fixture: append(fx, '\n'), Options: s.options(),
	})
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Fprintf(s.Out, "%s/%s\n", dir, f.Name)
	}
	return nil
}

func (s *Session) generate(args []string) error {
	dir := "."
	if len(args) > 2 {
		dir = args[2]
	}
	var files []forge.File
	switch args[0] {
	case "issueops":
		fields := scaffold.FieldsFromVars(s.vars)
		fs, err := scaffold.IssueOps(args[1], fields)
		if err != nil {
			return err
		}
		files = fs
	case "cicd":
		f, err := scaffold.Workflow(scaffold.CICD{Kind: args[1], Binary: "tmplforge", Inputs: scaffold.InputsFromVars(s.vars)})
		if err != nil {
			return err
		}
		files = []forge.File{f}
	default:
		return fmt.Errorf("g %s: want issueops or cicd", args[0])
	}
	return writeFiles(s.Out, s.path(dir), dir, files)
}

// writeFiles writes generated files under root, printing each path.
func writeFiles(w io.Writer, root, shown string, files []forge.File) error {
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			return err
		}
		fmt.Fprintln(w, filepath.ToSlash(filepath.Join(shown, f.Name)))
	}
	return nil
}

// shell runs cmd with the rendered output on its standard input. Trust
// model: whoever types at this prompt already has a shell; ! is a
// convenience, not an escalation. The web GUI has no equivalent.
func (s *Session) shell(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return errors.New("usage: !CMD")
	}
	out, err := s.render(s.data())
	if err != nil {
		return err
	}
	argv := append(append([]string{}, s.Shell...), cmd)
	c := exec.Command(argv[0], argv[1:]...)
	c.Dir = s.Dir
	c.Stdin = bytes.NewReader(out)
	c.Stdout, c.Stderr = s.Out, s.Out
	err = c.Run()
	fmt.Fprintln(s.Out, "!")
	return err
}

func (s *Session) status() {
	fx := s.fixName
	if fx == "" {
		fx = "(none)"
	}
	d := ""
	if s.left != "" {
		d = s.left + "," + s.right
	} else {
		d = "{{,}}"
	}
	mod := ""
	if s.dirty {
		mod = " (modified)"
	}
	fmt.Fprintf(s.Out, "name %s%s\nformat %s\ndelims %s\nlines %d\nvars %d\nfixture %s\n", s.name, mod, s.format, d, len(s.lines), len(s.vars), fx)
}

// rest returns arg after its first n space-separated fields, with the
// remainder's internal spacing kept exactly as typed.
func rest(arg string, n int) string {
	for i := 0; i < n; i++ {
		arg = strings.TrimLeft(arg, " \t")
		j := strings.IndexAny(arg, " \t")
		if j < 0 {
			return ""
		}
		arg = arg[j:]
	}
	return strings.TrimSpace(arg)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Help is the command summary printed by h.
const Help = `c NAME              load catalog entry (template, format, delims, fixture)
f [FORMAT]          show or set output format: html md json txt yaml
v NAME TYPE [DEF]   declare variable; TYPE is string int bool list map
V                   list declared variables
e                   edit template: $EDITOR if set, else read lines until "."
a                   append lines until "."
i [N]               insert lines before line N until "."
d N[,M]             delete lines N..M (M may be $)
n                   print template with line numbers
=                   print number of template lines
r FILE              append FILE to the template
p                   preview: render against fixture + declared defaults
D FILE              load fixture JSON
l                   lint: parse errors with lines; fields vs declared vars
t FIX1 FIX2 [...]   render each fixture, diff consecutive outputs
w FILE              write rendered output
W FILE              write template source
x DIR               export bundle: template fixture output run.go Makefile SHA256SUMS
g issueops NAME [DIR]  generate issue form + parse workflow + Go parser from vars
g cicd KIND [DIR]   generate workflow: build dispatch release repository_dispatch tag
s                   status
!CMD                run CMD with rendered output on stdin
h                   this help
q                   quit
`

func (s *Session) help() { fmt.Fprint(s.Out, Help) }
