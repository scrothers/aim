package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the embedded UI rooted at its index.html. It is never nil but
// may be empty when the UI has not been built.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return dist // unreachable: "dist" is embedded above
	}
	return sub
}

// Built reports whether the UI has been built into the binary.
func Built() bool {
	info, err := fs.Stat(dist, "dist/index.html")
	return err == nil && !info.IsDir()
}
