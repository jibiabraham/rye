// Package web holds the GUI's built files for ryed to serve. pnpm build writes them to web/dist. Every Go build needs
// them first.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Files returns the built files, with index.html at the root.
func Files() fs.FS {
	files, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
