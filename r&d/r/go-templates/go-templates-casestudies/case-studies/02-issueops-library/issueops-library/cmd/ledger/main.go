// Command ledger edits and reports on the library's loan file.
//
// Usage:
//
//	ledger [-f ledger/ledger.json] [-items items] borrow -item ID -by LOGIN -days 7|14 -issue N [-reason TEXT]
//	ledger [-f ...] return -item ID -by LOGIN [-force]
//	ledger [-f ...] overdue [-grace 7d] [-n]
//	ledger [-f ...] list [-json|-table] [-all]
//	ledger [-f ...] render -t template.tmpl [-d extra.json]
//
// borrow, return and overdue print a JSON result on stdout (the data for
// comment templates). A refusal by a library rule exits 3 and the JSON
// says why; usage errors exit 2; anything else exits 1.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/event"
	"github.com/CoolGitOrg/issueops-library/internal/funcs"
	"github.com/CoolGitOrg/issueops-library/internal/item"
	"github.com/CoolGitOrg/issueops-library/internal/ledger"

	"text/template"
)

var version = "dev"

// Result is the JSON printed by borrow and return.
type Result struct {
	Op     string       `json:"op"`
	OK     bool         `json:"ok"`
	Error  string       `json:"error,omitempty"`
	Item   string       `json:"item"`
	By     string       `json:"by"`
	Issue  int          `json:"issue,omitempty"`
	Loan   *ledger.Loan `json:"loan,omitempty"`
	Held   int          `json:"held"`
	Limit  int          `json:"limit"`
	ItemOK bool         `json:"item_exists"`
}

// errRefused marks rule refusals.
var errRefused = errors.New("refused")

// env is what a subcommand needs; tests build one directly.
type env struct {
	file, items string
	now         time.Time
	stdout      io.Writer
	stderr      io.Writer
}

func main() {
	file := flag.String("f", "ledger/ledger.json", "ledger `file`")
	items := flag.String("items", "items", "items `dir`, to check that borrowed items exist")
	stamp := flag.String("time", "", "pretend the time is `t` (RFC 3339); for tests and replays")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	e := env{file: *file, items: *items, now: time.Now().UTC(), stdout: os.Stdout, stderr: os.Stderr}
	if *stamp != "" {
		t, err := time.Parse(time.RFC3339, *stamp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ledger: -time: %v\n", err)
			os.Exit(2)
		}
		e.now = t
	}
	os.Exit(run(e, flag.Args()))
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: ledger [-f file] [-items dir] [-time t] command [flags]
commands:
  borrow  -item ID -by LOGIN -days 7|14 -issue N [-reason TEXT]
  return  -item ID -by LOGIN [-force]
  overdue [-grace 7d] [-n]
  list    [-json|-table] [-all]
  render  -t template [-d data.json]`)
	flag.PrintDefaults()
}

// run dispatches a subcommand and returns the exit status.
func run(e env, args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}
	var err error
	switch args[0] {
	case "borrow":
		err = borrow(e, args[1:])
	case "return":
		err = giveBack(e, args[1:])
	case "overdue":
		err = overdue(e, args[1:])
	case "list":
		err = list(e, args[1:])
	case "render":
		err = render(e, args[1:])
	default:
		fmt.Fprintf(e.stderr, "ledger: unknown command %q\n", args[0])
		return 2
	}
	var ue usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &ue):
		fmt.Fprintf(e.stderr, "ledger %s: %v\n", args[0], err)
		return 2
	case errors.Is(err, errRefused):
		fmt.Fprintf(e.stderr, "ledger %s: %v\n", args[0], err)
		return 3
	}
	fmt.Fprintf(e.stderr, "ledger %s: %v\n", args[0], err)
	return 1
}

type usageError struct{ error }

func parse(fs *flag.FlagSet, args []string) error {
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return usageError{err}
	}
	if fs.NArg() != 0 {
		return usageError{fmt.Errorf("unexpected argument %q", fs.Arg(0))}
	}
	return nil
}

func borrow(e env, args []string) error {
	fs := flag.NewFlagSet("borrow", flag.ContinueOnError)
	id := fs.String("item", "", "item id")
	by := fs.String("by", "", "borrower login")
	days := fs.Int("days", 7, "loan period, 7 or 14")
	issue := fs.Int("issue", 0, "borrow issue number")
	reason := fs.String("reason", "", "free text")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *id == "" || *by == "" || *issue == 0 {
		return usageError{errors.New("-item, -by and -issue are required")}
	}
	l, err := ledger.Load(e.file)
	if err != nil {
		return err
	}
	res := Result{Op: "borrow", Item: *id, By: *by, Issue: *issue, Limit: ledger.MaxOpen}
	clean, cerr := event.CleanItemID(*id)
	res.ItemOK = cerr == nil && item.Exists(e.items, clean)
	if res.ItemOK {
		// Phase 3 is the last: there is no next phase to reserve.
		if ph, err := item.PhaseOf(e.items, clean); err != nil {
			return err
		} else if ph >= 3 {
			res.Error = fmt.Sprintf("ledger: %s is phase %d, the final phase; nothing to borrow", clean, ph)
			return emit(e.stdout, res, fmt.Errorf("%w: %s", errRefused, res.Error))
		}
	}
	loan, err := l.Borrow(clean, *by, *days, *issue, *reason, res.ItemOK, e.now)
	res.Held = len(l.OpenBy(*by))
	if err != nil {
		res.Error = err.Error()
		return emit(e.stdout, res, fmt.Errorf("%w: %v", errRefused, err))
	}
	if err := l.Save(e.file); err != nil {
		return err
	}
	res.OK, res.Loan = true, loan
	return emit(e.stdout, res, nil)
}

func giveBack(e env, args []string) error {
	fs := flag.NewFlagSet("return", flag.ContinueOnError)
	id := fs.String("item", "", "item id")
	by := fs.String("by", "", "login returning the item")
	force := fs.Bool("force", false, "return even if -by is not the borrower (operators)")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *id == "" || *by == "" {
		return usageError{errors.New("-item and -by are required")}
	}
	l, err := ledger.Load(e.file)
	if err != nil {
		return err
	}
	res := Result{Op: "return", Item: *id, By: *by, Limit: ledger.MaxOpen}
	res.ItemOK = item.Exists(e.items, *id)
	loan, err := l.Return(strings.TrimSpace(*id), *by, *force, e.now)
	if err != nil {
		res.Error = err.Error()
		res.Held = len(l.OpenBy(*by))
		return emit(e.stdout, res, fmt.Errorf("%w: %v", errRefused, err))
	}
	if err := l.Save(e.file); err != nil {
		return err
	}
	res.OK, res.Loan, res.Held = true, loan, len(l.OpenBy(*by))
	return emit(e.stdout, res, nil)
}

// OverdueReport is printed by overdue.
type OverdueReport struct {
	Now     time.Time       `json:"now"`
	Grace   string          `json:"grace"`
	Applied bool            `json:"applied"`
	Notices []ledger.Notice `json:"notices"`
}

func overdue(e env, args []string) error {
	fs := flag.NewFlagSet("overdue", flag.ContinueOnError)
	graceS := fs.String("grace", "7d", "time after due before auto-return (e.g. 7d, 36h)")
	dry := fs.Bool("n", false, "report only; do not change the ledger")
	if err := parse(fs, args); err != nil {
		return err
	}
	grace, err := parseDuration(*graceS)
	if err != nil {
		return usageError{err}
	}
	l, err := ledger.Load(e.file)
	if err != nil {
		return err
	}
	ns := l.Overdue(e.now, grace, !*dry)
	rep := OverdueReport{Now: e.now.UTC().Truncate(time.Second), Grace: *graceS, Applied: !*dry, Notices: ns}
	if rep.Notices == nil {
		rep.Notices = []ledger.Notice{}
	}
	changed := false
	for _, n := range ns {
		changed = changed || n.Action == "return"
	}
	if changed && !*dry {
		if err := l.Save(e.file); err != nil {
			return err
		}
	}
	return emit(e.stdout, rep, nil)
}

// parseDuration accepts Go durations plus a "d" (day) suffix.
func parseDuration(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(s, "d"); ok {
		d, err := strconv.Atoi(n)
		if err != nil || d < 0 {
			return 0, fmt.Errorf("bad duration %q", s)
		}
		return time.Duration(d) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("bad duration %q", s)
	}
	return d, nil
}

func list(e env, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "JSON output")
	_ = fs.Bool("table", true, "aligned table output (default)")
	all := fs.Bool("all", false, "include returned loans")
	if err := parse(fs, args); err != nil {
		return err
	}
	l, err := ledger.Load(e.file)
	if err != nil {
		return err
	}
	loans := l.OpenLoans()
	if *all {
		loans = l.History()
	}
	if *asJSON {
		if loans == nil {
			loans = []*ledger.Loan{}
		}
		return emit(e.stdout, loans, nil)
	}
	tw := tabwriter.NewWriter(e.stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintln(tw, "ITEM\tBORROWER\tISSUE\tBORROWED\tDUE\tSTATE")
	for _, x := range loans {
		state := "out"
		switch {
		case !x.Open():
			state = "returned " + x.ReturnedAt.Format("2006-01-02")
			if x.ReturnedBy == "overdue" {
				state += " (auto)"
			}
		case e.now.After(x.Due):
			state = fmt.Sprintf("overdue %dd", int(e.now.Sub(x.Due)/(24*time.Hour)))
		}
		fmt.Fprintf(tw, "%s\t%s\t#%d\t%s\t%s\t%s\n", x.Item, x.Borrower, x.BorrowIssue,
			x.BorrowedAt.Format("2006-01-02"), x.Due.Format("2006-01-02"), state)
	}
	return tw.Flush()
}

// render runs a text/template over the ledger: {{.Open}}, {{.History}},
// {{.Now}}, {{.Data}} (from -d). The overdue workflow uses it to write
// warning comments.
func render(e env, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	tpath := fs.String("t", "", "template file")
	dpath := fs.String("d", "", "extra JSON data, available as .Data")
	if err := parse(fs, args); err != nil {
		return err
	}
	if *tpath == "" {
		return usageError{errors.New("-t is required")}
	}
	l, err := ledger.Load(e.file)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(*tpath)
	if err != nil {
		return err
	}
	t, err := template.New(*tpath).Funcs(funcs.Map()).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return err
	}
	var data any
	if *dpath != "" {
		b, err := os.ReadFile(*dpath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &data); err != nil {
			return fmt.Errorf("%s: %w", *dpath, err)
		}
	}
	return t.Execute(e.stdout, map[string]any{
		"Open": l.OpenLoans(), "History": l.History(), "Now": e.now, "Data": data, "Limit": ledger.MaxOpen,
	})
}

func emit(w io.Writer, v any, err error) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if e := enc.Encode(v); e != nil {
		return e
	}
	return err
}
