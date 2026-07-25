package services

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// SelectCertificate returns the image's real cert if certDir holds a valid
// fullchain.pem + privkey.pem (source "real"); otherwise a freshly generated
// self-signed cert for domain (source "self-signed"). It only errors if
// generating the fallback itself fails. An empty certDir always falls back.
func SelectCertificate(certDir, domain string) (tls.Certificate, string, error) {
	if certDir != "" {
		full := filepath.Join(certDir, "fullchain.pem")
		key := filepath.Join(certDir, "privkey.pem")
		if fileExists(full) && fileExists(key) {
			if cert, err := tls.LoadX509KeyPair(full, key); err == nil {
				return cert, "real", nil
			}
		}
	}
	cert, err := generateSelfSigned(domain)
	if err != nil {
		return tls.Certificate{}, "", err
	}
	return cert, "self-signed", nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func generateSelfSigned(domain string) (tls.Certificate, error) {
	if domain == "" {
		domain = "roost.local"
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain, Organization: []string{"Roost (emulator)"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{domain, "*." + domain, "localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}

func pemEncodeCert(w io.Writer, der []byte) error {
	return pem.Encode(w, &pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func pemEncodeKey(w io.Writer, key any) error {
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return pem.Encode(w, &pem.Block{Type: "PRIVATE KEY", Bytes: b})
}
