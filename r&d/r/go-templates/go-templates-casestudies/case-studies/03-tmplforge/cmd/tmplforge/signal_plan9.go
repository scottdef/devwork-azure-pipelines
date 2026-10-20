//go:build plan9

package main

import "os"

// stopSignals end `serve` gracefully. Plan 9 has notes, not signals; Go
// maps the "interrupt" note to os.Interrupt.
var stopSignals = []os.Signal{os.Interrupt}
