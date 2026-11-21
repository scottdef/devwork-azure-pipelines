//go:build linux

package kernels

import (
	"syscall"
	"testing"
	"unsafe"
)

// Assembly has no bounds checks, so this test supplies them. Each vector is
// placed so that its last element ends exactly where an unreadable page
// begins. A kernel that reads even one byte too far takes a SIGSEGV here
// instead of silently reading a neighbour's memory in production.
func guarded(t *testing.T, n int) []float32 {
	t.Helper()
	page := syscall.Getpagesize()
	need := 4 * n
	pages := (need + page - 1) / page
	if pages == 0 {
		pages = 1
	}
	m, err := syscall.Mmap(-1, 0, (pages+1)*page, syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Munmap(m) })
	if err := syscall.Mprotect(m[pages*page:], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		return nil
	}
	start := pages*page - need
	return unsafe.Slice((*float32)(unsafe.Pointer(&m[start])), n)
}

// guardedBytes is guarded for a byte slice.
func guardedBytes(t *testing.T, n int) []byte {
	t.Helper()
	page := syscall.Getpagesize()
	pages := (n + page - 1) / page
	m, err := syscall.Mmap(-1, 0, (pages+1)*page, syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Munmap(m) })
	if err := syscall.Mprotect(m[pages*page:], syscall.PROT_NONE); err != nil {
		t.Fatal(err)
	}
	return m[pages*page-n : pages*page : pages*page]
}

func TestQuantizedKernelNeverReadsPastTheBlock(t *testing.T) {
	for n := 1; n <= 9; n++ {
		src, xs := q8Fixture(n, 40)
		blocks, x := guardedBytes(t, len(src)), guarded(t, len(xs))
		copy(blocks, src)
		copy(x, xs)
		_ = DotQ8F32(blocks, x)
	}
}

func TestKernelsNeverReadPastTheSlice(t *testing.T) {
	for n := 1; n <= 131; n++ {
		a, b := guarded(t, n), guarded(t, n)
		copy(a, vec(n, 9))
		copy(b, vec(n, 11))
		_ = Dot(a, b)
		_ = DotDet(a, b)
		for _, tier := range guardTiers() {
			_ = tier(a, b)
		}
	}
}
