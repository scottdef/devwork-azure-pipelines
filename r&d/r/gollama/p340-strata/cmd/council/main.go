// council asks several seats the same question and prints the majority
// answer. A seat is a model alias; naming one alias k times gives k samples
// of one model (self-consistency), naming different aliases gives a mixed
// council. With -judge, a split vote goes to one more model with every
// draft attached. It speaks the OpenAI chat API, so it works against gate,
// Strata, llama-server or Unsloth Studio.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const tail = "\n\nReason briefly, then finish with exactly one line of the form:\nANSWER: <your answer>"

// extract returns the text after the last "ANSWER:" in a reply.
func extract(reply string) string {
	i := strings.LastIndex(strings.ToUpper(reply), "ANSWER:")
	if i < 0 {
		return ""
	}
	line, _, _ := strings.Cut(reply[i+len("ANSWER:"):], "\n")
	return strings.TrimSpace(line)
}

// vote returns the most common answer, ignoring case and edge punctuation,
// and how many seats gave it. Seats with no answer do not vote.
func vote(answers []string) (string, int) {
	count := map[string]int{}
	first := map[string]string{}
	var best string
	for _, a := range answers {
		k := strings.ToLower(strings.Trim(a, " .,;:!?\"'`*"))
		if k == "" {
			continue
		}
		if _, ok := first[k]; !ok {
			first[k] = a
		}
		count[k]++
		if count[k] > count[best] {
			best = k
		}
	}
	return first[best], count[best]
}

func ask(c *http.Client, base, key, model, prompt string, temp float64, max int) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model": model, "temperature": temp, "max_tokens": max, "stream": false,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequest("POST", strings.TrimRight(base, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("%s: %s: %s", model, resp.Status, bytes.TrimSpace(raw))
	}
	var out struct {
		Choices []struct {
			Message struct{ Content string }
		}
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("%s: unreadable reply", model)
	}
	return out.Choices[0].Message.Content, nil
}

// run holds the whole procedure so tests can drive it.
func run(c *http.Client, base, key string, seats []string, judge, question string, temp float64, max int, log io.Writer) (string, bool, error) {
	drafts := make([]string, len(seats))
	errs := make([]error, len(seats))
	var wg sync.WaitGroup
	for i, s := range seats {
		wg.Add(1)
		go func() {
			defer wg.Done()
			drafts[i], errs[i] = ask(c, base, key, s, question+tail, temp, max)
		}()
	}
	wg.Wait()
	answers := make([]string, len(seats))
	live := 0
	for i := range seats {
		if errs[i] != nil {
			fmt.Fprintf(log, "seat %d (%s): %v\n", i+1, seats[i], errs[i])
			continue
		}
		live++
		answers[i] = extract(drafts[i])
		fmt.Fprintf(log, "seat %d (%s): %q\n", i+1, seats[i], answers[i])
	}
	if live == 0 {
		return "", false, fmt.Errorf("no seat answered")
	}
	best, n := vote(answers)
	if 2*n > len(seats) {
		fmt.Fprintf(log, "majority: %d of %d\n", n, len(seats))
		return best, true, nil
	}
	if judge == "" {
		fmt.Fprintf(log, "split vote: best answer has %d of %d\n", n, len(seats))
		return best, false, nil
	}
	var b strings.Builder
	b.WriteString("Several assistants answered the question below. Some drafts may be wrong. " +
		"Weigh them against each other, do not simply repeat the most common one, and give the answer you can best defend.\n\nQuestion: ")
	b.WriteString(question)
	for i, d := range drafts {
		if errs[i] == nil {
			fmt.Fprintf(&b, "\n\n--- Draft %d ---\n%s", i+1, d)
		}
	}
	reply, err := ask(c, base, key, judge, b.String()+tail, 0, max)
	if err != nil {
		return best, false, err
	}
	fmt.Fprintf(log, "split vote: decided by judge %s\n", judge)
	return extract(reply), true, nil
}

func main() {
	base := flag.String("url", "http://127.0.0.1:4000", "base address of the gateway or server")
	seatList := flag.String("seats", "brain,brain,brain", "comma-separated model aliases, one per seat")
	judge := flag.String("judge", "", "alias that settles a split vote (optional)")
	temp := flag.Float64("temp", 0.8, "sampling temperature for the seats")
	max := flag.Int("max", 512, "max tokens per seat")
	keyEnv := flag.String("key-env", "GATE_KEY", "environment variable holding the bearer key")
	timeout := flag.Duration("timeout", 30*time.Minute, "time limit per request")
	flag.Parse()

	q := strings.Join(flag.Args(), " ")
	if q == "" {
		in, _ := io.ReadAll(os.Stdin)
		q = strings.TrimSpace(string(in))
	}
	if q == "" {
		fmt.Fprintln(os.Stderr, "usage: council [flags] question   (or the question on standard input)")
		os.Exit(64)
	}
	c := &http.Client{Timeout: *timeout}
	ans, decided, err := run(c, *base, os.Getenv(*keyEnv), strings.Split(*seatList, ","), *judge, q, *temp, *max, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "council:", err)
		os.Exit(1)
	}
	fmt.Println(ans)
	if !decided {
		os.Exit(2) // split vote and no judge: the caller should not trust the line above
	}
}
