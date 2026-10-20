// Command tmplforge builds, tests and serves Go templates.
//
//	tmplforge sh [-c name] [script.tf]       line-command TUI (ed/sam style)
//	tmplforge serve [-a addr] [-token t]      HTML GUI (html/template + htmx)
//	tmplforge render -t file [-d fixture]     one-shot render
//	tmplforge lint -t file                    file:line: diagnostics
//	tmplforge vars -t file                    JSON skeleton fixture
//	tmplforge test -t file fix1 fix2 ...      render each, diff consecutive outputs
//	tmplforge export -t file -d fix -o dir    reproducible bundle
//	tmplforge catalog [-json]                 list starter templates
//	tmplforge new issueops NAME [-fields ...] IssueOps kit
//	tmplforge new cicd KIND [-inputs ...]     GitHub Actions workflow
//	tmplforge version
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
	"github.com/CoolGitOrg/tmplforge/internal/scaffold"
	"github.com/CoolGitOrg/tmplforge/internal/tui"
	"github.com/CoolGitOrg/tmplforge/internal/web"
)

// version is set at link time: -ldflags "-X main.version=v0.1.0".
var version = "dev"

const usage = `usage: tmplforge <command> [flags]

commands:
  sh       line-command TUI; reads commands from stdin (h for help)
  serve    HTML GUI on 127.0.0.1:8088
  render   render one template (-sandbox for untrusted input)
  lint     report template errors as file:line: message
  vars     print a JSON skeleton fixture for a template
  test     render against several fixtures and diff the outputs
  export   write a reproducible bundle directory
  catalog  list the built-in starter templates
  new      generate IssueOps kits and CI/CD workflows
  version  print version

run 'tmplforge <command> -h' for flags.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case "sh":
		return tui.Main(args, stdin, stdout, stderr)
	case "serve":
		return serve(args, stderr)
	case "render":
		return forge.RenderCommand(args, stdin, stdout, stderr)
	case "lint":
		return lint(args, stdout, stderr)
	case "vars":
		return vars(args, stdout, stderr)
	case "test":
		return test(args, stdout, stderr)
	case "export":
		return export(args, stdout, stderr)
	case "catalog":
		return catalog(args, stdout, stderr)
	case "new":
		return newCmd(args, stdout, stderr)
	case "version", "-version", "--version":
		fmt.Fprintf(stdout, "tmplforge %s %s %s/%s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "tmplforge: unknown command %q\n%s", cmd, usage)
	return 2
}

func flags(name, synopsis string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: tmplforge %s %s\n", name, synopsis)
		fs.PrintDefaults()
	}
	return fs
}

func serve(args []string, stderr io.Writer) int {
	fs := flags("serve", "[-a addr] [-token t] [-rate n] [-assets dir] [-timeout d] [-j n] [-q]", stderr)
	addr := fs.String("a", "127.0.0.1:8088", "listen `address`; non-loopback requires -token")
	token := fs.String("token", os.Getenv("TMPLFORGE_TOKEN"), "bearer `token` (default $TMPLFORGE_TOKEN)")
	rate := fs.Float64("rate", 10, "per-IP requests/second (token bucket, burst 2x); 0 disables on loopback")
	assets := fs.String("assets", "", "`dir` overlaid on /static/ (e.g. holding echarts/echarts.min.js)")
	timeout := fs.Duration("timeout", 2*time.Second, "per-render sandbox time limit")
	jobs := fs.Int("j", 2*runtime.NumCPU(), "maximum concurrent sandbox processes")
	quiet := fs.Bool("q", false, "no request log")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	logger := log.New(stderr, "tmplforge: ", log.LstdFlags|log.LUTC)
	reqlog := logger
	if *quiet {
		reqlog = nil
	}
	if *timeout <= 0 || *timeout > 30*time.Second {
		logger.Print("serve: -timeout must be in (0, 30s]")
		return 2
	}
	cfg := web.Config{
		Addr: *addr, Token: *token, Rate: *rate, AssetsDir: *assets, Version: version,
		Sandbox:       &forge.Sandbox{Timeout: *timeout},
		MaxConcurrent: *jobs, Log: reqlog,
	}
	if *assets != "" {
		if _, err := os.Stat(filepath.Join(*assets, "echarts", "echarts.min.js")); err != nil {
			logger.Printf("note: %s/echarts/echarts.min.js not found; /chart will show its table only", *assets)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()
	logger.Printf("serving on http://%s (sandbox timeout %v, %d concurrent)", *addr, *timeout, *jobs)
	if err := web.ListenAndServe(ctx, cfg); err != nil {
		logger.Print(err)
		return 1
	}
	logger.Print("shut down")
	return 0
}

// templateFlags are shared by lint, vars, test and export.
type templateFlags struct {
	t, f, delims *string
}

func addTemplateFlags(fs *flag.FlagSet) templateFlags {
	return templateFlags{
		t:      fs.String("t", "", "template `file`"),
		f:      fs.String("f", "", "`format` html|md|json|txt|yaml (default from file name)"),
		delims: fs.String("delims", "", "action delimiters `L,R`"),
	}
}

func (tf templateFlags) load() (string, forge.Options, error) {
	if *tf.t == "" {
		return "", forge.Options{}, fmt.Errorf("-t file is required")
	}
	b, err := os.ReadFile(*tf.t)
	if err != nil {
		return "", forge.Options{}, err
	}
	l, r, err := forge.ParseDelims(*tf.delims)
	if err != nil {
		return "", forge.Options{}, err
	}
	format := *tf.f
	if format == "" {
		format = forge.FormatFromName(*tf.t)
	}
	return string(b), forge.Options{Name: filepath.Base(*tf.t), Format: format, Left: l, Right: r}, nil
}

func lint(args []string, stdout, stderr io.Writer) int {
	fs := flags("lint", "-t file [-f format] [-delims L,R] [-d fixture] [-vars a,b]", stderr)
	tf := addTemplateFlags(fs)
	fixture := fs.String("d", "", "warn about top-level fields missing from this fixture `file`")
	declared := fs.String("vars", "", "comma-separated declared `names`; warn on undeclared fields")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, o, err := tf.load()
	if err != nil {
		fmt.Fprintln(stderr, "lint:", err)
		return 2
	}
	lo := forge.LintOptions{Options: o}
	if *declared != "" {
		lo.Declared = strings.Split(*declared, ",")
	}
	if *fixture != "" {
		f, err := os.Open(*fixture)
		if err != nil {
			fmt.Fprintln(stderr, "lint:", err)
			return 2
		}
		lo.Fixture, err = forge.LoadFixture(f)
		f.Close()
		if err != nil {
			fmt.Fprintln(stderr, "lint:", err)
			return 2
		}
	}
	findings := forge.Lint(src, lo)
	for _, f := range findings {
		fmt.Fprintln(stdout, f.Format(*tf.t))
	}
	if forge.HasError(findings) {
		return 1
	}
	return 0
}

func vars(args []string, stdout, stderr io.Writer) int {
	fs := flags("vars", "-t file [-delims L,R] [-refs]", stderr)
	tf := addTemplateFlags(fs)
	refs := fs.Bool("refs", false, "list referenced paths (path, type, line) instead of a skeleton")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, o, err := tf.load()
	if err != nil {
		fmt.Fprintln(stderr, "vars:", err)
		return 2
	}
	v, err := forge.Vars(src, o.Left, o.Right)
	if err != nil {
		fmt.Fprintln(stderr, "vars:", err)
		return 1
	}
	if *refs {
		for _, r := range v.Refs {
			fmt.Fprintln(stdout, r)
		}
		return 0
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v.Skeleton); err != nil {
		fmt.Fprintln(stderr, "vars:", err)
		return 1
	}
	return 0
}

func test(args []string, stdout, stderr io.Writer) int {
	fs := flags("test", "-t file [-f format] [-delims L,R] [-U n] fixture1 fixture2 ...", stderr)
	tf := addTemplateFlags(fs)
	ctx := fs.Int("U", 3, "lines of diff context")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, o, err := tf.load()
	if err != nil || fs.NArg() < 2 {
		if err != nil {
			fmt.Fprintln(stderr, "test:", err)
		}
		fs.Usage()
		return 2
	}
	outs := make([]string, fs.NArg())
	for i, name := range fs.Args() {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintln(stderr, "test:", err)
			return 2
		}
		data, err := forge.LoadFixture(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(stderr, "test: %s: %v\n", name, err)
			return 2
		}
		out, err := forge.Render(src, data, o)
		if err != nil {
			fmt.Fprintf(stderr, "test: %s: %v\n", name, err)
			return 1
		}
		outs[i] = string(out)
	}
	changed := false
	for i := 1; i < len(outs); i++ {
		ops := forge.Diff(forge.Lines(outs[i-1]), forge.Lines(outs[i]))
		if forge.Changed(ops) {
			changed = true
			fmt.Fprint(stdout, forge.Unified(fs.Arg(i-1), fs.Arg(i), ops, *ctx))
		} else {
			fmt.Fprintf(stdout, "%s == %s\n", fs.Arg(i-1), fs.Arg(i))
		}
	}
	// diff(1) convention: 0 same, 1 different, 2 trouble.
	if changed {
		return 1
	}
	return 0
}

func export(args []string, stdout, stderr io.Writer) int {
	fs := flags("export", "-t file [-d fixture] -o dir [-f format] [-delims L,R] [-tar]", stderr)
	tf := addTemplateFlags(fs)
	fixture := fs.String("d", "", "fixture JSON `file`")
	dir := fs.String("o", "", "output `dir`")
	tgz := fs.Bool("tar", false, "write a .tar.gz to stdout instead of a directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	src, o, err := tf.load()
	if err != nil || (*dir == "" && !*tgz) {
		if err != nil {
			fmt.Fprintln(stderr, "export:", err)
		}
		fs.Usage()
		return 2
	}
	var fx []byte
	if *fixture != "" {
		if fx, err = os.ReadFile(*fixture); err != nil {
			fmt.Fprintln(stderr, "export:", err)
			return 2
		}
	}
	name := strings.TrimSuffix(filepath.Base(*tf.t), ".tmpl")
	b := forge.Bundle{Name: name, Template: src, Fixture: fx, Options: o}
	if *tgz {
		files, err := forge.ExportFiles(b)
		if err == nil {
			err = forge.WriteTarGz(stdout, forge.SafeName(name), files)
		}
		if err != nil {
			fmt.Fprintln(stderr, "export:", err)
			return 1
		}
		return 0
	}
	files, err := forge.Export(*dir, b)
	if err != nil {
		fmt.Fprintln(stderr, "export:", err)
		return 1
	}
	for _, f := range files {
		fmt.Fprintln(stdout, filepath.Join(*dir, f.Name))
	}
	return 0
}

func catalog(args []string, stdout, stderr io.Writer) int {
	fs := flags("catalog", "[-json] [-src name] [-fixture name]", stderr)
	asJSON := fs.Bool("json", false, "print index as JSON")
	src := fs.String("src", "", "print the template source of entry `name`")
	fix := fs.String("fixture", "", "print the fixture of entry `name`")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	c, err := forge.LoadCatalog()
	if err != nil {
		fmt.Fprintln(stderr, "catalog:", err)
		return 1
	}
	if *src != "" || *fix != "" {
		name := *src + *fix
		e, ok := c.Lookup(name)
		if !ok {
			fmt.Fprintf(stderr, "catalog: no entry %q\n", name)
			return 1
		}
		var b []byte
		if *src != "" {
			s, err := c.Source(e)
			if err != nil {
				fmt.Fprintln(stderr, "catalog:", err)
				return 1
			}
			b = []byte(s)
		} else if b, err = c.FixtureRaw(e); err != nil {
			fmt.Fprintln(stderr, "catalog:", err)
			return 1
		}
		stdout.Write(b)
		return 0
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(c.Entries)
		return 0
	}
	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	for _, e := range c.Entries {
		d := e.Delims
		if d == "" {
			d = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", e.Name, e.Format, d, e.Description)
	}
	tw.Flush()
	return 0
}

func newCmd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "usage: tmplforge new issueops NAME [-fields spec] [-o dir]\n       tmplforge new cicd KIND [-name n] [-binary b] [-inputs spec] [-types t1,t2] [-o dir]")
		return 2
	}
	what, name, rest := args[0], args[1], args[2:]
	var files []forge.File
	var dir *string
	switch what {
	case "issueops":
		fs := flags("new issueops", "NAME [-fields id:type[!][=opt|opt],...] [-o dir]", stderr)
		fields := fs.String("fields", "title:string!", "field `spec`: id:type, types string|text|int|bool|dropdown=a|b, ! = required")
		dir = fs.String("o", ".", "output `dir`")
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		fl, err := scaffold.ParseFields(*fields)
		if err == nil {
			files, err = scaffold.IssueOps(name, fl)
		}
		if err != nil {
			fmt.Fprintln(stderr, "new:", err)
			return 1
		}
	case "cicd":
		fs := flags("new cicd", "KIND [-name n] [-binary b] [-go v] [-inputs spec] [-types t1,t2] [-targets os/arch,...] [-o dir]", stderr)
		wname := fs.String("name", "", "workflow `name` (default KIND)")
		bin := fs.String("binary", "app", "Go command under cmd/")
		gov := fs.String("go", "1.24.x", "Go `version` fallback when vars.GO_VERSION is unset")
		inputs := fs.String("inputs", "", "inputs `spec`: id:type[!][=default|choices], types string|boolean|number|choice|environment (max 25)")
		types := fs.String("types", "", "repository_dispatch event `types`, comma separated")
		targets := fs.String("targets", strings.Join(scaffold.DefaultTargets, ","), "GOOS/GOARCH `list`")
		dir = fs.String("o", ".", "output `dir`")
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		in, err := scaffold.ParseInputs(*inputs)
		if err != nil {
			fmt.Fprintln(stderr, "new:", err)
			return 1
		}
		c := scaffold.CICD{Kind: name, Name: *wname, Binary: *bin, Go: *gov, Inputs: in, Targets: strings.Split(*targets, ",")}
		if *types != "" {
			c.Types = strings.Split(*types, ",")
		}
		f, err := scaffold.Workflow(c)
		if err != nil {
			fmt.Fprintln(stderr, "new:", err)
			return 1
		}
		files = []forge.File{f}
	default:
		fmt.Fprintf(stderr, "new: unknown generator %q (want issueops or cicd)\n", what)
		return 2
	}
	for _, f := range files {
		p := filepath.Join(*dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			fmt.Fprintln(stderr, "new:", err)
			return 1
		}
		if err := os.WriteFile(p, f.Data, 0o644); err != nil {
			fmt.Fprintln(stderr, "new:", err)
			return 1
		}
		fmt.Fprintln(stdout, p)
	}
	return 0
}
