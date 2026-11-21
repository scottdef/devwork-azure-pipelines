//go:build linux || darwin || freebsd

package gguf

import (
	"os"
	"syscall"
)

// Mapped is a GGUF file mapped read-only into memory. Tensor data is used in
// place: the operating system pages weights in from disk on first touch and
// can drop them again under memory pressure without writing anything out.
type Mapped struct {
	*File
	Bytes []byte
}

// Open maps path and parses its directory. The file descriptor is closed
// before returning; a mapping outlives the descriptor it came from.
func Open(path string) (*Mapped, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	st, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() == 0 || st.Size() != int64(int(st.Size())) {
		return nil, errShort
	}
	b, err := syscall.Mmap(int(fd.Fd()), 0, int(st.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		return nil, err
	}
	f, err := Parse(b)
	if err != nil {
		syscall.Munmap(b)
		return nil, err
	}
	return &Mapped{File: f, Bytes: b}, nil
}

// Close unmaps the file. Tensor slices taken from it must not be used again.
func (m *Mapped) Close() error {
	b := m.Bytes
	m.Bytes = nil
	return syscall.Munmap(b)
}
