// Package pretok splits text into the pieces a BPE tokenizer then merges.
//
// The GPT-2 family defines this step with a regular expression:
//
//	's|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+(?!\S)|\s+
//
// The (?!\S) is a lookahead, and Go's regexp package (RE2) has none. So the
// expression is implemented as a state-function lexer instead: each state
// consumes one piece and returns the state that handles what comes next.
package pretok

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type lexer struct {
	s   string
	pos int
	out []string
}

type stateFn func(*lexer) stateFn

// SplitGPT2 returns the pre-tokens of s. Joining them gives back s exactly.
func SplitGPT2(s string) []string {
	l := &lexer{s: s}
	for state := lexStart; state != nil; {
		state = state(l)
	}
	return l.out
}

func (l *lexer) emit(end int) {
	l.out = append(l.out, l.s[l.pos:end])
	l.pos = end
}

func (l *lexer) runeAt(i int) (rune, int) {
	if i >= len(l.s) {
		return -1, 0
	}
	return utf8.DecodeRuneInString(l.s[i:])
}

// scan returns the end of the run of runes starting at i that satisfy ok.
func (l *lexer) scan(i int, ok func(rune) bool) int {
	for i < len(l.s) {
		r, w := utf8.DecodeRuneInString(l.s[i:])
		if !ok(r) {
			break
		}
		i += w
	}
	return i
}

func isOther(r rune) bool { return !unicode.IsSpace(r) && !unicode.IsLetter(r) && !unicode.IsNumber(r) }

var contractions = []string{"'s", "'t", "'re", "'ve", "'m", "'ll", "'d"}

// lexStart tries the alternatives of the expression in their written order.
func lexStart(l *lexer) stateFn {
	if l.pos >= len(l.s) {
		return nil
	}
	rest := l.s[l.pos:]
	for _, c := range contractions {
		if strings.HasPrefix(rest, c) {
			l.emit(l.pos + len(c))
			return lexStart
		}
	}
	// " ?" : one optional leading space belongs to the word that follows it.
	i := l.pos
	if rest[0] == ' ' {
		i++
	}
	switch r, _ := l.runeAt(i); {
	case r < 0:
		// a lone trailing space; handled as whitespace below
	case unicode.IsLetter(r):
		l.emit(l.scan(i, unicode.IsLetter))
		return lexStart
	case unicode.IsNumber(r):
		l.emit(l.scan(i, unicode.IsNumber))
		return lexStart
	case !unicode.IsSpace(r):
		l.emit(l.scan(i, isOther))
		return lexStart
	}
	return lexSpace
}

// lexSpace handles `\s+(?!\S)|\s+`. A run of whitespace that reaches the end
// of the text is one piece. A run followed by text gives up its last
// character: that character either joins the next word (if it is a plain
// space) or stands alone.
func lexSpace(l *lexer) stateFn {
	end := l.scan(l.pos, unicode.IsSpace)
	if end == len(l.s) {
		l.emit(end)
		return nil
	}
	_, w := utf8.DecodeLastRuneInString(l.s[l.pos:end])
	if last := end - w; last > l.pos {
		l.emit(last) // all but the last whitespace character
		return lexStart
	}
	l.emit(end) // a single non-space whitespace character before text
	return lexStart
}
