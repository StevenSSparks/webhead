package main

import (
	"embed"
	"io/fs"
)

// embeddedImage is the default demo image (FriendlyPortal OS) baked into the
// binary so `webhead run` with no argument works zero-config. It also lives at
// examples/friendlyportal-os/ for people to copy and customize.
//
//go:embed all:examples/friendlyportal-os
var embeddedImage embed.FS

func defaultImageFS() fs.FS {
	sub, err := fs.Sub(embeddedImage, "examples/friendlyportal-os")
	if err != nil {
		panic(err) // build-time embed guarantees this path exists
	}
	return sub
}

// referenceFirmware is the generic ESP32 portal sketch, embedded so
// `webhead build-image` can compile it when an image ships no firmware/ of its own.
//
//go:embed all:firmware/webhead-portal
var referenceFirmware embed.FS
