// Command tpl renders one Go text/template with JSON data. It is how a
// borrower turns a downloaded next.tmpl into the next phase's payload, and
// how comment Markdown is written in workflows.
//
// Usage:
//
//	tpl -d data.json -t x.tmpl [-delims '[[,]]'] [-missingkey error] [-o out]
//
// -d - reads data from stdin. Without -d the data is an empty object.
// A "#!tmpl" header line in the template (see docs/FUNCS.md) is honoured;
// -delims and -missingkey override it. The output is refused if it
// contains "<no value>", which means a key was missing and had no default.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/CoolGitOrg/issueops-library/internal/funcs"
	"github.com/CoolGitOrg/issueops-library/internal/item"
)

var version = "dev"

type options struct {
	data, tmpl, delims, missing, out string
}

func main() {
	var o options
	flag.StringVar(&o.data, "d", "", "JSON data `file` ('-' for stdin)")
	flag.StringVar(&o.tmpl, "t", "", "template `file`")
	flag.StringVar(&o.delims, "delims", "", "`left,right` delimiters, e.g. '[[,]]'")
	flag.StringVar(&o.missing, "missingkey", "", "missing key `mode`: zero, error, default")
	flag.StringVar(&o.out, "o", "", "output `file` (default stdout; written atomically)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if o.tmpl == "" || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: tpl -t x.tmpl [-d data.json|-] [-delims l,r] [-missingkey mode] [-o out]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	if err := run(o, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "tpl: %v\n", err)
		os.Exit(1)
	}
}

func run(o options, stdin io.Reader, stdout io.Writer) error {
	src, err := os.ReadFile(o.tmpl)
	if err != nil {
		return err
	}
	h, body, err := item.ParseHeader(string(src))
	if err != nil {
		return err
	}
	if o.delims != "" {
		l, r, ok := strings.Cut(o.delims, ",")
		if !ok || l == "" || r == "" {
			return fmt.Errorf("-delims must be left,right")
		}
		h.Left, h.Right = l, r
	}
	if o.missing != "" {
		h.MissingKey = o.missing
	}
	t, err := template.New(filepath.Base(o.tmpl)).Delims(h.Left, h.Right).
		Funcs(funcs.Map()).Option("missingkey=" + h.MissingKey).Parse(body)
	if err != nil {
		return err
	}
	var data any = map[string]any{}
	if o.data != "" {
		var in io.Reader = stdin
		if o.data != "-" {
			f, err := os.Open(o.data)
			if err != nil {
				return err
			}
			defer f.Close()
			in = f
		}
		dec := json.NewDecoder(in)
		dec.UseNumber()
		if err := dec.Decode(&data); err != nil {
			return fmt.Errorf("data: %w", err)
		}
	}
	b, err := item.Execute(t, data)
	if err != nil {
		return err
	}
	if o.out == "" {
		_, err = io.Copy(stdout, bytes.NewReader(b))
		return err
	}
	tmp := o.out + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, o.out)
}
