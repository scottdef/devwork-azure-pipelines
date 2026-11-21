//go:build linux && (!amd64 || noasm)

package kernels

func guardTiers() []func(a, b []float32) float32 { return nil }
