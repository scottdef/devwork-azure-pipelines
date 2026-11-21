//go:build linux

package gguf

import "syscall"

// Advise tells the kernel how the mapping will be read. Sequential suits a
// first pass that warms the page cache; Random suits steady-state inference
// on a model larger than memory, where readahead would evict useful pages.
func (m *Mapped) Advise(sequential bool) error {
	advice := syscall.MADV_RANDOM
	if sequential {
		advice = syscall.MADV_SEQUENTIAL
	}
	return syscall.Madvise(m.Bytes, advice)
}
