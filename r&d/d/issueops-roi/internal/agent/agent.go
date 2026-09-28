// Package agent implements the agentic-task-request type: backend policy,
// clarifying questions, task specification, and dispatch to one of three
// backends:
//
//   - copilot-cloud-agent: creates a clean issue in the target repository
//     (rendered only from validated fields) assigned to copilot-swe-agent[bot]
//     with agent_assignment options, or starts an agent task via the agent
//     tasks API;
//   - agentic-workflow-catalog: dispatches a pre-approved GitHub Agentic
//     Workflow (gh-aw compiled .lock.yml) in the target repository;
//   - aks-foundry-agent: renders a ConfigMap + Job for the platform agent
//     runner on AKS, which calls a Foundry model deployment through Azure
//     workload identity.
package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/qa"
	"github.com/CoolEngOrg/issueops/internal/request"
	"github.com/CoolEngOrg/issueops/internal/tmpl"
)

// Backend names.
const (
	BackendCopilot = "copilot-cloud-agent"
	BackendCatalog = "agentic-workflow-catalog"
	BackendAKS     = "aks-foundry-agent"
)

// CopilotAssignee is the bot login used to assign issues to the Copilot cloud agent.
const CopilotAssignee = "copilot-swe-agent[bot]"

// Backend settings.
type Backend struct {
	MaxClassification       string   `json:"max_classification"`
	Mode                    string   `json:"mode,omitempty"`
	Environment             string   `json:"environment"`
	Namespace               string   `json:"namespace,omitempty"`
	Image                   string   `json:"image,omitempty"`
	ServiceAccount          string   `json:"service_account,omitempty"`
	FoundryEndpoint         string   `json:"foundry_endpoint,omitempty"`
	AllowedDeployments      []string `json:"allowed_deployments,omitempty"`
	ConfidentialDeployments []string `json:"confidential_deployments,omitempty"`
	MaxIterations           int      `json:"max_iterations,omitempty"`
	TimeoutSeconds          int      `json:"timeout_seconds,omitempty"`
}

// CatalogEntry is a pre-approved agentic workflow.
type CatalogEntry struct {
	Workflow    string            `json:"workflow"`
	Description string            `json:"description"`
	Inputs      map[string]string `json:"inputs"` // workflow input -> question id
}

// Settings is the registry settings block.
type Settings struct {
	RepositoryAllow []string                   `json:"repository_allow"`
	RepositoryDeny  []string                   `json:"repository_deny"`
	Backends        map[string]Backend         `json:"backends"`
	Catalog         map[string]CatalogEntry    `json:"catalog"`
	QuestionSets    map[string]json.RawMessage `json:"question_sets"`
}

var classRank = map[string]int{"public": 0, "internal": 1, "confidential": 2}

var repoNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// LoadSettings decodes the settings block.
func LoadSettings(c *request.Context) (*Settings, error) {
	var s Settings
	if err := c.Type.DecodeSettings(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Questions returns the base question set for the request plus dynamic
// follow-up questions raised by the agent (already numbered).
func Questions(s *Settings, backend, catalog string, dynamic []qa.Question) ([]qa.Question, error) {
	raw, ok := s.QuestionSets[backend]
	if !ok {
		return nil, fmt.Errorf("agent: no question set for backend %q", backend)
	}
	var qs []qa.Question
	if backend == BackendCatalog {
		var byCatalog map[string][]qa.Question
		if err := json.Unmarshal(raw, &byCatalog); err != nil {
			return nil, fmt.Errorf("agent: catalog question sets: %w", err)
		}
		qs = byCatalog[catalog]
	} else if err := json.Unmarshal(raw, &qs); err != nil {
		return nil, fmt.Errorf("agent: question set %s: %w", backend, err)
	}
	be := s.Backends[backend]
	for i := range qs {
		if qs[i].ChoicesFrom == "allowed_deployments" {
			qs[i].Choices = append([]string{}, be.AllowedDeployments...)
		}
	}
	return append(qs, dynamic...), nil
}

// Handler implements request.Handler.
type Handler struct{}

func targetRepo(c *request.Context) string {
	r := strings.TrimSpace(c.Values.String("agent_target_repository"))
	if o, n, ok := strings.Cut(r, "/"); ok && strings.EqualFold(o, c.Registry.Organization) {
		r = n
	}
	return r
}

// Facts implements request.Handler.
func (Handler) Facts(c *request.Context) (map[string]any, error) {
	s, err := LoadSettings(c)
	if err != nil {
		return nil, err
	}
	backend := c.Values.String("agent_backend")
	return map[string]any{
		"$type":                c.Type.ID,
		"$backend":             backend,
		"$backend_environment": s.Backends[backend].Environment,
		"$classification":      c.Values.String("agent_data_classification"),
	}, nil
}

// Validate implements request.Handler.
func (Handler) Validate(ctx context.Context, c *request.Context) ([]string, []string) {
	s, err := LoadSettings(c)
	if err != nil {
		return []string{err.Error()}, nil
	}
	var errs, warns []string
	v := c.Values
	backend := v.String("agent_backend")
	be, ok := s.Backends[backend]
	if !ok {
		errs = append(errs, fmt.Sprintf("Unknown agent backend %q.", backend))
	}
	repo := targetRepo(c)
	if strings.Contains(repo, "/") {
		errs = append(errs, fmt.Sprintf("Target repository must be in %s.", c.Registry.Organization))
	} else if !repoNameRe.MatchString(repo) {
		errs = append(errs, "Target repository must be a repository name.")
	} else if !globAny(s.RepositoryAllow, repo) || globAny(s.RepositoryDeny, repo) {
		errs = append(errs, fmt.Sprintf("Repository `%s` is not allowed for agentic tasks.", repo))
	}
	class := v.String("agent_data_classification")
	if _, ok := classRank[class]; !ok {
		errs = append(errs, "Choose a data classification.")
	} else if ok && be.MaxClassification != "" && classRank[class] > classRank[be.MaxClassification] {
		errs = append(errs, fmt.Sprintf("Backend `%s` is approved for data up to `%s`; this request is `%s`.", backend, be.MaxClassification, class))
	}
	catalog := v.String("agent_catalog_workflow")
	switch backend {
	case BackendCatalog:
		if _, ok := s.Catalog[catalog]; !ok {
			errs = append(errs, "Choose a catalog workflow for the agentic-workflow-catalog backend.")
		}
	default:
		if catalog != "" && catalog != "none" {
			warns = append(warns, "The catalog workflow is ignored for this backend.")
		}
	}
	if len(strings.TrimSpace(v.String("agent_objective"))) < 20 {
		errs = append(errs, "Describe the objective in at least a sentence.")
	}
	if len(lines(v.String("agent_acceptance_criteria"))) == 0 {
		errs = append(errs, "List at least one acceptance criterion.")
	}
	for _, k := range []string{"agent_objective", "agent_acceptance_criteria", "agent_task_title"} {
		if LooksLikeSecret(v.String(k)) {
			errs = append(errs, "The request appears to contain a credential or secret; remove it and rotate the secret.")
			break
		}
	}
	if c.GH != nil && len(errs) == 0 {
		r, err := c.GH.GetRepo(ctx, c.Registry.Organization, repo)
		switch {
		case err != nil:
			warns = append(warns, "Could not verify the target repository: "+err.Error())
		case r == nil:
			errs = append(errs, fmt.Sprintf("Repository `%s/%s` does not exist.", c.Registry.Organization, repo))
		case r.Archived:
			errs = append(errs, fmt.Sprintf("Repository `%s/%s` is archived.", c.Registry.Organization, repo))
		default:
			c.SetInfo("default_branch", r.DefaultBranch)
		}
	}
	return errs, warns
}

var secretRe = regexp.MustCompile(`(?i)(ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|gh[ousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN [A-Z ]*PRIVATE KEY-----|xox[baprs]-[A-Za-z0-9-]{10,}|AccountKey=[A-Za-z0-9+/=]{20,}|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})`)

// LooksLikeSecret flags obvious credentials (tokens, keys, JWTs).
func LooksLikeSecret(s string) bool { return secretRe.MatchString(s) }

func globAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(name)); ok {
			return true
		}
	}
	return false
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-*•"))
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// Summary implements request.Handler.
func (Handler) Summary(c *request.Context) []request.Row {
	v := c.Values
	rows := []request.Row{
		{Label: "Backend", Value: v.String("agent_backend"), Code: true},
		{Label: "Target repository", Value: c.Registry.Organization + "/" + targetRepo(c), Code: true},
		{Label: "Task", Value: v.String("agent_task_title")},
		{Label: "Data classification", Value: v.String("agent_data_classification")},
	}
	if v.String("agent_backend") == BackendCatalog {
		rows = append(rows, request.Row{Label: "Catalog workflow", Value: v.String("agent_catalog_workflow"), Code: true})
	}
	rows = append(rows, request.Row{Label: "Acceptance criteria", Value: fmt.Sprintf("%d item(s)", len(lines(v.String("agent_acceptance_criteria"))))})
	return rows
}

// Subject implements request.Handler.
func (Handler) Subject(c *request.Context) (string, map[string]string) {
	return targetRepo(c), nil
}

// Spec is the agent task specification. It contains only validated form
// fields and validated answers: never raw issue comments.
type Spec struct {
	RequestRef         string              `json:"request_ref"`
	RequestURL         string              `json:"request_url"`
	Requestor          string              `json:"requestor"`
	Backend            string              `json:"backend"`
	Org                string              `json:"org"`
	Repo               string              `json:"repo"`
	TargetRepo         string              `json:"target_repo"`
	Title              string              `json:"title"`
	Objective          string              `json:"objective"`
	AcceptanceCriteria []string            `json:"acceptance_criteria"`
	Classification     string              `json:"classification"`
	Catalog            string              `json:"catalog,omitempty"`
	Answers            map[string]string   `json:"answers"`
	Questions          []qa.QuestionStatus `json:"questions"`
	Digest             string              `json:"digest"`
	DefaultBranch      string              `json:"default_branch,omitempty"`
}

// BuildSpec assembles the specification.
func BuildSpec(c *request.Context, status *qa.Status, digest, requestURL string) *Spec {
	v := c.Values
	sp := &Spec{
		RequestRef:         fmt.Sprintf("%s/%s#%d", c.Registry.Organization, c.Registry.Repository, c.Issue),
		RequestURL:         requestURL,
		Requestor:          c.Requestor,
		Backend:            v.String("agent_backend"),
		Org:                c.Registry.Organization,
		Repo:               targetRepo(c),
		Title:              v.String("agent_task_title"),
		Objective:          v.String("agent_objective"),
		AcceptanceCriteria: lines(v.String("agent_acceptance_criteria")),
		Classification:     v.String("agent_data_classification"),
		Digest:             digest,
		Answers:            map[string]string{},
	}
	sp.TargetRepo = sp.Org + "/" + sp.Repo
	if sp.Backend == BackendCatalog {
		sp.Catalog = v.String("agent_catalog_workflow")
	}
	if status != nil {
		for k, a := range status.ByKey {
			sp.Answers[k] = a
		}
		sp.Questions = status.Questions
	}
	if db, ok := c.Info["default_branch"].(string); ok {
		sp.DefaultBranch = db
	}
	return sp
}

// DispatchResult reports what was started.
type DispatchResult struct {
	Backend   string `json:"backend"`
	Kind      string `json:"kind"` // issue | task | workflow | job
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
	Workflow  string `json:"workflow,omitempty"`
	JobName   string `json:"job_name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Manifest  string `json:"manifest,omitempty"`
	Message   string `json:"message"`
}

// DispatchCopilot delegates the task to the Copilot cloud agent. userGH must
// carry a user-to-server token (fine-grained PAT of a Copilot-licensed
// machine user or a GitHub App user token); installation tokens are rejected.
func DispatchCopilot(ctx context.Context, userGH *ghapi.Client, set *tmpl.Set, sp *Spec, be Backend) (*DispatchResult, error) {
	body, err := set.Render("agent.copilot-issue.md.tmpl", sp)
	if err != nil {
		return nil, err
	}
	instr, err := set.Render("agent.copilot-instructions.md.tmpl", sp)
	if err != nil {
		return nil, err
	}
	base := sp.Answers["base_branch"]
	if base == "" {
		base = sp.DefaultBranch
	}
	if be.Mode == "task" {
		t, err := userGH.CreateAgentTask(ctx, sp.Org, sp.Repo, body+"\n\n"+instr, base, "", true)
		if err != nil {
			return nil, fmt.Errorf("agent: create agent task: %w", err)
		}
		return &DispatchResult{Backend: BackendCopilot, Kind: "task", URL: t.HTMLURL, Reference: t.ID, Message: "Copilot cloud agent task started (" + t.State + ")."}, nil
	}
	is, err := userGH.CreateIssue(ctx, sp.Org, sp.Repo, ghapi.NewIssue{
		Title:     "[Agent] " + sp.Title,
		Body:      body,
		Assignees: []string{CopilotAssignee},
		AgentAssignment: &ghapi.AgentAssignment{
			TargetRepo:         sp.TargetRepo,
			BaseBranch:         base,
			CustomInstructions: instr,
			CustomAgent:        sp.Answers["custom_agent"],
		},
	})
	if err != nil {
		return nil, fmt.Errorf("agent: create and assign issue: %w", err)
	}
	return &DispatchResult{Backend: BackendCopilot, Kind: "issue", URL: is.HTMLURL, Reference: fmt.Sprintf("%s#%d", sp.TargetRepo, is.Number), Message: "Issue created in the target repository and assigned to the Copilot cloud agent; it will open a draft pull request."}, nil
}

// DispatchCatalog triggers a pre-approved agentic workflow in the target repository.
func DispatchCatalog(ctx context.Context, gh *ghapi.Client, s *Settings, sp *Spec, status *qa.Status) (*DispatchResult, error) {
	entry, ok := s.Catalog[sp.Catalog]
	if !ok {
		return nil, fmt.Errorf("agent: unknown catalog workflow %q", sp.Catalog)
	}
	byID := map[string]string{}
	if status != nil {
		for _, q := range status.Questions {
			byID[q.ID] = q.Answer
		}
	}
	inputs := map[string]string{"issueops_request": sp.RequestURL}
	for input, qid := range entry.Inputs {
		inputs[input] = byID[qid]
	}
	ref := sp.DefaultBranch
	if ref == "" {
		r, err := gh.GetRepo(ctx, sp.Org, sp.Repo)
		if err != nil || r == nil {
			return nil, fmt.Errorf("agent: resolve default branch: %v", err)
		}
		ref = r.DefaultBranch
	}
	if err := gh.DispatchWorkflow(ctx, sp.Org, sp.Repo, entry.Workflow, ref, inputs); err != nil {
		return nil, fmt.Errorf("agent: dispatch %s: %w", entry.Workflow, err)
	}
	return &DispatchResult{Backend: BackendCatalog, Kind: "workflow", Workflow: entry.Workflow,
		URL:     fmt.Sprintf("https://github.com/%s/%s/actions/workflows/%s", sp.Org, sp.Repo, entry.Workflow),
		Message: "Agentic workflow dispatched on `" + ref + "`."}, nil
}

// JobData is the input to the Kubernetes templates.
type JobData struct {
	Spec           *Spec
	SpecJSON       string
	SystemPrompt   string
	TaskPrompt     string
	JobName        string
	Namespace      string
	Image          string
	ServiceAccount string
	Endpoint       string
	Deployment     string
	MaxIterations  int
	TimeoutSeconds int
	// RunnerTimeoutSeconds is the runner's own deadline: shorter than the Job's
	// activeDeadlineSeconds so the runner always prints a result block before
	// Kubernetes kills the pod (scheduling and image pull use part of the budget).
	RunnerTimeoutSeconds int
	Issue                int
}

// RenderAKS renders the ConfigMap + Job manifests for the agent runner.
func RenderAKS(set *tmpl.Set, sp *Spec, be Backend, issue int) (string, *JobData, error) {
	dep := sp.Answers["model_deployment"]
	if dep == "" {
		return "", nil, errors.New("agent: model_deployment answer is required")
	}
	if !contains(be.AllowedDeployments, dep) {
		return "", nil, fmt.Errorf("agent: deployment %q is not allowed", dep)
	}
	if sp.Classification == "confidential" && !contains(be.ConfidentialDeployments, dep) {
		return "", nil, fmt.Errorf("agent: deployment %q is not approved for confidential data", dep)
	}
	iters := be.MaxIterations
	if n := sp.Answers["max_iterations"]; n != "" {
		fmt.Sscanf(n, "%d", &iters)
	}
	if iters < 1 || iters > be.MaxIterations {
		iters = be.MaxIterations
	}
	specJSON, _ := json.MarshalIndent(sp, "", "  ")
	sys, err := set.Render("agent.system-prompt.md.tmpl", sp)
	if err != nil {
		return "", nil, err
	}
	task, err := set.Render("agent.task-prompt.md.tmpl", sp)
	if err != nil {
		return "", nil, err
	}
	short := strings.TrimPrefix(sp.Digest, "sha256:")
	if len(short) > 8 {
		short = short[:8]
	}
	jd := &JobData{
		Spec: sp, SpecJSON: string(specJSON), SystemPrompt: sys, TaskPrompt: task,
		JobName:   fmt.Sprintf("issueops-agent-%d-%s", issue, short),
		Namespace: be.Namespace, Image: be.Image, ServiceAccount: be.ServiceAccount, Endpoint: be.FoundryEndpoint,
		Deployment: dep, MaxIterations: iters, TimeoutSeconds: be.TimeoutSeconds, RunnerTimeoutSeconds: runnerTimeout(be.TimeoutSeconds), Issue: issue,
	}
	cm, err := set.Render("k8s.agent-configmap.yaml.tmpl", jd)
	if err != nil {
		return "", nil, err
	}
	job, err := set.Render("k8s.agent-job.yaml.tmpl", jd)
	if err != nil {
		return "", nil, err
	}
	return cm + "---\n" + job, jd, nil
}

func runnerTimeout(jobSeconds int) int {
	if jobSeconds <= 0 {
		jobSeconds = 900
	}
	t := jobSeconds - 90
	if t < jobSeconds/2 {
		t = jobSeconds / 2
	}
	return t
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ---- runner result protocol ---------------------------------------------

// Result markers delimit the base64-encoded JSON result in the runner log.
const (
	ResultBegin = "ISSUEOPS_RESULT_BEGIN"
	ResultEnd   = "ISSUEOPS_RESULT_END"
)

// Usage is token accounting reported by the runner.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	Requests         int `json:"requests"`
}

// Result is what the agent runner reports.
type Result struct {
	Status      string        `json:"status"` // completed | needs_input | failed
	Deliverable string        `json:"deliverable,omitempty"`
	Questions   []qa.Question `json:"questions,omitempty"`
	Critique    string        `json:"critique,omitempty"`
	Iterations  int           `json:"iterations"`
	Model       string        `json:"model"`
	Usage       Usage         `json:"usage"`
	Error       string        `json:"error,omitempty"`
}

// EncodeResult renders the log block the runner prints.
func EncodeResult(r *Result) string {
	b, _ := json.Marshal(r)
	enc := base64.StdEncoding.EncodeToString(b)
	var sb strings.Builder
	sb.WriteString(ResultBegin + "\n")
	for len(enc) > 0 {
		n := min(len(enc), 76)
		sb.WriteString(enc[:n] + "\n")
		enc = enc[n:]
	}
	sb.WriteString(ResultEnd + "\n")
	return sb.String()
}

// ParseResult extracts the result from a runner log.
func ParseResult(log string) (*Result, error) {
	i := strings.LastIndex(log, ResultBegin)
	j := strings.LastIndex(log, ResultEnd)
	if i < 0 || j < i {
		return nil, errors.New("agent: runner log has no result block")
	}
	payload := strings.Join(strings.Fields(log[i+len(ResultBegin):j]), "")
	b, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, fmt.Errorf("agent: decode result: %w", err)
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("agent: parse result: %w", err)
	}
	switch r.Status {
	case "completed", "needs_input", "failed":
	default:
		return nil, fmt.Errorf("agent: unknown result status %q", r.Status)
	}
	if len(r.Deliverable) > 60000 {
		r.Deliverable = r.Deliverable[:60000] + "\n\n…(truncated; full output in the workflow artifact)"
	}
	if len(r.Questions) > 5 {
		r.Questions = r.Questions[:5]
	}
	return &r, nil
}

// SortedAnswerKeys is a template helper.
func (s *Spec) SortedAnswerKeys() []string {
	keys := make([]string, 0, len(s.Answers))
	for k := range s.Answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
