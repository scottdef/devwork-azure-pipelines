package qa

import "testing"

func TestParseAnswersMultilineAndFence(t *testing.T) {
	got := ParseAnswers(".answers\nA1: first\nA2: line one\nline two\n```\nA3: inside fence stays in A2\n```\na4 = lower-case prefix\n")
	if got["Q1"] != "first" {
		t.Errorf("Q1 = %q", got["Q1"])
	}
	if want := "line one\nline two\n```\nA3: inside fence stays in A2\n```"; got["Q2"] != want {
		t.Errorf("Q2 = %q, want %q", got["Q2"], want)
	}
	if _, ok := got["Q3"]; ok {
		t.Error("A3 inside a fence must not start an answer")
	}
	if got["Q4"] != "lower-case prefix" {
		t.Errorf("Q4 = %q", got["Q4"])
	}
}

func TestCollect(t *testing.T) {
	qs := []Question{
		{ID: "Q1", Key: "kind", Text: "?", Required: true, Choices: []string{"runbook", "adr"}},
		{ID: "Q2", Key: "iters", Text: "?", Required: false, Default: "3", Pattern: "^[1-4]$"},
		{ID: "Q3", Key: "ctx", Text: "?", Required: true, MaxLength: 5},
	}
	st := Collect(qs, []string{"A1: memo\nA3: toolong", "A2: 9"})
	if st.Complete {
		t.Fatal("invalid answers must not complete Q&A")
	}
	if len(st.Invalid) != 3 {
		t.Fatalf("invalid = %v", st.Invalid)
	}
	st = Collect(qs, []string{"A1: memo", "A1: RUNBOOK\nA3: ok"}) // later answers replace earlier ones
	if !st.Complete || st.ByKey["kind"] == "" || st.ByKey["iters"] != "3" {
		t.Fatalf("status = %+v", st)
	}
	if _, ok := st.Answers["Q2"]; ok {
		t.Fatal("defaults must not be recorded as raw answers (digest input)")
	}
}

func TestRenumber(t *testing.T) {
	base := []Question{{ID: "Q1"}, {ID: "Q2"}}
	dyn := Renumber(base, []Question{{ID: "D1", Key: "a", Text: "x"}, {ID: "D2", Key: "b", Text: "y"}})
	if len(dyn) != 2 || dyn[0].ID != "Q3" || dyn[1].ID != "Q4" || !dyn[0].Dynamic {
		t.Fatalf("renumbered = %+v", dyn)
	}
}
