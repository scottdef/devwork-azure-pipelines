package probe

import (
	"encoding/json"
	"testing"
)

func TestDiskClass(t *testing.T) {
	for _, c := range []struct {
		name               string
		rotational, remove bool
		want               string
	}{
		{"nvme0n1", false, false, "nvme"},
		{"sda", false, false, "ssd"},
		{"sda", true, false, "hdd"},
		{"mmcblk0", false, false, "flash"},
		{"sdb", false, true, "flash"},
		{"vda", false, false, "ssd"},
	} {
		if got := DiskClass(c.name, c.rotational, c.remove); got != c.want {
			t.Errorf("DiskClass(%q, rot=%v, rem=%v) = %q, want %q", c.name, c.rotational, c.remove, got, c.want)
		}
	}
}

func TestVendorName(t *testing.T) {
	if VendorName("0x10de") != "nvidia" || VendorName("0x1002") != "amd" || VendorName("0xffff") != "other" {
		t.Fatal("vendor table is wrong")
	}
}

func TestRunProducesJSON(t *testing.T) {
	b, err := json.MarshalIndent(Run(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", b)
}
