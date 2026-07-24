// Package assets embeds the dashboard page so the binary serves it zero-config.
package assets

import _ "embed"

//go:embed dashboard/index.html
var dashHTML []byte

// DashboardHTML returns the embedded live-console page.
func DashboardHTML() []byte { return dashHTML }
