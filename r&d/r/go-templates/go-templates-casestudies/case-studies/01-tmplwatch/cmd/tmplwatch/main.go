// Tmplwatch watches a key/value source, renders Go templates from it,
// writes the results atomically, and runs a command or signals a process
// when they change. It is a small, standard-library-only take on the
// consul-template pattern.
//
// Usage:
//
//	tmplwatch [-c config.json] [-once] [-dry] [-a addr] [-pprof] [-v]
//	tmplwatch -healthcheck [-a addr]
//	tmplwatch -version
//
// See docs/OPERATIONS.md.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/brain"
	"github.com/CoolGitOrg/tmplwatch/internal/config"
	"github.com/CoolGitOrg/tmplwatch/internal/ghapp"
	"github.com/CoolGitOrg/tmplwatch/internal/httpd"
	"github.com/CoolGitOrg/tmplwatch/internal/metrics"
	"github.com/CoolGitOrg/tmplwatch/internal/runner"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

// version is set by -ldflags "-X main.version=v1.2.3".
var version = "dev"

// Exit codes. Scripts and CI depend on them; see docs/OPERATIONS.md.
const (
	exitOK         = 0
	exitRender     = 1 // -once: a render, write or command failed
	exitUsage      = 2 // bad flags or configuration
	exitIncomplete = 3 // -once: a template is missing keys
	exitUnhealthy  = 4 // -healthcheck: not 200
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tmplwatch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("c", "tmplwatch.json", "configuration `file`")
	once := fs.Bool("once", false, "render once and exit (overrides config)")
	dry := fs.Bool("dry", false, "print diffs to stdout instead of writing (overrides config)")
	addr := fs.String("a", "", "listen `address` (overrides config listen; \"off\" disables)")
	pprofOn := fs.Bool("pprof", false, "serve /debug/pprof on the listener (loopback only)")
	health := fs.Bool("healthcheck", false, "GET /healthz on -a (or 127.0.0.1:9180) and exit 0 if healthy")
	showVer := fs.Bool("version", false, "print version and exit")
	verbose := fs.Bool("v", false, "log every poll")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: tmplwatch [-c config.json] [-once] [-dry] [-a addr] [-pprof] [-v]\n"+
			"       tmplwatch -healthcheck [-a addr]\n       tmplwatch -version\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return exitUsage
	}
	if *showVer {
		fmt.Fprintln(stdout, "tmplwatch", version)
		return exitOK
	}
	if *health {
		a := *addr
		if a == "" {
			a = "127.0.0.1:9180"
		}
		return healthcheck(a, stderr)
	}

	lg := log.New(stderr, "tmplwatch: ", log.LstdFlags|log.Lmsgprefix)
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		lg.Print(err)
		return exitUsage
	}
	if *once {
		cfg.Once = true
	}
	if *dry {
		cfg.Dry = true
	}
	if *addr != "" {
		cfg.Listen = *addr
	}
	if *pprofOn && !httpd.IsLoopback(cfg.Listen) {
		lg.Printf("-pprof refused: listener %s is not loopback", cfg.Listen)
		return exitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), runner.StopSignals()...)
	defer stop()

	src, err := newSource(ctx, cfg)
	if err != nil {
		lg.Print(err)
		return exitUsage
	}
	m := metrics.New(version)
	b := brain.New()
	r, err := runner.New(ctx, cfg, b, m, lg)
	if err != nil {
		lg.Print(err)
		return exitUsage
	}
	r.DryOut = stdout

	if cfg.Once {
		return runOnce(ctx, src, r, lg)
	}

	if len(cfg.Exec.Command) > 0 && !cfg.Dry {
		sup, err := supervisor(cfg, lg, m)
		if err != nil {
			lg.Print(err)
			return exitUsage
		}
		sup.Bind(ctx)
		r.Exec = sup
		defer sup.Wait()
	}

	var srv *http.Server
	if cfg.Listen != "off" {
		ln, err := net.Listen("tcp", cfg.Listen)
		if err != nil {
			lg.Print(err)
			return exitUsage
		}
		srv = httpd.Server(cfg.Listen, httpd.Handler(httpd.Options{State: r, Metrics: m, Pprof: *pprofOn}))
		go func() {
			if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				lg.Printf("http: %v", err)
			}
		}()
		lg.Printf("listening on http://%s", ln.Addr())
	}

	reload := make(chan os.Signal, 1)
	signal.Notify(reload, runner.ReloadSignals()...)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reload:
				lg.Print("reload: re-reading templates")
				r.Reload()
			}
		}
	}()

	lg.Printf("%s: watching %s every %v; %d templates; dry=%v", version, src, cfg.Source.Interval.D(), len(cfg.Templates), cfg.Dry)
	source.Watch(ctx, src, cfg.Source.Interval.D(), func(s source.Snapshot) {
		o := r.Update(s)
		lg.Printf("snapshot %s: %d keys, rendered %d, wrote %d", short(s.Version), len(s.Data), len(o.Rendered), len(o.Written))
	}, func(result string, err error) {
		m.Poll(result)
		if err != nil {
			lg.Printf("poll %s: %v", src, err)
		} else if *verbose {
			lg.Printf("poll %s: %s", src, result)
		}
	})

	lg.Print("shutting down")
	r.Stop()
	if srv != nil {
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}
	return exitOK
}

// runOnce does a single fetch and render: the CI and cron mode.
func runOnce(ctx context.Context, src source.Source, r *runner.Runner, lg *log.Logger) int {
	s, _, err := src.Fetch(ctx)
	if err != nil {
		lg.Print(err)
		return exitRender
	}
	o := r.Update(s)
	lg.Printf("once: %d keys, rendered %d, wrote %d, incomplete %d, errors %d",
		len(s.Data), len(o.Rendered), len(o.Written), len(o.Incomplete), len(o.Errors))
	switch {
	case len(o.Errors) > 0:
		for _, e := range o.Errors {
			lg.Print(e)
		}
		return exitRender
	case len(o.Incomplete) > 0:
		lg.Printf("incomplete: %s", strings.Join(o.Incomplete, " "))
		return exitIncomplete
	}
	return exitOK
}

func newSource(ctx context.Context, cfg *config.Config) (source.Source, error) {
	s := cfg.Source
	if s.Type == "dir" {
		return source.NewDir(s.Path, s.Prefix), nil
	}
	tok, err := tokenFunc(s)
	if err != nil {
		return nil, err
	}
	g, err := source.NewGitHub(s.APIURL, s.Repo, s.Ref, s.Prefix, tok)
	if err != nil {
		return nil, err
	}
	g.Agent = "tmplwatch/" + version
	return g, nil
}

// tokenFunc chooses GitHub App credentials when configured, else a
// static token from the environment.
func tokenFunc(s config.Source) (source.TokenFunc, error) {
	id := s.AppClientID
	if id == "" && s.AppClientIDEnv != "" {
		id = os.Getenv(s.AppClientIDEnv)
		if id == "" {
			return nil, fmt.Errorf("source.app_client_id_env: $%s is empty", s.AppClientIDEnv)
		}
	}
	if id == "" {
		return staticToken(s.TokenEnv), nil
	}
	var pemBytes []byte
	switch {
	case s.AppPrivateKeyEnv != "":
		pemBytes = []byte(os.Getenv(s.AppPrivateKeyEnv))
	case s.AppPrivateKeyFile != "":
		b, err := os.ReadFile(s.AppPrivateKeyFile)
		if err != nil {
			return nil, err
		}
		pemBytes = b
	default:
		return nil, errors.New("GitHub App auth needs app_private_key_file or app_private_key_env")
	}
	key, err := ghapp.ParseKey(pemBytes)
	if err != nil {
		return nil, err
	}
	owner, repo, _ := strings.Cut(s.Repo, "/")
	m := &ghapp.Minter{API: s.APIURL, ClientID: id, Key: key, Owner: owner, Repo: repo}
	return m.Token, nil
}

// staticToken reads the token from $name, or from the file named by
// $name_FILE (systemd LoadCredential, Docker/Kubernetes secrets). The file
// is re-read on every request so a rotated token is picked up without a
// restart.
func staticToken(name string) source.TokenFunc {
	file := os.Getenv(name + "_FILE")
	if file == "" && os.Getenv(name) == "" {
		fmt.Fprintf(os.Stderr, "tmplwatch: warning: $%s and $%s_FILE are empty; polling anonymously (60 requests/hour)\n", name, name)
	}
	return func(context.Context) (string, error) {
		if file == "" {
			return os.Getenv(name), nil
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
}

func supervisor(cfg *config.Config, lg *log.Logger, m *metrics.Metrics) (*runner.Supervisor, error) {
	e := cfg.Exec
	s := &runner.Supervisor{Args: e.Command, KillTimeout: e.KillTimeout.D(), Log: lg, OnRestart: m.ExecRestart}
	if e.ChangeSignal != "" {
		sig, err := runner.ParseSignal(e.ChangeSignal)
		if err != nil {
			return nil, fmt.Errorf("exec.change_signal: %w", err)
		}
		s.ChangeSig = sig
	}
	sig, err := runner.ParseSignal(e.KillSignal)
	if err != nil {
		return nil, fmt.Errorf("exec.kill_signal: %w", err)
	}
	s.KillSig = sig
	return s, nil
}

// healthcheck is for container HEALTHCHECK lines: a scratch image has no
// curl, so the binary checks itself.
func healthcheck(addr string, stderr io.Writer) int {
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Get("http://" + addr + "/healthz")
	if err != nil {
		fmt.Fprintln(stderr, "tmplwatch: healthcheck:", err)
		return exitUnhealthy
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "tmplwatch: healthcheck: %s: %s", resp.Status, body)
		return exitUnhealthy
	}
	return exitOK
}

func short(v string) string {
	if len(v) > 12 {
		return v[:12]
	}
	return v
}
