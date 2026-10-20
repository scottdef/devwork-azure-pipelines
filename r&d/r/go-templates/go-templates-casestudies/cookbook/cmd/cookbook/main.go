// Command cookbook lists, shows, runs and checks the formulas of the Go
// templates cookbook, and generates the Markdown cookbook from them.
//
//	cookbook list
//	cookbook show F07
//	cookbook run F07 [-d data.json]
//	cookbook gen [-o ../docs/COOKBOOK.md]
//	cookbook gen -funcs [-o ../docs/FUNCS-SHARED.md]
//	cookbook check [-docs ../docs] [-out testdata/out]
package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/CoolGitOrg/go-template-cookbook/formulas"
)

var version = "dev"

//go:embed cookbook.md.tmpl
var mdTmpl string

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// usageError marks a mistake in the command line: exit status 2, not 1.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

func usagef(format string, a ...any) error { return usageError{fmt.Errorf(format, a...)} }

const usageText = `usage: cookbook command [args]

	list                       list formula IDs and names
	show ID                    print a formula: problem, template, data, glue, output, notes
	run ID [-d file]           render a formula to stdout; -d replaces its data with JSON ("-" is stdin)
	gen [-o file]              write the Markdown cookbook (default ../docs/COOKBOOK.md, "-" is stdout)
	gen -funcs [-o file]       write the shared FuncMap reference (default ../docs/FUNCS-SHARED.md)
	check [-docs dir] [-out dir]
	                           render every formula and compare with Want, testdata/out and the docs
	version                    print the version
`

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usageText)
		return 2
	}
	var err error
	switch args[0] {
	case "list":
		err = cmdList(args[1:], stdout)
	case "show":
		err = cmdShow(args[1:], stdout)
	case "run":
		err = cmdRun(args[1:], stdin, stdout)
	case "gen":
		err = cmdGen(args[1:], stdout)
	case "check":
		err = cmdCheck(args[1:], stdout)
	case "version":
		fmt.Fprintln(stdout, version)
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usageText)
	default:
		fmt.Fprintf(stderr, "cookbook: unknown command %q\n%s", args[0], usageText)
		return 2
	}
	if err != nil {
		fmt.Fprintf(stderr, "cookbook: %v\n", err)
		var ue usageError
		if errors.As(err, &ue) {
			return 2
		}
		return 1
	}
	return 0
}

// parse parses flags that may come after positional arguments, which the
// flag package alone does not allow, and returns the positionals.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, usageError{err}
		}
		if fs.NArg() == 0 {
			return pos, nil
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

func lookup(id string) (formulas.Formula, error) {
	f, ok := formulas.Lookup(strings.ToUpper(id))
	if !ok {
		return f, usagef("no formula %q (try: cookbook list)", id)
	}
	return f, nil
}

func cmdList(args []string, out io.Writer) error {
	pos, err := parse(flag.NewFlagSet("list", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: cookbook list")
	}
	for _, f := range formulas.All {
		fmt.Fprintf(out, "%s\t%s\n", f.ID, f.Name)
	}
	return nil
}

func cmdShow(args []string, out io.Writer) error {
	pos, err := parse(flag.NewFlagSet("show", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: cookbook show ID")
	}
	f, err := lookup(pos[0])
	if err != nil {
		return err
	}
	v, err := newView(f)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s  %s\n\nProblem:\n  %s\n\n%s\n\n-- template --\n%s", f.ID, f.Name, f.Problem, v.Meta, ensureNL(f.Template))
	if v.DataJSON != "" {
		fmt.Fprintf(out, "\n-- data --\n%s\n", v.DataJSON)
	}
	if f.Glue != "" {
		fmt.Fprintf(out, "\n-- glue --\n%s\n", f.Glue)
	}
	fmt.Fprintf(out, "\n-- output --\n%s", ensureNL(v.Output))
	fmt.Fprintf(out, "\n-- notes --\n%s\n", f.Notes)
	return nil
}

func ensureNL(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func cmdRun(args []string, stdin io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	data := fs.String("d", "", "replace the formula's data with JSON from `file` (- for stdin)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usagef("usage: cookbook run ID [-d data.json]")
	}
	f, err := lookup(pos[0])
	if err != nil {
		return err
	}
	if *data != "" {
		if err := replaceData(&f, *data, stdin); err != nil {
			return err
		}
	}
	got, err := formulas.Render(f)
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, got)
	return err
}

// replaceData decodes JSON into a value of the same Go type as f.Data, so
// formulas whose glue expects a struct keep working. Formulas with no data,
// or with data that has no JSON form, decode into any.
func replaceData(f *formulas.Formula, path string, stdin io.Reader) error {
	var r io.Reader = stdin
	if path != "-" {
		fh, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fh.Close()
		r = fh
	}
	var target reflect.Value
	if f.Data != nil {
		target = reflect.New(reflect.TypeOf(f.Data))
	} else {
		target = reflect.New(reflect.TypeOf((*any)(nil)).Elem())
	}
	if err := json.NewDecoder(r).Decode(target.Interface()); err != nil {
		return fmt.Errorf("%s: -d %s: %w", f.ID, path, err)
	}
	f.Data = target.Elem().Interface()
	return nil
}

func cmdGen(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	out := fs.String("o", "", "output `file` (- for stdout); default depends on -funcs")
	funcsDoc := fs.Bool("funcs", false, "generate the shared FuncMap reference instead of the cookbook")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: cookbook gen [-funcs] [-o file]")
	}
	var doc, def string
	if *funcsDoc {
		doc, err = genFuncs()
		def = filepath.Join("..", "docs", "FUNCS-SHARED.md")
	} else {
		doc, err = genCookbook()
		def = filepath.Join("..", "docs", "COOKBOOK.md")
	}
	if err != nil {
		return err
	}
	if *out == "" {
		*out = def
	}
	if *out == "-" {
		_, err = io.WriteString(stdout, doc)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(*out, []byte(doc), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", *out, len(doc))
	return nil
}

func cmdCheck(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	docs := fs.String("docs", "", "also verify COOKBOOK.md and FUNCS-SHARED.md in `dir` are current")
	outDir := fs.String("out", "testdata/out", "verify rendered outputs in `dir` if it exists")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return usagef("usage: cookbook check [-docs dir] [-out dir]")
	}
	var bad []string
	failf := func(format string, a ...any) { bad = append(bad, fmt.Sprintf(format, a...)) }
	seen := map[string]bool{}
	for i, f := range formulas.All {
		if want := fmt.Sprintf("F%02d", i+1); f.ID != want {
			failf("%s: expected ID %s at position %d", f.ID, want, i+1)
		}
		if seen[f.ID] {
			failf("%s: duplicate ID", f.ID)
		}
		seen[f.ID] = true
		if f.Name == "" || f.Problem == "" || f.Notes == "" || f.Template == "" {
			failf("%s: Name, Problem, Template and Notes must all be set", f.ID)
		}
		if f.GlueRef != "" && (f.Glue == "" || strings.HasPrefix(f.Glue, "// glue.go")) {
			failf("%s: glue %q did not resolve: %s", f.ID, f.GlueRef, f.Glue)
		}
		got, err := formulas.Render(f)
		if err != nil {
			failf("%s: %v", f.ID, err)
			continue
		}
		if got != f.Want {
			failf("%s: output differs from Want (run: cookbook run %s)", f.ID, f.ID)
		}
		if st, err := os.Stat(*outDir); err == nil && st.IsDir() {
			disk, err := os.ReadFile(filepath.Join(*outDir, f.ID+".txt"))
			switch {
			case err != nil:
				failf("%s: %v", f.ID, err)
			case string(disk) != got:
				failf("%s: %s/%s.txt is stale (go test ./formulas -update)", f.ID, *outDir, f.ID)
			}
		}
	}
	if *docs != "" {
		for name, gen := range map[string]func() (string, error){"COOKBOOK.md": genCookbook, "FUNCS-SHARED.md": genFuncs} {
			want, err := gen()
			if err != nil {
				failf("%s: %v", name, err)
				continue
			}
			disk, err := os.ReadFile(filepath.Join(*docs, name))
			switch {
			case err != nil:
				failf("%s: %v", name, err)
			case string(disk) != want:
				failf("%s is stale (run: cookbook gen%s)", filepath.Join(*docs, name), map[bool]string{true: " -funcs"}[name == "FUNCS-SHARED.md"])
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%d problem(s):\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
	fmt.Fprintf(stdout, "ok: %d formulas\n", len(formulas.All))
	return nil
}
