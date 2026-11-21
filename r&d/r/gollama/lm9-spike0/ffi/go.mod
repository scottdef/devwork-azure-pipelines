// A separate module, so the rest of the spike keeps zero dependencies.
// Run `go mod tidy` once to fetch purego and write go.sum.
//
// NOT BUILT OR RUN in the session that produced this spike: the sandbox could
// not reach the Go module proxy. Treat main.go as a sketch until it has run.
module lm9spike/ffi

go 1.24

require github.com/ebitengine/purego v0.10.0
