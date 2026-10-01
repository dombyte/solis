// Package docs embeds the built Swagger UI (dist/, `make docs`) into the binary.
package docs

import (
	"embed"
	"io/fs"
)

// files holds dist/ when it was built. The glob also matches the committed dist.md, so
// the package compiles before the first docs build; such a binary serves no API docs.
//
//go:embed all:dist*
var files embed.FS

// Dist returns the built Swagger UI rooted at dist/; it has no index.html when dist/ was not
// built before the binary.
func Dist() (fs.FS, error) {
	return fs.Sub(files, "dist")
}
