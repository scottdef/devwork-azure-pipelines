// Package catalog embeds tmplforge's starter templates: one *.tmpl per
// entry, a fixture per entry under fixtures/, and index.json describing
// them. internal/forge reads it through fs.FS; nothing else lives here.
package catalog

import "embed"

// FS is the catalog file tree rooted at this directory.
//
//go:embed index.json *.tmpl fixtures/*.json
var FS embed.FS
