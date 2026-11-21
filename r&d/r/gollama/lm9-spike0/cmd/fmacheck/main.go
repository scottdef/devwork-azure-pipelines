// Command fmacheck shows that the same Go source can give different
// floating-point results on different architectures.
//
// The Go spec lets a compiler fuse x*y+z into a single multiply-add, which
// skips the rounding of x*y, and says an explicit conversion forbids it. The
// gc compiler does fuse on arm64 and does not on amd64, so plain() below
// returns 0 on an x86 machine and about 8.7e-19 on an ARM one, while pinned()
// returns 0 everywhere. A kernel that must be reproducible across machines
// has to be written like pinned().
//
// Check what your compiler does without leaving your desk:
//
//	GOARCH=amd64 go build -gcflags=-S ./cmd/fmacheck 2>&1 | grep -E 'plain|pinned|MULSD|VFMADD'
//	GOARCH=arm64 go build -gcflags=-S ./cmd/fmacheck 2>&1 | grep -E 'plain|pinned|FMADDD|FMULD'
package main

import (
	"fmt"
	"runtime"
	"strconv"
)

//go:noinline
func plain(x, y, z float64) float64 { return x*y + z }

//go:noinline
func pinned(x, y, z float64) float64 { return float64(x*y) + z }

func main() {
	// Parsed at run time so the compiler cannot fold the arithmetic.
	x, _ := strconv.ParseFloat("1.000000000931322574615478515625", 64) // 1 + 2^-30
	z, _ := strconv.ParseFloat("-1.00000000186264514923095703125", 64) // -(1 + 2^-29)
	a, b := plain(x, x, z), pinned(x, x, z)
	fmt.Printf("%s: x*y+z = %g, float64(x*y)+z = %g\n", runtime.GOARCH, a, b)
	if a != b {
		fmt.Println("fused: the two expressions disagree in this build")
		return
	}
	fmt.Println("not fused: the two expressions agree in this build")
}
