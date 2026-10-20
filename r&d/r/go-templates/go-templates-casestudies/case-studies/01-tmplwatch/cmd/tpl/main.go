// Tpl renders one Go template once, with tmplwatch's function library.
//
// Usage:
//
//	tpl -t file.tmpl [-d data.json] [-kv dir] [-delims '[[,]]']
//	    [-missingkey error|zero|default] [-o out]
//
// The JSON in -d (or stdin with -d -) becomes the template's dot. With
// -kv, the directory is loaded as a key/value store so key, ls and tree
// work as they do under tmplwatch. Output goes to stdout unless -o is
// given, in which case it is written atomically.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/tmplwatch/internal/funcs"
	"github.com/CoolGitOrg/tmplwatch/internal/runner"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tpl", flag.ContinueOnError)
	fs.SetOutput(stderr)
	tfile := fs.String("t", "", "template `file` (required)")
	dfile := fs.String("d", "", "JSON data `file` for dot; - for stdin")
	kv := fs.String("kv", "", "load `dir` as the key/value store for key/ls/tree")
	delims := fs.String("delims", "", "`left,right` delimiters, e.g. '[[,]]'")
	missing := fs.String("missingkey", "default", "map key `policy`: error, zero or default")
	out := fs.String("o", "", "write to `file` atomically instead of stdout")
	perm := fs.String("m", "0644", "`mode` for -o")
	showVer := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVer {
		fmt.Fprintln(stdout, "tpl", version)
		return 0
	}
	if *tfile == "" || fs.NArg() > 0 {
		fmt.Fprintln(stderr, "usage: tpl -t file.tmpl [-d data.json] [-kv dir] [-delims 'l,r'] [-missingkey error|zero|default] [-o out]")
		return 2
	}
	b, err := render(*tfile, *dfile, *kv, *delims, *missing, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "tpl:", err)
		return 1
	}
	if *out == "" {
		stdout.Write(b)
		return 0
	}
	var mode uint32
	if _, err := fmt.Sscanf(*perm, "%o", &mode); err != nil {
		fmt.Fprintln(stderr, "tpl: -m:", err)
		return 2
	}
	if _, err := runner.WriteAtomic(*out, b, os.FileMode(mode), false); err != nil {
		fmt.Fprintln(stderr, "tpl:", err)
		return 1
	}
	return 0
}

func render(tfile, dfile, kvdir, delims, missing string, stdin io.Reader) ([]byte, error) {
	text, err := os.ReadFile(tfile)
	if err != nil {
		return nil, err
	}
	var l, r string
	if delims != "" {
		var ok bool
		l, r, ok = strings.Cut(delims, ",")
		if !ok || l == "" || r == "" {
			return nil, fmt.Errorf("-delims %q: want left,right", delims)
		}
	}
	switch missing {
	case "error", "zero", "default":
	default:
		return nil, fmt.Errorf("-missingkey %q: want error, zero or default", missing)
	}
	var dot any
	if dfile != "" {
		var raw []byte
		if dfile == "-" {
			raw, err = io.ReadAll(stdin)
		} else {
			raw, err = os.ReadFile(dfile)
		}
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&dot); err != nil {
			return nil, fmt.Errorf("%s: %w", dfile, err)
		}
	}
	st := &funcs.MapStore{Data: map[string]string{}}
	if kvdir != "" {
		s, _, err := source.NewDir(kvdir, "").Fetch(context.Background())
		if err != nil {
			return nil, err
		}
		st.Data = s.Data
	}
	t, err := template.New(filepath.Base(tfile)).
		Delims(l, r).
		Option("missingkey=" + missing).
		Funcs(funcs.Map(funcs.Options{Store: st, Dir: os.DirFS(filepath.Dir(tfile))})).
		Parse(string(text))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, dot); err != nil {
		return nil, err
	}
	if len(st.Missed) > 0 {
		return nil, errors.New("missing keys: " + strings.Join(st.Missed, ", "))
	}
	return buf.Bytes(), nil
}
