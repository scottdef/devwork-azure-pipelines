// Command gendoc writes docs/FUNCS.md from funcs.Docs. Run via
// `go generate ./internal/funcs`.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

func main() {
	out := flag.String("o", "", "output file (default stdout)")
	flag.Parse()
	b, err := funcs.Markdown()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendoc:", err)
		os.Exit(1)
	}
	if *out == "" {
		os.Stdout.Write(b)
		return
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gendoc:", err)
		os.Exit(1)
	}
}
