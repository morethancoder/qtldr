// Package web holds the built web UI (dist/, committed so `go install` works
// without Node) and the theme palettes shared with the Go side.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// Themes is src/themes.json, the single source of truth for colors.
//
//go:embed src/themes.json
var Themes []byte

// Dist returns the built UI rooted at dist/.
func Dist() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
