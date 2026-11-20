package intake

import (
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"math"
	"os/exec"
	"strings"
	"testing"
)

func needs(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed; skipping the real-converter test", tool)
		}
	}
}

func TestWAVHeader(t *testing.T) {
	w := WAV(make([]byte, 32000)) // one second
	if len(w) != 44+32000 || string(w[:4]) != "RIFF" || string(w[8:16]) != "WAVEfmt " || string(w[36:40]) != "data" {
		t.Fatalf("bad layout: %q", w[:44])
	}
	if got := binary.LittleEndian.Uint32(w[24:28]); got != 16000 {
		t.Errorf("sample rate = %d", got)
	}
	if got := binary.LittleEndian.Uint16(w[22:24]); got != 1 {
		t.Errorf("channels = %d", got)
	}
	if got := binary.LittleEndian.Uint32(w[40:44]); got != 32000 {
		t.Errorf("data length = %d", got)
	}
}

func tone(seconds float64, amp float64) []int16 {
	out := make([]int16, int(seconds*Rate))
	for i := range out {
		out[i] = int16(amp * math.Sin(2*math.Pi*440*float64(i)/Rate))
	}
	return out
}

func TestSegmenter(t *testing.T) {
	var s Segmenter
	var stream []int16
	stream = append(stream, tone(1.0, 40)...)   // room noise
	stream = append(stream, tone(1.5, 9000)...) // an order
	stream = append(stream, tone(1.0, 40)...)
	stream = append(stream, tone(0.1, 9000)...) // a door slam: too short
	stream = append(stream, tone(1.0, 40)...)
	stream = append(stream, tone(0.8, 9000)...) // a second order
	stream = append(stream, tone(1.0, 40)...)

	var got [][]int16
	for i := 0; i < len(stream); i += 1000 { // arbitrary chunking, as from a pipe
		end := i + 1000
		if end > len(stream) {
			end = len(stream)
		}
		got = append(got, s.Write(stream[i:end])...)
	}
	if len(got) != 2 {
		t.Fatalf("got %d utterances, want 2", len(got))
	}
	for i, wantSec := range []float64{1.5, 0.8} {
		sec := float64(len(got[i]))/Rate - 0.5 // minus the 500 ms tail
		if math.Abs(sec-wantSec) > 0.1 {
			t.Errorf("utterance %d is %.2fs of speech, want about %.1fs", i, sec, wantSec)
		}
	}
	var silent Segmenter
	if u := silent.Write(make([]int16, Rate*3)); len(u) != 0 {
		t.Error("digital silence produced an utterance")
	}
}

func TestCactusHear(t *testing.T) {
	var gotArgs []string
	c := Cactus{Bin: "cactus", Needle: "needle3.cact", Whistle: "whistle.cact", Tools: "tools.json",
		Run: func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
			gotArgs = append([]string{name}, args...)
			return []byte(`{"success":true,"audio_text":"Switch to server three and run uptime.","audio_language":"en",
				"confidence":0.93,"function_calls":[
				{"name":"select_target","arguments":{"target":"server3"}},
				{"name":"run_command","arguments":{"command":"uptime"}}]}`), nil
		}}
	h, err := c.Hear(context.Background(), "/tmp/u.wav")
	if err != nil {
		t.Fatal(err)
	}
	want := "cactus --model needle3.cact --model whistle.cact --tools tools.json --audio /tmp/u.wav"
	if strings.Join(gotArgs, " ") != want {
		t.Errorf("argv = %q", strings.Join(gotArgs, " "))
	}
	calls, conf, ok, _ := h.Usable()
	if !ok || conf != 0.93 || len(calls) != 2 || calls[1].Str("command") != "uptime" {
		t.Errorf("calls=%v conf=%v ok=%v", calls, conf, ok)
	}
}

func TestUsableDropsPhantomsAndDefaultsConfidence(t *testing.T) {
	for _, text := range []string{"", "Thank you.", " thanks for watching! "} {
		h := Heard{Success: true, Text: text}
		if _, _, ok, _ := h.Usable(); ok {
			t.Errorf("%q was treated as an order", text)
		}
	}
	if _, _, ok, _ := (Heard{Success: false, Text: "run uptime"}).Usable(); ok {
		t.Error("a failed engine call produced an order")
	}
	h := Heard{Success: true, Text: "run uptime"}
	if _, _, ok, _ := h.Usable(); ok {
		t.Error("a transcript with no tool calls produced an order")
	}
	h.Calls = append(h.Calls, struct {
		Name string         `json:"name"`
		Args map[string]any `json:"arguments,omitempty"`
	}{Name: "status"})
	if _, conf, ok, _ := h.Usable(); !ok || conf != 0.6 {
		t.Errorf("missing confidence gave %v (ok=%v), want 0.6", conf, ok)
	}
}

func TestPrepInvertsDarkScreens(t *testing.T) {
	dark := image.NewRGBA(image.Rect(0, 0, 100, 50))
	dark.Set(10, 10, color.White) // one lit pixel on black
	g := Prep(dark)
	if g.Rect.Dx() != 200 || g.Rect.Dy() != 100 {
		t.Errorf("small frame was not doubled: %v", g.Rect)
	}
	if g.GrayAt(0, 0).Y != 255 || g.GrayAt(20, 20).Y != 0 {
		t.Error("dark background was not inverted to dark-on-light")
	}
	light := image.NewGray(image.Rect(0, 0, 1920, 4))
	for i := range light.Pix {
		light.Pix[i] = 240
	}
	g = Prep(light)
	if g.Rect.Dx() != 1920 || g.GrayAt(0, 0).Y != 240 {
		t.Error("a full-size light frame should pass through unchanged")
	}
}

func TestLocateTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"5\t1\t1\t1\t1\t1\t100\t100\t80\t20\t96\tApply\n" +
		"5\t1\t1\t1\t1\t2\t190\t100\t120\t20\t96\tsettings\n" +
		"5\t1\t1\t1\t1\t3\t320\t100\t60\t20\t96\tand\n" +
		"5\t1\t1\t1\t1\t4\t390\t100\t110\t20\t96\trestart?\n" +
		"5\t1\t2\t1\t1\t1\t800\t400\t200\t40\t95\tRestart\n" +
		"5\t1\t3\t1\t1\t1\t100\t600\t60\t20\t95\tOK\n" +
		"5\t1\t4\t1\t1\t1\t900\t600\t60\t20\t95\tOK\n"
	// The exact label beats the same word inside the question above it.
	if x, y, n := LocateTSV(tsv, "Restart", 2000, 1000); n != 1 || x != 450 || y != 420 {
		t.Errorf("Restart at %d,%d n=%d, want 450,420 n=1", x, y, n)
	}
	if x, y, n := LocateTSV(tsv, "Apply settings", 2000, 1000); n != 1 || x != 102 || y != 110 {
		t.Errorf("phrase at %d,%d n=%d, want 102,110 n=1", x, y, n)
	}
	if _, _, n := LocateTSV(tsv, "OK", 2000, 1000); n != 2 {
		t.Errorf("two OK buttons reported as %d matches; the caller must be told it is ambiguous", n)
	}
	if _, _, n := LocateTSV(tsv, "Apply Restart", 2000, 1000); n != 0 {
		t.Error("words on different lines matched as a phrase")
	}
	if _, _, n := LocateTSV(tsv, "Shutdown", 2000, 1000); n != 0 {
		t.Error("absent text was located")
	}
}

// The tests below run the real converters against committed fixtures.

func TestImageTextReadsAConsole(t *testing.T) {
	needs(t, "tesseract")
	txt, err := Tools{Run: Exec}.ImageText(context.Background(), "testdata/terminal.png")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"uptime", "load average: 0.08", "Filesystem", "40%"} {
		if !strings.Contains(txt, want) {
			t.Errorf("OCR text lacks %q:\n%s", want, txt)
		}
	}
}

func TestLocateFindsAButton(t *testing.T) {
	needs(t, "tesseract")
	x, y, err := Tools{Run: Exec}.Locate(context.Background(), "testdata/dialog.png", "Restart")
	if err != nil {
		t.Fatal(err)
	}
	// The button spans x 760..960 of 1280 and y 400..460 of 720.
	if x < 594 || x > 750 || y < 556 || y > 639 {
		t.Errorf("Restart located at %d,%d, outside the button (594..750, 556..639)", x, y)
	}
	if _, _, err := (Tools{Run: Exec}).Locate(context.Background(), "testdata/dialog.png", "Hibernate"); err == nil {
		t.Error("located a label that is not on screen")
	}
}

func TestPDFTextLayer(t *testing.T) {
	needs(t, "pdfinfo", "pdftotext")
	txt, err := Tools{Run: Exec}.PDFText(context.Background(), "testdata/text.pdf")
	if err != nil {
		t.Fatal(err)
	}
	pages := strings.Split(txt, "\f")
	if len(pages) != 2 || !strings.Contains(pages[0], "PDU channel 4") || !strings.Contains(pages[1], "on-duty operator") {
		t.Errorf("unexpected text:\n%q", txt)
	}
}

func TestPDFScanFallsBackToOCR(t *testing.T) {
	needs(t, "pdfinfo", "pdftotext", "pdftoppm", "tesseract")
	txt, err := Tools{Run: Exec}.PDFText(context.Background(), "testdata/scan.pdf")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"BMC Recovery Procedure", "amber light", "port 623"} {
		if !strings.Contains(txt, want) {
			t.Errorf("OCR of the scanned page lacks %q:\n%s", want, txt)
		}
	}
}

func TestPDFTextViaStreamsOverSSH(t *testing.T) {
	var argv string
	var sent int64
	tools := Tools{Run: func(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		argv = name + " " + strings.Join(args, " ")
		sent, _ = io.Copy(io.Discard, stdin)
		return []byte("converted on the lender\n"), nil
	}}
	txt, err := tools.PDFTextVia(context.Background(), "ops@bigbox", "testdata/text.pdf")
	if err != nil || txt != "converted on the lender\n" {
		t.Fatalf("txt=%q err=%v", txt, err)
	}
	if argv != "ssh -o BatchMode=yes -o ConnectTimeout=5 ops@bigbox pdftotext -layout - -" {
		t.Errorf("argv = %q", argv)
	}
	if sent == 0 {
		t.Error("the PDF was not streamed on standard input")
	}
}
