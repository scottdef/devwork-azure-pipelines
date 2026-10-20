package funcs

import "embed"

// Source holds this package's own implementation files. forge.Export
// copies them into a bundle (renamed to package main) so an exported
// run.go builds anywhere with nothing but the Go toolchain.
//
//go:embed funcs.go yaml.go
var Source embed.FS
