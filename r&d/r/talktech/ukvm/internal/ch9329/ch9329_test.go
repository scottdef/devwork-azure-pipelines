package ch9329

import (
	"bytes"
	"testing"

	"ukvm/internal/hid"
)

// Vectors below are the worked examples from the WCH protocol document and
// from a published absolute-mouse walkthrough (1920x1080 screen).

func TestKeyboardPressAndRelease(t *testing.T) {
	press := Keyboard(hid.KeyReport{0, 0, 0x04}) // 'A' key down
	want := []byte{0x57, 0xAB, 0x00, 0x02, 0x08, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10}
	if !bytes.Equal(press, want) {
		t.Errorf("press = % X\n want  % X", press, want)
	}
	release := Keyboard(hid.Release)
	want = []byte{0x57, 0xAB, 0x00, 0x02, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0C}
	if !bytes.Equal(release, want) {
		t.Errorf("release = % X\n want    % X", release, want)
	}
}

func TestMouseAbsRaw(t *testing.T) {
	// (460,480) on 1920x1080 -> x = 460*4096/1920 = 981, y = 480*4096/1080 = 1820.
	got := MouseAbsRaw(0, 981, 1820, 0)
	want := []byte{0x57, 0xAB, 0x00, 0x04, 0x07, 0x02, 0x00, 0xD5, 0x03, 0x1C, 0x07, 0x00, 0x0A}
	if !bytes.Equal(got, want) {
		t.Errorf("abs = % X\nwant  % X", got, want)
	}
	// (800,800) on 1920x1080 -> 1706, 3034.
	got = MouseAbsRaw(0, 1706, 3034, 0)
	want = []byte{0x57, 0xAB, 0x00, 0x04, 0x07, 0x02, 0x00, 0xAA, 0x06, 0xDA, 0x0B, 0x00, 0xA4}
	if !bytes.Equal(got, want) {
		t.Errorf("abs = % X\nwant  % X", got, want)
	}
}

func TestMouseAbsScale(t *testing.T) {
	f := MouseAbs(1, 1000, 500)
	_, data, err := ParseReply(f) // same framing in both directions
	if err != nil {
		t.Fatal(err)
	}
	x := int(data[2]) | int(data[3])<<8
	y := int(data[4]) | int(data[5])<<8
	if x != 4095 || y != 2047 || data[1] != 1 {
		t.Errorf("x=%d y=%d buttons=%d, want 4095 2047 1", x, y, data[1])
	}
}

func TestGetInfoFrame(t *testing.T) {
	want := []byte{0x57, 0xAB, 0x00, 0x01, 0x00, 0x03}
	if got := GetInfo(); !bytes.Equal(got, want) {
		t.Errorf("getinfo = % X, want % X", got, want)
	}
}

func TestParseReply(t *testing.T) {
	reply := Frame(CmdGetInfo|0x80, []byte{0x30, 0x01, 0x02, 0, 0, 0, 0, 0})
	cmd, data, err := ParseReply(reply)
	if err != nil || cmd != 0x81 {
		t.Fatalf("cmd=%#x err=%v", cmd, err)
	}
	info, err := ParseInfo(data)
	if err != nil || !info.Connected || !info.CapsLock || info.NumLock {
		t.Errorf("info = %+v err=%v", info, err)
	}
	reply[len(reply)-1]++
	if _, _, err := ParseReply(reply); err == nil {
		t.Error("corrupt checksum was accepted")
	}
	if _, _, err := ParseReply([]byte{1, 2, 3}); err == nil {
		t.Error("short garbage was accepted")
	}
}
