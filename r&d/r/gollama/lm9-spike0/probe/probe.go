// Package probe reports what hardware the engine is running on, using only
// files the kernel exposes. No vendor library is needed to answer "which GPUs
// are here, how are they connected, and what kind of disk holds the models".
package probe

// GPU is a display-class PCI device.
type GPU struct {
	Address   string `json:"address"`
	Vendor    string `json:"vendor"`
	VendorID  string `json:"vendor_id"`
	DeviceID  string `json:"device_id"`
	LinkSpeed string `json:"link_speed,omitempty"`     // e.g. "8.0 GT/s PCIe"
	LinkWidth string `json:"link_width,omitempty"`     // lanes, e.g. "16"
	MaxSpeed  string `json:"max_link_speed,omitempty"` // what the device could do
	MaxWidth  string `json:"max_link_width,omitempty"`
	Degraded  bool   `json:"link_degraded"` // running below its maximum
	VRAMBytes uint64 `json:"vram_bytes,omitempty"`
}

// Disk is a block device and the storage class the loader should assume.
type Disk struct {
	Name  string `json:"name"`
	Class string `json:"class"` // nvme, ssd, hdd, flash
}

// Report is the result of a probe.
type Report struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	CPUs   int    `json:"cpus"`
	Kernel string `json:"kernel_tier"`
	GPUs   []GPU  `json:"gpus"`
	Disks  []Disk `json:"disks"`
}

// VendorName maps a PCI vendor id to the names the --gpu-vendor flag accepts.
func VendorName(id string) string {
	switch id {
	case "0x10de":
		return "nvidia"
	case "0x1002":
		return "amd"
	case "0x8086":
		return "intel"
	case "0x106b":
		return "apple"
	}
	return "other"
}

// DiskClass applies the storage rules from the plan to what sysfs says.
func DiskClass(name string, rotational, removable bool) string {
	switch {
	case len(name) >= 4 && name[:4] == "nvme":
		return "nvme"
	case len(name) >= 6 && name[:6] == "mmcblk", removable:
		return "flash"
	case rotational:
		return "hdd"
	}
	return "ssd"
}
