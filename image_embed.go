package main

import (
	"embed"
	"io/fs"
)

// embeddedImage is the default demo image (FriendlyPortal OS) baked into the
// binary so `roost run` with no argument works zero-config. It also lives at
// examples/friendlyportal/ for people to copy and customize.
//
//go:embed all:examples/friendlyportal
var embeddedImage embed.FS

func defaultImageFS() fs.FS {
	sub, err := fs.Sub(embeddedImage, "examples/friendlyportal")
	if err != nil {
		panic(err) // build-time embed guarantees this path exists
	}
	return sub
}

// referenceFirmware is the generic ESP32 portal sketch, embedded so
// `roost build-image` can compile it when an image ships no firmware/ of its own.
//
//go:embed all:firmware/roost32
var referenceFirmware embed.FS
