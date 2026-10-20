package forge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Sandbox errors.
var (
	ErrTimeout = errors.New("render timed out")
	ErrCrashed = errors.New("render process crashed")
)

// RenderError is a template error reported by the sandboxed child. Line is
// the 1-based template line when the message carried one.
type RenderError struct {
	Msg  string
	Line int
}

func (e *RenderError) Error() string { return e.Msg }

// Sandbox renders untrusted templates in a child process: the same binary
// re-executed as `render -sandbox`. A process boundary is the only reliable
// way to stop a text/template execution (Execute takes no context) and to
// survive a runtime fatal error (stack exhaustion, out of memory), which a
// goroutine and recover cannot catch. The child gets a minimal environment,
// the Safe FuncMap, a template size cap, an output cap and its own timer;
// the parent adds a hard kill, an output read cap and a stderr cap.
type Sandbox struct {
	Exe       string        // binary to run; default os.Executable()
	Args      []string      // argv before the render flags; default ["render"]
	Env       []string      // child environment; default MinimalEnv()
	Timeout   time.Duration // default 2s
	MaxOutput int64         // default MaxOutput
	Grace     time.Duration // extra time before SIGKILL after Timeout; default 500ms
}

// MinimalEnv is the child environment: nothing inherited, so a template
// cannot learn secrets from the server's environment even if env were
// somehow reachable. GOMEMLIMIT makes the child's GC work hard before the
// kernel's OOM killer has to; GOMAXPROCS=2 keeps one render from eating
// the box.
func MinimalEnv() []string {
	env := []string{"TMPLFORGE_SANDBOX=1", "GOMEMLIMIT=128MiB", "GOMAXPROCS=2"}
	if v := os.Getenv("SOURCE_DATE_EPOCH"); v != "" {
		env = append(env, "SOURCE_DATE_EPOCH="+v)
	}
	if v := os.Getenv("TZ"); v != "" {
		env = append(env, "TZ="+v)
	}
	return env
}

func (s *Sandbox) defaults() (exe string, args, env []string, timeout, grace time.Duration, max int64, err error) {
	exe = s.Exe
	if exe == "" {
		if exe, err = os.Executable(); err != nil {
			return
		}
	}
	args = s.Args
	if args == nil {
		args = []string{"render"}
	}
	env = s.Env
	if env == nil {
		env = MinimalEnv()
	}
	timeout = s.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	grace = s.Grace
	if grace <= 0 {
		grace = 500 * time.Millisecond
	}
	max = s.MaxOutput
	if max <= 0 || max > MaxOutput {
		max = MaxOutput
	}
	return
}

// Render renders src against the fixture JSON in a child process.
func (s *Sandbox) Render(ctx context.Context, src string, fixture []byte, o Options) ([]byte, error) {
	exe, args, env, timeout, grace, max, err := s.defaults()
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	if len(src) > MaxTemplate {
		return nil, fmt.Errorf("%w: %d bytes > %d", ErrTemplateLimit, len(src), MaxTemplate)
	}
	if len(fixture) > MaxFixture {
		return nil, fmt.Errorf("fixture too large: %d bytes > %d", len(fixture), MaxFixture)
	}
	// The fixture travels in a temp file and the template on stdin: two
	// streams with no framing protocol to get wrong, and portable to Plan 9
	// (no ExtraFiles descriptor games).
	fx, err := os.CreateTemp("", "tmplforge-fixture-*.json")
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	defer os.Remove(fx.Name())
	if _, err := fx.Write(fixture); err != nil {
		fx.Close()
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	if err := fx.Close(); err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}

	format := o.Format
	if format == "" {
		format = "txt"
	}
	argv := append(append([]string{}, args...),
		"-sandbox", "-t", "-", "-d", fx.Name(), "-f", format,
		"-timeout", timeout.String(), "-max", strconv.FormatInt(max, 10))
	if o.Left != "" {
		argv = append(argv, "-delims", o.Left+","+o.Right)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout+grace)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, argv...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &capWriter{w: &stdout, n: max + 1}
	cmd.Stderr = &capWriter{w: &stderr, n: 64 << 10}
	cmd.WaitDelay = grace
	start := time.Now()
	err = cmd.Run()
	out := stdout.Bytes()
	if int64(len(out)) > max {
		return out[:max], fmt.Errorf("%w (%d bytes)", ErrOutputLimit, max)
	}
	if err == nil {
		return out, nil
	}
	if e, ok := classify(stderr.String()); ok {
		return out, e
	}
	if ctx.Err() != nil && time.Since(start) >= timeout {
		return out, fmt.Errorf("%w after %v (killed)", ErrTimeout, timeout)
	}
	// Untagged failure: the child died without reporting (runtime fatal
	// error, signal, OOM kill). The first stderr line is enough; the rest
	// would be a goroutine dump.
	msg, _, _ := strings.Cut(strings.TrimSpace(stderr.String()), "\n")
	if msg != "" {
		return out, fmt.Errorf("%w: %v: %s", ErrCrashed, err, msg)
	}
	return out, fmt.Errorf("%w: %v", ErrCrashed, err)
}

// capWriter keeps the first n bytes and silently discards the rest, so a
// child flooding its pipes can neither fill the parent's memory nor block
// on a full pipe.
type capWriter struct {
	w io.Writer
	n int64
}

func (c *capWriter) Write(p []byte) (int, error) {
	if c.n > 0 {
		k := min(int64(len(p)), c.n)
		c.w.Write(p[:k])
		c.n -= k
	}
	return len(p), nil
}
