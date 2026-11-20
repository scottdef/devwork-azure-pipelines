// Coolctl is the command-line tool of the CoolDeploy platform.
//
// Developers use it to scaffold and check an app. Workflow jobs use it to
// talk to cool-api. It is built from the standard library alone.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

type command struct {
	run  func(ctx context.Context, args []string) error
	help string
}

var commands map[string]command

func init() {
	commands = map[string]command{
		"version":  {func(context.Context, []string) error { fmt.Println("coolctl", version); return nil }, "print the version"},
		"init":     {cmdInit, "scaffold cool.json and the caller workflow"},
		"validate": {cmdValidate, "check cool.json"},
		"render":   {cmdRender, "render a dockerfile, workflow, dashboard, portal or chart"},
		"issue":    {cmdIssue, "read the fields of an issue-form body"},
		"call":     {cmdCall, "call a URL with a bearer token; used by cron pods"},
		"serve":    {cmdServe, "serve a directory over HTTP, for local previews of the portal"},
		"deploy":   {cmdDeploy, "send a built image to cool-api as a new release"},
		"preview":  {cmdPreview, "remove a pull-request preview"},
		"promote":  {cmdPromote, "copy a release from one environment to the next"},
		"rollback": {cmdRollback, "swap the active release with the standby"},
		"weights":  {cmdWeights, "set a release's share of traffic in a rolling release"},
		"config":   {cmdConfig, "set overrides, domains or the identity of an app in an environment"},
		"register": {cmdRegister, "register a new app"},
		"retire":   {cmdRetire, "remove an app everywhere"},
		"status":   {cmdStatus, "show an app's releases in every environment"},
		"wait":     {cmdWait, "wait until Argo CD reports a release healthy"},
		"open":     {cmdOpen, "print an app's URL"},
		"gate":     {cmdGate, "check a release against its error budget"},
		"logs":     {cmdLogs, "tail an app's logs with kubectl"},
	}
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help" {
		usage()
		return
	}
	c, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "coolctl: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := c.run(ctx, os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "coolctl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: coolctl <command> [flags]\n\ncommands:")
	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(os.Stderr, "  %-9s %s\n", n, commands[n].help)
	}
	fmt.Fprintln(os.Stderr, "\nRun coolctl <command> -h for the flags of a command.")
}

// flags returns a flag set that prints usage for one command.
func flags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet("coolctl "+name, flag.ExitOnError)
	return fs
}

// list is a flag that may be given more than once.
type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(s string) error { *l = append(*l, s); return nil }

// splitApp splits "team/app".
func splitApp(s string) (team, app string, err error) {
	team, app, ok := strings.Cut(s, "/")
	if !ok || team == "" || app == "" {
		return "", "", fmt.Errorf("-app %q: want team/app", s)
	}
	return team, app, nil
}

// printJSON writes v to standard output, indented, without HTML escaping.
func printJSON(v any) error {
	e := json.NewEncoder(os.Stdout)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	return e.Encode(v)
}

// output writes b to the named file, or to standard output for "" or "-".
func output(path string, b []byte) error {
	if path == "" || path == "-" {
		_, err := os.Stdout.Write(b)
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
