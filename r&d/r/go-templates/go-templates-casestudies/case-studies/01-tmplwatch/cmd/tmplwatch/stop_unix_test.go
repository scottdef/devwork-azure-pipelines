//go:build !plan9

package main

import (
	"syscall"
	"testing"
)

// stopSelf delivers SIGTERM to this process; run's NotifyContext catches it.
func stopSelf(t *testing.T) {
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
}
