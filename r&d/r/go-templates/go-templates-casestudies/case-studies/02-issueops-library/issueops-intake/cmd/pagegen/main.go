// Command pagegen renders the intake side's text.
//
// Site mode builds the static intake site from the issue forms:
//
//	pagegen -intake [-forms .github/ISSUE_TEMPLATE] [-schema schema] [-flags flags.json] -o _site
//
// Comment mode renders an issue comment from a text/template and a JSON
// document on stdin (or -d file):
//
//	validate ... | pagegen -comment templates/comment-invalid.md.tmpl > comment.md
//
// Comments are Markdown, so they go through text/template; the data was
// written by our own binaries, but any string that started life in an
// issue is passed through the "code" function, which fences it.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/CoolGitOrg/issueops-intake/internal/site"
)

var version = "dev"

func main() {
	intake := flag.Bool("intake", false, "render the intake site")
	forms := flag.String("forms", ".github/ISSUE_TEMPLATE", "issue form `dir`")
	schemaDir := flag.String("schema", "schema", "rules `dir` (<form>.json)")
	flags := flag.String("flags", "", "beta flags `file` copied to flags.json (default: submit enabled)")
	outDir := flag.String("o", "_site", "output `dir`")
	repo := flag.String("repo", envOr("GITHUB_REPOSITORY", "CoolGitOrg/issueops-intake"), "intake `owner/name`")
	library := flag.String("library", "https://coolgitorg.github.io/issueops-library/", "library Pages `url`")
	maxURL := flag.Int("maxurl", 7000, "prefill URL length `limit`")
	stamp := flag.String("time", "", "build `time` to print (RFC 3339; default now, UTC)")
	comment := flag.String("comment", "", "render comment `template` with JSON data")
	dataPath := flag.String("d", "-", "comment data `file`")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: pagegen -intake [-o dir] | pagegen -comment x.md.tmpl [-d data.json]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	var err error
	switch {
	case *intake && *comment == "":
		ts := *stamp
		if ts == "" {
			ts = time.Now().UTC().Format(time.RFC3339)
		}
		cfg := site.Config{Repo: *repo, GitHub: "https://github.com", Library: *library, MaxURL: *maxURL, Version: version, BuildTime: ts}
		var s *site.Site
		if s, err = site.Load(cfg, *forms, *schemaDir, *flags); err == nil {
			err = s.Write(*outDir)
		}
	case *comment != "" && !*intake:
		err = renderComment(*comment, *dataPath, os.Stdin, os.Stdout)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "pagegen: %v\n", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// commentFuncs are the helpers comment templates may use.
var commentFuncs = template.FuncMap{
	// code fences untrusted text so Markdown and @mentions stay inert.
	"code": func(s string) string {
		fence := "```"
		for strings.Contains(s, fence) {
			fence += "`"
		}
		return fence + "text\n" + s + "\n" + fence
	},
	// inline puts a short value in backticks, flattening newlines and
	// escaping the pipe so it cannot break out of a table cell.
	"inline": func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		s = strings.NewReplacer("`", "'", "|", `\|`).Replace(s)
		return "`" + s + "`"
	},
	// No "env" function on purpose: comments are public and the runner
	// environment holds tokens.
}

func renderComment(tmplPath, dataPath string, stdin io.Reader, stdout io.Writer) error {
	t, err := template.New(tmplName(tmplPath)).Funcs(commentFuncs).Option("missingkey=error").ParseFiles(tmplPath)
	if err != nil {
		return err
	}
	in := stdin
	if dataPath != "-" {
		f, err := os.Open(dataPath)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	var data map[string]any
	if err := json.NewDecoder(in).Decode(&data); err != nil {
		return fmt.Errorf("comment data: %w", err)
	}
	return t.Execute(stdout, data)
}

func tmplName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
