// Command pagegen applies a library event and rebuilds the Pages site.
//
// Usage:
//
//	printf '%s' "$EVENT" | pagegen -event - -site site -items items -ledger ledger/ledger.json
//	pagegen -site site -items items -ledger ledger/ledger.json        # rebuild only
//	printf '%s' "$EVENT" | pagegen -event - -describe                 # print KEY=value for $GITHUB_OUTPUT
//
// EVENT is the whole github.event object of a repository_dispatch run.
// For library-publish, pagegen renders the item into items/; borrow and
// return are applied by the ledger command before pagegen runs, so here
// they only trigger the rebuild. Either way the site is rebuilt in full.
//
// pagegen prints a JSON result on stdout (the data for the comment
// templates) and exits 0 on success, 3 when the event was refused by a
// library rule (the result says why), 1 on any other error.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoolGitOrg/issueops-library/internal/event"
	"github.com/CoolGitOrg/issueops-library/internal/item"
	"github.com/CoolGitOrg/issueops-library/internal/ledger"
	"github.com/CoolGitOrg/issueops-library/internal/site"
)

var version = "dev"

// Result is printed on stdout.
type Result struct {
	Action    string `json:"action"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	ItemID    string `json:"item_id,omitempty"`
	ItemURL   string `json:"item_url,omitempty"`
	Issue     int    `json:"issue,omitempty"`
	Repo      string `json:"repo,omitempty"`
	Requester string `json:"requester,omitempty"`
	Phase     int    `json:"phase,omitempty"`
	Artifact  string `json:"artifact,omitempty"`
	// ClosedBorrow is the borrow issue whose loan this publish returned.
	ClosedBorrow int `json:"closed_borrow_issue,omitempty"`
	Items        int `json:"items"`
	Files        int `json:"files"`
}

type options struct {
	event, siteDir, items, ledger, templates, meta string
	cfg                                            site.Config
	describe                                       bool
	now                                            time.Time
}

// errRefused marks a rule refusal (exit 3) as opposed to a failure.
var errRefused = errors.New("refused")

func main() {
	var o options
	flag.StringVar(&o.event, "event", "", "event JSON `file` ('-' for stdin); empty means rebuild only")
	flag.StringVar(&o.siteDir, "site", "site", "output site `dir` (replaced)")
	flag.StringVar(&o.items, "items", "items", "items `dir`")
	flag.StringVar(&o.ledger, "ledger", "ledger/ledger.json", "ledger `file`")
	flag.StringVar(&o.templates, "templates", "templates/items", "item templates `dir`")
	flag.StringVar(&o.meta, "meta", "templates/meta", "meta-templates `dir`")
	flag.StringVar(&o.cfg.BaseURL, "base", "https://coolgitorg.github.io/issueops-library/", "site base `url`")
	flag.StringVar(&o.cfg.Repo, "repo", envOr("GITHUB_REPOSITORY", "CoolGitOrg/issueops-library"), "library `owner/name`")
	flag.StringVar(&o.cfg.IntakeRepo, "intake", "CoolGitOrg/issueops-intake", "intake `owner/name`")
	flag.StringVar(&o.cfg.IntakeURL, "intake-url", "https://coolgitorg.github.io/issueops-intake/", "intake site `url`")
	flag.BoolVar(&o.describe, "describe", false, "validate the event and print KEY=value lines; change nothing")
	stamp := flag.String("time", "", "fixed build `time` (RFC 3339) for reproducible output")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if flag.NArg() != 0 || (o.describe && o.event == "") {
		fmt.Fprintln(os.Stderr, "usage: pagegen [-event file|-] [-describe] [-site dir] [-items dir] [-ledger file]")
		flag.PrintDefaults()
		os.Exit(2)
	}
	o.cfg.GitHub, o.cfg.Version = "https://github.com", version
	o.now = time.Now().UTC().Truncate(time.Second) // library.json "generated" is compared as text by the notify job
	if *stamp != "" {
		t, err := time.Parse(time.RFC3339, *stamp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pagegen: -time: %v\n", err)
			os.Exit(2)
		}
		o.now = t
	}
	o.cfg.Now = o.now
	res, err := run(o, os.Stdin, os.Stdout)
	switch {
	case errors.Is(err, errRefused):
		fmt.Fprintf(os.Stderr, "pagegen: refused: %s\n", res.Error)
		os.Exit(3)
	case err != nil:
		fmt.Fprintf(os.Stderr, "pagegen: %v\n", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func run(o options, stdin io.Reader, stdout io.Writer) (Result, error) {
	res := Result{Action: "rebuild"}
	var ev *event.Event
	if o.event != "" {
		in := stdin
		if o.event != "-" {
			f, err := os.Open(o.event)
			if err != nil {
				return res, err
			}
			defer f.Close()
			in = f
		}
		var err error
		if ev, err = event.Decode(in); err != nil {
			return res, err
		}
		res.Action, res.Issue, res.Repo, res.Requester, res.ItemID = ev.Action, ev.Issue, ev.Repo, ev.Requester, ev.ItemID
		res.Phase = ev.Phase
	}
	if o.describe {
		return res, describe(stdout, ev)
	}
	l, err := ledger.Load(o.ledger)
	if err != nil {
		return res, err
	}
	if ev != nil && ev.Action == event.Publish {
		if err := publish(o, ev, l, &res); err != nil {
			if errors.Is(err, errRefused) {
				return res, emit(stdout, res, err)
			}
			return res, err
		}
	}
	items, err := item.Load(o.items)
	if err != nil {
		return res, err
	}
	s := site.New(o.cfg, o.items, items, l)
	files, err := s.Files()
	if err != nil {
		return res, err
	}
	if err := s.Write(o.siteDir); err != nil {
		return res, err
	}
	res.OK, res.Items, res.Files = true, len(items), len(files)
	if res.ItemID != "" {
		res.ItemURL = strings.TrimRight(o.cfg.BaseURL, "/") + "/items/" + res.ItemID + "/"
	}
	return res, emit(stdout, res, nil)
}

// publish renders the item. A phase 2 or 3 request must come from whoever
// holds the loan on the previous phase of the same source and template,
// if anyone does; publishing closes that loan.
func publish(o options, ev *event.Event, l *ledger.Ledger, res *Result) error {
	var held *ledger.Loan
	if ev.Phase > 1 {
		prev, err := predecessors(o.items, ev)
		if err != nil {
			return err
		}
		for _, id := range prev {
			loan := l.OpenLoan(id)
			if loan == nil {
				continue
			}
			if !strings.EqualFold(loan.Borrower, ev.Requester) {
				res.Error = fmt.Sprintf("phase %d of %s/%s is reserved by %s until %s (loan on %s)",
					ev.Phase, ev.SourceID, ev.TemplateID, loan.Borrower, loan.Due.Format("2006-01-02"), id)
				return errRefused
			}
			held = loan
		}
	}
	r := &item.Renderer{TemplateDir: o.templates, MetaDir: o.meta, Now: func() time.Time { return o.now }}
	supersedes := ""
	if held != nil {
		supersedes = held.Item
	}
	it, files, err := r.Render(ev, supersedes)
	if err != nil {
		// A template error is the requester's payload missing a key, or
		// ours; either way the requester needs to hear it.
		res.Error = err.Error()
		return fmt.Errorf("%w: %v", errRefused, err)
	}
	if held != nil {
		// Publishing the next phase is how a borrower gives the item back.
		if _, err := l.Return(held.Item, ev.Requester, false, o.now); err != nil {
			return err
		}
		res.ClosedBorrow = held.BorrowIssue
		if err := l.Save(o.ledger); err != nil {
			return err
		}
	}
	if err := item.Write(o.items, it, files); err != nil {
		return err
	}
	res.Artifact = it.Artifact
	return nil
}

// predecessors lists items of the same source and template one phase back.
func predecessors(root string, ev *event.Event) ([]string, error) {
	cfgs, err := filepath.Glob(filepath.Join(root, ev.SourceID, ev.TemplateID, "*", "config.json"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range cfgs {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var c item.Config
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if c.Phase == ev.Phase-1 {
			out = append(out, c.ID)
		}
	}
	return out, nil
}

func emit(w io.Writer, res Result, err error) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if e := enc.Encode(res); e != nil {
		return e
	}
	return err
}

// describe prints validated event fields as KEY=value lines. Every value
// has passed event.Decode's checks (fixed alphabets, bounded lengths), so
// they are safe to append to $GITHUB_OUTPUT. Free text (reason, payload)
// is deliberately not printed.
func describe(w io.Writer, ev *event.Event) error {
	_, err := fmt.Fprintf(w, "action=%s\nissue=%d\nrepo=%s\nrequester=%s\nitem=%s\ndays=%d\nphase=%d\n",
		ev.Action, ev.Issue, ev.Repo, ev.Requester, ev.ItemID, ev.Days, ev.Phase)
	return err
}
