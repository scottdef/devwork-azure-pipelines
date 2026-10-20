package tui

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
)

// Main implements `tmplforge sh [-c NAME] [-d DIR] [script]`.
func Main(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("sh", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cat := fs.String("c", "", "load catalog entry `NAME` first")
	dir := fs.String("d", ".", "base `dir` for relative file names")
	noprompt := fs.Bool("n", false, "never print the prompt")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tmplforge sh [-c name] [-d dir] [-n] [script.tf]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	in := stdin
	interactive := false
	if f, ok := stdin.(*os.File); ok && fs.NArg() == 0 {
		interactive = isTerminal(f)
	}
	if fs.NArg() > 0 {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			fmt.Fprintln(stderr, "sh:", err)
			return 1
		}
		defer f.Close()
		in = f
	}
	s := New(in, stdout)
	s.In = bufio.NewReader(in)
	s.Prompt = interactive && !*noprompt
	s.Color = ColorEnabled(stdout, os.Getenv)
	s.Dir = *dir
	s.EdIn, s.EdOut = stdin, stdout
	if *cat != "" {
		if _, err := s.Exec("c " + *cat); err != nil {
			s.report(err)
			return 1
		}
	}
	if err := s.Run(); err != nil {
		return 1
	}
	return 0
}
