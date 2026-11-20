// Package hid is the one place that knows USB HID keyboard and pointer
// encoding. Every actuator (CH9329 serial bridge, Linux USB gadget, dry run)
// consumes the reports built here, so a layout fix lands everywhere at once.
//
// Usage IDs come from the USB HID Usage Tables, Keyboard/Keypad page (0x07).
// The table is US QWERTY. A usage ID names a key position, and the target
// computer's own keymap decides what that key prints, so a target configured
// for another layout needs another table.
package hid

import (
	"fmt"
	"strings"
)

// Modifier bits: byte 0 of the boot keyboard report.
const (
	LCtrl byte = 1 << iota
	LShift
	LAlt
	LGUI
	RCtrl
	RShift
	RAlt
	RGUI
)

// KeyReport is the 8-byte boot-protocol keyboard input report:
// [modifiers, reserved, key1 ... key6].
type KeyReport [8]byte

// Release is the all-keys-up report.
var Release KeyReport

type key struct{ mod, code byte }

var layoutUS = func() map[rune]key {
	m := map[rune]key{'\n': {0, 0x28}, '\t': {0, 0x2b}, ' ': {0, 0x2c}}
	for i := 0; i < 26; i++ {
		m[rune('a'+i)] = key{0, byte(0x04 + i)}
		m[rune('A'+i)] = key{LShift, byte(0x04 + i)}
	}
	digits, shifted := "1234567890", "!@#$%^&*()"
	for i := range digits {
		m[rune(digits[i])] = key{0, byte(0x1e + i)}
		m[rune(shifted[i])] = key{LShift, byte(0x1e + i)}
	}
	for _, p := range []struct {
		plain, shift rune
		code         byte
	}{
		{'-', '_', 0x2d}, {'=', '+', 0x2e}, {'[', '{', 0x2f}, {']', '}', 0x30},
		{'\\', '|', 0x31}, {';', ':', 0x33}, {'\'', '"', 0x34}, {'`', '~', 0x35},
		{',', '<', 0x36}, {'.', '>', 0x37}, {'/', '?', 0x38},
	} {
		m[p.plain] = key{0, p.code}
		m[p.shift] = key{LShift, p.code}
	}
	return m
}()

var named = func() map[string]byte {
	m := map[string]byte{
		"enter": 0x28, "return": 0x28, "esc": 0x29, "escape": 0x29,
		"backspace": 0x2a, "tab": 0x2b, "space": 0x2c, "capslock": 0x39,
		"printscreen": 0x46, "insert": 0x49, "home": 0x4a, "pageup": 0x4b,
		"pgup": 0x4b, "delete": 0x4c, "del": 0x4c, "end": 0x4d,
		"pagedown": 0x4e, "pgdn": 0x4e, "right": 0x4f, "left": 0x50,
		"down": 0x51, "up": 0x52,
	}
	for i := 1; i <= 12; i++ {
		m[fmt.Sprintf("f%d", i)] = byte(0x3a + i - 1)
	}
	return m
}()

var modNames = map[string]byte{
	"ctrl": LCtrl, "control": LCtrl, "shift": LShift, "alt": LAlt,
	"win": LGUI, "gui": LGUI, "super": LGUI, "meta": LGUI, "cmd": LGUI,
}

// NewReport builds a report from modifier bits and up to six key codes.
func NewReport(mod byte, codes ...byte) (KeyReport, error) {
	var r KeyReport
	if len(codes) > 6 {
		return r, fmt.Errorf("hid: %d keys in one report, the boot protocol carries 6", len(codes))
	}
	r[0] = mod
	copy(r[2:], codes)
	return r, nil
}

// ParseChord turns "ctrl+alt+t", "enter" or "shift+tab" into one report.
// The caller sends it followed by Release.
func ParseChord(s string) (KeyReport, error) {
	var mod byte
	var codes []byte
	for _, p := range strings.Split(strings.ToLower(strings.TrimSpace(s)), "+") {
		p = strings.TrimSpace(p)
		if p == "" {
			return Release, fmt.Errorf("hid: empty key in chord %q", s)
		}
		if m, ok := modNames[p]; ok {
			mod |= m
			continue
		}
		if c, ok := named[p]; ok {
			codes = append(codes, c)
			continue
		}
		if r := []rune(p); len(r) == 1 {
			if k, ok := layoutUS[r[0]]; ok {
				mod |= k.mod
				codes = append(codes, k.code)
				continue
			}
		}
		return Release, fmt.Errorf("hid: unknown key %q in chord %q", p, s)
	}
	return NewReport(mod, codes...)
}

// Type returns the press/release report sequence that types s.
// It fails on the first rune the layout cannot produce; nothing is sent
// half-typed because the caller gets no reports at all.
func Type(s string) ([]KeyReport, error) {
	out := make([]KeyReport, 0, 2*len(s))
	for i, r := range s {
		k, ok := layoutUS[r]
		if !ok {
			return nil, fmt.Errorf("hid: cannot type %q (byte offset %d) on a US layout", r, i)
		}
		out = append(out, KeyReport{k.mod, 0, k.code}, Release)
	}
	return out, nil
}

// AbsMax is the logical maximum of the gadget's absolute pointer axes.
const AbsMax = 32767

// AbsReport is the 5-byte absolute pointer report used by the USB gadget
// descriptor in deploy/gadget-pod.sh: [buttons, x lo, x hi, y lo, y hi].
// Coordinates are given on the 0..1000 scale used throughout this system.
func AbsReport(buttons byte, x1000, y1000 int) [5]byte {
	x, y := Scale(x1000, AbsMax), Scale(y1000, AbsMax)
	return [5]byte{buttons, byte(x), byte(x >> 8), byte(y), byte(y >> 8)}
}

// Scale maps n on 0..1000 (clamped) to 0..max.
func Scale(n, max int) int {
	if n < 0 {
		n = 0
	}
	if n > 1000 {
		n = 1000
	}
	return n * max / 1000
}
