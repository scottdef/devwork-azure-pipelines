//go:build !plan9

package runner

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

var signals = map[string]os.Signal{
	"SIGHUP":   syscall.SIGHUP,
	"SIGINT":   syscall.SIGINT,
	"SIGQUIT":  syscall.SIGQUIT,
	"SIGTERM":  syscall.SIGTERM,
	"SIGKILL":  syscall.SIGKILL,
	"SIGUSR1":  syscall.SIGUSR1,
	"SIGUSR2":  syscall.SIGUSR2,
	"SIGWINCH": syscall.SIGWINCH,
}

// ParseSignal maps a name such as "SIGHUP" or "HUP" to a signal.
func ParseSignal(name string) (os.Signal, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if !strings.HasPrefix(n, "SIG") {
		n = "SIG" + n
	}
	if s, ok := signals[n]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("unknown signal %q", name)
}

// ReloadSignals are the signals that make tmplwatch re-render everything.
func ReloadSignals() []os.Signal { return []os.Signal{syscall.SIGHUP} }

// StopSignals are the signals that make tmplwatch exit cleanly.
func StopSignals() []os.Signal { return []os.Signal{syscall.SIGINT, syscall.SIGTERM} }
