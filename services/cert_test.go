package services

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectCertificateFallback(t *testing.T) {
	cert, source, err := SelectCertificate(t.TempDir(), "wififun.net")
	if err != nil {
		t.Fatal(err)
	}
	if source != "self-signed" {
		t.Fatalf("source=%q want self-signed", source)
	}
	if len(cert.Certificate) == 0 {
		t.Fatal("no cert material")
	}
}

func TestSelectCertificateUsesRealWhenPresent(t *testing.T) {
	dir := t.TempDir()
	c, err := generateSelfSigned("wififun.net")
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, dir, c)
	_, source, err := SelectCertificate(dir, "wififun.net")
	if err != nil {
		t.Fatal(err)
	}
	if source != "real" {
		t.Fatalf("source=%q want real", source)
	}
}

func TestSelectCertificateEmptyDirFallsBack(t *testing.T) {
	_, source, err := SelectCertificate("", "roost.local")
	if err != nil || source != "self-signed" {
		t.Fatalf("source=%q err=%v", source, err)
	}
}

func writePEM(t *testing.T, dir string, c tls.Certificate) {
	t.Helper()
	certOut, err := os.Create(filepath.Join(dir, "fullchain.pem"))
	if err != nil {
		t.Fatal(err)
	}
	defer certOut.Close()
	if err := pemEncodeCert(certOut, c.Certificate[0]); err != nil {
		t.Fatal(err)
	}
	keyOut, err := os.Create(filepath.Join(dir, "privkey.pem"))
	if err != nil {
		t.Fatal(err)
	}
	defer keyOut.Close()
	if err := pemEncodeKey(keyOut, c.PrivateKey); err != nil {
		t.Fatal(err)
	}
}
