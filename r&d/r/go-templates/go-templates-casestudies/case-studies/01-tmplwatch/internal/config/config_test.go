package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseExample(t *testing.T) {
	c, err := Load("../../config/tmplwatch.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Source.Interval.D() != 15*time.Second || c.Source.APIURL != "https://api.github.com" {
		t.Errorf("source defaults: %+v", c.Source)
	}
	tp := c.Templates[0]
	if tp.Name != "Caddyfile" || tp.Wait.D() != 2*time.Second {
		t.Errorf("template: %+v", tp)
	}
	if m, _ := tp.Mode(); m != 0o644 {
		t.Errorf("mode %v", m)
	}
}

func TestParseTestdata(t *testing.T) {
	c, err := Load("../../config/testdata.json")
	if err != nil {
		t.Fatal(err)
	}
	if c.Source.Type != "dir" || len(c.Templates) < 9 {
		t.Errorf("testdata config: %+v", c.Source)
	}
}

func TestParseErrors(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`{"source":{"type":"ftp"},"templates":[{}]}`, "source.type"},
		{`{"source":{"type":"dir"}}`, "source.path"},
		{`{"source":{"type":"dir","path":"x"}}`, "no templates"},
		{`{"source":{"type":"dir","path":"x","interval":"10ms"},"templates":[{"source":"a","destination":"b"}]}`, "minimum"},
		{`{"source":{"type":"dir","path":"x"},"templates":[{"source":"a","destination":"b","perms":"999"}]}`, "perms"},
		{`{"source":{"type":"dir","path":"x"},"templates":[{"source":"a","destination":"b","left_delimiter":"[["}]}`, "delimiters"},
		{`{"source":{"type":"dir","path":"x"},"templates":[{"source":"a.tmpl","destination":"b"},{"source":"x/a.tmpl","destination":"c"}]}`, "duplicate"},
		{`{"source":{"type":"github","repo":"nope"},"templates":[{"source":"a","destination":"b"}]}`, "owner/name"},
		{`{"sorce":{}}`, "unknown field"},
	} {
		_, err := Parse([]byte(tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.in, err, tc.want)
		}
	}
}
