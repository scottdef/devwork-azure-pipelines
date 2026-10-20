//go:build plan9

package main

import (
	"os"
	"syscall"
	"testing"
)

// stopSelf posts an interrupt note to this process.
func stopSelf(t *testing.T) {
	p, _ := os.FindProcess(os.Getpid())
	if err := p.Signal(syscall.Note("interrupt")); err != nil {
		t.Fatal(err)
	}
}
