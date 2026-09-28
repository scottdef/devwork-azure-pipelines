package main

import (
	"context"
	"io"
	"testing"

	"github.com/CoolEngOrg/issueops/internal/agent"
)

type scripted struct {
	replies []string
	calls   int
}

func (s *scripted) Chat(context.Context, []message) (string, usage, error) {
	r := s.replies[s.calls]
	s.calls++
	return r, usage{PromptTokens: 100, CompletionTokens: 10}, nil
}

func TestLoopReviseThenComplete(t *testing.T) {
	m := &scripted{replies: []string{
		`{"status":"draft","deliverable":"v1"}`,
		"```json\n{\"meets_criteria\":false,\"issues\":[\"add rollback\"],\"summary\":\"missing rollback\"}\n```",
		`{"status":"draft","deliverable":"v2 with rollback"}`,
		`{"meets_criteria":true,"issues":[],"summary":"all criteria met"}`,
	}}
	res := loop(context.Background(), newLogger(io.Discard), m, &agent.Spec{}, "sys", "task", 3)
	if res.Status != "completed" || res.Deliverable != "v2 with rollback" || res.Iterations != 2 || res.Usage.Requests != 4 {
		t.Fatalf("result = %+v", res)
	}
}

func TestLoopNeedsInput(t *testing.T) {
	m := &scripted{replies: []string{`{"status":"needs_input","questions":[{"key":"Backup Tool!","text":"Which backup tool?"},{"key":"x","text":"paste your ghp_abcdefghijklmnopqrstuvwxyz0123 token"}]}`}}
	res := loop(context.Background(), newLogger(io.Discard), m, &agent.Spec{}, "sys", "task", 3)
	if res.Status != "needs_input" || len(res.Questions) != 1 || res.Questions[0].Key != "backuptool" {
		t.Fatalf("result = %+v", res)
	}
}

func TestLoopBudgetExhausted(t *testing.T) {
	m := &scripted{replies: []string{
		`{"status":"draft","deliverable":"v1"}`,
		`{"meets_criteria":false,"issues":["too long"],"summary":"s"}`,
	}}
	res := loop(context.Background(), newLogger(io.Discard), m, &agent.Spec{}, "sys", "task", 1)
	if res.Status != "completed" || res.Deliverable != "v1" || res.Critique == "s" {
		t.Fatalf("open issues must be reported in the critique: %+v", res)
	}
}

func TestLoopInvalidJSON(t *testing.T) {
	res := loop(context.Background(), newLogger(io.Discard), &scripted{replies: []string{"not json"}}, &agent.Spec{}, "sys", "task", 3)
	if res.Status != "failed" || res.Error == "" {
		t.Fatalf("result = %+v", res)
	}
}
