# Go for UI Automation, Input Simulation, and Terminal Control: A Field Guide (late 2026)

You can build every piece of this stack in Go using only the standard library and still produce a single static binary. That covers USB HID gadget keyboards and mice, uinput virtual devices, evdev recording, a VNC/RFB client, PTY recording to asciicast files, template matching, and LLM-driven action loops. Most of the third-party ecosystem has moved past Go 1.24, so a Go 1.24 constraint pushes you toward stdlib code anyway. The one place that is hard without third-party code is the desktop side: Wayland libei, macOS CGEvent, and BlueZ D-Bus.

## TL;DR
- **Hardware and protocol layer: stdlib-only is the right default.** `/dev/hidg*` writes, the `/dev/uinput` ioctls, `/dev/input/event*` decoding, RFC 6143 RFB over `net.Conn`, a PTY via `/dev/ptmx`, and asciicast NDJSON are each 50–200 lines of Go 1.24 code, need no CGO, and cross-compile to Pi targets. Fast-moving libraries now require newer toolchains: go-vgo/robotgo v1.3.5 needs Go 1.26+ and CGO by default, the official MCP go-sdk v1.8.0 and chromedp v0.23.x need Go 1.25, and mark3labs/mcp-go v1.5.0 declares go 1.25.5.
- **Recommended architecture.** Run one control-plane binary (`kvmd`) plus one Pi agent (`kvm-agent`) per target. The agent exposes Plan 9-style files (`ctl`, `kbd`, `mouse`, `screen`, `audit`) over 9P or plain FIFOs. All actions are text lines, so recordings, runbooks, LLM output, and audit logs share one grammar. Prefer the target's BMC (Redfish) when one exists. Use Pi gadgets or commercial IP-KVMs only where there is no BMC, and keep all of them on an isolated management network. Eclypsium's March 17, 2026 disclosure found "9 vulnerabilities across 4 KVM vendors, spanning 7 distinct vulnerability classes" in the GL.iNet Comet RM-1, Angeet/Yeeso ES3, Sipeed NanoKVM, and JetKVM. The Angeet ES3's CVSS 9.8 flaw (CVE-2026-32297) has no fix available.
- **NL and vision.** Treat the LLM as an untrusted planner. It emits JSON actions, which a Go validator checks against an allowlist, then a rate limit and a confirmation gate, before any keystroke is sent. Anthropic's current computer-use interface is the `computer_toolset_20260801` client toolset (no beta header, 17 member tools, batched actions).\[1\] OpenAI exposes a native computer tool in the Responses API, starting with GPT-5.4.\[2\]\[3\] For deterministic replay, compile recordings into a small DSL and assert with template matching or OCR instead of asking a model.

## 1. Ecosystem Survey (verified where noted, Nov 2026)

| Package | Purpose | OS | CGO | License | Go req. / latest | Notes |
|---|---|---|---|---|---|---|
| go-vgo/robotgo | Desktop mouse, keyboard, and screen | Win/mac/X11 | Yes (pure-Go mode experimental) | Apache-2.0 | **Go 1.26+**, v1.3.5 (Oct 2026) | Disqualified on both constraints |
| marang/robotgo fork | Adds Wayland/portal support | Linux/mac/Win | Partial | Apache-2.0 | **Go 1.26+**, v1.0.0 (Oct 2026) | Disqualified |
| shaolei/robotgo fork | Pure-Go robotgo (syscall+purego) | Linux/mac/Win | No | Apache-2.0 | Go 1.21+, v1.0.0 (Apr 2026) | Only robotgo variant that fits; small project |
| holoplot/go-evdev | evdev read and uinput create | Linux | No | MIT | Active (2026) | Best third-party evdev library |
| bendahl/uinput | Virtual keyboard, mouse, and touchpad | Linux | No | MIT | v1.7.0 (Nov 2023) | Stable but quiet |
| creack/pty | PTY allocation | Unix | No | MIT | v1.1.24 (Oct 2024) is the latest real release | The `/v2` path is not a real upstream release\[4\]\[5\] |
| chromedp/chromedp | CDP browser automation | any | No | MIT | **Go 1.25** (v0.23.1, Nov 2026) | Pin an older tag or speak CDP yourself over WebSocket |
| modelcontextprotocol/go-sdk | Official MCP SDK | any | No | Apache-2.0/MIT | **Go 1.25**, v1.8.0 (Sep 2026) | Supports MCP 2026-07-28; roots, sampling, and logging deprecated |
| mark3labs/mcp-go | MCP SDK | any | No | MIT | go 1.25.5, v1.5.0 (Oct 2026) | Disqualified on Go version |
| knusbaum/go9p | 9P2000 server and client | Plan 9, Linux | No | MIT | v1.18.0 (2021) | Simple Srv/fs API\[6\]\[7\] |
| hugelgupf/p9 | 9P2000.L (from gVisor) | Linux/BSD/Win | No | Apache-2.0 | Updated May 2026 | Mounts directly with Linux v9fs\[8\]\[9\]\[10\] |

Rule of thumb: if a dependency's go.mod says `go 1.25` or later, your Go 1.24 build refuses it. Check with `go mod download -json <mod>@<ver>` before adding anything.

## 2. Physical Device Emulation

### 2.1 USB HID gadget: byte layouts

The kernel HID function (`usb_f_hid`) exposes four attributes: `protocol`, `subclass`, `report_length`, and `report_desc`. Reports are sent and received with plain `read`/`write` on `/dev/hidgN`.\[11\]\[12\] For a boot keyboard, protocol=1, subclass=1, and report_length=8, with the 63-byte descriptor from the kernel docs:\[13\]

```
05 01 09 06 a1 01 05 07 19 e0 29 e7 15 00 25 01 75 01 95 08 81 02 95 01 75 08 81 03
95 05 75 01 05 08 19 01 29 05 91 02 95 01 75 03 91 03 95 06 75 08 15 00 25 65 05 07
19 00 29 65 81 00 c0
```

| Report | Bytes | Layout |
|---|---|---|
| Boot keyboard IN | 8 | `[mods][reserved=0][k1..k6]`. Mod bits: 0 LCtrl, 1 LShift, 2 LAlt, 3 LGUI, 4 RCtrl, 5 RShift, 6 RAlt, 7 RGUI |
| Keyboard LED OUT (read from hidg) | 1 | bit0 NumLock, bit1 CapsLock, bit2 ScrollLock, bit3 Compose, bit4 Kana |
| Boot relative mouse | 3 (+1 wheel) | `[buttons b0..b2][dx int8][dy int8]` |
| Absolute mouse (custom) | 6 | `[buttons][X lo][X hi][Y lo][Y hi][wheel int8]`, X and Y logical range 0..32767 |
| Consumer control | 2 | 16-bit usage: 0xE9 Vol+, 0xEA Vol−, 0xE2 Mute, 0xCD Play/Pause |

Absolute mouse descriptor (protocol=0, subclass=0, report_length=6):
```
05 01 09 02 a1 01 09 01 a1 00 05 09 19 01 29 03 15 00 25 01 95 03 75 01 81 02
95 01 75 05 81 03 05 01 09 30 09 31 16 00 00 26 ff 7f 75 10 95 02 81 02
09 38 15 81 25 7f 75 08 95 01 81 06 c0 c0
```

HID usage IDs for the US layout: a–z = 0x04–0x1D; 1–9 = 0x1E–0x26, 0 = 0x27; Enter 0x28, Esc 0x29, Backspace 0x2A, Tab 0x2B, Space 0x2C; `- = [ ] \` = 0x2D–0x31; `; ' `` ` `` , . /` = 0x33–0x38; F1–F12 = 0x3A–0x45; Insert 0x49, Home 0x4A, PgUp 0x4B, Delete 0x4C, End 0x4D, PgDn 0x4E, Right/Left/Down/Up = 0x4F–0x52.

**Layouts.** HID sends *positions*, not characters. The host's configured layout decides which character a position produces, so on a German host, usage 0x1C (US "y") prints "z". Keep one rune→(usage, mods) table per host layout and record which one a script assumes. Read the LED OUT report before typing: if CapsLock is on, toggle it off first. Otherwise your text comes out inverted.

**Timing.** A 4–10 ms gap per report works for most OSes. Increase it to 20–30 ms for BIOS/UEFI setup screens and VMs behind slow KVM chains. Always send an all-zero release report between repeated characters, because the host deduplicates held keys.

### 2.2 Go: HID writer (stdlib only)

```go
// Package hid writes USB HID boot-keyboard and absolute-mouse reports.
package hid

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ModCtrl  = 0x01
	ModShift = 0x02
	ModAlt   = 0x04
	ModGUI   = 0x08
)

type stroke struct{ code, mod byte }

var us = map[rune]stroke{}

var named = map[string]byte{"enter": 0x28, "esc": 0x29, "backspace": 0x2A, "tab": 0x2B,
	"space": 0x2C, "insert": 0x49, "home": 0x4A, "pageup": 0x4B, "delete": 0x4C,
	"end": 0x4D, "pagedown": 0x4E, "right": 0x4F, "left": 0x50, "down": 0x51, "up": 0x52}

func init() {
	for r := 'a'; r <= 'z'; r++ {
		us[r] = stroke{byte(0x04 + r - 'a'), 0}
		us[r-32] = stroke{byte(0x04 + r - 'a'), ModShift}
	}
	plain, shifted := "1234567890", "!@#$%^&*()"
	for i := range plain {
		us[rune(plain[i])] = stroke{byte(0x1E + i), 0}
		us[rune(shifted[i])] = stroke{byte(0x1E + i), ModShift}
	}
	codes := []byte{0x2D, 0x2E, 0x2F, 0x30, 0x31, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38}
	plain, shifted = "-=[]\\;'`,./", "_+{}|:\"~<>?"
	for i, c := range codes {
		us[rune(plain[i])] = stroke{c, 0}
		us[rune(shifted[i])] = stroke{c, ModShift}
	}
	us['\n'], us['\t'], us[' '] = stroke{0x28, 0}, stroke{0x2B, 0}, stroke{0x2C, 0}
	for i := 1; i <= 12; i++ {
		named[fmt.Sprintf("f%d", i)] = byte(0x3A + i - 1)
	}
}

type Device struct {
	f     *os.File
	Delay time.Duration
}

func Open(path string) (*Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &Device{f: f, Delay: 8 * time.Millisecond}, nil
}

func (d *Device) write(b []byte) error {
	_, err := d.f.Write(b)
	time.Sleep(d.Delay)
	return err
}

func (d *Device) Tap(mod, code byte) error {
	if err := d.write([]byte{mod, 0, code, 0, 0, 0, 0, 0}); err != nil {
		return err
	}
	return d.write(make([]byte, 8))
}

// Type sends text using the US layout table.
func (d *Device) Type(s string) error {
	for _, r := range s {
		st, ok := us[r]
		if !ok {
			return fmt.Errorf("hid: no US mapping for %q", r)
		}
		if err := d.Tap(st.mod, st.code); err != nil {
			return err
		}
	}
	return nil
}

// Combo parses "ctrl+alt+delete", "gui+r", "f2".
func (d *Device) Combo(spec string) error {
	var mod, code byte
	for _, p := range strings.Split(strings.ToLower(spec), "+") {
		switch p {
		case "ctrl":
			mod |= ModCtrl
		case "shift":
			mod |= ModShift
		case "alt":
			mod |= ModAlt
		case "gui", "win", "super":
			mod |= ModGUI
		default:
			if c, ok := named[p]; ok {
				code = c
			} else if st, ok := us[rune(p[0])]; ok && len(p) == 1 {
				code = st.code
			} else {
				return fmt.Errorf("hid: unknown key %q", p)
			}
		}
	}
	return d.Tap(mod, code)
}

// MoveAbs maps a pixel in a w×h capture to HID 0..32767 and writes a 6-byte report.
func (d *Device) MoveAbs(x, y, w, h int, buttons byte) error {
	hx := uint16(x * 32767 / (w - 1))
	hy := uint16(y * 32767 / (h - 1))
	return d.write([]byte{buttons, byte(hx), byte(hx >> 8), byte(hy), byte(hy >> 8), 0})
}
```

To read keyboard LEDs, do a blocking 1-byte `Read` on the same `/dev/hidg0` file; the host sends an output report whenever CapsLock, NumLock, or ScrollLock changes.

### 2.3 Software virtual devices: uinput, evdev, and per-OS options

The Linux `input_event` struct is 24 bytes on 64-bit: `timeval{sec int64, usec int64}` (16 bytes), then `type u16`, `code u16`, `value s32`. On a 32-bit userland (armv6/armv7 Raspberry Pi OS) it is 16 bytes, so decode by `GOARCH`. `uinput_setup` is 92 bytes: `input_id{bustype, vendor, product, version u16}` (8), `name[80]`, then `ff_effects_max u32`. The ioctl numbers are: UI_SET_EVBIT 0x40045564, UI_SET_KEYBIT 0x40045565, UI_SET_RELBIT 0x40045566, UI_SET_ABSBIT 0x40045567, UI_DEV_SETUP 0x405c5503, UI_ABS_SETUP 0x401c5504, UI_DEV_CREATE 0x5501, UI_DEV_DESTROY 0x5502, and EVIOCGRAB 0x40044590. Linux keycodes are *not* HID usages (KEY_A=30, KEY_ENTER=28, KEY_LEFTSHIFT=42, BTN_LEFT=0x110), so the two need separate tables.

```go
//go:build linux && (amd64 || arm64)

package uinput

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

const (
	evSyn, evKey            = 0, 1
	uiSetEvbit, uiSetKeybit = 0x40045564, 0x40045565
	uiDevSetup, uiDevCreate = 0x405c5503, 0x5501
)

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

type Keyboard struct{ f *os.File }

func NewKeyboard(name string) (*Keyboard, error) {
	f, err := os.OpenFile("/dev/uinput", os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	fd := f.Fd()
	if err := ioctl(fd, uiSetEvbit, evKey); err != nil {
		return nil, err
	}
	for k := uintptr(1); k < 256; k++ {
		if err := ioctl(fd, uiSetKeybit, k); err != nil {
			return nil, err
		}
	}
	var s [92]byte
	binary.LittleEndian.PutUint16(s[0:], 0x06) // BUS_VIRTUAL
	binary.LittleEndian.PutUint16(s[2:], 0x1209)
	binary.LittleEndian.PutUint16(s[4:], 0x0001)
	copy(s[8:87], name)
	if err := ioctl(fd, uiDevSetup, uintptr(unsafe.Pointer(&s[0]))); err != nil {
		return nil, err
	}
	return &Keyboard{f}, ioctl(fd, uiDevCreate, 0)
}

func (k *Keyboard) emit(typ, code uint16, val int32) error {
	var b [24]byte // timeval zero: kernel stamps it
	binary.LittleEndian.PutUint16(b[16:], typ)
	binary.LittleEndian.PutUint16(b[18:], code)
	binary.LittleEndian.PutUint32(b[20:], uint32(val))
	_, err := k.f.Write(b[:])
	return err
}

// Tap presses and releases a Linux keycode (e.g. 30 = KEY_A).
func (k *Keyboard) Tap(code uint16) error {
	for _, v := range []int32{1, 0} {
		if err := k.emit(evKey, code, v); err != nil {
			return err
		}
		if err := k.emit(evSyn, 0, 0); err != nil {
			return err
		}
	}
	return nil
}
```

Wait about 200 ms after UI_DEV_CREATE before you send events, because udev and the compositor need time to pick up the new device. Call UI_DEV_DESTROY (0x5502) before closing. The same pattern works for a mouse: set EV_REL with REL_X/REL_Y/REL_WHEEL, or set EV_ABS and call UI_ABS_SETUP with a 28-byte `{code u16, pad, absinfo[6]s32}`.

| Target | Mechanism | Go route |
|---|---|---|
| Linux, any session incl. console/Wayland | uinput (kernel) | stdlib (above). Needs root or a udev rule granting `/dev/uinput`. ydotool works the same way. |
| X11 | XTEST extension | jezek/xgb `xtest` (pure Go), or RFB into x11vnc |
| Wayland GNOME/KDE | libei via `org.freedesktop.portal.RemoteDesktop` (user consent dialog, restore token); GNOME ≥45/46 and KDE Plasma ≥6.1 | godbus/dbus for the portal. No Go libei, so speak EIS yourself or use uinput |
| Wayland wlroots (sway, Hyprland) | `zwlr_virtual_pointer_v1`, `zwp_virtual_keyboard_v1` | Wayland wire protocol by hand, or wayvnc + RFB |
| Windows | `SendInput` (user32) | stdlib `syscall.NewLazyDLL("user32.dll").NewProc("SendInput")`. No CGO; INPUT struct is 40 bytes on amd64 |
| macOS | `CGEventPost` + Accessibility permission | cgo, or purego. Cannot be CGO-free with stdlib alone |

### 2.4 Display side

- **Capture.** USB UVC HDMI dongles (MS2109 class) show up as `/dev/videoN` and emit MJPEG. PiKVM-style CSI-2 bridges (TC358743) need an EDID loaded with `v4l2-ctl --set-edid` before they detect a signal. The Unix-y path is to run `ffmpeg -f v4l2 -input_format mjpeg -i /dev/video0 -frames:v 1 -f image2 -` as a subprocess, then `image/jpeg.Decode` the stdout.
- **Virtual displays.**\[14\] Xvfb (Anthropic's reference computer-use environment runs on Xvfb with Mutter and Tint2),\[1\] Xvnc/TigerVNC, sway or cage with `WLR_BACKENDS=headless` plus wayvnc, and vkms for KMS-level tests. Prefer Xvnc or wayvnc when you want one stdlib protocol (RFB) for both input and pixels. On a text console, `/dev/fb0` gives raw pixels. Fix the resolution with a dummy plug or EDID, because coordinate mapping depends on it.

### 2.5 Serial, Bluetooth, and PS/2 bridges

- **CH9329 serial-to-HID** (cheap, no OTG needed). Frames are `57 AB 00 CMD LEN DATA… SUM`, where SUM is the low byte of the sum of all preceding bytes. CMD 0x02 is keyboard (8-byte boot report). The default rate is 9600 baud. Verify these against your chip's datasheet. In stdlib Go, run `stty -F /dev/ttyUSB0 9600 raw -echo` as a pre-flight step, then `os.OpenFile`. RP2040/Arduino bridges should carry the same 8-byte report so the encoder is shared.
- **Bluetooth HID.** Register a BlueZ `Profile1` with an HID SDP record over D-Bus and serve L2CAP PSM 17/19, which in practice needs godbus/dbus. **PS/2** needs a microcontroller (PiKVM's Pico HID handles USB and PS/2), and Go only talks serial to it.

### 2.6 Network KVM protocols

**RFB (RFC 6143)** is the most useful protocol here. Every message is big-endian.

| Message | Layout |
|---|---|
| Version handshake | 12 bytes `"RFB 003.008\n"` each way |
| KeyEvent (C→S) | `u8 type=4, u8 down, u16 pad, u32 keysym` (8 bytes). Keysyms: ASCII = Latin-1; Return 0xff0d, BackSpace 0xff08, Tab 0xff09, Escape 0xff1b, Shift_L 0xffe1, Control_L 0xffe3, Alt_L 0xffe9, Delete 0xffff, F1 0xffbe |
| PointerEvent (C→S) | `u8 type=5, u8 button-mask, u16 x, u16 y` (6 bytes) |
| FramebufferUpdateRequest | `u8 type=3, u8 incremental, u16 x,y,w,h` (10 bytes) |
| ServerInit | `u16 w, u16 h, 16-byte pixel format, u32 name-len, name` |

```go
package rfb

import (
	"bytes"
	"crypto/des"
	"encoding/binary"
	"errors"
	"io"
	"math/bits"
	"net"
	"time"
)

type Conn struct {
	c    net.Conn
	W, H int
}

var be = binary.BigEndian

func Dial(addr, password string) (*Conn, error) {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 24)
	if _, err = io.ReadFull(c, buf[:12]); err != nil {
		return nil, err
	}
	c.Write([]byte("RFB 003.008\n"))
	io.ReadFull(c, buf[:1])
	types := make([]byte, buf[0])
	if _, err = io.ReadFull(c, types); err != nil || len(types) == 0 {
		return nil, errors.New("rfb: no security types")
	}
	want := byte(1) // None
	if password != "" {
		want = 2 // VNC auth
	}
	if !bytes.Contains(types, []byte{want}) {
		return nil, errors.New("rfb: security type not offered")
	}
	c.Write([]byte{want})
	if want == 2 {
		chal := make([]byte, 16)
		io.ReadFull(c, chal)
		key := make([]byte, 8)
		copy(key, password)
		for i := range key {
			key[i] = bits.Reverse8(key[i]) // VNC's bit-reversed DES key
		}
		blk, _ := des.NewCipher(key)
		blk.Encrypt(chal[:8], chal[:8])
		blk.Encrypt(chal[8:], chal[8:])
		c.Write(chal)
	}
	io.ReadFull(c, buf[:4])
	if be.Uint32(buf[:4]) != 0 {
		return nil, errors.New("rfb: auth failed")
	}
	c.Write([]byte{1}) // ClientInit: shared
	if _, err = io.ReadFull(c, buf[:24]); err != nil {
		return nil, err
	}
	io.CopyN(io.Discard, c, int64(be.Uint32(buf[20:24])))
	return &Conn{c: c, W: int(be.Uint16(buf[0:])), H: int(be.Uint16(buf[2:]))}, nil
}

func (r *Conn) Key(keysym uint32, down bool) error {
	b := []byte{4, 0, 0, 0, 0, 0, 0, 0}
	if down {
		b[1] = 1
	}
	be.PutUint32(b[4:], keysym)
	_, err := r.c.Write(b)
	return err
}

func (r *Conn) Pointer(mask byte, x, y int) error {
	b := []byte{5, mask, 0, 0, 0, 0}
	be.PutUint16(b[2:], uint16(x))
	be.PutUint16(b[4:], uint16(y))
	_, err := r.c.Write(b)
	return err
}
```

To take a screenshot, send SetPixelFormat (32bpp true-colour), SetEncodings listing only Raw (0), and a full-screen FramebufferUpdateRequest. Then read each FramebufferUpdate rectangle (`x,y,w,h`, `s32` encoding, `w*h*4` bytes of BGRX) into an `image.RGBA`, skipping Bell and ServerCutText messages. VNC auth is DES-based and weak, so run RFB only over SSH tunnels, WireGuard, or TLS (VeNCrypt).

- **IPMI/Redfish/BMC.** Redfish is plain HTTPS+JSON, so `net/http` is enough. Use `POST …/VirtualMedia/{slot}/Actions/VirtualMedia.InsertMedia {"Image": url}` to attach an ISO and `POST …/Systems/{id}/Actions/ComputerSystem.Reset {"ResetType":"ForceRestart"}` to reboot. **If the machine has a BMC, use it before reaching for HID injection.**
- **Intel AMT/vPro.** Offers KVM redirection (RFB-compatible once enabled) and IDE-R/USB-R virtual media.
- **SPICE and RDP.** No mature pure-Go clients exist. Bridge through `freerdp`/`remote-viewer` or terminate on a VNC server.
- **PiKVM API.** Authenticate with `X-KVMD-User`/`X-KVMD-Passwd` headers (or Basic auth). Endpoints: `POST /api/hid/print?keymap=de` with the text in the body; `/api/hid/events/send_key?key=Enter&state=true` (always pair it with `state=false` or `finish=1`,\[15\] because a bare call can leave the key auto-repeating);\[16\] `/api/hid/events/send_mouse_move?to_x=&to_y=` (absolute, 0,0 = screen centre); and WebSocket `wss://host/api/ws?stream=0` with `{"event_type":"key","event":{"key":"Enter","state":true}}`.\[15\]
- **NanoKVM.** No official API doc. Community notes describe `POST /api/hid/paste` (1024-char limit) and a WebSocket at `/api/ws`. Login uses AES with a hardcoded key, so treat the auth as weak.\[17\]\[18\]
- **JetKVM.** Control runs as JSON-RPC over a WebRTC data channel (`keyboardReport`, `absMouseReport` 0–32767, `setATXPowerAction`).\[19\] An HTTP `POST /api/rpc` is only *proposed* (PR #1719, unmerged as of late Oct 2026).\[20\]

## 3. Plan 9 as the Design Template

Plan 9 already solved "remote KVM" with files. `/dev/mouse` reads as `m` followed by x, y, buttons, and msec, and writing `m x y` warps the cursor. `/dev/cons` together with `/dev/consctl` (`rawon`) gives raw keyboard input. In 9front, `/dev/kbd` emits `k`/`K` (keys down/up) and `c` (character) lines. `/dev/screen` is readable as an image(6) file, and `/dev/draw` is the graphics protocol. rio multiplexes all of these per window under `/dev/wsys/N/`, which is a KVM switch implemented as a namespace. `drawterm` and `cpu` go the other way: the remote CPU server imports *your* terminal's `/dev` through 9P. Go supports `GOOS=plan9`, so the same agent can run on Plan 9 natively.

The same idea maps onto the Go design:

```
/n/kvm/web01/ctl      write: "type uptime\n", "key ctrl+alt+delete", "move 960 540", "click left"
/n/kvm/web01/screen   read: current frame as PNG
/n/kvm/web01/audit    read: append-only JSONL of injected actions
/n/kvm/web01/power    write: "on" | "off" | "reset" (ATX relay or Redfish)
```

You can serve this with knusbaum/go9p (9P2000, MIT) or hugelgupf/p9 (9P2000.L, Apache-2.0, mountable with `mount -t 9p -o trans=tcp,port=5640,version=9p2000.L host /n/kvm`).\[6\]\[10\] Under a strict stdlib policy, serve the same names as **FIFOs** under `/run/kvm/<target>/` locally and expose them remotely over SSH: `ssh kvm-pi 'cat > /run/kvm/web01/ctl' <<<'type uptime'`. 9P has no transport security, so bind it only to a WireGuard/Tailscale interface.

## 4. User Input Recording

| Layer | Source | Fidelity | Go route |
|---|---|---|---|
| Kernel | `/dev/input/event*` (24-byte events), `EVIOCGRAB` for exclusive capture | Keycodes and timing, no characters | stdlib |
| Compositor | libinput record (YAML), X11 XRecord/XInput2, Windows `WH_KEYBOARD_LL`, macOS CGEventTap | Per-session | xgb / syscall / cgo |
| USB | usbmon (`/sys/kernel/debug/usb/usbmon/Nu`), or a **Pi HID proxy**: real keyboard → Pi host port → grab evdev (or read `/dev/hidrawN`) → log → `/dev/hidg0` → target | Exact reports, OS-independent, works in BIOS | stdlib |
| Terminal | PTY wrapper (`script`, asciinema `--capture-input`), ttyrec | Exact bytes in both directions | stdlib |
| Browser | CDP `Input.*`/DOM events | Semantic (selectors) | chromedp (Go 1.25) or raw CDP over WebSocket |

The HID proxy is the most universal recorder. It records exactly what the target received, including in UEFI, and replays byte-for-byte through the same gadget.

**asciicast.** asciinema 3.0 (Sep 2025, Rust rewrite) writes v3 by default.\[21\]\[22\] v3 is NDJSON: the header is `{"version":3,"term":{"cols":80,"rows":24,"type":"xterm-256color"},"timestamp":…}`, and each event is `[interval, code, data]`. Intervals are deltas from the previous event (v2 used absolute times).\[23\]\[24\] Codes are `o` output, `i` input, `r` resize (`"COLSxROWS"`), `m` marker, and `x` exit status. Lines starting with `#` are comments.\[24\] v2 headers carry `width`/`height` at top level.

```go
// Package cast reads asciicast v2/v3 into absolute-time events.
package cast

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
)

type Event struct {
	T    float64
	Code string
	Data string
}

func Read(r io.Reader) (version int, evs []Event, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var t float64
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if first {
			var h struct{ Version int }
			if err = json.Unmarshal([]byte(line), &h); err != nil {
				return
			}
			version, first = h.Version, false
			continue
		}
		var raw [3]json.RawMessage
		if err = json.Unmarshal([]byte(line), &raw); err != nil {
			return
		}
		var e Event
		var ts float64
		json.Unmarshal(raw[0], &ts)
		json.Unmarshal(raw[1], &e.Code)
		json.Unmarshal(raw[2], &e.Data)
		if version >= 3 {
			t += ts
		} else {
			t = ts
		}
		e.T = t
		evs = append(evs, e)
	}
	return version, evs, sc.Err()
}
```

**PTY recorder (stdlib, Linux).** Open `/dev/ptmx` with `O_NOCTTY`, clear the lock with `ioctl(TIOCSPTLCK, &zero)`, read the index with `TIOCGPTN`, and open `/dev/pts/N`. Start the child with `SysProcAttr{Setsid: true, Setctty: true}` and the pts as stdin/stdout/stderr. Then tee the master to stdout plus a v3 writer (`o` events) and tee stdin to the master (`i` events). Put the outer terminal in raw mode with `TCGETS`/`TCSETS` on `syscall.Termios`, and propagate `SIGWINCH` with `TIOCSWINSZ` (emitting `r` events).

## 5. From Raw Events to Editable Test Scripts

**Coalescing rules (compiler `kvmc`):**
1. Track modifier state. A run of printable key taps whose inter-key gaps are under 300 ms becomes `type "…"`, with Backspace applied inside the run.
2. A non-printable key with modifiers becomes `key ctrl+alt+delete`.
3. Collapse mouse moves to their final position before a button-down. A down/up pair within 250 ms and under 4 px of travel becomes `click X Y`; a double within 400 ms becomes `dclick`; anything else becomes `drag X1 Y1 X2 Y2`.
4. Gaps over 1 s become `wait`, rounded to 100 ms. Shorter gaps are dropped and the replayer's default pacing applies.
5. Insert `expect` checkpoints at markers (`m` events, a hotkey on the proxy) and before any `wait` over 3 s, because long pauses usually mean the human was waiting for something visible.

**DSL (`kvmscript`): one action per line, shell-like quoting, diffable in Git:**
```
# kvmscript v1  layout=us  screen=1920x1080
target  web01
key     ctrl+alt+f2
expect  text "login:" timeout=20s
type    "admin\n"
wait    1s
type    "sudo systemctl restart nginx\n"
expect  text "active (running)" timeout=15s region=0,800,1920,280
shot    after-restart.png
click   960 540
```
DuckyScript (`STRING`/`ENTER`/`DELAY`) has no assertions. VHS `.tape` files and Expect scripts are terminal-only. kvmscript takes Expect's assertion model and DuckyScript's device-level reach. For determinism, assert before you act, prefer `expect` over `wait`, pin resolution, layout, and fonts, and refuse to run when the header doesn't match.

## 6. Natural Language → Actions

**Pattern.** NL → planner (LLM) → JSON actions → **validator** → policy gate (dry-run, confirm, rate limit) → backend (HID / uinput / RFB / PTY / Redfish) → observation (screenshot, OCR, command output) → loop.

```go
package agent

import (
	"fmt"
	"image"
	"strings"
	"time"
)

type Action struct {
	Op   string   `json:"op"` // type|key|click|move|wait|run|screenshot
	Text string   `json:"text,omitempty"`
	Keys string   `json:"keys,omitempty"`
	X    int      `json:"x,omitempty"`
	Y    int      `json:"y,omitempty"`
	Ms   int      `json:"ms,omitempty"`
	Argv []string `json:"argv,omitempty"`
}

type Backend interface {
	Type(string) error
	Combo(string) error
	Move(x, y int) error
	Click(button string) error
	Screenshot() (image.Image, error)
}

type Policy struct {
	W, H       int
	MaxText    int
	AllowRun   map[string]bool // argv[0] allowlist
	DenyCombos map[string]bool // e.g. "ctrl+alt+delete" unless runbook says so
	Confirm    func(Action) bool
	MinGap     time.Duration
}

func (p Policy) Validate(a Action) error {
	switch a.Op {
	case "type":
		if len(a.Text) > p.MaxText || strings.ContainsAny(a.Text, "\x1b") {
			return fmt.Errorf("type: rejected")
		}
	case "key":
		if p.DenyCombos[strings.ToLower(a.Keys)] {
			return fmt.Errorf("key %q denied", a.Keys)
		}
	case "click", "move":
		if a.X < 0 || a.Y < 0 || a.X >= p.W || a.Y >= p.H {
			return fmt.Errorf("coords out of bounds")
		}
	case "run":
		if len(a.Argv) == 0 || !p.AllowRun[a.Argv[0]] {
			return fmt.Errorf("run: %v not allowlisted", a.Argv)
		}
	case "wait", "screenshot":
	default:
		return fmt.Errorf("unknown op %q", a.Op)
	}
	return nil
}
```

The `run` op executes with `exec.Command(argv...)`, never `sh -c`, so the model can't smuggle pipes or `;`. For typed shell commands, run the text through the same allowlist on the first token, and default to **dry-run** that prints the kvmscript the model *would* run.

**Provider APIs (all callable with `net/http`):**
- **Anthropic.** `POST /v1/messages` with `tools:[{"type":"computer_toolset_20260801"}]` and no beta header. Claude returns one or more `tool_use` blocks whose `name` is a member tool (`left_click`, `type`, `screenshot`, `zoom`, …) and which carry `"toolset_name":"computer"`. Run them **in order**, stop at the first failure, and return one `tool_result` per block. Blocks after a failure get `is_error:true` with the exact text "Not executed: an earlier computer action in this turn failed."\[1\] The older `computer_20251124` tool (beta header `computer-use-2025-11-24`) added `zoom`.\[25\] The toolset rejects oversized images instead of downscaling them, so resize screenshots yourself.\[1\] Anthropic recommends a dedicated VM/container, no credentials in view, a domain allowlist, and human confirmation for consequential actions.\[1\]
- **OpenAI.** The Responses API exposes a native computer tool from GPT-5.4 (March 2026). It returns `computer_call` items with an `actions` array (`click`, `type`, `screenshot`, …).\[2\] The earlier route was `computer-use-preview` (CUA). OpenAI's March 11, 2025 announcement says it achieved "38.1% success on OSWorld for full computer use tasks, 58.1% on WebArena, and 87% on WebVoyager" at launch.
- **Local models.** Ollama and llama.cpp servers expose OpenAI-compatible `/v1/chat/completions`. Ask for a JSON array of `Action` and validate it. Small local models are fine for NL→CLI; pixel grounding needs a VLM.

**Coordinate scaling.** If you downscale a 1920×1080 capture to 1280×720 for the model, multiply the model's coordinates by 1.5 before HID mapping (`x*32767/(W-1)`). If the capture card letterboxes a non-native mode, subtract the bars first.

**MCP without an SDK.** Both Go MCP SDKs need Go ≥1.25,\[26\]\[27\] so under 1.24 implement the stdio transport yourself. It is newline-delimited JSON-RPC 2.0 on stdin/stdout: answer `initialize` (protocolVersion, `capabilities:{tools:{}}`, serverInfo), `tools/list` (each tool has `name`, `description`, `inputSchema` JSON Schema), and `tools/call` (returns `content:[{type:"text",text:…}]` or an image block). Expose `kvm_type`, `kvm_key`, `kvm_click`, `kvm_screenshot`, and `kvm_run_runbook`, and route every call through the same `Policy.Validate`. That is about 150 lines of `encoding/json`. Track MCP 2026-07-28, which deprecates roots, sampling, and logging.\[28\]

## 7. Images and Recordings → Operations

**Screenshot → element.** Use the cheapest technique that works, in this order:
1. Accessibility tree or DOM (exact, when available).
2. Template matching on `image.Image` for known icons and buttons.
3. OCR with the `tesseract` CLI as a subprocess, using `tsv` output for word boxes. This keeps the binary CGO-free; gosseract needs cgo.
4. A UI-grounding VLM. The ByteDance/Tsinghua UI-TARS paper (arXiv 2501.12326) reports that UI-TARS-7B scored 89.5 on ScreenSpot and 91.6 on ScreenSpot v2. UI-Venus-2 (Sep 2026) reports 73.0% on ScreenSpot-Pro for its 9B model. OmniParser V2 parses screens into elements for set-of-mark prompting, where you draw numbered boxes so the model answers "click 17" instead of raw pixels.

ScreenSpot-Pro (high-resolution professional apps) is the benchmark to watch, because small targets are where agents misclick.\[29\]\[30\]

```go
package vision

import "image"

func gray(m image.Image) ([]uint8, int, int) {
	b := m.Bounds()
	g := make([]uint8, b.Dx()*b.Dy())
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, gg, bb, _ := m.At(b.Min.X+x, b.Min.Y+y).RGBA()
			g[y*b.Dx()+x] = uint8((299*r + 587*gg + 114*bb) / 1000 >> 8)
		}
	}
	return g, b.Dx(), b.Dy()
}

// Find returns the centre of the best match if mean abs diff <= tol.
func Find(hay, needle image.Image, tol int) (cx, cy int, ok bool) {
	H, hw, hh := gray(hay)
	N, nw, nh := gray(needle)
	best := -1
	for y := 0; y+nh <= hh; y++ {
		for x := 0; x+nw <= hw; x++ {
			sad := 0
			for j := 0; j < nh && (best < 0 || sad < best); j++ {
				for i := 0; i < nw; i++ {
					d := int(H[(y+j)*hw+x+i]) - int(N[j*nw+i])
					if d < 0 {
						d = -d
					}
					sad += d
				}
			}
			if best < 0 || sad < best {
				best, cx, cy = sad, x+nw/2, y+nh/2
			}
		}
	}
	return cx, cy, best >= 0 && best/(nw*nh) <= tol
}
```
This is O(W·H·w·h). Restrict it with a `region` or downscale both images by 2–4× first. For video, extract keyframes with `ffmpeg -vf "select='gt(scene,0.02)'"`, diff frames, track the cursor with `Find`, and treat the result as a *draft* script for human review.

**Terminal recordings → input.** If the cast has `i` events, replay them directly; they are exact. Otherwise, strip VT sequences (CSI `ESC [ params final@-~`, OSC `ESC ] … BEL|ESC \`, and two-byte `ESC x`). Detect the prompt with a regex (`\$ $`, `# $`), take the echoed text up to `\r\n` as the typed command, and treat the rest as output, which becomes `expect` lines. Emit kvmscript, Expect, or VHS. Full-screen TUIs (vim, top) can't be reconstructed from output alone, so record input.

## 8. Use Case A — Universal KVM Fleet for an IoT Admin Server

**Architecture:**
```
 admin server (kvmd)  ──mTLS/WireGuard──►  kvm-agent (Pi CM4/Pi 4/Pi 5) ──USB OTG──► target #1
   ├─ REST/WebSocket API, 9P export          ├─ /dev/hidg0 kbd, hidg1 abs mouse, mass_storage (ISO)
   ├─ runbooks/ (kvmscript, Git)             ├─ HDMI → TC358743 CSI or UVC → frames
   ├─ systemd timers (scheduled runbooks)    ├─ ATX relay via GPIO (power/reset)
   ├─ audit log (hash-chained JSONL)         └─ local kill switch (GPIO button → UDC unbind)
   └─ backends: agent | PiKVM API | Redfish | RFB
```
Agents dial *out* to `kvmd`, which works behind NAT. `kvmd` multiplexes targets by name and serializes actions per target with one goroutine and channel each, so two runbooks can't interleave keystrokes on the same machine. Any action source, whether REST, WebSocket, 9P `ctl` write, MCP call, or timer, becomes a kvmscript line, so there is one grammar to validate and audit. Lay the repo out as `cmd/{kvmd,kvm-agent,hidd,kvmrec,kvmc,kvmplay}` plus `internal/{hid,uinput,rfb,cast,script,agent,vision,audit,backend}`. Schedule runbooks with a templated `kvm-runbook@.service` and an `OnCalendar=` timer per runbook.

**Bill of materials (per target):** Pi 4 or CM4 on a carrier with OTG, or a Pi 5 (power via GPIO 5V, USB-C for data); TC358743 HDMI-CSI board, or a USB UVC HDMI dongle; an optocoupler/relay board for ATX front-panel headers; an optional PoE HAT; and high-endurance storage.

**Audit.** Each record is `{ts, actor, target, line, prev_sha256, sha256}`, which forms a hash chain. For `type`, store the literal text unless the runbook marks it `secret`; then store a length and an HMAC instead. Ship the log off-box so a compromised agent can't rewrite history.

**Security model:**
- mTLS between `kvmd` and agents (`crypto/tls` with `ClientAuth: RequireAndVerifyClientCert`), and agents reachable only on WireGuard.
- Per-target RBAC.
- Runbooks are signed or pinned by Git commit SHA.
- A two-person rule for destructive combos (Ctrl+Alt+Del, BIOS entry).
- A per-target rate limit (e.g., 50 reports/s).
- A watchdog that sends an all-zero report and unbinds the UDC on agent crash.

Eclypsium's researchers, quoted by The Hacker News on March 18, 2026, summarized the commercial IP-KVM findings this way: "The common themes are damning: missing firmware signature validation, no brute-force protection, broken access controls, and exposed debug interfaces." Eclypsium's CVE table lists JetKVM's CVE-2026-32294 (insufficient update verification) and CVE-2026-32295 (insufficient rate limiting) as fixed in version 0.5.4. NanoKVM's CVE-2026-32296 (config endpoint exposure) is fixed in NanoKVM v2.3.1 and NanoKVM Pro 1.2.4. PiKVM was not among the affected vendors.

**Build vs buy:**

| Option | Price (2026) | Strength | Weakness |
|---|---|---|---|
| Redfish/IPMI BMC | included in servers | Virtual media + power, no extra hardware | BMC firmware CVEs; not on IoT/SBC targets |
| PiKVM V4 Mini/Plus | ~$270–$400 | Most mature, GPLv3, full HTTP API | Price per port (PiKVM Switch helps) |
| JetKVM | $103 / $119 PoE | Polished, ~60 ms latency in hands-on review | API is WebRTC JSON-RPC; HTTP API unmerged |
| NanoKVM Full/Pro | ~$55–$155 | Cheap; Pro reaches 4K@45 | Weaker security history; Full ~145 ms latency |
| DIY Pi agent (this guide) | ~$60–$120 in parts | Full control, scriptable, auditable, Go end-to-end | You own reliability and video quality |

**Recommendation:** use Redfish where it exists. For a few critical boxes, buy PiKVM units and drive them through a `backend/pikvm` adapter. Build the DIY agent for fleets of SBC/IoT targets where you need scripted, audited automation at scale.

## 9. Use Case B — Raspberry Pi as a Physical Keyboard and Mouse

**Step 1: OS and boot config.** Flash Raspberry Pi OS Lite 64-bit (32-bit armv6 for an original Pi Zero W). In `/boot/firmware/config.txt`, under `[all]`, add `dtoverlay=dwc2,dr_mode=peripheral`. Remove `otg_mode=1` and any `dtoverlay=dwc2,dr_mode=host` lines,\[31\] which some images ship for CM4/CM5.\[32\] Add `dwc2` and `libcomposite` to `/etc/modules`. Wire the Zero's "USB" port, or the Pi 4/5 USB-C port, to the target. Power the Pi 4/5 from the GPIO 5V/GND pins: the Pi 5's PCIe-attached USB controller can't do OTG, so only the SoC's native port on the USB-C connector works.\[14\] After reboot, `ls /sys/class/udc` should list a controller.\[14\]\[33\]

**Step 2: configfs gadget script.**
```bash
#!/usr/bin/env bash
# /usr/local/sbin/kvm-gadget.sh  {up|down}  MODE=bios|os
set -euo pipefail
G=/sys/kernel/config/usb_gadget/kvm
MODE="${MODE:-os}"
ISO="${ISO:-}"

preflight() {
  [[ $EUID -eq 0 ]] || { echo "must be root" >&2; exit 1; }
  modprobe libcomposite
  mountpoint -q /sys/kernel/config || mount -t configfs none /sys/kernel/config
  [[ -n "$(ls /sys/class/udc)" ]] || { echo "no UDC: check dtoverlay=dwc2" >&2; exit 1; }
}
hidfn() { # name protocol subclass len hexdesc
  mkdir -p "$G/functions/hid.$1"
  echo "$2" >"$G/functions/hid.$1/protocol"
  echo "$3" >"$G/functions/hid.$1/subclass"
  echo "$4" >"$G/functions/hid.$1/report_length"
  printf '%b' "$(sed 's/\([0-9a-f][0-9a-f]\)/\\x\1/g; s/ //g' <<<"$5")" >"$G/functions/hid.$1/report_desc"
  ln -s "$G/functions/hid.$1" "$G/configs/c.1/"
}
up() {
  preflight
  [[ -d $G ]] && down
  mkdir -p "$G/strings/0x409" "$G/configs/c.1/strings/0x409"
  echo 0x1d6b >"$G/idVendor"; echo 0x0104 >"$G/idProduct"
  echo 0x0100 >"$G/bcdDevice"; echo 0x0200 >"$G/bcdUSB"
  echo "CoolGitOrg" >"$G/strings/0x409/manufacturer"
  echo "KVM HID" >"$G/strings/0x409/product"
  awk '/Serial/{print $3}' /proc/cpuinfo >"$G/strings/0x409/serialnumber"
  echo "HID" >"$G/configs/c.1/strings/0x409/configuration"
  echo 250 >"$G/configs/c.1/MaxPower"
  # Keyboard FIRST => interface 0 => /dev/hidg0 (BIOS-friendly)
  hidfn usb0 1 1 8 "05 01 09 06 a1 01 05 07 19 e0 29 e7 15 00 25 01 75 01 95 08 81 02 95 01 75 08 81 03 95 05 75 01 05 08 19 01 29 05 91 02 95 01 75 03 91 03 95 06 75 08 15 00 25 65 05 07 19 00 29 65 81 00 c0"
  if [[ $MODE == bios ]]; then   # boot relative mouse
    hidfn usb1 2 1 3 "05 01 09 02 a1 01 09 01 a1 00 05 09 19 01 29 03 15 00 25 01 95 03 75 01 81 02 95 01 75 05 81 03 05 01 09 30 09 31 15 81 25 7f 75 08 95 02 81 06 c0 c0"
  else                           # absolute mouse, 0..32767
    hidfn usb1 0 0 6 "05 01 09 02 a1 01 09 01 a1 00 05 09 19 01 29 03 15 00 25 01 95 03 75 01 81 02 95 01 75 05 81 03 05 01 09 30 09 31 16 00 00 26 ff 7f 75 10 95 02 81 02 09 38 15 81 25 7f 75 08 95 01 81 06 c0 c0"
  fi
  if [[ -n $ISO && -f $ISO ]]; then
    mkdir -p "$G/functions/mass_storage.usb0"
    echo 1 >"$G/functions/mass_storage.usb0/lun.0/cdrom"
    echo 1 >"$G/functions/mass_storage.usb0/lun.0/ro"
    echo "$ISO" >"$G/functions/mass_storage.usb0/lun.0/file"
    ln -s "$G/functions/mass_storage.usb0" "$G/configs/c.1/"
  fi
  ls /sys/class/udc | head -n1 >"$G/UDC"
}
down() {
  [[ -d $G ]] || return 0
  echo "" >"$G/UDC" || true
  find "$G/configs/c.1" -maxdepth 1 -type l -delete
  rmdir "$G"/configs/c.1/strings/0x409 "$G"/configs/c.1 "$G"/functions/* "$G"/strings/0x409 "$G"
}
"${1:-up}"
```

**BIOS compatibility.** Keep the boot keyboard as the first interface. Use `MODE=bios` (boot relative mouse) for firmware setup screens, because many BIOSes ignore absolute pointers or composite extras. Switch to `MODE=os` for pixel-exact clicking. Switching re-enumerates the device (UDC unbind/rebind), which the target sees as an unplug. Avoid hubs between the Pi and the target during POST.

**Step 3: `hidd` daemon (stdlib).** It reads kvmscript lines from any `io.Reader`: a FIFO, stdin over SSH, a serial port, or an HTTP POST body. One parser serves all transports.
```go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"example.com/kvm/internal/hid"
)

func run(kbd, mouse *hid.Device, r io.Reader, w, h int) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		var err error
		switch f[0] {
		case "type":
			s, e := strconv.Unquote(strings.TrimSpace(strings.TrimPrefix(sc.Text(), "type")))
			if e != nil {
				return e
			}
			err = kbd.Type(s)
		case "key":
			err = kbd.Combo(f[1])
		case "move", "click":
			x, _ := strconv.Atoi(f[1])
			y, _ := strconv.Atoi(f[2])
			if err = mouse.MoveAbs(x, y, w, h, 0); err == nil && f[0] == "click" {
				if err = mouse.MoveAbs(x, y, w, h, 1); err == nil {
					err = mouse.MoveAbs(x, y, w, h, 0)
				}
			}
		default:
			err = fmt.Errorf("unknown verb %q", f[0])
		}
		log.Printf("audit %q err=%v", sc.Text(), err)
		if err != nil {
			return err
		}
	}
	return sc.Err()
}

func main() {
	listen := flag.String("http", "", "e.g. 127.0.0.1:8080 (empty = read stdin)")
	w := flag.Int("w", 1920, "target width")
	h := flag.Int("h", 1080, "target height")
	flag.Parse()
	kbd, err := hid.Open("/dev/hidg0")
	if err != nil {
		log.Fatal(err)
	}
	mouse, err := hid.Open("/dev/hidg1")
	if err != nil {
		log.Fatal(err)
	}
	if *listen == "" {
		if err := run(kbd, mouse, os.Stdin, *w, *h); err != nil {
			log.Fatal(err)
		}
		return
	}
	http.HandleFunc("POST /run", func(rw http.ResponseWriter, req *http.Request) {
		if err := run(kbd, mouse, http.MaxBytesReader(rw, req.Body, 64<<10), *w, *h); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
		}
	})
	log.Fatal(http.ListenAndServe(*listen, nil))
}
```
Usage: `printf 'key ctrl+alt+f2\ntype "root\\n"\n' | ssh kvm-pi hidd`. Bind HTTP only to localhost or WireGuard, and put mTLS in front with `kvm-agent`.

**Step 4: systemd units.**
```ini
# kvm-gadget.service
[Unit]
Description=USB HID gadget
After=sys-kernel-config.mount
[Service]
Type=oneshot
RemainAfterExit=yes
Environment=MODE=os
ExecStart=/usr/local/sbin/kvm-gadget.sh up
ExecStop=/usr/local/sbin/kvm-gadget.sh down
[Install]
WantedBy=multi-user.target

# hidd.service
[Unit]
Requires=kvm-gadget.service
After=kvm-gadget.service
[Service]
User=kvm
ExecStart=/usr/local/bin/hidd -http 127.0.0.1:8080
DevicePolicy=closed
DeviceAllow=/dev/hidg0 rw
DeviceAllow=/dev/hidg1 rw
NoNewPrivileges=yes
ProtectSystem=strict
[Install]
WantedBy=multi-user.target
```
Add the udev rule `KERNEL=="hidg*", GROUP="kvm", MODE="0660"`.

**Step 5: verify on the target.** `lsusb -d 1d6b:0104 -v` shows the interfaces and the boot protocol. `dmesg -w` should show "input: CoolGitOrg KVM HID". `sudo evtest` lets you pick the new device and see `KEY_A` events. On the Pi, `cat /dev/hidg0 | xxd` shows LED reports when you toggle CapsLock on the target.\[12\]\[13\]

## 10. Use Case C — Recording Synthetic User-Test Scripts

**Binaries:**
- `kvmrec evdev --grab /dev/input/by-id/…-kbd` prints TSV `t type code value`.
- `kvmrec proxy` runs the Pi HID pass-through and logs raw 8-byte reports.
- `kvmrec pty -- bash` writes asciicast v3 with `i` and `o` events.
- `kvmc` compiles TSV, report logs, or casts into kvmscript using the coalescing rules in §5.
- `kvmplay --backend=gadget|uinput|rfb://host:5901|pty` replays the script.

Text between stages keeps them composable: `kvmrec evdev … | kvmc --layout=us > login.kvm`.

**Assertions:** `expect text` (tesseract TSV within a region), `expect image tpl.png tol=12` (template `Find`), and `expect pty /regex/` for terminals. Failures save the frame, the diff region, and the last 50 actions.

**CI on a self-hosted runner with Xvnc (stdlib RFB for input and pixels):**
```yaml
# .github/workflows/ui-replay.yml  (CoolGitEnterprise / CoolGitOrg)
name: ui-replay
on:
  workflow_dispatch:
    inputs: { script: { required: true, default: tests/login.kvm } }
  issues:
    types: [labeled]
permissions: { contents: read, issues: write }
jobs:
  replay:
    if: github.event_name == 'workflow_dispatch' || github.event.label.name == 'run-ui-test'
    runs-on: [self-hosted, linux, x64, ui-rig]
    timeout-minutes: 20
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24.x' }
      - uses: actions/create-github-app-token@v1
        id: app
        with:
          app-id: ${{ vars.ISSUEOPS_APP_ID }}
          private-key: ${{ secrets.ISSUEOPS_AUTOADMIN_APP_KEY }}
          owner: CoolGitOrg
      - name: Start virtual display
        run: |
          set -euo pipefail
          Xvnc :1 -geometry 1920x1080 -depth 24 -SecurityTypes None -localhost &
          echo $! > xvnc.pid; sleep 2
          DISPLAY=:1 ./scripts/start-app-under-test.sh &
      - name: Replay
        run: |
          set -euo pipefail
          make build-host
          ./bin/kvmplay --backend=rfb://127.0.0.1:5901 \
            --artifacts out/ "${{ inputs.script || 'tests/login.kvm' }}"
      - name: Report
        if: always()
        run: |
          set -euo pipefail
          ./bin/kvmreport --in out/ --html out/report.html
          (cd out && sha256sum * > SHA256SUMS)
          kill "$(cat xvnc.pid)" || true
      - uses: actions/upload-artifact@v4
        if: always()
        with: { name: ui-replay, path: out/ }
      - name: Comment on issue (issueops-autoadmin-app)
        if: always() && github.event_name == 'issues'
        env: { GH_TOKEN: '${{ steps.app.outputs.token }}' }
        run: gh issue comment ${{ github.event.issue.number }} --body "Replay finished; see run ${{ github.run_id }}"
```
For a physical rig, the runner is the admin host. It drives a Pi gadget wired to a test laptop, and the `--backend=gadget` replays over SSH, with HDMI capture for assertions. **go-echarts** fits for `kvmreport`: plot the per-action latency (action sent → expected text visible) and the inter-key timing histograms from the recording. Check the go-echarts go.mod `go` line against your 1.24 ceiling before adding it.

**Makefile (unified interface):**
```make
SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
VERSION ?= $(shell git describe --tags --always --dirty)
LDFLAGS := -s -w -X main.version=$(VERSION)
CMDS := kvmd kvm-agent hidd kvmrec kvmc kvmplay kvmreport
PLATFORMS := linux/amd64 linux/arm64 linux/arm/6 linux/arm/7
export CGO_ENABLED := 0

.PHONY: all preflight test build-host dist checksums
all: preflight test dist checksums
preflight:
	@go version | grep -Eq 'go1\.(2[4-9])' || { echo "need Go >=1.24 toolchain"; exit 1; }
	@grep -q '^go 1.24' go.mod || { echo "go.mod must pin go 1.24"; exit 1; }
test:
	go vet ./... && go test -race ./...
build-host:
	mkdir -p bin && for c in $(CMDS); do go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$$c ./cmd/$$c; done
dist:
	for p in $(PLATFORMS); do IFS=/ read -r os arch arm <<<"$$p"; \
	  for c in $(CMDS); do out=dist/$$c-$$os-$$arch$${arm:+v$$arm}; \
	    GOOS=$$os GOARCH=$$arch GOARM=$$arm go build -trimpath -ldflags '$(LDFLAGS)' -o $$out ./cmd/$$c; done; done
checksums:
	cd dist && sha256sum * > SHA256SUMS
```
A release workflow on tag push runs `make all`, then `gh release create "$GITHUB_REF_NAME" dist/*` with the app token. Agents verify `SHA256SUMS` before self-updating.

## Recommendations
1. **Keep `go 1.24` as the language ceiling, but compile with a supported toolchain.** Go's release policy says "Each major Go release is supported until there are two newer major releases." Go 1.27 shipped on August 19, 2026, so only 1.26 and 1.27 are supported. Go 1.24 lost support when Go 1.26 shipped on February 10, 2026 (per the eol.frees.jp tracker), and your control plane terminates TLS. Go 1.27's `go test` runs the `stdversion` vet check by default, which flags std symbols newer than your go.mod.\[34\]
2. **Use stdlib for the hardware and protocol layer** (HID, uinput, evdev, RFB, PTY, asciicast, Redfish, LLM HTTP). Make exceptions only where the OS requires them: godbus for Wayland portals and BlueZ, cgo/purego for macOS.
3. **Make RFB your test bus.** Xvnc, wayvnc, x11vnc, Intel AMT, and most hypervisors speak it, so a 150-line client gives you input plus screenshots everywhere without uinput privileges.
4. **Prefer the BMC, then PiKVM's API, then the DIY gadget.** Wrap all three behind one `Backend` interface so runbooks don't care which is used.
5. **Treat every action source as untrusted text.** Route recordings, LLMs, MCP, and humans alike through parse → validate → rate-limit → audit → execute. Default to dry-run for LLM plans and require confirmation for power, BIOS, and destructive combos.

## Caveats
- **Unverified items.** Library status for jezek/xgb, godbus, go-ole, gocv/gosseract (both need CGO), kbinani/screenshot, go.bug.st/serial, periph.io, vhs, goexpect, go-rod, and go-echarts was *not* re-verified. The mcp-go `go 1.25.5` and go-sdk "Go 1.25" requirements come from secondary sources, not the raw go.mod files.
- **Vendor-controlled facts.** Price and latency figures come from 2026 hands-on reviews (WarpKVM, ComputingForGeeks), not vendor specs. The NanoKVM and JetKVM API details come from community projects and unmerged PRs.
- **Benchmarks.** Computer-use and grounding numbers are self-reported by model authors. Keep humans in the loop for consequential actions.
- **Authorization.** Keystroke injection is the BadUSB attack class. Use it only on systems you own or are authorized to administer. USB device-control policies (VID/PID allowlists, Windows Device Installation Restrictions, USBGuard) may block your gadget, so use a distinct, documented VID:PID and serial that security tooling can allowlist explicitly.

## Sources

1. [Computer use tool](https://platform.claude.com/docs/en/agents-and-tools/tool-use/computer-use-tool)
2. [GPT-5.4 Native Computer Use. The first general-purpose model that…](https://cobusgreyling.medium.com/gpt-5-4-native-computer-use-c8ad242d60a2)
3. [OpenAI Computer Use API Developer Guide 2026: Build Browser Automation Agents](https://baeseokjae.github.io/posts/openai-computer-use-api-guide-2026/)
4. [GitHub - creack/pty: PTY interface for Go](https://github.com/creack/pty?tab=readme-ov-file)
5. [pty package - github.com/creack/pty - Go Packages](https://pkg.go.dev/github.com/creack/pty)
6. [go9p package - github.com/knusbaum/go9p - Go Packages](https://pkg.go.dev/github.com/knusbaum/go9p)
7. [GitHub - knusbaum/go9p: 9P2000 implementation in Go · GitHub](https://github.com/knusbaum/go9p)
8. [v9fs · GitHub](https://github.com/v9fs/)
9. [p9 package - github.com/hugelgupf/p9/p9 - Go Packages](https://pkg.go.dev/github.com/hugelgupf/p9/p9)
10. [github.com](https://github.com/hugelgupf/p9)
11. [https://www.kernel.org/doc/Documentation/usb//gadg...](https://www.kernel.org/doc/Documentation/usb//gadget_hid.rst)
12. [Gadget Testing — The Linux Kernel documentation](https://docs.kernel.org/usb/gadget-testing.html)
13. [gadget-testing.txt](https://www.kernel.org/doc/Documentation/usb/gadget-testing.txt)
14. [Raspberry Pi](https://pip-assets.raspberrypi.com/categories/685-app-notes-guides-whitepapers/documents/RP-009276-WP/Using-OTG-mode-on-Raspberry-Pi-SBCs)
15. [HTTP API reference - PiKVM Handbook](https://docs.pikvm.org/api/)
16. [HID: send\_key without 'state' leaves the key held on the host (auto-repeats until released) · Issue #1711 · pikvm/pikvm · GitHub](https://github.com/pikvm/pikvm/issues/1711)
17. [API Documentation · Issue #90 · sipeed/NanoKVM · GitHub](https://github.com/sipeed/NanoKVM/issues/90)
18. [nanokvm-mcp/API\_REFERENCE.md at main · scgreenhalgh/nanokvm-mcp · GitHub](https://github.com/scgreenhalgh/nanokvm-mcp/blob/main/API_REFERENCE.md)
19. [GitHub - onyxhat/jetkvm-mcp: Give an AI eyes and hands on a physical computer — MCP server that lets Claude see and drive any machine through a JetKVM (screenshots, keyboard/mouse, boot media, power) on stock firmware · GitHub](https://github.com/onyxhat/jetkvm-mcp)
20. [feat: serve the JSON-RPC methods over HTTP by markfrancisonly · Pull Request #1719 · jetkvm/kvm · GitHub](https://github.com/jetkvm/kvm/pull/1719)
21. [Asciinema: making movies at the command-line \[LWN.net\]](https://lwn.net/Articles/1053355/)
22. [asciinema/CHANGELOG.md at develop · asciinema/asciinema · GitHub](https://github.com/asciinema/asciinema/blob/develop/CHANGELOG.md)
23. [asciicast v3 - asciinema docs](https://docs.asciinema.org/manual/asciicast/v3/)
24. [3.0 · asciinema blog](https://blog.asciinema.org/post/three-point-o/)
25. [Computer-Using Agent: How Computer Use Works](https://memx.app/glossary/computer-use-agent/)
26. [feat!: generate MCP servers on the official Go MCP SDK · Issue #89 · stablekernel/protoc-gen-go-mcp · GitHub](https://github.com/stablekernel/protoc-gen-go-mcp/issues/89)
27. [fix(deps): update module github.com/mark3labs/mcp-go to v1.5.0 by renovate\[bot\] · Pull Request #3681 · giantswarm/devctl · GitHub](https://github.com/giantswarm/devctl/pull/3681)
28. [GitHub - modelcontextprotocol/go-sdk: The official Go SDK for Model Context Protocol servers and clients. Maintained in collaboration with Google. · GitHub](https://github.com/modelcontextprotocol/go-sdk)
29. [UI-Venus-2 Technical Report](https://arxiv.org/pdf/2609.00028)
30. [UI-R1: Enhancing Efficient Action Prediction of GUI Agents by Reinforcement Learning](https://arxiv.org/pdf/2503.21620)
31. [Raspberry Pi 5 USB-Gadget for hamradio and programming - DL1GKK.com](https://dl1gkk.com/raspberry-pi-5-ipad-usb-c-gadget-ham-radio/)
32. [RPi 5 USB gadget mode possible? - Raspberry Pi Forums](https://forums.raspberrypi.com/viewtopic.php?t=358573)
33. [AutoRPi](https://autorpi.net/threads/4/)
34. [Go 1.27 Release Notes - The Go Programming Language](https://go.dev/doc/go1.27)
