// Package foundrytui holds the files that ship inside the binary:
// the Go templates that shape every write, and the workflows that
// carry those writes out. One binary, no install tree.
package foundrytui

import "embed"

// Templates holds dispatch payloads, the HTML report and the probe prompt.
//
//go:embed templates
var Templates embed.FS

// Workflows holds the GitHub Actions workflows this tool dispatches.
// "foundry-tui scaffold" copies them into a repository.
//
//go:embed .github/workflows/*.yml
var Workflows embed.FS
