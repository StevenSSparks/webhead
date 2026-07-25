// Package image loads a Roost device image — a directory holding a
// roost.json manifest, a data/ payload, and optional certs/ — and resolves it
// against built-in defaults so the runtime can boot from it.
package image

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// Service configures one emulated service in the image manifest.
type Service struct {
	Enabled bool   `json:"enabled"`
	Addr    string `json:"addr"`
	User    string `json:"user,omitempty"`    // ssh
	Pass    string `json:"pass,omitempty"`    // ssh
	CertDir string `json:"certDir,omitempty"` // https, relative to image dir
}

// Manifest is the parsed roost.json.
type Manifest struct {
	Name          string             `json:"name"`
	Hostname      string             `json:"hostname"`
	Prompt        string             `json:"prompt"`
	SSID          string             `json:"ssid"`
	Domain        string             `json:"domain"`
	DNSAnswer     string             `json:"dnsAnswer"`
	ExtendedShell bool               `json:"extendedShell"`
	Motd          string             `json:"motd"` // optional login message-of-the-day
	Services      map[string]Service `json:"services"`
}

// Image is a loaded, validated device image ready to boot.
type Image struct {
	Manifest Manifest
	Dir      string // on-disk image dir; "" for the embedded default image
	Payload  fs.FS  // the data/ payload
}

// KnownServices are the service keys the runtime understands, in display order.
var KnownServices = []string{"http", "https", "dns", "ssh", "dashboard"}

var defaultAddrs = map[string]string{
	"http": ":8080", "https": ":8443", "dns": ":5354", "ssh": ":2222", "dashboard": ":9090",
}

// Default returns the built-in manifest used for the embedded demo image and as
// the merge base for every loaded image (omitted fields inherit these).
func Default() Manifest {
	return Manifest{
		Name:          "Roost",
		Hostname:      "roost",
		Prompt:        "roost# ",
		SSID:          "Roost",
		Domain:        "roost.local",
		DNSAnswer:     "127.0.0.1",
		ExtendedShell: false,
		Services: map[string]Service{
			"http":      {Enabled: true, Addr: ":8080"},
			"https":     {Enabled: true, Addr: ":8443", CertDir: "certs"},
			"dns":       {Enabled: true, Addr: ":5354"},
			"ssh":       {Enabled: true, Addr: ":2222", User: "roost", Pass: "roost"},
			"dashboard": {Enabled: true, Addr: ":9090"},
		},
	}
}

// normalize fills any missing per-service defaults after a manifest merge, so a
// service entry that only sets {"enabled":true} still gets a usable Addr.
func normalize(m *Manifest) {
	if m.Services == nil {
		m.Services = map[string]Service{}
	}
	def := Default()
	for _, key := range KnownServices {
		svc, ok := m.Services[key]
		if !ok {
			continue // absent = inherit default entry
		}
		d := def.Services[key]
		if svc.Addr == "" {
			svc.Addr = d.Addr
			if svc.Addr == "" {
				svc.Addr = defaultAddrs[key]
			}
		}
		if key == "ssh" {
			if svc.User == "" {
				svc.User = d.User
			}
			if svc.Pass == "" {
				svc.Pass = d.Pass
			}
		}
		if key == "https" && svc.CertDir == "" {
			svc.CertDir = d.CertDir
		}
		m.Services[key] = svc
	}
}

// Validate checks that every enabled known service has a parseable address.
func (m Manifest) Validate() error {
	if m.DNSAnswer != "" && net.ParseIP(m.DNSAnswer) == nil {
		return fmt.Errorf("dnsAnswer %q is not a valid IP", m.DNSAnswer)
	}
	for _, key := range KnownServices {
		svc, ok := m.Services[key]
		if !ok || !svc.Enabled {
			continue
		}
		_, port, err := net.SplitHostPort(svc.Addr)
		if err != nil {
			return fmt.Errorf("service %q: invalid addr %q: %w", key, svc.Addr, err)
		}
		if _, err := strconv.Atoi(port); err != nil {
			return fmt.Errorf("service %q: non-numeric port in %q", key, svc.Addr)
		}
	}
	return nil
}

// Svc returns a service entry by key.
func (m Manifest) Svc(name string) (Service, bool) {
	s, ok := m.Services[name]
	return s, ok
}

// UnknownServices lists any service keys in the manifest the runtime doesn't
// understand, so the caller can warn about them.
func (m Manifest) UnknownServices() []string {
	known := map[string]bool{}
	for _, k := range KnownServices {
		known[k] = true
	}
	var out []string
	for k := range m.Services {
		if !known[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func mergeInto(m *Manifest, data []byte) error {
	if err := json.Unmarshal(data, m); err != nil {
		return err
	}
	normalize(m)
	return nil
}

// Load reads and validates an on-disk image directory.
func Load(dir string) (*Image, error) {
	m := Default()
	cfg := filepath.Join(dir, "roost.json")
	if b, err := os.ReadFile(cfg); err == nil {
		if err := mergeInto(&m, b); err != nil {
			return nil, fmt.Errorf("%s: %w", cfg, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	} else {
		normalize(&m)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	dataDir := filepath.Join(dir, "data")
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("image %q has no data/ directory", dir)
	}
	return &Image{Manifest: m, Dir: dir, Payload: os.DirFS(dataDir)}, nil
}

// LoadFS reads and validates an image from an fs.FS (the embedded demo image).
func LoadFS(fsys fs.FS) (*Image, error) {
	m := Default()
	if b, err := fs.ReadFile(fsys, "roost.json"); err == nil {
		if err := mergeInto(&m, b); err != nil {
			return nil, fmt.Errorf("embedded roost.json: %w", err)
		}
	} else {
		normalize(&m)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	sub, err := fs.Sub(fsys, "data")
	if err != nil {
		return nil, err
	}
	return &Image{Manifest: m, Payload: sub}, nil
}

// CertDirPath returns the absolute cert directory for the https service, or ""
// if the image is embedded (no on-disk certs).
func (im *Image) CertDirPath() string {
	if im.Dir == "" {
		return ""
	}
	svc, ok := im.Manifest.Svc("https")
	if !ok || svc.CertDir == "" {
		return ""
	}
	return filepath.Join(im.Dir, svc.CertDir)
}

// Summary renders the resolved config for `roost flash` (dry run).
func (im *Image) Summary() string {
	m := im.Manifest
	src := im.Dir
	if src == "" {
		src = "(embedded demo image)"
	}
	s := fmt.Sprintf("image    : %s\nname     : %s\nhostname : %s\nssid     : %s\ndomain   : %s → %s\nprompt   : %q\nextended : %v\nservices :",
		src, m.Name, m.Hostname, m.SSID, m.Domain, m.DNSAnswer, m.Prompt, m.ExtendedShell)
	for _, key := range KnownServices {
		svc := m.Services[key]
		state := "off"
		if svc.Enabled {
			state = "on " + svc.Addr
		}
		s += fmt.Sprintf("\n  %-9s %s", key, state)
	}
	return s
}
