//go:build linux || darwin || freebsd

package kernels

import (
	"syscall"
	"unsafe"
)

// Arena is tensor memory the Go heap does not own: an anonymous mapping, page
// aligned, never moved, and never counted by the garbage collector's pacer.
// The real engine adds huge pages, mlock and NUMA placement at this point.
type Arena struct{ buf []byte }

// NewArena maps size bytes of zeroed memory outside the Go heap.
func NewArena(size int) (*Arena, error) {
	b, err := syscall.Mmap(-1, 0, size, syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	return &Arena{buf: b}, nil
}

// Float32s returns n float32s starting at byte offset off.
func (a *Arena) Float32s(off, n int) []float32 {
	if off < 0 || n < 0 || off%4 != 0 || off+4*n > len(a.buf) {
		panic("kernels: arena range out of bounds")
	}
	if n == 0 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&a.buf[off])), n)
}

// Close unmaps the arena. Slices handed out earlier must not be used again.
func (a *Arena) Close() error {
	b := a.buf
	a.buf = nil
	return syscall.Munmap(b)
}
