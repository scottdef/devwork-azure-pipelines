//go:build linux

package pool

import (
	"syscall"
	"unsafe"
)

// cpuSet is the kernel's bitmask of CPUs: 1,024 bits here.
type cpuSet [16]uint64

// allowedCPUs lists the CPUs this process may run on. Inside a container the
// set can be sparse, so it is read, never assumed to be 0..n-1.
func allowedCPUs() []int {
	var set cpuSet
	_, _, e := syscall.RawSyscall(syscall.SYS_SCHED_GETAFFINITY, 0,
		unsafe.Sizeof(set), uintptr(unsafe.Pointer(&set)))
	if e != 0 {
		return nil
	}
	var cpus []int
	for i := 0; i < len(set)*64; i++ {
		if set[i/64]&(1<<(uint(i)%64)) != 0 {
			cpus = append(cpus, i)
		}
	}
	return cpus
}

// pinTo binds the calling OS thread to one CPU. Thread id 0 means "this
// thread", which is why the caller must have locked its goroutine first.
func pinTo(cpu int) error {
	var set cpuSet
	set[cpu/64] |= 1 << (uint(cpu) % 64)
	_, _, e := syscall.RawSyscall(syscall.SYS_SCHED_SETAFFINITY, 0,
		unsafe.Sizeof(set), uintptr(unsafe.Pointer(&set)))
	if e != 0 {
		return e
	}
	return nil
}
