//go:build !plan9

package runner

import (
	"context"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestMain lets the test binary act as the supervised child: with
// TMPLWATCH_CHILD set it prints a line per SIGHUP and exits on SIGTERM.
func TestMain(m *testing.M) {
	if os.Getenv("TMPLWATCH_CHILD") == "1" {
		c := make(chan os.Signal, 4)
		signal.Notify(c, syscall.SIGHUP, syscall.SIGTERM)
		os.Stdout.WriteString("child up\n")
		for s := range c {
			if s == syscall.SIGTERM {
				os.Stdout.WriteString("child term\n")
				os.Exit(0)
			}
			os.Stdout.WriteString("child hup\n")
		}
	}
	os.Exit(m.Run())
}

type syncBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitFor(t *testing.T, what string, f func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if f() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestSupervisor(t *testing.T) {
	os.Setenv("TMPLWATCH_CHILD", "1")
	defer os.Unsetenv("TMPLWATCH_CHILD")
	exe, _ := os.Executable()
	var out syncBuf
	s := &Supervisor{
		Args: []string{exe, "-test.run=^$"}, ChangeSig: syscall.SIGHUP, KillSig: syscall.SIGTERM,
		KillTimeout: 2 * time.Second, Stdout: &out, Stderr: io.Discard, Log: log.New(io.Discard, "", 0),
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.Bind(ctx)
	s.Changed(false) // not ready: no start
	if s.Status().Running {
		t.Fatal("started before ready")
	}
	s.Changed(true)
	waitFor(t, "child up", func() bool { return strings.Contains(out.String(), "child up") })
	s.Changed(true)
	waitFor(t, "child hup", func() bool { return strings.Contains(out.String(), "child hup") })
	if st := s.Status(); !st.Running || st.PID == 0 {
		t.Errorf("status %+v", st)
	}
	cancel()
	s.Wait()
	if !strings.Contains(out.String(), "child term") {
		t.Errorf("child not terminated gracefully: %q", out.String())
	}
}

// TestSupervisorStartsWithoutWrites covers a restart of tmplwatch when
// every destination is already up to date: no write, but the child runs.
func TestSupervisorStartsWithoutWrites(t *testing.T) {
	os.Setenv("TMPLWATCH_CHILD", "1")
	defer os.Unsetenv("TMPLWATCH_CHILD")
	exe, _ := os.Executable()
	var out syncBuf
	sup := &Supervisor{
		Args: []string{exe, "-test.run=^$"}, ChangeSig: syscall.SIGHUP, KillSig: syscall.SIGTERM,
		KillTimeout: 2 * time.Second, Stdout: &out, Stderr: io.Discard, Log: log.New(io.Discard, "", 0),
	}
	r, dir := setup(t, map[string]string{"a": `{{ key "x" }}`}, nil)
	os.MkdirAll(filepath.Join(dir, "out"), 0o755)
	os.WriteFile(filepath.Join(dir, "out", "a"), []byte("v"), 0o644)
	ctx, cancel := context.WithCancel(context.Background())
	sup.Bind(ctx)
	r.Exec = sup
	if o := r.Update(snap("x", "v")); len(o.Written) != 0 {
		t.Fatalf("expected no writes, got %v", o.Written)
	}
	waitFor(t, "child up", func() bool { return strings.Contains(out.String(), "child up") })
	cancel()
	sup.Wait()
}
