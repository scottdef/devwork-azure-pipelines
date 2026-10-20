package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func TestComments(t *testing.T) {
	for _, c := range []struct{ tmpl, data, golden string }{
		{"comment-invalid.md.tmpl", "../../testdata/golden/validate/request-invalid.report.json", "comment-invalid.md"},
		{"comment-dispatched.md.tmpl", "../../testdata/golden/dispatch/publish.receipt.json", "comment-dispatched.md"},
		{"comment-failed.md.tmpl", "../../testdata/failed.json", "comment-failed.md"},
	} {
		var out bytes.Buffer
		if err := renderComment("../../templates/"+c.tmpl, c.data, nil, &out); err != nil {
			t.Fatalf("%s: %v", c.tmpl, err)
		}
		golden.Check(t, "../../testdata/golden/comments/"+c.golden, out.Bytes())
	}
}

func TestCodeFence(t *testing.T) {
	got := commentFuncs["code"].(func(string) string)("a ``` b")
	if !strings.HasPrefix(got, "````text\n") || !strings.HasSuffix(got, "\n````") {
		t.Errorf("code() = %q", got)
	}
	in := commentFuncs["inline"].(func(string) string)("x`y\n@team|z")
	if in != "`x'y @team\\|z`" {
		t.Errorf("inline() = %q", in)
	}
}

func TestMissingKey(t *testing.T) {
	err := renderComment("../../templates/comment-failed.md.tmpl", "-", strings.NewReader(`{}`), &bytes.Buffer{})
	if err == nil {
		t.Error("missing key: want error")
	}
}
