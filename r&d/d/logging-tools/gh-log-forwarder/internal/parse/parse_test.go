package parse

import (
	"bytes"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

var steps = []Step{
	{Number: 4, Name: "Post Run actions/checkout@v4", Start: ts("2026-10-27T10:15:07Z")},
	{Number: 1, Name: "Set up job", Start: ts("2026-10-27T10:15:00Z")},
	{Number: 2, Name: "Run actions/checkout@v4", Start: ts("2026-10-27T10:15:02Z")},
	{Number: 3, Name: "Run make build", Start: ts("2026-10-27T10:15:05Z")},
}

func golden(t *testing.T, o Options) ([]Line, Stats) {
	t.Helper()
	f, err := os.Open("../../testdata/job.log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got []Line
	st, err := Parse(f, o, func(l Line) error { got = append(got, l); return nil })
	if err != nil {
		t.Fatal(err)
	}
	return got, st
}

func TestGoldenJobLog(t *testing.T) {
	got, st := golden(t, Options{
		Steps:     steps,
		Redact:    []*regexp.Regexp{regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,255}`)},
		DropNoise: true,
		StripANSI: true,
	})
	type want struct {
		n      int
		text   string
		sev    string
		marker string
		step   int
	}
	wants := []want{
		{1, "Current runner version: '2.329.0'", Info, "", 1},
		{2, "Runner Image", Info, "group", 1},
		{3, "Image: ubuntu-24.04", Info, "", 1},
		{5, "Run actions/checkout@v4", Info, "group", 2},
		{6, "with:", Info, "", 2},
		{8, "Syncing repository: CoolGitOrg/app-a", Info, "", 2},
		{10, "Run make build", Info, "group", 3},
		{11, "make build", Info, "command", 3},
		{12, "go build -o bin/app ./cmd/app", Info, "", 3},
		{13, "debug: token=[REDACTED] leaked by a tool", Info, "", 3},
		{14, "Node.js 16 actions are deprecated.", Warn, "warning", 3},
		{15, "Evaluating condition for step: 'Run make build'", Debug, "debug", 3},
		{16, "Process completed with exit code 2.", Error, "error", 3},
		{17, "Unicode ok: héllo — 日本", Info, "", 3},
		{18, "Post job cleanup.", Info, "", 4},
	}
	if len(got) != len(wants) {
		for _, l := range got {
			t.Logf("%d %q", l.N, l.Text)
		}
		t.Fatalf("got %d lines, want %d", len(got), len(wants))
	}
	for i, w := range wants {
		g := got[i]
		if g.N != w.n || g.Text != w.text || g.Severity != w.sev || g.Marker != w.marker {
			t.Errorf("line %d: got {%d %q %s %q}, want {%d %q %s %q}", i, g.N, g.Text, g.Severity, g.Marker, w.n, w.text, w.sev, w.marker)
		}
		if g.Step == nil || g.Step.Number != w.step {
			t.Errorf("line %d (%q): step %+v, want %d", g.N, g.Text, g.Step, w.step)
		}
		if !g.HasTime {
			t.Errorf("line %d has no timestamp", g.N)
		}
	}
	if !got[0].Time.Equal(ts("2026-10-27T10:15:00.1Z")) {
		t.Errorf("first timestamp %s (BOM not stripped?)", got[0].Time)
	}
	if !got[9].Redacted {
		t.Error("token line not marked redacted")
	}
	if st.Lines != 18 || st.Emitted != 15 || st.Dropped != 3 || st.Redactions != 1 {
		t.Errorf("stats %+v", st)
	}
}

func TestKeepNoise(t *testing.T) {
	got, st := golden(t, Options{})
	if len(got) != 18 || st.Dropped != 0 {
		t.Fatalf("got %d lines, dropped %d", len(got), st.Dropped)
	}
	if !strings.Contains(got[7].Text, "\x1b[36;1m") {
		t.Error("ANSI stripped although StripANSI is false")
	}
	if got[8].Text != "" || !got[8].HasTime {
		t.Errorf("blank line: %+v", got[8])
	}
}

func TestLongLineTruncatedOnRuneBoundary(t *testing.T) {
	long := "2026-10-27T10:00:00.0000000Z " + strings.Repeat("é", 1000) + "\n" + "2026-10-27T10:00:01.0000000Z next\n"
	var got []Line
	_, err := Parse(strings.NewReader(long), Options{MaxLine: 301}, func(l Line) error { got = append(got, l); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines", len(got))
	}
	if !got[0].Truncated || len(got[0].Text) > 301 || !utf8Valid(got[0].Text) {
		t.Errorf("truncation: len=%d truncated=%v", len(got[0].Text), got[0].Truncated)
	}
	if got[1].Text != "next" || got[1].Truncated {
		t.Errorf("line after long line: %+v", got[1])
	}
}

func TestHugeLineDoesNotBlowUp(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("2026-10-27T10:00:00.0000000Z ")
	b.Write(bytes.Repeat([]byte("x"), 20<<20)) // 20 MiB, beyond bufio.Scanner's default
	b.WriteString("\nafter\n")
	var got []Line
	if _, err := Parse(&b, Options{MaxLine: 1024}, func(l Line) error { got = append(got, l); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || len(got[0].Text) != 1024 || got[1].Text != "after" {
		t.Fatalf("got %d lines; first len %d", len(got), len(got[0].Text))
	}
	if !got[1].Time.Equal(got[0].Time) {
		t.Error("untimestamped line should inherit the previous timestamp")
	}
}

func TestInvalidUTF8Replaced(t *testing.T) {
	var got []Line
	Parse(strings.NewReader("2026-10-27T10:00:00Z bad \xff\xfe bytes\n"), Options{}, func(l Line) error { got = append(got, l); return nil })
	if len(got) != 1 || !utf8Valid(got[0].Text) || !strings.Contains(got[0].Text, "�") {
		t.Fatalf("%+v", got)
	}
}

func TestEmitErrorStops(t *testing.T) {
	boom := errors.New("boom")
	n := 0
	_, err := Parse(strings.NewReader("a\nb\nc\n"), Options{}, func(Line) error { n++; return boom })
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("err=%v n=%d", err, n)
	}
	n = 0
	_, err = Parse(strings.NewReader("a\nb\nc\n"), Options{}, func(Line) error { n++; return ErrStop })
	if err != nil || n != 1 {
		t.Fatalf("ErrStop: err=%v n=%d", err, n)
	}
}

func TestEmpty(t *testing.T) {
	st, err := Parse(strings.NewReader(""), Options{}, func(Line) error { t.Fatal("emit on empty input"); return nil })
	if err != nil || st.Lines != 0 {
		t.Fatalf("%v %+v", err, st)
	}
}

func utf8Valid(s string) bool { return strings.ToValidUTF8(s, "") == s }
