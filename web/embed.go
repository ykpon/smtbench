// Package web holds the static UI served by the collector.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// FS is the UI root (index.html at the top).
var FS, _ = fs.Sub(files, "static")
