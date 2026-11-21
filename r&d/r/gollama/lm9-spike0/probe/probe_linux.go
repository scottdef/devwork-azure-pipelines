//go:build linux

package probe

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"lm9spike/kernels"
)

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Run gathers the report. Missing files are normal (virtual machines hide
// most of this) and simply leave fields empty.
func Run() Report {
	r := Report{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Kernel: kernels.Tier()}

	devs, _ := filepath.Glob("/sys/bus/pci/devices/*")
	for _, d := range devs {
		// PCI class 0x03xxxx is a display controller: VGA, 3D, or other.
		if class := read(filepath.Join(d, "class")); !strings.HasPrefix(class, "0x03") {
			continue
		}
		g := GPU{
			Address:   filepath.Base(d),
			VendorID:  read(filepath.Join(d, "vendor")),
			DeviceID:  read(filepath.Join(d, "device")),
			LinkSpeed: read(filepath.Join(d, "current_link_speed")),
			LinkWidth: read(filepath.Join(d, "current_link_width")),
			MaxSpeed:  read(filepath.Join(d, "max_link_speed")),
			MaxWidth:  read(filepath.Join(d, "max_link_width")),
		}
		g.Vendor = VendorName(g.VendorID)
		g.Degraded = g.LinkSpeed != g.MaxSpeed || g.LinkWidth != g.MaxWidth
		// amdgpu publishes its memory size here; other drivers need their own API.
		g.VRAMBytes, _ = strconv.ParseUint(read(filepath.Join(d, "mem_info_vram_total")), 10, 64)
		r.GPUs = append(r.GPUs, g)
	}

	blocks, _ := filepath.Glob("/sys/block/*")
	for _, b := range blocks {
		name := filepath.Base(b)
		if strings.HasPrefix(name, "loop") || strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram") {
			continue
		}
		r.Disks = append(r.Disks, Disk{Name: name, Class: DiskClass(name,
			read(filepath.Join(b, "queue/rotational")) == "1",
			read(filepath.Join(b, "removable")) == "1")})
	}
	return r
}
