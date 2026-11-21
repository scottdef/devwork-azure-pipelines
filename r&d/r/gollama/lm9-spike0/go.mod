module lm9spike

// The plan targets `go 1.26`. This spike says 1.24 only because the sandbox it
// was tested in had Go 1.24.7 and no way to fetch a newer toolchain. Nothing
// here needs more than 1.24 except kernels/dot_simd_amd64.go, which is behind
// the goexperiment.simd build tag and needs Go 1.26+.
go 1.24
