// Package ch9329 encodes frames for the WCH CH9329 UART-to-USB-HID bridge.
//
// The chip sits in a USB cable: one end is a serial port on a staff node,
// the other enumerates on the target computer as an ordinary USB keyboard
// and mouse. Because it is a real HID device with a boot-protocol keyboard,
// it works in firmware setup screens and needs nothing installed on the
// target.
//
// Frame layout (WCH "CH9329 serial communication protocol" V1.0):
//
//	HEAD(2)=57 AB | ADDR(1)=00 | CMD(1) | LEN(1) | DATA(LEN) | SUM(1)
//
// SUM is the low byte of the sum of every preceding byte. The default line
// setting is 9600 8N1.
package ch9329

import (
	"errors"
	"fmt"

	"ukvm/internal/hid"
)

// Command codes sent to the chip.
const (
	CmdGetInfo  byte = 0x01
	CmdKeyboard byte = 0x02
	CmdMouseAbs byte = 0x04
	CmdMouseRel byte = 0x05
)

// AbsMax is the largest coordinate of the chip's 12-bit absolute pointer.
const AbsMax = 4095

// Frame wraps data in a protocol frame addressed to the default chip (0x00).
func Frame(cmd byte, data []byte) []byte {
	f := make([]byte, 0, 6+len(data))
	f = append(f, 0x57, 0xAB, 0x00, cmd, byte(len(data)))
	f = append(f, data...)
	var sum byte
	for _, b := range f {
		sum += b
	}
	return append(f, sum)
}

// Keyboard carries one 8-byte boot keyboard report.
func Keyboard(r hid.KeyReport) []byte { return Frame(CmdKeyboard, r[:]) }

// MouseAbsRaw positions the pointer using the chip's own 0..4095 axes.
func MouseAbsRaw(buttons byte, x, y uint16, wheel int8) []byte {
	return Frame(CmdMouseAbs, []byte{0x02, buttons, byte(x), byte(x >> 8), byte(y), byte(y >> 8), byte(wheel)})
}

// MouseAbs positions the pointer using the system-wide 0..1000 scale.
func MouseAbs(buttons byte, x1000, y1000 int) []byte {
	return MouseAbsRaw(buttons, uint16(hid.Scale(x1000, AbsMax)), uint16(hid.Scale(y1000, AbsMax)), 0)
}

// MouseRel moves the pointer relatively; some firmware setup screens only
// understand relative mice.
func MouseRel(buttons byte, dx, dy, wheel int8) []byte {
	return Frame(CmdMouseRel, []byte{0x01, buttons, byte(dx), byte(dy), byte(wheel)})
}

// GetInfo asks the chip for its version, USB state and lock LEDs.
func GetInfo() []byte { return Frame(CmdGetInfo, nil) }

// ParseReply checks a reply frame and returns its command byte and payload.
// A successful reply echoes the command with bit 7 set.
func ParseReply(b []byte) (cmd byte, data []byte, err error) {
	if len(b) < 6 || b[0] != 0x57 || b[1] != 0xAB {
		return 0, nil, errors.New("ch9329: not a frame")
	}
	n := int(b[4])
	if len(b) != 6+n {
		return 0, nil, fmt.Errorf("ch9329: frame is %d bytes, header says %d", len(b), 6+n)
	}
	var sum byte
	for _, x := range b[:len(b)-1] {
		sum += x
	}
	if sum != b[len(b)-1] {
		return 0, nil, fmt.Errorf("ch9329: checksum %#02x, want %#02x", b[len(b)-1], sum)
	}
	return b[3], b[5 : 5+n], nil
}

// Info is the decoded GetInfo reply. Connected reports whether the target
// has enumerated the USB device, which makes it a cheap liveness probe for
// the tether and for the target's USB stack.
type Info struct {
	Version                       byte
	Connected                     bool
	NumLock, CapsLock, ScrollLock bool
}

// ParseInfo decodes the 8-byte GetInfo payload: version, USB enumeration
// state, lock-LED bits, then reserved bytes. Check the field order against
// the protocol document shipped with your module before relying on it.
func ParseInfo(data []byte) (Info, error) {
	if len(data) < 3 {
		return Info{}, fmt.Errorf("ch9329: info payload is %d bytes", len(data))
	}
	return Info{
		Version:    data[0],
		Connected:  data[1] != 0,
		NumLock:    data[2]&1 != 0,
		CapsLock:   data[2]&2 != 0,
		ScrollLock: data[2]&4 != 0,
	}, nil
}
