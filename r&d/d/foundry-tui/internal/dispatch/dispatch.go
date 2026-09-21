// Package dispatch is the write path, and the only one.
//
// A change is: parameters → Go template → JSON object of strings →
// "gh workflow run FILE --json" on standard input. The workflow does the
// work under its own identity; this program never touches Azure with
// intent. Every request is validated twice (here, and again in the
// workflow) and logged twice (the local audit chain, and the run).
package dispatch

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
)

// MaxInputs is kept at GitHub's historical workflow_dispatch limit so
// the templates work on every GitHub Enterprise version.
const MaxInputs = 10

var (
	reDeployment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
	reToken      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	reFormat     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`) // "Mistral AI" has a space
	reCapacity   = regexp.MustCompile(`^[1-9][0-9]{0,5}$`)
	reBool       = regexp.MustCompile(`^(true|false)$`)
	reAction     = regexp.MustCompile(`^(create|update|delete)$`)
	reMode       = regexp.MustCompile(`^(api|copilot|both)$`)
	reReason     = regexp.MustCompile(`^[^\x00-\x1f]{8,200}$`)
)

// Field is one validated parameter.
type Field struct {
	Name     string
	Pattern  *regexp.Regexp
	Optional bool
	Hint     string
}

// Spec describes one kind of request.
type Spec struct {
	Kind     string
	Template string // file under templates/dispatch
	Workflow func(config.Workflows) string
	Fields   []Field
}

// Specs is the catalogue of writes. Adding one means adding a template,
// a workflow and an entry here; nothing else changes.
var Specs = map[string]Spec{
	"deploy": {
		Kind: "deploy", Template: "deploy.json.tmpl",
		Workflow: func(w config.Workflows) string { return w.Deploy },
		Fields: []Field{
			{Name: "action", Pattern: reAction, Hint: "create | update | delete"},
			{Name: "deployment", Pattern: reDeployment, Hint: "deployment name"},
			{Name: "model_format", Pattern: reFormat, Optional: true, Hint: "OpenAI, Anthropic, Mistral AI, ..."},
			{Name: "model_name", Pattern: reToken, Optional: true},
			{Name: "model_version", Pattern: reToken, Optional: true},
			{Name: "sku_name", Pattern: reToken, Optional: true, Hint: "GlobalStandard, DataZoneStandard, Standard, ..."},
			{Name: "sku_capacity", Pattern: reCapacity, Optional: true, Hint: "units of 1K TPM for token SKUs"},
			{Name: "dry_run", Pattern: reBool, Optional: true, Hint: "defaults to true"},
			{Name: "reason", Pattern: reReason, Hint: "why, in 8 to 200 characters"},
		},
	},
	"report": {
		Kind: "report", Template: "report.json.tmpl",
		Workflow: func(w config.Workflows) string { return w.Report },
		Fields:   []Field{{Name: "reason", Pattern: reReason}},
	},
	"test": {
		Kind: "test", Template: "test.json.tmpl",
		Workflow: func(w config.Workflows) string { return w.Test },
		Fields: []Field{
			{Name: "deployment", Pattern: reDeployment},
			{Name: "mode", Pattern: reMode, Optional: true, Hint: "api | copilot | both"},
			{Name: "reason", Pattern: reReason},
		},
	},
}

// Kinds lists spec names in a stable order.
func Kinds() []string {
	ks := make([]string, 0, len(Specs))
	for k := range Specs {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Validate checks params against the spec. Unknown names are errors:
// a misspelt parameter must not silently become a default.
func (s Spec) Validate(params map[string]string) error {
	known := map[string]bool{"request_id": true}
	for _, f := range s.Fields {
		known[f.Name] = true
		v := params[f.Name]
		if v == "" {
			if f.Optional {
				continue
			}
			return fmt.Errorf("%s: %s is required", s.Kind, f.Name)
		}
		if !f.Pattern.MatchString(v) {
			return fmt.Errorf("%s: %s=%q is not valid", s.Kind, f.Name, v)
		}
	}
	for k := range params {
		if !known[k] {
			return fmt.Errorf("%s: unknown parameter %q", s.Kind, k)
		}
	}
	if s.Kind == "deploy" && params["action"] != "delete" {
		for _, need := range []string{"model_format", "model_name", "model_version", "sku_name", "sku_capacity"} {
			if params[need] == "" {
				return fmt.Errorf("deploy: %s is required for %s", need, params["action"])
			}
		}
	}
	return nil
}

// NewRequestID returns an id that sorts by time and is unique enough
// to find a run by: ftui-20260921T141503Z-9f2c1a.
func NewRequestID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no entropy: nothing sensible to do
	}
	return "ftui-" + time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b)
}

// Funcs are available to every dispatch template.
var Funcs = template.FuncMap{
	// json quotes a string as a JSON string. Every value goes through it.
	"json": func(s string) (string, error) {
		b, err := json.Marshal(s)
		return string(b), err
	},
	"default": func(def, s string) string {
		if s == "" {
			return def
		}
		return s
	},
	"lower": strings.ToLower,
}

// Request is a rendered, validated dispatch, ready to show or send.
type Request struct {
	Kind      string
	ID        string
	Workflow  string
	Repo, Ref string
	Params    map[string]string
	Payload   []byte // JSON object of strings
	SHA256    string // of Payload
}

// Cmd is the exact gh invocation. The payload goes on standard input.
func (r Request) Cmd() run.Cmd {
	return run.Cmd{
		Name:  "gh",
		Args:  []string{"workflow", "run", r.Workflow, "--repo", r.Repo, "--ref", r.Ref, "--json"},
		Stdin: r.Payload,
	}
}

// DryRun reports whether the request asks the workflow to change nothing.
func (r Request) DryRun() bool {
	var m map[string]string
	_ = json.Unmarshal(r.Payload, &m)
	v, ok := m["dry_run"]
	return !ok || v == "true"
}

// Dispatcher renders and sends requests.
type Dispatcher struct {
	R     run.Runner
	Cfg   config.Config
	FS    fs.FS // root containing dispatch/*.json.tmpl
	Log   *audit.Log
	Actor string
}

// TemplateFS picks the template root: a directory if configured, the
// embedded files otherwise.
func TemplateFS(embedded fs.FS, dir string) (fs.FS, error) {
	if dir != "" {
		return os.DirFS(dir), nil
	}
	return fs.Sub(embedded, "templates")
}

// Render validates params and executes the template. It sends nothing.
func (d Dispatcher) Render(kind string, params map[string]string) (Request, error) {
	spec, ok := Specs[kind]
	if !ok {
		return Request{}, fmt.Errorf("unknown request kind %q (have %s)", kind, strings.Join(Kinds(), ", "))
	}
	p := map[string]string{}
	for k, v := range params {
		p[k] = strings.TrimSpace(v)
	}
	if err := spec.Validate(p); err != nil {
		return Request{}, err
	}
	if p["request_id"] == "" {
		p["request_id"] = NewRequestID()
	}
	t, err := template.New(spec.Template).Funcs(Funcs).Option("missingkey=zero").
		ParseFS(d.FS, "dispatch/"+spec.Template)
	if err != nil {
		return Request{}, err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, p); err != nil {
		return Request{}, err
	}
	// The template's output is data, not trusted text: parse it back.
	var inputs map[string]string
	if err := json.Unmarshal(buf.Bytes(), &inputs); err != nil {
		return Request{}, fmt.Errorf("%s did not render a JSON object of strings: %w", spec.Template, err)
	}
	if len(inputs) > MaxInputs {
		return Request{}, fmt.Errorf("%s renders %d inputs; workflow_dispatch allows %d", spec.Template, len(inputs), MaxInputs)
	}
	payload, _ := json.MarshalIndent(inputs, "", "  ")
	sum := sha256.Sum256(payload)
	return Request{
		Kind: kind, ID: p["request_id"], Workflow: spec.Workflow(d.Cfg.Workflows),
		Repo: d.Cfg.Repo, Ref: d.Cfg.Ref, Params: p,
		Payload: payload, SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

// Send logs the intent, runs gh, and logs the outcome. The intent is
// written first: a crash between the two still leaves a trace.
func (d Dispatcher) Send(ctx context.Context, r Request) (string, error) {
	detail := map[string]string{
		"request_id": r.ID, "workflow": r.Workflow, "repo": r.Repo, "ref": r.Ref,
		"payload_sha256": r.SHA256, "payload": string(r.Payload), "command": r.Cmd().String(),
	}
	subject := r.Params["deployment"]
	if subject == "" {
		subject = r.Workflow
	}
	if _, err := d.Log.Append(d.Actor, "dispatch.request", subject, detail); err != nil {
		return "", fmt.Errorf("audit log is not writable, refusing to dispatch: %w", err)
	}
	out, err := d.R.Run(ctx, r.Cmd())
	result := map[string]string{"request_id": r.ID, "output": strings.TrimSpace(string(out))}
	kind := "dispatch.sent"
	if err != nil {
		kind, result["error"] = "dispatch.error", err.Error()
	}
	if _, lerr := d.Log.Append(d.Actor, kind, subject, result); lerr != nil && err == nil {
		err = lerr
	}
	return result["output"], err
}
