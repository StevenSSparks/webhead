package device

import (
	"testing"
	"testing/fstest"
)

func testFS() *VFS {
	return NewVFS(fstest.MapFS{
		"index.html":       {Data: []byte("<h1>hi</h1>")},
		"games/2048.html":  {Data: []byte("game")},
		"games/simon.html": {Data: []byte("game2")},
	})
}

func TestVFSListRoot(t *testing.T) {
	v := testFS()
	got, err := v.List("/")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, fi := range got {
		names[fi.Name] = true
	}
	if !names["index.html"] || !names["games"] {
		t.Fatalf("root listing=%v", got)
	}
}

func TestVFSReadAndExists(t *testing.T) {
	v := testFS()
	b, err := v.Read("/index.html")
	if err != nil || string(b) != "<h1>hi</h1>" {
		t.Fatalf("read=%q err=%v", b, err)
	}
	if !v.Exists("/games/2048.html") {
		t.Fatal("should exist")
	}
}

func TestVFSRemoveOverlay(t *testing.T) {
	v := testFS()
	if err := v.Remove("/games/2048.html"); err != nil {
		t.Fatal(err)
	}
	if v.Exists("/games/2048.html") {
		t.Fatal("should be removed from view")
	}
	if _, err := v.Read("/games/2048.html"); err == nil {
		t.Fatal("read of removed should error")
	}
	if !v.Exists("/games/simon.html") {
		t.Fatal("sibling should remain")
	}
}
