// Command gendoc writes docs/CATALOG.md from catalog/index.json.
//
//	go run ./catalog/gendoc -o docs/CATALOG.md
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
)

func main() {
	out := flag.String("o", "", "output file (default stdout)")
	flag.Parse()
	c, err := forge.LoadCatalog()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gendoc:", err)
		os.Exit(1)
	}
	b, err := c.Markdown()
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
