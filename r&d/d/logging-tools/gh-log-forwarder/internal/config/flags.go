package config

import (
	"flag"
	"path"
	"strings"
	"time"
)

// matchGlob is path.Match, case-insensitively (GitHub names are).
func matchGlob(pattern, name string) (bool, error) {
	return path.Match(strings.ToLower(pattern), strings.ToLower(name))
}

// listFlag is a repeatable string flag. The first Set replaces any
// value that came from the file or environment.
type listFlag struct {
	dst   *[]string
	fresh bool
}

func (l *listFlag) String() string {
	if l.dst == nil {
		return ""
	}
	return strings.Join(*l.dst, ",")
}

func (l *listFlag) Set(v string) error {
	if !l.fresh {
		*l.dst = nil
		l.fresh = true
	}
	*l.dst = append(*l.dst, SplitList(v)...)
	return nil
}

// kvFlag is a repeatable key=value flag merged into a map.
type kvFlag struct{ dst map[string]string }

func (k kvFlag) String() string { return "" }

func (k kvFlag) Set(v string) error {
	key, val, ok := strings.Cut(v, "=")
	if !ok || key == "" {
		return errBadKV
	}
	k.dst[key] = val
	return nil
}

type flagErr string

func (e flagErr) Error() string { return string(e) }

const errBadKV = flagErr("want key=value")

// ConfigPath finds --config / -config in args without a full parse, so
// the file can be loaded before flags overlay it. GHLOG_CONFIG is the
// fallback; the bool reports whether the user asked for it explicitly.
func ConfigPath(args []string, getenv func(string) string) (string, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		for _, p := range []string{"--config", "-config"} {
			if a == p && i+1 < len(args) {
				return args[i+1], true
			}
			if strings.HasPrefix(a, p+"=") {
				return strings.TrimPrefix(a, p+"="), true
			}
		}
	}
	if v := getenv("GHLOG_CONFIG"); v != "" {
		return v, true
	}
	return "ghlog2dt.json", false
}

// Scope selects which flag groups a subcommand exposes.
type Scope uint

const (
	ScopeGitHub Scope = 1 << iota
	ScopeDynatrace
	ScopeParse
	ScopeState
)

// BindFlags registers flags whose defaults are the current values, so an
// unset flag leaves the file/env value alone.
func (c *Config) BindFlags(fs *flag.FlagSet, s Scope) {
	var ignored string
	fs.StringVar(&ignored, "config", "ghlog2dt.json", "JSON config `file` (env GHLOG_CONFIG)")
	if s&ScopeGitHub != 0 {
		fs.Var(&listFlag{dst: &c.Targets}, "repo", "target `owner/name`; repeatable or comma separated (env GHLOG_TARGET_REPOS)")
		fs.BoolVar(&c.All, "all", c.All, "target every repository the installation token can see")
		fs.Var(&listFlag{dst: &c.Include}, "include", "glob of repos to include with --all; repeatable")
		fs.Var(&listFlag{dst: &c.Exclude}, "exclude", "glob of repos to exclude; repeatable")
		fs.Var(&listFlag{dst: &c.Workflows}, "workflow", "only workflows whose name or path matches this glob; repeatable")
		fs.DurationVar((*time.Duration)(&c.Lookback), "lookback", c.Lookback.D(), "overlap window behind the high-water mark (env GHLOG_LOOKBACK)")
		fs.DurationVar((*time.Duration)(&c.MaxAge), "max-age", c.MaxAge.D(), "never look further back than this; must stay under Dynatrace's 24h limit")
		fs.StringVar(&c.GitHubAPI, "gh-api", c.GitHubAPI, "GitHub REST base `url` (env GITHUB_API_URL)")
		fs.StringVar(&c.APIVersion, "gh-api-version", c.APIVersion, "X-GitHub-Api-Version header")
		fs.IntVar(&c.Concurrency, "concurrency", c.Concurrency, "parallel job-log downloads (env GHLOG_CONCURRENCY)")
		fs.Int64Var(&c.MaxLogBytes, "max-log-bytes", c.MaxLogBytes, "truncate a job log beyond this many bytes")
	}
	if s&ScopeParse != 0 {
		fs.IntVar(&c.MaxLineBytes, "max-line-bytes", c.MaxLineBytes, "truncate a log line beyond this many bytes")
		fs.StringVar(&c.Stale, "stale", c.Stale, "lines older than --max-age: drop | rewrite (stamp with ingest time)")
		fs.BoolVar(&c.DropNoise, "drop-noise", c.DropNoise, "drop ##[endgroup] and empty lines")
		fs.BoolVar(&c.StripANSI, "strip-ansi", c.StripANSI, "remove ANSI colour escapes from content")
		fs.Var(kvFlag{dst: c.Attributes}, "attr", "extra `key=value` attribute on every record; repeatable")
	}
	if s&ScopeDynatrace != 0 {
		fs.StringVar(&c.DT.URL, "dt-url", c.DT.URL, "Dynatrace environment `url`, e.g. https://abc12345.live.dynatrace.com (env DT_URL)")
		fs.StringVar(&c.DT.AuthScheme, "dt-auth", c.DT.AuthScheme, "Authorization scheme: Api-Token | Bearer (env DT_AUTH_SCHEME)")
		fs.IntVar(&c.DT.BatchBytes, "batch-bytes", c.DT.BatchBytes, "max bytes per ingest request (limit 10MB)")
		fs.IntVar(&c.DT.BatchRecords, "batch-records", c.DT.BatchRecords, "max records per ingest request (limit 50000)")
		fs.BoolVar(&c.DT.Gzip, "gzip", c.DT.Gzip, "gzip request bodies (UNVERIFIED on this endpoint; env GHLOG_GZIP)")
		fs.BoolVar(&c.DT.HeaderAttrs, "header-attrs", c.DT.HeaderAttrs, "send job-level attributes once per request in X-Dynatrace-Attr")
		fs.DurationVar((*time.Duration)(&c.DT.Timeout), "dt-timeout", c.DT.Timeout.D(), "per-request timeout")
		fs.IntVar(&c.DT.MaxRetries, "dt-retries", c.DT.MaxRetries, "retries on 429/502/503/504")
		fs.BoolVar(&c.SelfLog, "self-log", c.SelfLog, "ship one summary record (log.source=ghlog2dt.self) per run")
	}
	if s&ScopeState != 0 {
		fs.StringVar(&c.StateDir, "state", c.StateDir, "checkpoint `dir` (env GHLOG_STATE_DIR)")
		fs.DurationVar((*time.Duration)(&c.PendingTTL), "pending-ttl", c.PendingTTL.D(), "forget unfinished runs after this long")
		fs.DurationVar((*time.Duration)(&c.ShippedTTL), "shipped-ttl", c.ShippedTTL.D(), "remember shipped job IDs this long")
	}
}
