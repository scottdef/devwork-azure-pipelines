//go:build !plan9

package tui

// shellArgv runs !cmd lines: sh -c CMD.
var shellArgv = []string{"sh", "-c"}
