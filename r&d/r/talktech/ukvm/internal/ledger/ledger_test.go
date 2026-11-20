package ledger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendReopenVerify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := l.Append("act", map[string]any{"n": i, "target": "server3"}); err != nil {
			t.Fatal(err)
		}
	}
	l.Close()

	l, err = Open(path) // must pick the chain up where it stopped
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append("act", map[string]any{"n": 3}); err != nil {
		t.Fatal(err)
	}
	seq, head := l.Head()
	l.Close()

	b, _ := os.ReadFile(path)
	n, last, err := Verify(bytes.NewReader(b))
	if err != nil || n != 4 || n != seq || last != head {
		t.Fatalf("verify: n=%d seq=%d err=%v", n, seq, err)
	}
}

func TestTamperingIsDetected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	l, _ := Open(path)
	l.Append("order", map[string]string{"text": "run uptime"})
	l.Append("act", map[string]string{"text": "typed uptime"})
	l.Append("result", map[string]string{"text": "ok"})
	l.Close()
	orig, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(orig)), "\n")

	edited := strings.Replace(string(orig), "run uptime", "run reboot", 1)
	if _, _, err := Verify(strings.NewReader(edited)); err == nil {
		t.Error("an edited record verified")
	}
	dropped := strings.Join([]string{lines[0], lines[2]}, "\n")
	if _, _, err := Verify(strings.NewReader(dropped)); err == nil {
		t.Error("a removed record went unnoticed")
	}
	swapped := strings.Join([]string{lines[1], lines[0], lines[2]}, "\n")
	if _, _, err := Verify(strings.NewReader(swapped)); err == nil {
		t.Error("reordered records verified")
	}

	os.WriteFile(path, []byte(edited), 0o640)
	if _, err := Open(path); err == nil {
		t.Error("a damaged ledger was opened for appending")
	}
}
