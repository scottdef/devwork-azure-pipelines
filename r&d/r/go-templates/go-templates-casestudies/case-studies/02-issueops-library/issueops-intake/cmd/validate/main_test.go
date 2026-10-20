package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CoolGitOrg/issueops-intake/internal/golden"
)

func TestRun(t *testing.T) {
	for _, c := range []struct {
		name, doc string
		ok        bool
	}{
		{"request-basic", "../../testdata/golden/issueparse/request-basic.json", true},
		{"request-invalid", "../../testdata/golden/issueparse/request-invalid.json", false},
	} {
		var out bytes.Buffer
		ok, err := run("../../schema/request.json", c.doc, nil, &out)
		if err != nil {
			t.Fatal(err)
		}
		if ok != c.ok {
			t.Errorf("%s: ok = %v", c.name, ok)
		}
		golden.Check(t, "../../testdata/golden/validate/"+c.name+".report.json", out.Bytes())
	}
}

func TestNotJSON(t *testing.T) {
	var out bytes.Buffer
	ok, err := run("../../schema/request.json", "-", strings.NewReader("not json"), &out)
	if err != nil || ok || !strings.Contains(out.String(), `"rule": "json"`) {
		t.Errorf("ok=%v err=%v out=%s", ok, err, out.String())
	}
	if _, err := run("missing.json", "-", strings.NewReader("{}"), &out); err == nil {
		t.Error("missing schema: want error")
	}
}
