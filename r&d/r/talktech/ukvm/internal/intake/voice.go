// Package intake converts what the staff hears and sees into text and
// structured orders: microphone audio, display frames, images and PDFs.
//
// Heavy lifting is done by small external programs (cactus, tesseract,
// pdftotext, v4l2-ctl, ssh) joined by pipes, so each stage can be run,
// replaced or moved to another machine on its own.
package intake

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"ukvm/internal/order"
)

// Runner runs an external program and returns its standard output.
type Runner func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

// Exec is the real Runner.
func Exec(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		return out, fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return out, nil
}

// Rate is the only sample rate used: 16 kHz mono signed 16-bit.
const Rate = 16000

// WAV wraps raw s16le mono PCM in a canonical 44-byte header.
func WAV(pcm []byte) []byte {
	var b bytes.Buffer
	le := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	le(uint32(36 + len(pcm)))
	b.WriteString("WAVEfmt ")
	le(uint32(16))       // fmt chunk size
	le(uint16(1))        // PCM
	le(uint16(1))        // mono
	le(uint32(Rate))     // sample rate
	le(uint32(Rate * 2)) // byte rate
	le(uint16(2))        // block align
	le(uint16(16))       // bits per sample
	b.WriteString("data")
	le(uint32(len(pcm)))
	b.Write(pcm)
	return b.Bytes()
}

// Segmenter is a streaming energy gate that cuts a continuous microphone
// stream into utterances. It is intentionally simple: push-to-talk or a
// wake word in front of it does the real job of deciding when to listen.
type Segmenter struct {
	Frame      int     // samples per frame (default 320 = 20 ms)
	StartRatio float64 // speech starts above noise*StartRatio (default 4)
	EndRatio   float64 // and ends below noise*EndRatio (default 2)
	TailFrames int     // quiet frames that close an utterance (default 25 = 500 ms)
	MinSamples int     // shorter blips are dropped (default 0.5 s)
	MaxSamples int     // utterances are cut at this length (default 15 s)
	MinEnergy  float64 // absolute floor so digital silence never triggers

	noise float64
	buf   []int16
	quiet int
	carry []int16
}

func (s *Segmenter) defaults() {
	if s.Frame == 0 {
		s.Frame, s.StartRatio, s.EndRatio = 320, 4, 2
		s.TailFrames, s.MinSamples, s.MaxSamples = 25, Rate/2, Rate*15
		s.MinEnergy, s.noise = 2e4, 2e4
	}
}

// Write feeds samples and returns any utterances completed by them.
func (s *Segmenter) Write(samples []int16) [][]int16 {
	s.defaults()
	var out [][]int16
	s.carry = append(s.carry, samples...)
	for len(s.carry) >= s.Frame {
		f := s.carry[:s.Frame]
		s.carry = s.carry[s.Frame:]
		var e float64
		for _, v := range f {
			e += float64(v) * float64(v)
		}
		e /= float64(len(f))
		floor := s.noise
		if floor < s.MinEnergy {
			floor = s.MinEnergy
		}
		if len(s.buf) == 0 {
			if e < floor*s.StartRatio {
				s.noise = (s.noise*31 + e) / 32 // track the room while idle
				continue
			}
		}
		s.buf = append(s.buf, f...)
		if e < floor*s.EndRatio {
			s.quiet++
		} else {
			s.quiet = 0
		}
		if s.quiet >= s.TailFrames || len(s.buf) >= s.MaxSamples {
			if len(s.buf)-s.quiet*s.Frame >= s.MinSamples {
				out = append(out, append([]int16(nil), s.buf...))
			}
			s.buf, s.quiet = s.buf[:0], 0
		}
	}
	return out
}

// Heard is the JSON printed by one cactus call that loads Needle and Whistle
// together: the transcript plus the tool calls decoded from it.
type Heard struct {
	Success    bool         `json:"success"`
	Calls      []order.Call `json:"function_calls"`
	Held       []order.Call `json:"suppressed_calls"`
	Confidence *float64     `json:"confidence"`
	Text       string       `json:"audio_text"`
	Language   string       `json:"audio_language"`
}

// Cactus invokes the Cactus CLI runner as a subprocess.
//
// The argument shape follows the Cactus 4.0 documentation as summarised in
// the project's field guide. Confirm it against the runner you install; it
// is the one part of this package that could not be exercised without the
// engine and its model files.
type Cactus struct {
	Bin, Needle, Whistle, Tools string
	Run                         Runner
}

// Hear transcribes one WAV clip and decodes tool calls from it.
func (c Cactus) Hear(ctx context.Context, wavPath string) (Heard, error) {
	var h Heard
	out, err := c.Run(ctx, nil, c.Bin,
		"--model", c.Needle, "--model", c.Whistle,
		"--tools", c.Tools, "--audio", wavPath)
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(out, &h); err != nil {
		return h, fmt.Errorf("cactus output is not JSON: %w", err)
	}
	return h, nil
}

// phantoms are transcripts small speech models produce from noise.
var phantoms = map[string]bool{"": true, "thank you": true, "thanks": true, "you": true, "thanks for watching": true}

// Usable extracts an order from what was heard, or says why there is none.
// A missing confidence (fine-tuned weights report none) is treated as 0.6,
// which the gate turns into "ask first".
func (h Heard) Usable() (calls []order.Call, conf float64, ok bool, why string) {
	if !h.Success {
		return nil, 0, false, "the speech engine reported failure"
	}
	t := strings.ToLower(strings.Trim(strings.TrimSpace(h.Text), ".!?"))
	if phantoms[t] {
		return nil, 0, false, "nothing intelligible was said"
	}
	if len(h.Calls) == 0 {
		return nil, 0, false, "no order recognised in: " + h.Text
	}
	conf = 0.6
	if h.Confidence != nil {
		conf = *h.Confidence
	}
	return h.Calls, conf, true, ""
}
