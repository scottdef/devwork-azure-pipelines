// Command issueparse converts a GitHub issue-form body into JSON keyed by
// field id.
//
// Usage:
//
//	issueparse -form .github/ISSUE_TEMPLATE/request.yml [body.md] > req.json
//
// The body is read from the named file, or stdin when none is given or
// the name is "-". Keys are the form's field ids, matching the keying of
// issue-ops/parser v5. Exit status is 1 on a bad form or a body with no
// recognisable fields.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/CoolGitOrg/issueops-intake/internal/form"
	"github.com/CoolGitOrg/issueops-intake/internal/issuebody"
)

var version = "dev"

// maxBody is GitHub's issue body limit (65536 chars); anything larger did
// not come from GitHub.
const maxBody = 1 << 18

func main() {
	formPath := flag.String("form", "", "issue form `yml` that produced the body")
	pretty := flag.Bool("p", true, "indent output")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: issueparse -form form.yml [body.md|-]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *formPath == "" || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*formPath, flag.Arg(0), *pretty, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "issueparse: %v\n", err)
		os.Exit(1)
	}
}

func run(formPath, bodyPath string, pretty bool, stdin io.Reader, stdout io.Writer) error {
	f, err := form.Load(formPath)
	if err != nil {
		return err
	}
	in := stdin
	if bodyPath != "" && bodyPath != "-" {
		fh, err := os.Open(bodyPath)
		if err != nil {
			return err
		}
		defer fh.Close()
		in = fh
	}
	body, err := io.ReadAll(io.LimitReader(in, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("body larger than %d bytes", maxBody)
	}
	v, err := issuebody.Parse(f, string(body))
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}
