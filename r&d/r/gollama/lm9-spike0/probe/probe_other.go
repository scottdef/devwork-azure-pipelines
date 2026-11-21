//go:build !linux

package probe

import (
	"runtime"

	"lm9spike/kernels"
)

// Run reports what is knowable without an OS-specific probe. macOS needs
// IOKit and Metal, Windows needs DXGI, Plan 9 reads /dev/sysstat and #P.
func Run() Report {
	return Report{OS: runtime.GOOS, Arch: runtime.GOARCH, CPUs: runtime.NumCPU(), Kernel: kernels.Tier()}
}
