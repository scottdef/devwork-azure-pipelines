package runner

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Supervisor runs one child process for the life of tmplwatch, the
// consul-template "exec mode". The child starts once every template has
// rendered, is restarted with backoff if it exits, and is sent
// ChangeSignal whenever a destination changes. With no ChangeSignal the
// child is restarted on change instead.
type Supervisor struct {
	Args        []string
	ChangeSig   os.Signal // nil: restart on change
	KillSig     os.Signal
	KillTimeout time.Duration
	Stdout      io.Writer
	Stderr      io.Writer
	Log         *log.Logger
	OnRestart   func()

	mu       sync.Mutex
	ctx      context.Context
	started  bool
	cmd      *exec.Cmd
	done     chan struct{} // closed when the current child exits
	restarts int
	lastErr  string
	stopped  chan struct{}
	bounce   bool // the next exit was requested; restart without backoff
}

// ExecStatus is the supervisor's state, served by /status.
type ExecStatus struct {
	Command  []string `json:"command"`
	PID      int      `json:"pid,omitempty"`
	Running  bool     `json:"running"`
	Restarts int      `json:"restarts"`
	LastExit string   `json:"last_exit,omitempty"`
}

// Status reports the child's state.
func (s *Supervisor) Status() ExecStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := ExecStatus{Command: s.Args, Restarts: s.restarts, LastExit: s.lastErr}
	if s.cmd != nil && s.cmd.Process != nil {
		st.PID, st.Running = s.cmd.Process.Pid, true
	}
	return st
}

// Changed is called after destinations were written. ready reports
// whether every template has rendered; the child is not started before.
func (s *Supervisor) Changed(ready bool) {
	s.mu.Lock()
	ctx, started := s.ctx, s.started
	if !started {
		s.mu.Unlock()
		if ready && ctx != nil {
			s.start()
		}
		return
	}
	cmd := s.cmd
	if cmd == nil || cmd.Process == nil {
		s.mu.Unlock()
		return
	}
	if s.ChangeSig == nil {
		s.bounce = true
		s.mu.Unlock()
		s.terminate(cmd)
		return
	}
	s.mu.Unlock()
	if err := cmd.Process.Signal(s.ChangeSig); err != nil {
		s.Log.Printf("exec: signal %v: %v", s.ChangeSig, err)
	}
}

// Ready starts the child if it has not been started; it never signals.
func (s *Supervisor) Ready() {
	s.mu.Lock()
	ctx, started := s.ctx, s.started
	s.mu.Unlock()
	if !started && ctx != nil {
		s.start()
	}
}

// Bind sets the context whose cancellation stops the child. The child is
// started by the first Changed(true).
func (s *Supervisor) Bind(ctx context.Context) {
	s.mu.Lock()
	s.ctx = ctx
	s.stopped = make(chan struct{})
	s.mu.Unlock()
}

// Wait blocks until the supervisor has stopped the child after its
// context was cancelled, or returns at once if it never started.
func (s *Supervisor) Wait() {
	s.mu.Lock()
	started, stopped := s.started, s.stopped
	s.mu.Unlock()
	if started {
		<-stopped
	}
}

func (s *Supervisor) start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()
	go s.loop()
}

func (s *Supervisor) loop() {
	defer close(s.stopped)
	backoff := time.Second
	for {
		began := time.Now()
		err := s.runOnce()
		s.mu.Lock()
		bounce := s.bounce
		s.bounce = false
		s.cmd = nil
		if err != nil {
			s.lastErr = err.Error()
		} else {
			s.lastErr = "exit status 0"
		}
		s.mu.Unlock()
		if s.ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		s.restarts++
		s.mu.Unlock()
		if s.OnRestart != nil {
			s.OnRestart()
		}
		if bounce {
			s.Log.Printf("exec: restarting %s after change", s.Args[0])
			continue
		}
		if time.Since(began) > 10*time.Second {
			backoff = time.Second
		}
		s.Log.Printf("exec: %s exited (%v); restarting in %v", s.Args[0], err, backoff)
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (s *Supervisor) runOnce() error {
	cmd := exec.Command(s.Args[0], s.Args[1:]...)
	cmd.Stdout, cmd.Stderr = s.Stdout, s.Stderr
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	s.mu.Lock()
	s.cmd, s.done = cmd, done
	s.mu.Unlock()
	s.Log.Printf("exec: started %s (pid %d)", s.Args[0], cmd.Process.Pid)
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait(); close(done) }()
	select {
	case err := <-waitErr:
		return err
	case <-s.ctx.Done():
		s.terminate(cmd)
		return <-waitErr
	}
}

// terminate sends KillSig, then kills after KillTimeout.
func (s *Supervisor) terminate(cmd *exec.Cmd) {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	sig := s.KillSig
	if sig == nil {
		sig = os.Kill
	}
	if err := cmd.Process.Signal(sig); err != nil && !errors.Is(err, os.ErrProcessDone) {
		s.Log.Printf("exec: signal %v: %v", sig, err)
	}
	select {
	case <-done:
	case <-time.After(s.KillTimeout):
		s.Log.Printf("exec: %s did not exit in %v; killing", s.Args[0], s.KillTimeout)
		cmd.Process.Kill()
	}
}
