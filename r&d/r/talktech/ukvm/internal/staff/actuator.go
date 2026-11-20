package staff

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"ukvm/internal/ch9329"
	"ukvm/internal/hid"
	"ukvm/internal/intake"
)

// Hands is a keyboard and pointer plugged into one target computer.
type Hands interface {
	Keys(reports []hid.KeyReport) error
	Pointer(buttons byte, x1000, y1000 int) error
}

// Eyes is a view of one target computer's display.
type Eyes interface {
	// Frame returns the path of a fresh image of the display and a
	// function that discards it.
	Frame(ctx context.Context) (path string, done func(), err error)
}

// SerialHands drives a CH9329 USB cable through its serial end. The port is
// opened for each burst and closed afterwards, so unplugging and replugging
// a tether needs no restart.
type SerialHands struct {
	Dev  string
	Gap  time.Duration                            // pause between frames (default 20 ms; the link is 9600 baud)
	Open func(dev string) (io.WriteCloser, error) // nil means a real serial port
}

func (h SerialHands) open() (io.WriteCloser, time.Duration, error) {
	gap := h.Gap
	if gap == 0 {
		gap = 20 * time.Millisecond
	}
	if h.Open != nil {
		w, err := h.Open(h.Dev)
		return w, gap, err
	}
	if out, err := exec.Command("stty", "-F", h.Dev, "9600", "cs8", "-cstopb", "-parenb", "raw", "-echo").CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("stty %s: %v: %s", h.Dev, err, strings.TrimSpace(string(out)))
	}
	f, err := os.OpenFile(h.Dev, os.O_RDWR|syscall.O_NOCTTY, 0)
	return f, gap, err
}

func (h SerialHands) Keys(reps []hid.KeyReport) error {
	w, gap, err := h.open()
	if err != nil {
		return err
	}
	defer w.Close()
	defer w.Write(ch9329.Keyboard(hid.Release)) // whatever happens, leave no key held
	for _, r := range reps {
		if _, err := w.Write(ch9329.Keyboard(r)); err != nil {
			return err
		}
		time.Sleep(gap)
	}
	return nil
}

func (h SerialHands) Pointer(buttons byte, x, y int) error {
	w, gap, err := h.open()
	if err != nil {
		return err
	}
	defer w.Close()
	if _, err := w.Write(ch9329.MouseAbs(buttons, x, y)); err != nil {
		return err
	}
	time.Sleep(gap)
	return nil
}

// GadgetHands writes reports to Linux USB gadget HID devices on a board
// whose own USB port is plugged into the target (see deploy/gadget-pod.sh).
type GadgetHands struct {
	Kbd, Ptr string // for example /dev/hidg0 and /dev/hidg1
	Gap      time.Duration
}

func (h GadgetHands) Keys(reps []hid.KeyReport) error {
	f, err := os.OpenFile(h.Kbd, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	defer f.Write(hid.Release[:])
	gap := h.Gap
	if gap == 0 {
		gap = 8 * time.Millisecond
	}
	for _, r := range reps {
		if _, err := f.Write(r[:]); err != nil {
			return err
		}
		time.Sleep(gap)
	}
	return nil
}

func (h GadgetHands) Pointer(buttons byte, x, y int) error {
	f, err := os.OpenFile(h.Ptr, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	r := hid.AbsReport(buttons, x, y)
	_, err = f.Write(r[:])
	return err
}

// RecordHands remembers what it was asked to do. It backs the simulator and
// the tests.
type RecordHands struct {
	mu     sync.Mutex
	Keyed  []hid.KeyReport
	Points [][3]int // buttons, x, y
}

func (h *RecordHands) Keys(reps []hid.KeyReport) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Keyed = append(h.Keyed, reps...)
	return nil
}

func (h *RecordHands) Pointer(buttons byte, x, y int) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Points = append(h.Points, [3]int{int(buttons), x, y})
	return nil
}

// Counts reports how many key reports and pointer reports were recorded.
func (h *RecordHands) Counts() (keys, points int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.Keyed), len(h.Points)
}

// FileEyes serves an image file that something else keeps current: a
// simulator fixture, a camera snapshot, or stills copied off a recorder.
type FileEyes struct{ Path string }

func (e FileEyes) Frame(context.Context) (string, func(), error) {
	if _, err := os.Stat(e.Path); err != nil {
		return "", func() {}, err
	}
	return e.Path, func() {}, nil
}

// CaptureEyes grabs one frame from an HDMI capture device by running a
// command. Argv is a template; "{dev}" and "{out}" are substituted. The
// default uses ffmpeg, which decodes the MJPEG variants cheap UVC dongles
// emit and writes a PNG that the rest of the pipeline can always read.
type CaptureEyes struct {
	Dev  string
	Argv []string
	Run  intake.Runner
}

// DefaultGrab is the frame-grab command used when Argv is empty.
var DefaultGrab = []string{"ffmpeg", "-loglevel", "error", "-f", "v4l2", "-input_format", "mjpeg",
	"-i", "{dev}", "-ss", "0.3", "-frames:v", "1", "-y", "{out}"}

func (e CaptureEyes) Frame(ctx context.Context) (string, func(), error) {
	tmp, err := os.CreateTemp("", "cos-frame-*.png")
	if err != nil {
		return "", func() {}, err
	}
	tmp.Close()
	done := func() { os.Remove(tmp.Name()) }
	argv := e.Argv
	if len(argv) == 0 {
		argv = DefaultGrab
	}
	args := make([]string, len(argv))
	for i, a := range argv {
		args[i] = strings.NewReplacer("{dev}", e.Dev, "{out}", tmp.Name()).Replace(a)
	}
	run := e.Run
	if run == nil {
		run = intake.Exec
	}
	if _, err := run(ctx, nil, args[0], args[1:]...); err != nil {
		done()
		return "", func() {}, err
	}
	return tmp.Name(), done, nil
}
