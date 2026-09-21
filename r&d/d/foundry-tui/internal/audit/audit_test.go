package audit

import (
	"os"
	"strings"
	"testing"
)

func newLog(t *testing.T) *Log {
	t.Helper()
	l, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"dispatch.request", "dispatch.sent", "probe.api"} {
		if _, err := l.Append("crafty", k, "gpt-4o", map[string]string{"request_id": "ftui-1"}); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func TestChainVerifies(t *testing.T) {
	l := newLog(t)
	recs, err := l.Read()
	if err != nil {
		t.Fatal(err)
	}
	if n, err := Verify(recs); n != 3 || err != nil {
		t.Fatalf("Verify = %d, %v", n, err)
	}
	if recs[1].Prev != recs[0].Hash || recs[0].Prev != "" {
		t.Error("records are not chained")
	}
}

func TestTamperingIsDetected(t *testing.T) {
	edit := func(t *testing.T, fn func(lines []string) []string) error {
		l := newLog(t)
		b, _ := os.ReadFile(l.Path)
		lines := fn(strings.Split(strings.TrimSpace(string(b)), "\n"))
		os.WriteFile(l.Path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
		recs, err := l.Read()
		if err != nil {
			return err
		}
		_, err = Verify(recs)
		return err
	}
	if err := edit(t, func(l []string) []string { l[1] = strings.Replace(l[1], "gpt-4o", "gpt-5", 1); return l }); err == nil {
		t.Error("an edited record verified")
	}
	if err := edit(t, func(l []string) []string { return append(l[:1], l[2:]...) }); err == nil {
		t.Error("a deleted record went unnoticed")
	}
	if err := edit(t, func(l []string) []string { l[0], l[1] = l[1], l[0]; return l }); err == nil {
		t.Error("reordered records verified")
	}
}
