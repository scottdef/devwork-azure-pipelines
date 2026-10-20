//go:build !plan9

package main

import (
	"os"
	"syscall"
)

// stopSignals end `serve` gracefully: ^C and the service manager's TERM.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
