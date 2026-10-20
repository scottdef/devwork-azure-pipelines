// Command dispatch sends a parsed, validated issue to the library repo as
// a repository_dispatch event.
//
// Usage:
//
//	GH_TOKEN=... dispatch -repo CoolGitOrg/issueops-library -event library-publish \
//	    -issue 42 -source payments-api -requester alice req.json > receipt.json
//
// On success it prints a JSON receipt (the data for
// templates/comment-dispatched.md.tmpl). With -n it prints the request
// body it would send and does not touch the network or need a token.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/CoolGitOrg/issueops-intake/internal/dispatch"
)

var version = "dev"

// Receipt is printed on success.
type Receipt struct {
	Event      string `json:"event"`
	Repo       string `json:"repo"`
	Issue      int    `json:"issue"`
	SourceID   string `json:"source_id,omitempty"`
	TemplateID string `json:"template_id,omitempty"`
	Phase      string `json:"phase,omitempty"`
	ItemID     string `json:"item_id"`
	ItemURL    string `json:"item_url"`
	SHA256     string `json:"sha256,omitempty"`
	Keys       int    `json:"keys"`
	DryRun     bool   `json:"dry_run"`
}

type options struct {
	repo, event, source, requester, origin, api, base, token string
	issue                                                    int
	dry                                                      bool
	timeout                                                  time.Duration
}

func main() {
	var o options
	flag.StringVar(&o.repo, "repo", "", "target `owner/name` (the library repo)")
	flag.StringVar(&o.event, "event", dispatch.EventPublish, "event `type`: library-publish, library-borrow, library-return")
	flag.IntVar(&o.issue, "issue", 0, "originating issue `number`")
	flag.StringVar(&o.source, "source", "", "expected source-id; must match the document when set")
	flag.StringVar(&o.requester, "requester", os.Getenv("ISSUE_AUTHOR"), "issue author `login` (default $ISSUE_AUTHOR)")
	flag.StringVar(&o.origin, "origin", os.Getenv("GITHUB_REPOSITORY"), "intake `owner/name` (default $GITHUB_REPOSITORY)")
	flag.StringVar(&o.api, "api", envOr("GITHUB_API_URL", "https://api.github.com"), "API base `url`")
	flag.StringVar(&o.base, "base", "https://coolgitorg.github.io/issueops-library/", "library Pages base `url` for the receipt")
	flag.BoolVar(&o.dry, "n", false, "print the request body; do not send")
	flag.DurationVar(&o.timeout, "t", 2*time.Minute, "overall `timeout`")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: dispatch -repo owner/name -event type -issue N [-source id] [-n] [req.json|-]\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if o.repo == "" || o.issue <= 0 || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	o.token = os.Getenv("GH_TOKEN")
	if err := run(o, flag.Arg(0), os.Stdin, os.Stdout, nil); err != nil {
		fmt.Fprintf(os.Stderr, "dispatch: %v\n", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// run does the work; c may be supplied by tests.
func run(o options, docPath string, stdin io.Reader, stdout io.Writer, c *dispatch.Client) error {
	in := stdin
	if docPath != "" && docPath != "-" {
		f, err := os.Open(docPath)
		if err != nil {
			return err
		}
		defer f.Close()
		in = f
	}
	var req map[string]any
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return fmt.Errorf("reading document: %w", err)
	}
	if o.source != "" {
		if got, _ := req["source-id"].(string); strings.TrimSpace(got) != o.source {
			return fmt.Errorf("-source %q does not match document source-id %q", o.source, got)
		}
	}
	p, err := dispatch.Build(o.event, req, dispatch.Meta{Issue: o.issue, Repo: o.origin, Requester: o.requester})
	if err != nil {
		return err
	}
	rc := Receipt{Event: o.event, Repo: o.repo, Issue: o.issue, Keys: len(p), DryRun: o.dry}
	if o.event == dispatch.EventPublish {
		rc.SourceID, rc.TemplateID, rc.Phase = p["source_id"].(string), p["template_id"].(string), p["phase"].(string)
		rc.SHA256 = p["sha256"].(string)
		rc.ItemID = fmt.Sprintf("%s/%s/%d", rc.SourceID, rc.TemplateID, o.issue)
	} else {
		rc.ItemID = p["item_id"].(string)
	}
	rc.ItemURL = strings.TrimRight(o.base, "/") + "/items/" + rc.ItemID + "/"

	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if o.dry {
		return enc.Encode(map[string]any{"event_type": o.event, "client_payload": p, "receipt": rc})
	}
	if c == nil {
		c = dispatch.New(o.api, o.token)
		c.Agent += "/" + version
	}
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	if err := c.Send(ctx, o.repo, o.event, p); err != nil {
		return err
	}
	return enc.Encode(rc)
}
