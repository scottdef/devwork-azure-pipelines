//go:build plan9

package runner

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// Plan 9 has notes, not signals. A note is a string; os.Process.Signal
// writes sig.String() to /proc/<pid>/note, which is exactly how one posts
// a note from rc: echo hangup >/proc/$pid/note. The Unix names are mapped
// to the notes Go's runtime understands; any other name is posted as is.
var signals = map[string]os.Signal{
	"SIGHUP":  syscall.Note("hangup"),
	"SIGINT":  syscall.Note("interrupt"),
	"SIGTERM": syscall.Note("interrupt"), // "kill" cannot be caught
	"SIGKILL": syscall.Note("kill"),
	"SIGQUIT": syscall.Note("interrupt"),
	"SIGUSR1": syscall.Note("usr1"),
	"SIGUSR2": syscall.Note("usr2"),
}

// ParseSignal maps a Unix signal name or a raw note text to a note.
func ParseSignal(name string) (os.Signal, error) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if !strings.HasPrefix(n, "SIG") {
		n = "SIG" + n
	}
	if s, ok := signals[n]; ok {
		return s, nil
	}
	if name = strings.TrimSpace(name); name != "" {
		return syscall.Note(name), nil
	}
	return nil, fmt.Errorf("empty note")
}

// ReloadSignals are the notes that make tmplwatch re-render everything.
func ReloadSignals() []os.Signal { return []os.Signal{syscall.Note("hangup")} }

// StopSignals are the notes that make tmplwatch exit cleanly.
func StopSignals() []os.Signal { return []os.Signal{syscall.Note("interrupt")} }
