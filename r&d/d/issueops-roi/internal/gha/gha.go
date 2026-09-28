// Package gha contains helpers for running inside GitHub Actions: step outputs
// (always written with a random heredoc delimiter so untrusted multi-line
// values cannot inject additional outputs), job summaries, masks and
// workflow-command annotations.
package gha

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

// Stdout is where annotations and fallback outputs are written. Tests replace it.
var Stdout io.Writer = os.Stdout

// InActions reports whether the process runs inside a GitHub Actions job.
func InActions() bool { return os.Getenv("GITHUB_ACTIONS") == "true" }

func delimiter() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing is fatal for security-sensitive delimiters.
		panic(fmt.Sprintf("gha: crypto/rand: %v", err))
	}
	return "ISSUEOPS_EOF_" + hex.EncodeToString(b)
}

// SetOutput writes a step output. Every value uses the heredoc form with a
// random delimiter, so a value containing newlines or a guessed delimiter can
// never smuggle extra outputs into $GITHUB_OUTPUT.
func SetOutput(name, value string) error {
	return appendKV(os.Getenv("GITHUB_OUTPUT"), name, value)
}

// SetEnv exports an environment variable to subsequent steps.
func SetEnv(name, value string) error {
	return appendKV(os.Getenv("GITHUB_ENV"), name, value)
}

func appendKV(path, name, value string) error {
	if strings.ContainsAny(name, "\r\n=<") || name == "" {
		return fmt.Errorf("gha: invalid key %q", name)
	}
	if path == "" {
		// Local run: print so the operator can see what would be set.
		_, err := fmt.Fprintf(Stdout, "[output] %s=%s\n", name, value)
		return err
	}
	d := delimiter()
	for strings.Contains(value, d) {
		d = delimiter()
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", name, d, value, d)
	return err
}

// AppendSummary appends Markdown to the job summary.
func AppendSummary(markdown string) error {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, markdown+"\n")
	return err
}

// Mask registers a secret value so the runner redacts it from logs.
func Mask(value string) {
	if value == "" {
		return
	}
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintf(Stdout, "::add-mask::%s\n", escapeData(line))
		}
	}
}

// Notice, Warning and Error emit workflow-command annotations.
func Notice(msg string)  { annotate("notice", msg) }
func Warning(msg string) { annotate("warning", msg) }
func Error(msg string)   { annotate("error", msg) }

func annotate(level, msg string) {
	if !InActions() {
		fmt.Fprintf(Stdout, "%s: %s\n", strings.ToUpper(level), msg)
		return
	}
	fmt.Fprintf(Stdout, "::%s::%s\n", level, escapeData(msg))
}

// escapeData implements the workflow-command data escaping rules.
func escapeData(s string) string {
	r := strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	return r.Replace(s)
}

// Group wraps log output in a collapsible group.
func Group(title string, fn func() error) error {
	if InActions() {
		fmt.Fprintf(Stdout, "::group::%s\n", escapeData(title))
		defer fmt.Fprintln(Stdout, "::endgroup::")
	}
	return fn()
}
