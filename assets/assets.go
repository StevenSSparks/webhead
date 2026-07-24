// Package assets embeds the default demo image and the dashboard page so the
// binary runs zero-config.
package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:image
var imageFS embed.FS

//go:embed dashboard/index.html
var dashHTML []byte

// ImageFS returns the embedded demo image tree (webhead.json + data/).
func ImageFS() fs.FS {
	sub, err := fs.Sub(imageFS, "image")
	if err != nil {
		panic(err) // build-time embed guarantees this exists
	}
	return sub
}

// DashboardHTML returns the embedded live-console page.
func DashboardHTML() []byte { return dashHTML }
