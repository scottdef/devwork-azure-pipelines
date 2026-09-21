// Package run is the only place this program starts another process.
// az, gh and copilot all go through a Runner, so tests can replace the
// world with a table of canned replies.
package run

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Cmd describes one process. Env entries are added to the parent
// environment and are never printed: that is where secrets travel.
type Cmd struct {
	Name  string
	Args  []string
	Stdin []byte
	Env   []string
	Dir   string
}

// String renders the command for humans and audit logs. No Env.
func (c Cmd) String() string {
	parts := []string{c.Name}
	for _, a := range c.Args {
		parts = append(parts, quote(a))
	}
	return strings.Join(parts, " ")
}

func quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"$&|;<>()*?[]{}\\`!#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Runner runs a command and returns its standard output.
type Runner interface {
	Run(ctx context.Context, c Cmd) ([]byte, error)
}

// Exec runs real processes.
type Exec struct{}

func (Exec) Run(ctx context.Context, c Cmd) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Dir = c.Dir
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.Bytes(), &Error{Cmd: c, Err: err, Stderr: errb.String()}
	}
	return out.Bytes(), nil
}

// Error carries stderr so callers can show the tool's own words.
type Error struct {
	Cmd    Cmd
	Err    error
	Stderr string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if len(msg) > 300 {
		if i := strings.IndexByte(msg, '\n'); i > 0 {
			msg = msg[:i]
		}
	}
	if msg == "" {
		return fmt.Sprintf("%s: %v", e.Cmd.Name, e.Err)
	}
	return fmt.Sprintf("%s: %v: %s", e.Cmd.Name, e.Err, msg)
}

func (e *Error) Unwrap() error { return e.Err }

// Fake answers commands from a table. The key is a prefix of the
// rendered command line; the longest matching prefix wins.
type Fake struct {
	mu      sync.Mutex
	Replies map[string]Reply
	calls   []Cmd
}

// Reply is one canned answer.
type Reply struct {
	Out string
	Err error
}

func (f *Fake) Run(_ context.Context, c Cmd) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	// Keys may be written quoted, as String prints them, or plain.
	line, best, found := c.String(), "", false
	plain := strings.Join(append([]string{c.Name}, c.Args...), " ")
	for k := range f.Replies {
		if (strings.HasPrefix(line, k) || strings.HasPrefix(plain, k)) && len(k) >= len(best) {
			best, found = k, true
		}
	}
	if !found {
		return nil, fmt.Errorf("fake: no reply for %q", line)
	}
	r := f.Replies[best]
	return []byte(r.Out), r.Err
}

// Calls returns a copy of every command the Fake has seen.
func (f *Fake) Calls() []Cmd {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Cmd(nil), f.calls...)
}
