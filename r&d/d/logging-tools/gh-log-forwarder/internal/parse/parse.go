// Package parse turns a GitHub Actions job log into lines with a
// timestamp, a severity, the step that produced them, and secrets
// removed. It knows nothing about Dynatrace.
//
// A job log line looks like
//
//	2026-10-27T10:15:02.1234567Z ##[error]Process completed with exit code 1.
//
// The first line may begin with a UTF-8 BOM. Workflow commands are
// rendered as ##[name] markers.
package parse

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Severity levels, in the spelling Dynatrace's loglevel uses.
const (
	Error = "ERROR"
	Warn  = "WARN"
	Info  = "INFO"
	Debug = "DEBUG"
)

// Step is a job step's time window.
type Step struct {
	Number int
	Name   string
	Start  time.Time
}

// Line is one parsed log line.
type Line struct {
	N         int       // 1-based line number in the job log
	Time      time.Time // from the line, or carried from the previous line
	HasTime   bool
	Text      string
	Severity  string
	Marker    string // group, endgroup, command, error, warning, notice, debug, section
	Step      *Step
	Truncated bool
	Redacted  bool
}

// Options controls parsing.
type Options struct {
	Steps     []Step
	Redact    []*regexp.Regexp
	MaxLine   int  // bytes of text kept per line; 0 means 64 KiB
	DropNoise bool // drop ##[endgroup] and blank lines
	StripANSI bool
}

// Stats counts what Parse saw.
type Stats struct {
	Lines      int
	Emitted    int
	Dropped    int
	Redactions int
	Truncated  int
}

var markers = []struct {
	tag, name, sev string
}{
	{"##[error]", "error", Error},
	{"##[warning]", "warning", Warn},
	{"##[notice]", "notice", Info},
	{"##[debug]", "debug", Debug},
	{"##[group]", "group", Info},
	{"##[endgroup]", "endgroup", Info},
	{"##[command]", "command", Info},
	{"##[section]", "section", Info},
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// ErrStop may be returned by emit to stop parsing early without error.
var ErrStop = errors.New("parse: stop")

// Parse streams r, calling emit for each kept line.
func Parse(r io.Reader, o Options, emit func(Line) error) (Stats, error) {
	if o.MaxLine <= 0 {
		o.MaxLine = 64 << 10
	}
	steps := append([]Step(nil), o.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		if !steps[i].Start.Equal(steps[j].Start) {
			return steps[i].Start.Before(steps[j].Start)
		}
		return steps[i].Number < steps[j].Number
	})

	var st Stats
	var last time.Time
	br := bufio.NewReaderSize(r, 64<<10)
	// Keep enough room for the 28-byte timestamp prefix plus a marker.
	keep := o.MaxLine + 64
	for n := 1; ; n++ {
		raw, cut, err := readLine(br, keep)
		if err != nil && !(errors.Is(err, io.EOF) && len(raw) > 0) {
			if errors.Is(err, io.EOF) {
				return st, nil
			}
			return st, err
		}
		st.Lines++
		if n == 1 {
			raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
		}
		line := Line{N: n, Time: last, Severity: Info, Truncated: cut}

		text := string(raw)
		if i := strings.IndexByte(text, ' '); i >= 20 && i <= 40 && text[i-1] == 'Z' {
			if t, perr := time.Parse(time.RFC3339Nano, text[:i]); perr == nil {
				line.Time, line.HasTime = t.UTC(), true
				text = text[i+1:]
			}
		} else if len(text) >= 20 && text[len(text)-1] == 'Z' && !strings.Contains(text, " ") {
			// A bare timestamp: an empty line in the original output.
			if t, perr := time.Parse(time.RFC3339Nano, text); perr == nil {
				line.Time, line.HasTime = t.UTC(), true
				text = ""
			}
		}
		last = line.Time

		if o.StripANSI && strings.IndexByte(text, 0x1b) >= 0 {
			text = ansiRe.ReplaceAllString(text, "")
		}
		for _, m := range markers {
			if strings.HasPrefix(text, m.tag) {
				line.Marker, line.Severity = m.name, m.sev
				text = text[len(m.tag):]
				break
			}
		}
		if o.DropNoise && (line.Marker == "endgroup" || strings.TrimSpace(text) == "") {
			st.Dropped++
			if errors.Is(err, io.EOF) {
				return st, nil
			}
			continue
		}
		for _, re := range o.Redact {
			if re.MatchString(text) {
				text = re.ReplaceAllString(text, "[REDACTED]")
				line.Redacted = true
				st.Redactions++
			}
		}
		if len(text) > o.MaxLine {
			text = truncateUTF8(text, o.MaxLine)
			line.Truncated = true
		}
		if line.Truncated {
			st.Truncated++
		}
		line.Text = strings.ToValidUTF8(text, "�")
		line.Step = stepFor(steps, line.Time)

		st.Emitted++
		if eerr := emit(line); eerr != nil {
			if errors.Is(eerr, ErrStop) {
				return st, nil
			}
			return st, eerr
		}
		if errors.Is(err, io.EOF) {
			return st, nil
		}
	}
}

// readLine reads through the next '\n', keeping at most keep bytes and
// discarding the rest of an over-long line. It strips \r\n.
func readLine(br *bufio.Reader, keep int) ([]byte, bool, error) {
	var buf []byte
	cut := false
	for {
		frag, err := br.ReadSlice('\n')
		if room := keep - len(buf); room > 0 {
			if len(frag) > room {
				buf = append(buf, frag[:room]...)
				cut = true
			} else {
				buf = append(buf, frag...)
			}
		} else if len(frag) > 0 && !(len(frag) == 1 && frag[0] == '\n') {
			cut = true
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		buf = bytes.TrimRight(buf, "\r\n")
		return buf, cut, err
	}
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// stepFor picks the last step that started at or before t. Step times
// from the API have one-second resolution, log lines have 100ns, so the
// comparison is on t truncated to the second.
func stepFor(steps []Step, t time.Time) *Step {
	if t.IsZero() {
		return nil
	}
	ts := t.Truncate(time.Second)
	for i := len(steps) - 1; i >= 0; i-- {
		if !steps[i].Start.IsZero() && !ts.Before(steps[i].Start) {
			return &steps[i]
		}
	}
	return nil
}
