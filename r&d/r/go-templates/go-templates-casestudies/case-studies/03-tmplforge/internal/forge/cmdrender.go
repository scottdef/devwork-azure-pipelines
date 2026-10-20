package forge

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

// Exit statuses of `tmplforge render`. Go's runtime uses 2 for a fatal
// error or unrecovered panic, so usage errors use 64 (sysexits EX_USAGE)
// to keep "the child crashed" distinguishable.
const (
	ExitOK          = 0
	ExitRenderError = 1
	ExitTimeout     = 3
	ExitOutputLimit = 4
	ExitUsage       = 64
)

// The child's first stderr line starts with one of these tags. The parent
// classifies failures by tag, not by exit status, because Plan 9 exit
// statuses are strings and do not round-trip small integers. No tag means
// the child died without saying why: a crash.
const (
	tagRender  = "sandbox:render: "
	tagTimeout = "sandbox:timeout: "
	tagLimit   = "sandbox:limit: "
	tagUsage   = "sandbox:usage: "
)

// sandboxMaxStack caps goroutine stacks in the child. Deep template
// recursion then dies fast as a runtime fatal error instead of first
// eating the 1 GiB default; the parent reports it as a crash.
const sandboxMaxStack = 32 << 20

// RenderCommand implements `tmplforge render`. It is in this package, not
// in cmd/, so tests of any package can re-exec their own test binary as a
// sandbox child (see SandboxChildMain).
func RenderCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tfile := fs.String("t", "", "template `file` (- for stdin)")
	dfile := fs.String("d", "", "fixture JSON `file` (- for stdin)")
	format := fs.String("f", "", "output `format`: html|md|json|txt|yaml (default from extension, else txt)")
	delims := fs.String("delims", "", "action delimiters `L,R`, e.g. [[,]]")
	sandbox := fs.Bool("sandbox", false, "untrusted mode: Safe funcs, 64KiB template cap, output cap, timeout")
	timeout := fs.Duration("timeout", 2*time.Second, "render time limit (with -sandbox)")
	max := fs.Int64("max", MaxOutput, "output byte limit (with -sandbox)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tmplforge render -t file [-d fixture] [-f format] [-delims L,R] [-sandbox] [-timeout 2s] [-max bytes]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return ExitUsage
	}
	usage := func(msg string) int {
		if *sandbox {
			fmt.Fprint(stderr, tagUsage)
		}
		fmt.Fprintln(stderr, "render:", msg)
		return ExitUsage
	}
	if *tfile == "" || fs.NArg() > 0 {
		fs.Usage()
		return ExitUsage
	}
	if *tfile == "-" && *dfile == "-" {
		return usage("-t and -d cannot both be stdin")
	}
	if *format == "" {
		*format = FormatFromName(*tfile)
	}
	l, r, err := ParseDelims(*delims)
	if err != nil {
		return usage(err.Error())
	}
	o := Options{Name: nameOf(*tfile), Format: *format, Left: l, Right: r}
	if *sandbox {
		if *max <= 0 || *max > MaxOutput {
			*max = MaxOutput
		}
		o.Funcs = funcs.Safe()
		o.MaxTemplate = MaxTemplate
		o.MaxOutput = *max
		debug.SetMaxStack(sandboxMaxStack)
		// Execute cannot be cancelled, so the child enforces its own
		// deadline by exiting. The parent's kill is the backstop.
		t := time.AfterFunc(*timeout, func() {
			fmt.Fprintf(stderr, "%srender exceeded %v\n", tagTimeout, *timeout)
			os.Exit(ExitTimeout)
		})
		defer t.Stop()
	}
	src, err := readInput(*tfile, stdin, MaxTemplate, *sandbox)
	if err != nil {
		return fail(stderr, *sandbox, err)
	}
	var data map[string]any = map[string]any{}
	if *dfile != "" {
		var f io.Reader
		if *dfile == "-" {
			f = stdin
		} else {
			fh, err := os.Open(*dfile)
			if err != nil {
				return fail(stderr, *sandbox, err)
			}
			defer fh.Close()
			f = fh
		}
		if data, err = LoadFixture(f); err != nil {
			return fail(stderr, *sandbox, err)
		}
	}
	out, err := Render(string(src), data, o)
	if err == nil {
		err = CheckOutput(o.Format, out)
	}
	if err != nil {
		return fail(stderr, *sandbox, err)
	}
	if _, err := stdout.Write(out); err != nil {
		fmt.Fprintln(stderr, "render:", err)
		return ExitRenderError
	}
	return ExitOK
}

func fail(stderr io.Writer, sandbox bool, err error) int {
	code, tag := ExitRenderError, tagRender
	switch {
	case errors.Is(err, ErrOutputLimit):
		code, tag = ExitOutputLimit, tagLimit
	}
	if sandbox {
		fmt.Fprint(stderr, tag)
	} else {
		fmt.Fprint(stderr, "render: ")
	}
	fmt.Fprintln(stderr, strings.ReplaceAll(err.Error(), "\n", " "))
	return code
}

func readInput(name string, stdin io.Reader, max int, capped bool) ([]byte, error) {
	var r io.Reader
	if name == "-" {
		r = stdin
	} else {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	if !capped {
		return io.ReadAll(r)
	}
	b, err := io.ReadAll(io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > max {
		return nil, fmt.Errorf("%w: more than %d bytes", ErrTemplateLimit, max)
	}
	return b, nil
}

func nameOf(file string) string {
	if file == "-" || file == "" {
		return "stdin"
	}
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		return file[i+1:]
	}
	return file
}

// FormatFromName guesses a format from a file name: "x.html.tmpl" is html,
// "notes.md" is md, anything unknown is txt.
func FormatFromName(name string) string {
	n := strings.TrimSuffix(strings.TrimSuffix(name, ".tmpl"), ".tpl")
	switch {
	case strings.HasSuffix(n, ".html"), strings.HasSuffix(n, ".htm"), strings.HasSuffix(n, ".svg"):
		return "html"
	case strings.HasSuffix(n, ".md"):
		return "md"
	case strings.HasSuffix(n, ".json"):
		return "json"
	case strings.HasSuffix(n, ".yml"), strings.HasSuffix(n, ".yaml"):
		return "yaml"
	}
	return "txt"
}

// classify turns a sandbox child's stderr into an error.
func classify(stderr string) (error, bool) {
	line, _, _ := strings.Cut(stderr, "\n")
	switch {
	case strings.HasPrefix(line, tagRender):
		msg := strings.TrimPrefix(line, tagRender)
		return &RenderError{Msg: msg, Line: ErrorLine(errors.New(msg))}, true
	case strings.HasPrefix(line, tagTimeout):
		return fmt.Errorf("%w: %s", ErrTimeout, strings.TrimPrefix(line, tagTimeout)), true
	case strings.HasPrefix(line, tagLimit):
		return fmt.Errorf("%w: %s", ErrOutputLimit, strings.TrimPrefix(line, tagLimit)), true
	case strings.HasPrefix(line, tagUsage):
		return fmt.Errorf("sandbox: child usage error: %s", strings.TrimPrefix(line, tagUsage)), true
	}
	return nil, false
}

// SandboxChildMain lets a test binary stand in for tmplforge as the
// sandbox child. Call it first thing in TestMain:
//
//	func TestMain(m *testing.M) { forge.SandboxChildMain(); os.Exit(m.Run()) }
//
// When the process was started by Sandbox (TMPLFORGE_SANDBOX=1 and argv
// begins with "render"), it runs the render command and exits.
func SandboxChildMain() {
	if os.Getenv("TMPLFORGE_SANDBOX") != "1" || len(os.Args) < 2 || os.Args[1] != "render" {
		return
	}
	os.Exit(RenderCommand(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
}
