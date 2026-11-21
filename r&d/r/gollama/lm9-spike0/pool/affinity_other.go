//go:build !linux

package pool

// Pinning is a Linux feature in this spike. Windows needs
// SetThreadAffinityMask; macOS offers only hints; Plan 9 has none.
func allowedCPUs() []int { return nil }
func pinTo(int) error    { return nil }
