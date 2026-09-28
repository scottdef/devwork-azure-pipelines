package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// NormalizeBody canonicalizes an issue body before hashing: CRLF -> LF,
// trailing whitespace trimmed per line, surrounding blank lines removed.
func NormalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// Digest binds approvals to the exact request content. Any edit of the issue
// body or of Q&A answers changes the digest and invalidates prior approvals.
func Digest(typeID, body string, answers map[string]string) string {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make([][2]string, 0, len(keys))
	for _, k := range keys {
		ordered = append(ordered, [2]string{k, strings.TrimSpace(answers[k])})
	}
	payload, _ := json.Marshal(struct {
		Type    string      `json:"type"`
		Body    string      `json:"body"`
		Answers [][2]string `json:"answers"`
	}{typeID, NormalizeBody(body), ordered})
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Short returns the first 12 hex characters of a digest for display.
func Short(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
