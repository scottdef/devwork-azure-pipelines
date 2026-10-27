package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Intervals that divide the hour evenly; GitHub's floor is 5 minutes.
var validIntervals = []int{5, 6, 10, 12, 15, 20, 30, 60}

// Schedule is one export cadence.
type Schedule struct {
	IntervalMinutes int    `json:"interval_minutes"`
	OffsetMinutes   int    `json:"offset_minutes"`
	Cron            string `json:"cron"`
	Lookback        string `json:"lookback"`
}

// ComputeSchedule derives the cron expression and the lookback window.
// Minutes avoid :00, when GitHub's scheduler is busiest and may delay or
// drop runs. Lookback = max(2*interval, 60m) + 30m: a missed or late
// tick is covered by the next one, and the pending set covers runs
// that outlive the window. Terraform's locals use the same formula.
func ComputeSchedule(interval, offset int) (Schedule, error) {
	if !slices.Contains(validIntervals, interval) {
		return Schedule{}, fmt.Errorf("interval must be one of %v minutes", validIntervals)
	}
	if offset < 0 {
		return Schedule{}, fmt.Errorf("offset must be >= 0")
	}
	off := offset % interval
	var mins []string
	for m := off; m < 60; m += interval {
		mins = append(mins, strconv.Itoa(m))
	}
	lb := time.Duration(max(2*interval, 60)+30) * time.Minute
	return Schedule{
		IntervalMinutes: interval,
		OffsetMinutes:   off,
		Cron:            strings.Join(mins, ",") + " * * * *",
		Lookback:        fmt.Sprintf("%dm", int(lb.Minutes())),
	}, nil
}

// cronLineRe finds the managed line: `- cron: "..."  # ghlog2dt:cron`.
var cronLineRe = regexp.MustCompile(`(?m)^(\s*-\s*cron:\s*)"[^"]*"(\s*#\s*ghlog2dt:cron.*)$`)

// RewriteCron replaces the managed cron line in a workflow file.
func RewriteCron(src []byte, cron string) ([]byte, error) {
	if n := len(cronLineRe.FindAll(src, -1)); n != 1 {
		return nil, fmt.Errorf("want exactly one `- cron: \"...\" # ghlog2dt:cron` line, found %d", n)
	}
	return cronLineRe.ReplaceAll(src, []byte(`${1}"`+cron+`"${2}`)), nil
}

func cmdSchedule(_ context.Context, e *env, args []string) error {
	fs := flag.NewFlagSet("ghlog2dt schedule", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	interval := fs.Int("interval", 15, "export interval in minutes: one of 5,6,10,12,15,20,30,60")
	offset := fs.Int("offset", 7, "minute offset within the interval (keep off :00)")
	rewrite := fs.String("rewrite", "", "rewrite the `# ghlog2dt:cron` line of this workflow `file` in place")
	asJSON := fs.Bool("json", false, "print JSON instead of key=value lines")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	s, err := ComputeSchedule(*interval, *offset)
	if err != nil {
		return usageErr("%v", err)
	}
	if *rewrite != "" {
		src, err := os.ReadFile(*rewrite)
		if err != nil {
			return err
		}
		out, err := RewriteCron(src, s.Cron)
		if err != nil {
			return usageErr("%s: %v", *rewrite, err)
		}
		tmp, err := os.CreateTemp(filepath.Dir(*rewrite), ".wf-*")
		if err != nil {
			return err
		}
		defer os.Remove(tmp.Name())
		if _, err := tmp.Write(out); err != nil {
			tmp.Close()
			return err
		}
		if err := tmp.Close(); err != nil {
			return err
		}
		if err := os.Chmod(tmp.Name(), 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp.Name(), *rewrite); err != nil {
			return err
		}
	}
	if *asJSON {
		return json.NewEncoder(e.stdout).Encode(s)
	}
	fmt.Fprintf(e.stdout, "interval_minutes=%d\noffset_minutes=%d\ncron=%s\nlookback=%s\n", s.IntervalMinutes, s.OffsetMinutes, s.Cron, s.Lookback)
	return nil
}
