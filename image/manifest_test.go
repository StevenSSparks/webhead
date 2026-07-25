package image

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultHasFiveEnabledServices(t *testing.T) {
	m := Default()
	for _, k := range KnownServices {
		svc, ok := m.Svc(k)
		if !ok || !svc.Enabled || svc.Addr == "" {
			t.Fatalf("default service %q not ready: %+v", k, svc)
		}
	}
}

func TestLoadMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := `{
	  "name": "Demo OS",
	  "domain": "demo.net",
	  "prompt": "demo# ",
	  "services": { "ssh": { "enabled": true, "user": "demo", "pass": "demo" },
	                "dns": { "enabled": false } }
	}`
	os.WriteFile(filepath.Join(dir, "roost.json"), []byte(cfg), 0644)

	im, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := im.Manifest
	if m.Name != "Demo OS" || m.Domain != "demo.net" {
		t.Fatalf("overrides not applied: %+v", m)
	}
	if m.SSID != "Roost" { // untouched field keeps default
		t.Fatalf("default SSID lost: %q", m.SSID)
	}
	ssh, _ := m.Svc("ssh")
	if ssh.User != "demo" || ssh.Pass != "demo" || ssh.Addr != ":2222" {
		t.Fatalf("ssh merge wrong: %+v", ssh) // addr filled from default
	}
	dns, _ := m.Svc("dns")
	if dns.Enabled {
		t.Fatalf("dns should be disabled")
	}
	http, _ := m.Svc("http")
	if !http.Enabled || http.Addr != ":8080" {
		t.Fatalf("untouched http service lost defaults: %+v", http)
	}
}

func TestLoadRequiresDataDir(t *testing.T) {
	dir := t.TempDir() // no data/
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for missing data/ dir")
	}
}

func TestValidateRejectsBadAddr(t *testing.T) {
	m := Default()
	svc := m.Services["http"]
	svc.Addr = "not-an-addr"
	m.Services["http"] = svc
	if err := m.Validate(); err == nil {
		t.Fatal("expected validation error for bad addr")
	}
}

func TestUnknownServicesReported(t *testing.T) {
	m := Default()
	m.Services["mqtt"] = Service{Enabled: true, Addr: ":1883"}
	got := m.UnknownServices()
	if len(got) != 1 || got[0] != "mqtt" {
		t.Fatalf("unknown services=%v", got)
	}
}
