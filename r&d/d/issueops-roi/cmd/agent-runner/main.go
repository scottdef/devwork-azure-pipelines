// Command agent-runner is the container entrypoint for the aks-foundry-agent
// backend of agentic-task-request. It runs as a Kubernetes Job in the
// issueops-agents namespace on AKS, authenticates to Microsoft Foundry with
// Azure AD Workload Identity (no secrets), and executes a bounded
// plan → draft → critique → revise loop against one approved model
// deployment.
//
// Inputs are mounted from a ConfigMap rendered by `issueops agent render-k8s`:
//
//	/spec/spec.json         validated task specification (never raw comments)
//	/spec/system-prompt.md  system prompt (Go template output)
//	/spec/task-prompt.md    task prompt (Go template output)
//
// The result is printed to stdout as a base64 block between
// ISSUEOPS_RESULT_BEGIN / ISSUEOPS_RESULT_END, which the execute workflow
// reads with `kubectl logs` and `issueops agent result`. Diagnostics go to
// stderr as JSON lines.
//
// Only the Go standard library is used.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/agent"
)

// Version is set with -ldflags "-X main.Version=...".
var Version = "dev"

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

func main() {
	specPath := flag.String("spec", "/spec/spec.json", "task specification")
	systemPath := flag.String("system", "/spec/system-prompt.md", "system prompt")
	taskPath := flag.String("task", "/spec/task-prompt.md", "task prompt")
	endpoint := flag.String("endpoint", os.Getenv("FOUNDRY_ENDPOINT"), "Foundry / Azure OpenAI endpoint, e.g. https://<resource>.openai.azure.com")
	deployment := flag.String("deployment", os.Getenv("FOUNDRY_DEPLOYMENT"), "model deployment name")
	apiVersion := flag.String("api-version", envOr("FOUNDRY_API_VERSION", "2024-10-21"), "Azure OpenAI data-plane API version")
	maxIter := flag.Int("max-iterations", envInt("AGENT_MAX_ITERATIONS", 3), "maximum draft/critique iterations")
	timeout := flag.Int("timeout", envInt("AGENT_TIMEOUT_SECONDS", 900), "overall timeout in seconds")
	maxTokens := flag.Int("max-completion-tokens", envInt("AGENT_MAX_COMPLETION_TOKENS", 6000), "per-call completion token limit")
	replay := flag.String("replay", "", "replay model responses from a JSON array file (local testing, no network)")
	version := flag.Bool("version", false, "print the version")
	flag.Parse()
	if *version {
		fmt.Println(Version)
		return
	}
	log := newLogger(os.Stderr)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	res := run(ctx, log, config{
		specPath: *specPath, systemPath: *systemPath, taskPath: *taskPath,
		endpoint: *endpoint, deployment: *deployment, apiVersion: *apiVersion,
		maxIterations: *maxIter, maxTokens: *maxTokens, replay: *replay,
	})
	fmt.Print(agent.EncodeResult(res))
	if res.Status == "failed" {
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type config struct {
	specPath, systemPath, taskPath string
	endpoint, deployment           string
	apiVersion                     string
	maxIterations, maxTokens       int
	replay                         string
}

// run never panics and always returns a result: failures are reported to the
// issue instead of disappearing into a crashed pod.
func run(ctx context.Context, log *logger, cfg config) (res *agent.Result) {
	res = &agent.Result{Status: "failed", Model: cfg.deployment}
	defer func() {
		if r := recover(); r != nil {
			res.Status, res.Error = "failed", fmt.Sprintf("internal error: %v", r)
		}
	}()
	var spec agent.Spec
	b, err := os.ReadFile(cfg.specPath)
	if err == nil {
		err = json.Unmarshal(b, &spec)
	}
	if err != nil {
		res.Error = "read spec: " + err.Error()
		return res
	}
	system, err1 := os.ReadFile(cfg.systemPath)
	task, err2 := os.ReadFile(cfg.taskPath)
	if err := errors.Join(err1, err2); err != nil {
		res.Error = "read prompts: " + err.Error()
		return res
	}
	var model chatter
	if cfg.replay != "" {
		model, err = newReplay(cfg.replay)
	} else {
		if cfg.endpoint == "" || cfg.deployment == "" {
			res.Error = "FOUNDRY_ENDPOINT and FOUNDRY_DEPLOYMENT are required"
			return res
		}
		var cred credential
		cred, err = newCredential(log)
		if err == nil {
			model = &foundryClient{endpoint: strings.TrimRight(cfg.endpoint, "/"), deployment: cfg.deployment, apiVersion: cfg.apiVersion,
				maxTokens: cfg.maxTokens, cred: cred, log: log}
		}
	}
	if err != nil {
		res.Error = err.Error()
		return res
	}
	log.info("agent run starting", map[string]any{"request": spec.RequestRef, "deployment": cfg.deployment, "digest": spec.Digest, "max_iterations": cfg.maxIterations})
	out := loop(ctx, log, model, &spec, string(system), string(task), cfg.maxIterations)
	out.Model = cfg.deployment
	log.info("agent run finished", map[string]any{"status": out.Status, "iterations": out.Iterations, "prompt_tokens": out.Usage.PromptTokens, "completion_tokens": out.Usage.CompletionTokens})
	return out
}
