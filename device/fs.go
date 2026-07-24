package device

import (
	"errors"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

type FileInfo struct {
	Name  string
	Size  int64
	IsDir bool
}

// VFS is a read view over an fs.FS payload with an in-memory removal overlay, so
// `rm` behaves without mutating the underlying (often embedded) source.
type VFS struct {
	base    fs.FS
	mu      sync.Mutex
	removed map[string]bool // clean paths marked deleted
}

func NewVFS(base fs.FS) *VFS {
	return &VFS{base: base, removed: map[string]bool{}}
}

// clean converts a firmware-style path ("/", "/games/x.html") to an fs.FS key
// ("." , "games/x.html").
func clean(p string) string {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

func (v *VFS) isRemoved(key string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.removed[key]
}

func (v *VFS) List(dir string) ([]FileInfo, error) {
	key := clean(dir)
	entries, err := fs.ReadDir(v.base, key)
	if err != nil {
		return nil, err
	}
	var out []FileInfo
	for _, e := range entries {
		child := e.Name()
		if key != "." {
			child = key + "/" + e.Name()
		}
		if v.isRemoved(child) {
			continue
		}
		var size int64
		if info, err := e.Info(); err == nil {
			size = info.Size()
		}
		out = append(out, FileInfo{Name: e.Name(), Size: size, IsDir: e.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (v *VFS) Read(path string) ([]byte, error) {
	key := clean(path)
	if v.isRemoved(key) {
		return nil, errors.New("no such file")
	}
	return fs.ReadFile(v.base, key)
}

func (v *VFS) Exists(path string) bool {
	key := clean(path)
	if v.isRemoved(key) {
		return false
	}
	if _, err := fs.Stat(v.base, key); err != nil {
		return false
	}
	return true
}

func (v *VFS) Remove(path string) error {
	key := clean(path)
	if _, err := fs.Stat(v.base, key); err != nil {
		return errors.New("could not remove")
	}
	v.mu.Lock()
	v.removed[key] = true
	v.mu.Unlock()
	return nil
}
