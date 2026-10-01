// Package frontend embeds the built SPA (dist/, `make frontend`) into the binary.
package frontend

import (
	"embed"
	"io/fs"
)

// files holds dist/ when it was built. The glob also matches the committed dist.md, so
// the package compiles before the first frontend build; such a binary serves no UI.
//
//go:embed all:dist*
var files embed.FS

// Dist returns the built SPA rooted at dist/; it has no index.html when dist/ was not
// built before the binary.
func Dist() (fs.FS, error) {
	return fs.Sub(files, "dist")
}
