package tui

import (
	"io"
	"os"
	"strings"
)

// SGR colour codes. Eight colours are enough; a line editor is not a
// light show.
const (
	red    = "31"
	green  = "32"
	yellow = "33"
	cyan   = "36"
	bold   = "1"
	dim    = "2"
)

// painter wraps text in ANSI SGR sequences when enabled, else returns it
// unchanged. Plan 9's rio and acme windows do not interpret escapes, and
// neither do pipes, so colour is opt-in by environment, never assumed.
type painter bool

func (p painter) paint(code, s string) string {
	if !p || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// isTerminal reports whether f is a character device: a terminal on Unix,
// /dev/cons on Plan 9. It is how the REPL decides whether to prompt,
// without termios (which Plan 9 does not have and the rules forbid).
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ColorEnabled reports whether ANSI colour should be used on w: TERM is set
// and not "dumb", NO_COLOR is unset (https://no-color.org), and w is a
// terminal.
func ColorEnabled(w io.Writer, getenv func(string) string) bool {
	term := getenv("TERM")
	if term == "" || strings.EqualFold(term, "dumb") || getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	return ok && isTerminal(f)
}
