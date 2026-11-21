//go:build !(linux || darwin || freebsd)

package kernels

// Arena on platforms without an mmap path in this spike (Plan 9, Windows,
// wasm) falls back to the Go heap. float32 slices hold no pointers, so the
// collector does not scan them; what is lost is control over paging.
type Arena struct{ buf []float32 }

// NewArena allocates size bytes on the Go heap.
func NewArena(size int) (*Arena, error) {
	return &Arena{buf: make([]float32, size/4)}, nil
}

// Float32s returns n float32s starting at byte offset off.
func (a *Arena) Float32s(off, n int) []float32 {
	if off < 0 || n < 0 || off%4 != 0 || off/4+n > len(a.buf) {
		panic("kernels: arena range out of bounds")
	}
	return a.buf[off/4 : off/4+n : off/4+n]
}

// Close releases the arena.
func (a *Arena) Close() error { a.buf = nil; return nil }
