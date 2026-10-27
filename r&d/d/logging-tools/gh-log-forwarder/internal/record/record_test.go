package record

import (
	"encoding/json"
	"math/rand/v2"
	"net/url"
	"testing"
	"unicode/utf8"
)

func TestAppendStringRoundTrip(t *testing.T) {
	cases := []string{
		"", "plain", `quote " and \ backslash`, "tab\tnl\ncr\r", "\x00\x01\x1f\x7f",
		"<html>&amp;", "héllo 日本 🚀", "  ", "bad \xff\xfe utf8", "\x1b[36;1mansi\x1b[0m",
	}
	r := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 2000; i++ {
		b := make([]byte, r.IntN(40))
		for j := range b {
			b[j] = byte(r.IntN(256))
		}
		cases = append(cases, string(b))
	}
	for _, s := range cases {
		enc := AppendString(nil, s)
		if !json.Valid(enc) {
			t.Fatalf("invalid JSON for %q: %s", s, enc)
		}
		var back string
		if err := json.Unmarshal(enc, &back); err != nil {
			t.Fatal(err)
		}
		if want := toValid(s); back != want {
			t.Fatalf("round trip %q -> %q, want %q", s, back, want)
		}
	}
}

// toValid mirrors AppendString: each invalid byte becomes U+FFFD.
func toValid(s string) string {
	var out []rune
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		out = append(out, r)
		i += n
	}
	return string(out)
}

func TestBuilder(t *testing.T) {
	var b Builder
	b.Reset()
	b.Str("content", "x")
	b.Int("gh.line", 7)
	b.Bool("gh.log.truncated", true)
	b.Any(KV{"n", 3})
	b.Any(KV{"s", "y"})
	got := string(b.Bytes())
	want := `{"content":"x","gh.line":7,"gh.log.truncated":true,"n":3,"s":"y"}`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	b.Reset()
	if got := string(b.Bytes()); got != "{}" {
		t.Fatalf("empty object %s", got)
	}
}

func TestHeaderValue(t *testing.T) {
	h := HeaderValue([]KV{{"log.source", "github-actions"}, {"gh.run_attempt", 2}, {"a&b", "c=d e"}})
	q, err := url.ParseQuery(h)
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("log.source") != "github-actions" || q.Get("gh.run_attempt") != "2" || q.Get("a&b") != "c=d e" {
		t.Fatalf("%s -> %v", h, q)
	}
}
