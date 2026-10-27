// Command ghlog2dt ships GitHub Actions job logs to Dynatrace through
// the Log Monitoring API v2 (POST /api/v2/logs/ingest).
//
// Usage:
//
//	ghlog2dt run        [flags]   sweep repos: list | fetch | ship + checkpoint
//	ghlog2dt list       [flags]   print unshipped completed jobs as NDJSON
//	ghlog2dt fetch      [flags]   jobs (NDJSON, stdin) -> records (NDJSON, stdout)
//	ghlog2dt ship       [flags]   records (NDJSON, stdin) -> Dynatrace
//	ghlog2dt checkpoint show|merge|prune|mark|rewind ...
//	ghlog2dt token      [flags]   mint a GitHub App installation token
//	ghlog2dt schedule   [flags]   cron + lookback for an export interval
//	ghlog2dt version
//
// Exit status: 0 ok, 1 error, 2 usage/config, 3 partial success,
// 4 authentication, 5 rate limit or Dynatrace budget, 6 checkpoint I/O.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

// Set by -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = "none"
)

const (
	exitOK      = 0
	exitFail    = 1
	exitUsage   = 2
	exitPartial = 3
	exitAuth    = 4
	exitBudget  = 5
	exitState   = 6
)

// env is the process environment, injectable for tests.
type env struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	getenv func(string) string
}

type command struct {
	run  func(ctx context.Context, e *env, args []string) error
	help string
}

var commands map[string]command

func init() {
	commands = map[string]command{
		"run":        {cmdRun, "sweep target repos and ship new job logs (production path)"},
		"list":       {cmdList, "print unshipped completed jobs as NDJSON"},
		"fetch":      {cmdFetch, "read jobs (NDJSON) on stdin, write Dynatrace records (NDJSON)"},
		"ship":       {cmdShip, "read records (NDJSON) on stdin and POST them to Dynatrace"},
		"checkpoint": {cmdCheckpoint, "show | merge | prune | mark | rewind checkpoint files"},
		"token":      {cmdToken, "mint a GitHub App installation token"},
		"schedule":   {cmdSchedule, "compute cron + lookback for an interval; rewrite a workflow"},
		"version":    {cmdVersion, "print version"},
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := realMain(ctx, os.Args[1:], &env{os.Stdin, os.Stdout, os.Stderr, os.Getenv})
	stop()
	os.Exit(code)
}

func realMain(ctx context.Context, args []string, e *env) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		usage(e.stderr)
		if len(args) == 0 {
			return exitUsage
		}
		return exitOK
	}
	c, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(e.stderr, "ghlog2dt: unknown command %q\n\n", args[0])
		usage(e.stderr)
		return exitUsage
	}
	err := c.run(ctx, e, args[1:])
	if err == nil {
		return exitOK
	}
	var x *exitError
	if errors.As(err, &x) {
		if x.err != nil {
			fmt.Fprintf(e.stderr, "ghlog2dt %s: %v\n", args[0], x.err)
		}
		return x.code
	}
	fmt.Fprintf(e.stderr, "ghlog2dt %s: %v\n", args[0], err)
	return codeFor(err)
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "usage: ghlog2dt <command> [flags]\n\ncommands:\n")
	for _, name := range []string{"run", "list", "fetch", "ship", "checkpoint", "token", "schedule", "version"} {
		fmt.Fprintf(w, "  %-11s %s\n", name, commands[name].help)
	}
	fmt.Fprintf(w, "\nrun 'ghlog2dt <command> -h' for flags; see ghlog2dt(1).\n")
}

// exitError carries an explicit exit status.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err == nil {
		return fmt.Sprintf("exit %d", e.code)
	}
	return e.err.Error()
}

func (e *exitError) Unwrap() error { return e.err }

func usageErr(format string, a ...any) error {
	return &exitError{exitUsage, fmt.Errorf(format, a...)}
}
