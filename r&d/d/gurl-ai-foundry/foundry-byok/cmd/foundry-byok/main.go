// Command foundry-byok builds Azure AI Foundry endpoint URLs and generates
// GitHub Copilot BYOK configuration (gh-aw workflows and enterprise custom
// models) from a model name and a deployment name.
//
// Standard library only; builds with Go 1.21.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
	"github.com/CoolGitOrg/foundry-byok/internal/inventory"
	"github.com/CoolGitOrg/foundry-byok/internal/render"
	"github.com/CoolGitOrg/foundry-byok/internal/verify"
)

// version is set at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

// Exit codes are stable so pipelines can branch on them.
const (
	exitOK           = 0
	exitError        = 1
	exitUsage        = 2
	exitDrift        = 3
	exitVerifyFailed = 4
)

const tokenEnv = "AZURE_FOUNDRY_BEARER_TOKEN"

const usage = `foundry-byok - Azure AI Foundry URLs and GitHub Copilot BYOK configuration

Usage:
  foundry-byok <command> [flags]

Commands:
  url        Print every URL and Copilot setting for one deployment
  generate   Render scripts, code, gh-aw workflows and the enterprise runbook
  validate   Check an inventory without writing anything
  verify     Check deployments against the live Azure resource
  templates  List or export the built-in templates for customization
  version    Print the version

Describe a deployment with flags (--resource, --model, --deployment, ...) or
describe many with --inventory FILE. Run "foundry-byok <command> -h" for flags.

Secrets are never passed as flags. "verify" reads the API key from the
environment variable named by --api-key-secret (default AZURE_OPENAI_API_KEY),
or a bearer token from AZURE_FOUNDRY_BEARER_TOKEN.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "url":
		return cmdURL(rest, stdout, stderr)
	case "generate":
		return cmdGenerate(rest, stdout, stderr)
	case "validate":
		return cmdValidate(rest, stdout, stderr)
	case "verify":
		return cmdVerify(rest, stdout, stderr)
	case "templates":
		return cmdTemplates(rest, stdout, stderr)
	case "version", "--version", "-version":
		fmt.Fprintln(stdout, version)
		return exitOK
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
		return exitUsage
	}
}

// common holds flags shared by every command that takes deployments.
type common struct {
	spec      foundry.Spec
	inventory string
	onlyEnv   string
	logJSON   bool
	verbose   bool
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet("foundry-byok "+name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

func (c *common) bind(fs *flag.FlagSet) {
	s := &c.spec
	fs.StringVar(&c.inventory, "inventory", "", "inventory JSON `file` (\"-\" for stdin); replaces the single-deployment flags")
	fs.StringVar(&c.onlyEnv, "only-env", "", "with --inventory: keep only this `environment`")
	fs.BoolVar(&c.logJSON, "log-json", false, "log as JSON (for CI log shippers)")
	fs.BoolVar(&c.verbose, "v", false, "verbose logging")

	fs.StringVar(&s.Resource, "resource", "", "Azure AI Foundry / Azure OpenAI resource `name`")
	fs.StringVar(&s.Endpoint, "endpoint", "", "full endpoint `URL` instead of --resource (private DNS, APIM, sovereign cloud)")
	fs.StringVar((*string)(&s.HostKind), "host-kind", "", "`kind` of hostname: openai (default), cognitiveservices, services-ai")
	fs.StringVar(&s.Environment, "environment", "", "environment `label`, e.g. dev, uat, prod")
	fs.StringVar(&s.Model, "model", "", "model `name` from the Foundry catalog, e.g. gpt-5.4")
	fs.StringVar(&s.ModelVersion, "model-version", "", "model `version`, e.g. 2026-03-05")
	fs.StringVar(&s.ModelID, "model-id", "", "explicit Azure model `ID` (overrides model + version)")
	fs.StringVar(&s.Deployment, "deployment", "", "Azure deployment `name`")
	fs.StringVar((*string)(&s.Surface), "surface", "", "API `surface`: auto (default), openai-v1, anthropic")
	fs.StringVar((*string)(&s.WireAPI), "wire-api", "", "Copilot wire `API`: auto (default), responses, completions")
	fs.StringVar((*string)(&s.Auth), "auth", "", "auth `mode`: api-key (default) or entra")
	fs.StringVar(&s.APIKeySecret, "api-key-secret", "", "`name` of the GitHub secret / env var holding the key (default "+foundry.DefaultAPIKeySecret+")")
	fs.StringVar(&s.TenantID, "tenant-id", "", "Entra tenant `GUID` (auth=entra)")
	fs.StringVar(&s.ClientID, "client-id", "", "Entra app/managed identity client `GUID` (auth=entra)")
	fs.StringVar(&s.EntraAuthorityHost, "entra-authority-host", "", "Entra login `host` (default "+foundry.DefaultEntraAuthorityHost+")")
	fs.StringVar(&s.LegacyAPIVersion, "legacy-api-version", "", "api-`version` for deployment-in-path URLs (default "+foundry.DefaultLegacyAPIVersion+")")
	fs.StringVar((*string)(&s.EnterpriseURLStyle), "enterprise-url-style", "", "Deployment URL `style` for Copilot custom models: v1 (default), deployment, root")
	fs.StringVar(&s.EnterpriseKeyName, "enterprise-key-name", "", "`name` of the key entry in the Copilot custom-models form")
	fs.StringVar(&s.RunsOn, "runs-on", "", "comma-separated runner `labels` for generated gh-aw workflows")
	fs.BoolVar(&s.DisableModelFallback, "disable-model-fallback", false, "emit sandbox.agent.model-fallback: false in gh-aw output")
}

func (c *common) logger(stderr io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if c.verbose {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	if c.logJSON {
		return slog.New(slog.NewJSONHandler(stderr, opts))
	}
	return slog.New(slog.NewTextHandler(stderr, opts))
}

// bundle resolves either the inventory or the single-deployment flags.
func (c *common) bundle() (inventory.Bundle, error) {
	var specs []foundry.Spec
	if c.inventory != "" {
		// Silently ignoring a flag would be worse than refusing it: settings
		// for inventory runs belong in the file's "defaults" block.
		if c.spec != (foundry.Spec{}) {
			return inventory.Bundle{}, errors.New("--inventory cannot be combined with single-deployment flags; put shared settings in the inventory's \"defaults\"")
		}
		f, err := inventory.LoadFile(c.inventory)
		if err != nil {
			return inventory.Bundle{}, err
		}
		if specs, err = f.Specs(); err != nil {
			return inventory.Bundle{}, err
		}
		if c.onlyEnv != "" {
			kept := specs[:0]
			for _, s := range specs {
				if s.Environment == c.onlyEnv {
					kept = append(kept, s)
				}
			}
			if len(kept) == 0 {
				return inventory.Bundle{}, fmt.Errorf("no deployments in environment %q", c.onlyEnv)
			}
			specs = kept
		}
	} else {
		if c.onlyEnv != "" {
			return inventory.Bundle{}, errors.New("--only-env requires --inventory")
		}
		specs = []foundry.Spec{c.spec}
	}
	return inventory.Build(version, specs)
}

func parse(fs *flag.FlagSet, args []string) (int, bool) {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK, false
		}
		return exitUsage, false
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(fs.Output(), "unexpected argument %q\n", fs.Arg(0))
		return exitUsage, false
	}
	return exitOK, true
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "error:", err)
	return exitError
}

// ---------------------------------------------------------------- url

func cmdURL(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlagSet("url", stderr)
	c.bind(fs)
	format := fs.String("format", "text", "output `format`: text, json, env")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	b, err := c.bundle()
	if err != nil {
		return fail(stderr, err)
	}
	switch *format {
	case "json":
		var v any = b.Items
		if len(b.Items) == 1 && c.inventory == "" {
			v = b.Items[0]
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(v); err != nil {
			return fail(stderr, err)
		}
	case "env":
		if len(b.Items) != 1 {
			return fail(stderr, errors.New("--format env needs exactly one deployment (use --only-env or single-deployment flags)"))
		}
		printEnv(stdout, b.Items[0])
	case "text":
		for i, it := range b.Items {
			if i > 0 {
				fmt.Fprintln(stdout, strings.Repeat("-", 72))
			}
			printText(stdout, it)
		}
	default:
		fmt.Fprintf(stderr, "unknown --format %q\n", *format)
		return exitUsage
	}
	return exitOK
}

func printText(w io.Writer, it foundry.Resolved) {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(tw, "  %s\t%s\n", k, v)
		}
	}
	head := func(s string) { fmt.Fprintf(tw, "\n%s\n", s) }

	fmt.Fprintf(tw, "DEPLOYMENT\n")
	row("environment", it.Spec.Environment)
	row("host", it.Host)
	row("deployment name", it.Spec.Deployment)
	row("model ID", it.ModelID)
	row("surface", string(it.Surface))
	row("wire API", string(it.WireAPI))
	row("auth", string(it.Spec.Auth))

	head("DIRECT API (curl / code)")
	row("v1 chat completions", it.URLs.V1ChatCompletions)
	row("v1 responses", it.URLs.V1Responses)
	row("v1 embeddings", it.URLs.V1Embeddings)
	row("v1 list models", it.URLs.V1Models)
	row("legacy chat completions", it.URLs.LegacyChatCompletions)
	row("model inference (retired)", it.URLs.ModelInferenceChatCompletions)
	row("anthropic messages", it.URLs.AnthropicMessages)
	if it.Surface == foundry.SurfaceOpenAIV1 {
		row("request body", fmt.Sprintf(`"model": %q  (v1: deployment name; legacy: omit)`, it.Spec.Deployment))
		row("auth header", "api-key: <key>   or   Authorization: Bearer <token for "+foundry.EntraScope+">")
	} else {
		row("request body", fmt.Sprintf(`"model": %q`, it.Spec.Deployment))
		row("auth header", "x-api-key: <key>   or   Authorization: Bearer <token>;  anthropic-version: 2023-06-01")
	}

	head("GITHUB AGENTIC WORKFLOWS (gh-aw) and COPILOT CLI")
	row("COPILOT_PROVIDER_BASE_URL", it.GhAw.ProviderBaseURL)
	row("engine.model / COPILOT_MODEL", it.GhAw.EngineModel)
	row("COPILOT_PROVIDER_MODEL_ID", it.GhAw.ProviderModelID)
	row("COPILOT_PROVIDER_WIRE_API", string(it.GhAw.WireAPI))
	row("COPILOT_PROVIDER_TYPE", it.GhAw.ProviderType)
	row("COPILOT_PROVIDER_API_KEY", it.GhAw.APIKeyExpr)
	row("network.allowed", strings.Join(it.GhAw.NetworkAllowed, ", "))

	head("COPILOT SDK")
	row("provider.type", it.Client.SDKType)
	row("provider.baseUrl", it.Client.SDKBaseURL)
	row("model", it.Client.SDKModel)

	head("COPILOT ENTERPRISE / ORG CUSTOM MODELS (Copilot Chat, CLI, IDEs)")
	if it.Enterprise.Supported {
		row("Provider", it.Enterprise.Provider)
		row("Name", it.Enterprise.KeyName)
		row("Deployment URL", it.Enterprise.DeploymentURL)
		row("Model ID", it.Enterprise.ModelID)
		for i, a := range it.Enterprise.Alternates {
			row(fmt.Sprintf("fallback URL %d", i+1), a)
		}
	} else {
		row("not available", "the form accepts an API key only (auth=entra)")
	}

	if len(it.Warnings) > 0 {
		head("WARNINGS")
		for _, w := range it.Warnings {
			fmt.Fprintf(tw, "  - %s\n", w)
		}
	}
	tw.Flush()
}

// printEnv emits KEY=value lines suitable for `>> "$GITHUB_OUTPUT"` or `eval`.
func printEnv(w io.Writer, it foundry.Resolved) {
	kv := map[string]string{
		"FOUNDRY_HOST":                      it.Host,
		"FOUNDRY_DEPLOYMENT":                it.Spec.Deployment,
		"FOUNDRY_MODEL_ID":                  it.ModelID,
		"FOUNDRY_V1_BASE":                   it.URLs.V1Base,
		"FOUNDRY_V1_CHAT_COMPLETIONS":       it.URLs.V1ChatCompletions,
		"FOUNDRY_V1_RESPONSES":              it.URLs.V1Responses,
		"FOUNDRY_V1_MODELS":                 it.URLs.V1Models,
		"FOUNDRY_LEGACY_CHAT_COMPLETIONS":   it.URLs.LegacyChatCompletions,
		"FOUNDRY_ANTHROPIC_MESSAGES":        it.URLs.AnthropicMessages,
		"COPILOT_PROVIDER_BASE_URL":         it.GhAw.ProviderBaseURL,
		"COPILOT_MODEL":                     it.GhAw.EngineModel,
		"COPILOT_PROVIDER_MODEL_ID":         it.GhAw.ProviderModelID,
		"COPILOT_PROVIDER_WIRE_API":         string(it.GhAw.WireAPI),
		"COPILOT_PROVIDER_TYPE":             it.GhAw.ProviderType,
		"COPILOT_ENTERPRISE_DEPLOYMENT_URL": it.Enterprise.DeploymentURL,
		"COPILOT_ENTERPRISE_MODEL_ID":       it.Enterprise.ModelID,
	}
	keys := make([]string, 0, len(kv))
	for k, v := range kv {
		if v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "%s=%s\n", k, kv[k])
	}
}

// ---------------------------------------------------------------- generate

func cmdGenerate(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlagSet("generate", stderr)
	c.bind(fs)
	out := fs.String("out", "out", "output `dir`ectory")
	tmplDir := fs.String("template-dir", "", "`dir` with template overrides (see: foundry-byok templates --export)")
	wfDir := fs.String("workflows-dir", "", "also copy the gh-aw smoke workflows into this `dir` (e.g. .github/workflows)")
	check := fs.Bool("check", false, "write nothing; exit 3 if the files on disk differ from what would be generated")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	log := c.logger(stderr)

	b, err := c.bundle()
	if err != nil {
		return fail(stderr, err)
	}
	files, err := render.Renderer{OverrideDir: *tmplDir}.Render(b)
	if err != nil {
		return fail(stderr, err)
	}
	var workflows []render.File
	for _, f := range files {
		if f.Kind == render.KindWorkflow {
			wf := f
			wf.Path = path.Base(f.Path)
			workflows = append(workflows, wf)
		}
	}
	for _, it := range b.Items {
		for _, w := range it.Warnings {
			log.Warn(w.Message, "scope", w.Scope, "environment", it.Spec.Environment, "deployment", it.Spec.Deployment)
		}
	}

	if *check {
		drift, err := render.Diff(*out, files)
		if err != nil {
			return fail(stderr, err)
		}
		if *wfDir != "" {
			more, err := render.Diff(*wfDir, workflows)
			if err != nil {
				return fail(stderr, err)
			}
			for _, m := range more {
				drift = append(drift, path.Join(*wfDir, m))
			}
		}
		if len(drift) > 0 {
			fmt.Fprintf(stderr, "drift: %d file(s) differ from the inventory:\n", len(drift))
			for _, d := range drift {
				fmt.Fprintln(stderr, "  "+d)
			}
			fmt.Fprintln(stderr, "regenerate with: foundry-byok generate (same flags, without --check)")
			return exitDrift
		}
		log.Info("no drift", "files", len(files))
		return exitOK
	}

	if err := render.Write(*out, files); err != nil {
		return fail(stderr, err)
	}
	if *wfDir != "" {
		if err := render.Write(*wfDir, workflows); err != nil {
			return fail(stderr, err)
		}
	}
	log.Info("generated", "deployments", len(b.Items), "files", len(files), "out", *out,
		"enterprise_key_entries", len(b.EnterpriseGroups))
	fmt.Fprintln(stdout, path.Join(*out, "SUMMARY.md"))
	return exitOK
}

// ---------------------------------------------------------------- validate

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlagSet("validate", stderr)
	c.bind(fs)
	strict := fs.Bool("strict", false, "treat warnings as errors")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	b, err := c.bundle()
	if err != nil {
		return fail(stderr, err)
	}
	// Rendering exercises the templates too, so a broken override fails here.
	if _, err := (render.Renderer{}).Render(b); err != nil {
		return fail(stderr, err)
	}
	warned := 0
	for _, it := range b.Items {
		for _, w := range it.Warnings {
			warned++
			fmt.Fprintf(stderr, "warning: %s/%s/%s: %s\n", it.Spec.Environment, it.Spec.Resource, it.Spec.Deployment, w)
		}
	}
	fmt.Fprintf(stdout, "ok: %d deployment(s), %d enterprise key entr(ies), %d warning(s)\n",
		len(b.Items), len(b.EnterpriseGroups), warned)
	if *strict && warned > 0 {
		return exitError
	}
	return exitOK
}

// ---------------------------------------------------------------- verify

func cmdVerify(args []string, stdout, stderr io.Writer) int {
	var c common
	fs := newFlagSet("verify", stderr)
	c.bind(fs)
	probe := fs.Bool("probe", false, "also send one small, billable inference request per deployment")
	timeout := fs.Duration("timeout", 3*time.Minute, "overall `timeout`")
	asJSON := fs.Bool("json", false, "print results as JSON")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	b, err := c.bundle()
	if err != nil {
		return fail(stderr, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()

	var results []verify.Result
	allOK := true
	for _, it := range b.Items {
		cred := verify.Credentials{BearerToken: os.Getenv(tokenEnv)}
		if it.Spec.APIKeySecret != "" {
			cred.APIKey = os.Getenv(it.Spec.APIKeySecret)
		}
		res := verify.Check(ctx, it, cred, verify.Options{Probe: *probe})
		results = append(results, res)
		allOK = allOK && res.OK()
	}

	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return fail(stderr, err)
		}
	} else {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(tw, "ENV\tRESOURCE\tDEPLOYMENT\tCHECK\tSTATUS\tDETAIL")
		for _, r := range results {
			for _, f := range r.Findings {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", dash(r.Environment), dash(r.Resource), r.Deployment, f.Check, f.Status, f.Detail)
				if f.Hint != "" {
					fmt.Fprintf(tw, "\t\t\t\t\t  hint: %s\n", f.Hint)
				}
			}
		}
		tw.Flush()
	}
	if !allOK {
		return exitVerifyFailed
	}
	return exitOK
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// ---------------------------------------------------------------- templates

func cmdTemplates(args []string, stdout, stderr io.Writer) int {
	fs := newFlagSet("templates", stderr)
	export := fs.String("export", "", "copy the built-in templates into this `dir` (never overwrites)")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if *export == "" {
		for _, n := range render.TemplateNames() {
			fmt.Fprintln(stdout, n)
		}
		return exitOK
	}
	written, err := render.ExportTemplates(*export)
	if err != nil {
		return fail(stderr, err)
	}
	for _, w := range written {
		fmt.Fprintln(stdout, w)
	}
	return exitOK
}
