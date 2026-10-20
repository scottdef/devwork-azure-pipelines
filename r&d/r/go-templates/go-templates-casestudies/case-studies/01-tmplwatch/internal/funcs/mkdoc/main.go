// Mkdoc writes docs/FUNCS.md from funcs.Docs using a text/template.
//
// Usage:
//
//	go run ./internal/funcs/mkdoc [-o docs/FUNCS.md]
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"

	"github.com/CoolGitOrg/tmplwatch/internal/funcs"
)

func main() {
	out := flag.String("o", "", "output file (default stdout)")
	flag.Parse()
	var buf bytes.Buffer
	if err := funcs.WriteDoc(&buf); err != nil {
		fmt.Fprintln(os.Stderr, "mkdoc:", err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(buf.Bytes())
		return
	}
	if err := os.WriteFile(*out, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "mkdoc:", err)
		os.Exit(1)
	}
}
