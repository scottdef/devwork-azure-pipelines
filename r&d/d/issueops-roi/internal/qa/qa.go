// Package qa implements the clarifying-question loop used by agentic requests:
// the platform asks numbered questions (Q1..Qn) in a bot comment, the
// requestor answers with a comment that starts with `.answers` and contains
// `A<n>: value` lines, and the request becomes ready once every required
// question has a valid answer. Later answers override earlier ones; follow-up
// (dynamic) questions raised by an agent continue the numbering.
package qa

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Question is one clarifying question.
type Question struct {
	ID          string   `json:"id"`
	Key         string   `json:"key"`
	Text        string   `json:"text"`
	Required    bool     `json:"required"`
	Default     string   `json:"default,omitempty"`
	Pattern     string   `json:"pattern,omitempty"`
	Choices     []string `json:"choices,omitempty"`
	ChoicesFrom string   `json:"choices_from,omitempty"`
	MaxLength   int      `json:"max_length,omitempty"`
	Dynamic     bool     `json:"dynamic,omitempty"`
}

// QuestionStatus is a question with its current answer.
type QuestionStatus struct {
	Question
	Answer      string `json:"answer"`
	Answered    bool   `json:"answered"`
	FromDefault bool   `json:"from_default,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Status is the aggregate Q&A state.
type Status struct {
	Questions []QuestionStatus `json:"questions"`
	Complete  bool             `json:"complete"`
	Missing   []string         `json:"missing"`
	Invalid   []string         `json:"invalid"`
	// Answers holds raw answers keyed by question id (input to the digest).
	Answers map[string]string `json:"answers"`
	// ByKey holds effective answers (defaults applied) keyed by question key.
	ByKey map[string]string `json:"by_key"`
}

var (
	answerLine = regexp.MustCompile(`^\s*[Aa](\d{1,3})\s*[:=]\s?(.*)$`)
	skipValues = map[string]bool{"skip": true, "n/a": true, "na": true, "-": true, "none": true}
)

// ParseAnswers extracts A<n> answers from a `.answers` comment. Lines that do
// not start a new answer continue the previous one (multi-line answers).
func ParseAnswers(text string) map[string]string {
	out := map[string]string{}
	var cur string
	var buf []string
	flush := func() {
		if cur != "" {
			v := strings.TrimSpace(strings.Join(buf, "\n"))
			v = strings.TrimSpace(strings.TrimPrefix(v, "|"))
			out[cur] = v
		}
	}
	inFence := false
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			if cur != "" {
				buf = append(buf, line)
			}
			continue
		}
		if !inFence {
			if m := answerLine.FindStringSubmatch(line); m != nil {
				flush()
				n, _ := strconv.Atoi(m[1])
				cur, buf = fmt.Sprintf("Q%d", n), []string{m[2]}
				continue
			}
		}
		if cur != "" {
			buf = append(buf, line)
		}
	}
	flush()
	return out
}

// Collect merges answers (oldest comment first) and validates them.
func Collect(questions []Question, answerTexts []string) *Status {
	raw := map[string]string{}
	for _, t := range answerTexts {
		for k, v := range ParseAnswers(t) {
			raw[k] = v
		}
	}
	st := &Status{Answers: map[string]string{}, ByKey: map[string]string{}, Complete: true}
	known := map[string]bool{}
	for _, q := range questions {
		known[q.ID] = true
		qs := QuestionStatus{Question: q}
		v, has := raw[q.ID]
		if has {
			st.Answers[q.ID] = v
		}
		if has && skipValues[strings.ToLower(v)] {
			v, has = "", false
		}
		if has && v != "" {
			norm, err := validate(q, v)
			if err != nil {
				qs.Error = err.Error()
				st.Invalid = append(st.Invalid, q.ID)
				st.Complete = false
			} else {
				qs.Answer, qs.Answered = norm, true
			}
		} else if q.Default != "" {
			qs.Answer, qs.Answered, qs.FromDefault = q.Default, true, true
		}
		if !qs.Answered && q.Required && qs.Error == "" {
			st.Missing = append(st.Missing, q.ID)
			st.Complete = false
		}
		if qs.Answered {
			st.ByKey[q.Key] = qs.Answer
		}
		st.Questions = append(st.Questions, qs)
	}
	// Answers to unknown question ids are ignored (and excluded from the digest).
	for k := range st.Answers {
		if !known[k] {
			delete(st.Answers, k)
		}
	}
	sort.Strings(st.Missing)
	return st
}

func validate(q Question, v string) (string, error) {
	if q.MaxLength > 0 && len(v) > q.MaxLength {
		return "", fmt.Errorf("answer is %d characters; the limit is %d", len(v), q.MaxLength)
	}
	if len(q.Choices) > 0 {
		for _, c := range q.Choices {
			if strings.EqualFold(strings.TrimSpace(v), c) {
				return c, nil
			}
		}
		return "", fmt.Errorf("must be one of: %s", strings.Join(q.Choices, ", "))
	}
	if q.Pattern != "" {
		re, err := regexp.Compile(q.Pattern)
		if err != nil {
			return "", fmt.Errorf("question %s has an invalid pattern", q.ID)
		}
		if !re.MatchString(v) {
			return "", fmt.Errorf("does not match the required format `%s`", q.Pattern)
		}
	}
	return v, nil
}

// NextID returns the next free question id after the given questions.
func NextID(qs []Question) int {
	maxN := 0
	for _, q := range qs {
		if n, err := strconv.Atoi(strings.TrimPrefix(q.ID, "Q")); err == nil && n > maxN {
			maxN = n
		}
	}
	return maxN + 1
}

// Renumber assigns ids to dynamic questions continuing after existing ones.
func Renumber(existing []Question, dynamic []Question) []Question {
	n := NextID(existing)
	out := make([]Question, 0, len(dynamic))
	for i, q := range dynamic {
		q.ID = fmt.Sprintf("Q%d", n+i)
		if q.Key == "" {
			q.Key = fmt.Sprintf("followup_%d", n+i)
		}
		q.Dynamic, q.Required = true, true
		out = append(out, q)
	}
	return out
}
