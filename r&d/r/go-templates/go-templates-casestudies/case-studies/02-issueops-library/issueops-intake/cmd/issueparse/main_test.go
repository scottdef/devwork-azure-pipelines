package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	err := run("../../.github/ISSUE_TEMPLATE/request.yml", "../../testdata/bodies/request-basic.md", true, nil, &out)
	if err != nil {
		t.Fatal(err)
	}
	golden.Check(t, "../../testdata/golden/issueparse/request-basic.json", out.Bytes())
}

func TestStdinAndLimits(t *testing.T) {
	var out bytes.Buffer
	if err := run("../../.github/ISSUE_TEMPLATE/return.yml", "-", false, strings.NewReader("### Item ID\n\na/readme/1\n"), &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "{\"item-id\":\"a/readme/1\"}\n" {
		t.Errorf("got %q", got)
	}
	big := strings.NewReader(strings.Repeat("x", maxBody+1))
	if err := run("../../.github/ISSUE_TEMPLATE/return.yml", "", false, big, &out); err == nil {
		t.Error("oversized body: want error")
	}
	if err := run("nope.yml", "", false, strings.NewReader(""), &out); err == nil {
		t.Error("missing form: want error")
	}
}
