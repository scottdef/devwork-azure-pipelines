package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/CoolEngOrg/issueops/internal/agent"
	"github.com/CoolEngOrg/issueops/internal/qa"
)

// message is a chat message.
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// usage is token accounting for one call.
type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// chatter is a chat-completions backend (Foundry or a replay file).
type chatter interface {
	Chat(ctx context.Context, msgs []message) (string, usage, error)
}

// draftReply is the JSON the model returns for draft and revise steps.
type draftReply struct {
	Status      string `json:"status"` // draft | needs_input
	Deliverable string `json:"deliverable"`
	Questions   []struct {
		Key  string `json:"key"`
		Text string `json:"text"`
	} `json:"questions"`
	Notes string `json:"notes"`
}

// critiqueReply is the JSON the model returns for critique steps.
type critiqueReply struct {
	MeetsCriteria bool     `json:"meets_criteria"`
	Issues        []string `json:"issues"`
	Summary       string   `json:"summary"`
}

const draftInstructions = `Respond with a single JSON object and nothing else:
{"status":"draft","deliverable":"<complete Markdown deliverable>","notes":"<one line on assumptions>"}
If, and only if, an acceptance criterion cannot be met without information that is not in the specification, respond instead with:
{"status":"needs_input","questions":[{"key":"<snake_case_key>","text":"<one precise question>"}]}
Ask at most 3 questions. Never ask for secrets or credentials.`

const critiqueInstructions = `You are reviewing a draft deliverable against the task specification above.
Check every acceptance criterion and hard constraint, the accuracy of each command, and that nothing is invented.
Respond with a single JSON object and nothing else:
{"meets_criteria":true|false,"issues":["<specific, actionable issue>", ...],"summary":"<one paragraph>"}`

// loop runs draft → critique → revise until the critique passes or the
// iteration budget is spent. It returns needs_input when the model asks for
// information on the first draft (follow-up questions become Q&A on the issue).
func loop(ctx context.Context, log *logger, model chatter, spec *agent.Spec, system, task string, maxIter int) *agent.Result {
	if maxIter < 1 {
		maxIter = 1
	}
	res := &agent.Result{Status: "failed"}
	call := func(step string, msgs []message, v any) error {
		out, u, err := model.Chat(ctx, msgs)
		res.Usage.Requests++
		res.Usage.PromptTokens += u.PromptTokens
		res.Usage.CompletionTokens += u.CompletionTokens
		if err != nil {
			return fmt.Errorf("%s: %w", step, err)
		}
		if err := json.Unmarshal([]byte(extractJSON(out)), v); err != nil {
			return fmt.Errorf("%s: model did not return valid JSON: %w", step, err)
		}
		log.info("model step", map[string]any{"step": step, "prompt_tokens": u.PromptTokens, "completion_tokens": u.CompletionTokens})
		return nil
	}

	base := []message{{Role: "system", Content: system}, {Role: "user", Content: task + "\n\n" + draftInstructions}}
	var d draftReply
	if err := call("draft", base, &d); err != nil {
		res.Error = err.Error()
		return res
	}
	res.Iterations = 1
	if d.Status == "needs_input" && len(d.Questions) > 0 {
		res.Status = "needs_input"
		for i, q := range d.Questions {
			if i == 3 {
				break
			}
			key := sanitizeKey(q.Key, i)
			if agent.LooksLikeSecret(q.Text) || strings.TrimSpace(q.Text) == "" {
				continue
			}
			res.Questions = append(res.Questions, qa.Question{ID: fmt.Sprintf("D%d", i+1), Key: key, Text: strings.TrimSpace(q.Text), Required: true, MaxLength: 2000})
		}
		if len(res.Questions) > 0 {
			return res
		}
		res.Status = "failed"
		res.Error = "the model asked for more information but produced no usable question"
		return res
	}
	draft := d.Deliverable
	for {
		var c critiqueReply
		msgs := []message{{Role: "system", Content: system}, {Role: "user", Content: task + "\n\n## Draft deliverable\n\n" + draft + "\n\n" + critiqueInstructions}}
		if err := call("critique", msgs, &c); err != nil {
			res.Error = err.Error()
			return res
		}
		res.Critique = c.Summary
		if c.MeetsCriteria || res.Iterations >= maxIter {
			if !c.MeetsCriteria && len(c.Issues) > 0 {
				res.Critique = strings.TrimSpace(c.Summary + "\n\nOpen issues after the iteration budget:\n- " + strings.Join(c.Issues, "\n- "))
			}
			break
		}
		res.Iterations++
		rev := append(append([]message{}, base...),
			message{Role: "assistant", Content: mustJSON(draftReply{Status: "draft", Deliverable: draft})},
			message{Role: "user", Content: "Revise the deliverable to fix these review issues:\n- " + strings.Join(c.Issues, "\n- ") + "\n\n" + draftInstructions})
		var r draftReply
		if err := call(fmt.Sprintf("revise-%d", res.Iterations), rev, &r); err != nil {
			res.Error = err.Error()
			return res
		}
		if strings.TrimSpace(r.Deliverable) != "" {
			draft = r.Deliverable
		}
	}
	if strings.TrimSpace(draft) == "" {
		res.Error = "the model returned an empty deliverable"
		return res
	}
	res.Status, res.Deliverable = "completed", draft
	return res
}

// extractJSON tolerates a fenced JSON reply.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func sanitizeKey(k string, i int) string {
	var b strings.Builder
	for _, r := range strings.ToLower(k) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 || b.Len() > 40 {
		return fmt.Sprintf("followup_%d", i+1)
	}
	return b.String()
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
