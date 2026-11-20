package hid

import "testing"

func TestTypeUptime(t *testing.T) {
	reps, err := Type("Up\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []KeyReport{
		{LShift, 0, 0x18}, Release, // U
		{0, 0, 0x13}, Release, // p
		{0, 0, 0x28}, Release, // Enter
	}
	if len(reps) != len(want) {
		t.Fatalf("got %d reports, want %d", len(reps), len(want))
	}
	for i := range want {
		if reps[i] != want[i] {
			t.Errorf("report %d = % x, want % x", i, reps[i], want[i])
		}
	}
}

func TestTypeEveryPrintableASCII(t *testing.T) {
	for r := rune(0x20); r < 0x7f; r++ {
		if _, err := Type(string(r)); err != nil {
			t.Errorf("printable ASCII %q is not typeable: %v", r, err)
		}
	}
}

func TestTypeRejectsUnmapped(t *testing.T) {
	if _, err := Type("naïve"); err == nil {
		t.Fatal("expected an error for a rune outside the US layout")
	}
}

func TestParseChord(t *testing.T) {
	cases := map[string]KeyReport{
		"ctrl+alt+t":      {LCtrl | LAlt, 0, 0x17},
		"Ctrl+Alt+Delete": {LCtrl | LAlt, 0, 0x4c},
		"enter":           {0, 0, 0x28},
		"shift+tab":       {LShift, 0, 0x2b},
		"win":             {LGUI},
		"f12":             {0, 0, 0x45},
		"ctrl+c":          {LCtrl, 0, 0x06},
	}
	for in, want := range cases {
		got, err := ParseChord(in)
		if err != nil {
			t.Errorf("%q: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("%q = % x, want % x", in, got, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "ctrl+nosuchkey"} {
		if _, err := ParseChord(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestAbsReport(t *testing.T) {
	if got, want := AbsReport(1, 1000, 0), [5]byte{1, 0xff, 0x7f, 0, 0}; got != want {
		t.Errorf("corner = % x, want % x", got, want)
	}
	if got, want := AbsReport(0, 500, 500), [5]byte{0, 0xff, 0x3f, 0xff, 0x3f}; got != want {
		t.Errorf("centre = % x, want % x", got, want)
	}
	if got := AbsReport(0, 5000, -3); got != [5]byte{0, 0xff, 0x7f, 0, 0} {
		t.Errorf("out-of-range input was not clamped: % x", got)
	}
}
