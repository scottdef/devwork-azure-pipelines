package intake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // frames from capture devices arrive as MJPEG stills
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Tools groups the document and image converters around one Runner.
type Tools struct{ Run Runner }

// PDFText converts a PDF to text page by page. Pages with a text layer go
// through pdftotext; pages without one (scans) are rasterised and read with
// Tesseract. Pages are separated by form feeds.
func (t Tools) PDFText(ctx context.Context, path string) (string, error) {
	info, err := t.Run(ctx, nil, "pdfinfo", path)
	if err != nil {
		return "", err
	}
	pages := 0
	for _, line := range strings.Split(string(info), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[0] == "Pages:" {
			pages, _ = strconv.Atoi(f[1])
		}
	}
	if pages < 1 {
		return "", errors.New("pdfinfo reported no pages")
	}
	tmp, err := os.MkdirTemp("", "cos-pdf-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)

	var all strings.Builder
	for p := 1; p <= pages; p++ {
		n := strconv.Itoa(p)
		txt, err := t.Run(ctx, nil, "pdftotext", "-layout", "-f", n, "-l", n, path, "-")
		if err != nil {
			return "", fmt.Errorf("page %d: %w", p, err)
		}
		if len(strings.Join(strings.Fields(string(txt)), "")) < 20 { // no text layer
			base := filepath.Join(tmp, "p"+n)
			if _, err := t.Run(ctx, nil, "pdftoppm", "-r", "200", "-f", n, "-l", n, "-singlefile", "-png", path, base); err != nil {
				return "", fmt.Errorf("page %d: %w", p, err)
			}
			if txt, err = t.Run(ctx, nil, "tesseract", base+".png", "stdout", "--psm", "3"); err != nil {
				return "", fmt.Errorf("page %d: %w", p, err)
			}
		}
		if p > 1 {
			all.WriteByte('\f')
		}
		all.Write(bytes.TrimRight(txt, "\f\n "))
		all.WriteByte('\n')
	}
	return all.String(), nil
}

// PDFTextVia converts a PDF on a lender: the file goes over the SSH
// connection on standard input and the text comes back on standard output.
// Nothing is installed or left behind on the lender beyond poppler itself.
// It handles text-layer PDFs; scans fall back to the local path.
func (t Tools) PDFTextVia(ctx context.Context, ssh, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	out, err := t.Run(ctx, f, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", ssh, "pdftotext", "-layout", "-", "-")
	return string(out), err
}

// ImageText reads the text in a screenshot or captured frame.
func (t Tools) ImageText(ctx context.Context, path string) (string, error) {
	prepped, _, _, err := prepFile(path)
	if err != nil {
		return "", err
	}
	defer os.Remove(prepped)
	out, err := t.Run(ctx, nil, "tesseract", prepped, "stdout", "--psm", "6")
	return string(out), err
}

// ImageTextVia does the same on a lender, streaming the prepared image.
func (t Tools) ImageTextVia(ctx context.Context, ssh, path string) (string, error) {
	prepped, _, _, err := prepFile(path)
	if err != nil {
		return "", err
	}
	defer os.Remove(prepped)
	f, err := os.Open(prepped)
	if err != nil {
		return "", err
	}
	defer f.Close()
	out, err := t.Run(ctx, f, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", ssh, "tesseract", "stdin", "stdout", "--psm", "6")
	return string(out), err
}

// Locate finds a word or phrase on screen and returns the centre of its
// bounding box on the 0..1000 scale used for pointer positions.
func (t Tools) Locate(ctx context.Context, path, phrase string) (x1000, y1000 int, err error) {
	prepped, w, h, err := prepFile(path)
	if err != nil {
		return 0, 0, err
	}
	defer os.Remove(prepped)
	tsv, err := t.Run(ctx, nil, "tesseract", prepped, "stdout", "--psm", "11", "tsv")
	if err != nil {
		return 0, 0, err
	}
	x, y, n := LocateTSV(string(tsv), phrase, w, h)
	switch {
	case n == 0:
		return 0, 0, fmt.Errorf("%q is not visible on the screen", phrase)
	case n > 1:
		return 0, 0, fmt.Errorf("%q appears %d times on the screen; click by position instead", phrase, n)
	}
	return x, y, nil
}

func prepFile(path string) (out string, w, h int, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return "", 0, 0, fmt.Errorf("%s: %w", path, err)
	}
	g := Prep(src)
	tmp, err := os.CreateTemp("", "cos-ocr-*.png")
	if err != nil {
		return "", 0, 0, err
	}
	defer tmp.Close()
	if err := png.Encode(tmp, g); err != nil {
		os.Remove(tmp.Name())
		return "", 0, 0, err
	}
	return tmp.Name(), g.Rect.Dx(), g.Rect.Dy(), nil
}

// Prep makes a frame easier for Tesseract: greyscale, dark-on-light (console
// screens are the reverse), and doubled in size only when the capture is a
// low-resolution mode such as a 640x480 firmware screen. Larger frames are
// left at their own size: on the test fixtures, enlarging a 720p dialog made
// Tesseract stop seeing the text inside button outlines.
func Prep(src image.Image) *image.Gray {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	g := image.NewGray(image.Rect(0, 0, w, h))
	var sum uint64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := color.GrayModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
			g.Pix[y*g.Stride+x] = v
			sum += uint64(v)
		}
	}
	if w == 0 || h == 0 {
		return g
	}
	invert := sum/uint64(w*h) < 110
	scale := 1
	if w < 1000 {
		scale = 2
	}
	out := image.NewGray(image.Rect(0, 0, w*scale, h*scale))
	for y := 0; y < h*scale; y++ {
		for x := 0; x < w*scale; x++ {
			v := g.Pix[(y/scale)*g.Stride+x/scale]
			if invert {
				v = 255 - v
			}
			out.Pix[y*out.Stride+x] = v
		}
	}
	return out
}

// LocateTSV searches Tesseract TSV output for phrase (consecutive words on
// one line) in an image of w by h pixels. A match that equals the phrase
// exactly outranks one that differs in case or trailing punctuation, so the
// button "Restart" wins over the sentence "...and restart?". n is the number
// of matches at the winning rank: 0 means absent, more than 1 means the
// caller cannot know which one was meant.
func LocateTSV(tsv, phrase string, w, h int) (x1000, y1000, n int) {
	type word struct {
		line         string
		l, t, wd, ht int
		raw, text    string
	}
	var ws []word
	for _, row := range strings.Split(tsv, "\n") {
		c := strings.Split(strings.TrimRight(row, "\r"), "\t")
		if len(c) < 12 || c[0] != "5" {
			continue
		}
		num := func(i int) int { v, _ := strconv.Atoi(c[i]); return v }
		ws = append(ws, word{strings.Join(c[1:5], "."), num(6), num(7), num(8), num(9), strings.TrimSpace(c[11]), norm(c[11])})
	}
	exact := strings.Fields(phrase)
	want := make([]string, len(exact))
	for i, e := range exact {
		want[i] = norm(e)
	}
	if len(want) == 0 || w == 0 || h == 0 {
		return 0, 0, 0
	}
	best := 0
	for i := 0; i+len(want) <= len(ws); i++ {
		rank := 2
		for j := range want {
			if ws[i+j].line != ws[i].line || ws[i+j].text != want[j] {
				rank = 0
				break
			}
			if ws[i+j].raw != exact[j] {
				rank = 1
			}
		}
		if rank == 0 || rank < best {
			continue
		}
		if rank > best {
			best, n = rank, 0
		}
		if n++; n == 1 {
			first, last := ws[i], ws[i+len(want)-1]
			x1000 = (first.l + last.l + last.wd) / 2 * 1000 / w
			y1000 = (first.t + first.ht/2) * 1000 / h
		}
	}
	return x1000, y1000, n
}

func norm(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), ".,:;!?()[]{}<>|\"'"))
}
