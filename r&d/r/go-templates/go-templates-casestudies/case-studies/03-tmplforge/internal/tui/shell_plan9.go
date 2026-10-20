//go:build plan9

package tui

// shellArgv runs !cmd lines with rc, Plan 9's shell: rc -c CMD.
var shellArgv = []string{"rc", "-c"}
