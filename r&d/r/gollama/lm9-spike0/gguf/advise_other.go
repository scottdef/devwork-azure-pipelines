//go:build darwin || freebsd

package gguf

// Advise is a no-op here. The frozen syscall package has Madvise only on
// Linux; golang.org/x/sys/unix has it everywhere, which is the practical
// argument for taking that one dependency in the real engine.
func (m *Mapped) Advise(sequential bool) error { return nil }
