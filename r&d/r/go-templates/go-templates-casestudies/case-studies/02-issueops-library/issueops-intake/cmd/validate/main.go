// Command validate checks a parsed issue against a rules file.
//
// Usage:
//
//	validate -schema schema/request.json req.json
//
// It always prints a JSON report on stdout:
//
//	{"ok": false, "errors": [{"field": "phase", "rule": "enum", "message": "..."}]}
//
// and exits 0 when the document is valid, 1 when it is not, 2 on usage or
// I/O errors. The report is the data for templates/comment-invalid.md.tmpl.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/CoolGitOrg/issueops-intake/internal/schema"
)

var version = "dev"

// Report is the JSON written to stdout.
type Report struct {
	OK     bool           `json:"ok"`
	Errors []schema.Error `json:"errors"`
}

func main() {
	schemaPath := flag.String("schema", "", "rules `file`")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: validate -schema rules.json [doc.json|-]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *schemaPath == "" || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	ok, err := run(*schemaPath, flag.Arg(0), os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "validate: %v\n", err)
		os.Exit(2)
	}
	if !ok {
		os.Exit(1)
	}
}

func run(schemaPath, docPath string, stdin io.Reader, stdout io.Writer) (bool, error) {
	s, err := schema.Load(schemaPath)
	if err != nil {
		return false, err
	}
	in := stdin
	if docPath != "" && docPath != "-" {
		f, err := os.Open(docPath)
		if err != nil {
			return false, err
		}
		defer f.Close()
		in = f
	}
	var doc map[string]any
	rep := Report{Errors: []schema.Error{}}
	if err := json.NewDecoder(in).Decode(&doc); err != nil {
		// A document that is not even JSON is a validation failure the
		// requester should hear about, not an operator error.
		rep.Errors = append(rep.Errors, schema.Error{Field: "-", Rule: "json", Message: "issue did not parse: " + err.Error()})
	} else {
		rep.Errors = append(rep.Errors, s.Check(doc)...)
	}
	rep.OK = len(rep.Errors) == 0
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return rep.OK, enc.Encode(rep)
}
