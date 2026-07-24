package main

import (
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/stevenssparks/webhead/image"
)

// cmdCert handles `webhead cert <status|refresh>` — inspecting and renewing the
// image's TLS cert via acme.sh, then installing it into the image's certDir.
func cmdCert(args []string) {
	if len(args) == 0 {
		certUsage()
		os.Exit(2)
	}
	switch args[0] {
	case "status":
		certStatus(args[1:])
	case "refresh":
		certRefresh(args[1:])
	default:
		certUsage()
		os.Exit(2)
	}
}

func certUsage() {
	fmt.Print(`webhead cert — inspect and refresh the image's TLS cert

usage:
  webhead cert status <image>            show the installed cert + expiry
  webhead cert refresh <image> [--force] renew via acme.sh and install into the image

refresh renews the image domain's Let's Encrypt cert (acme.sh, DNS-01) and copies
fullchain.pem + privkey.pem into the image's cert dir. --force renews even if the
cert isn't near expiry (watch Let's Encrypt rate limits).
`)
}

// certImage loads an on-disk image (cert commands need a real dir + domain).
func certImage(path string) *image.Image {
	if path == "" {
		fatal(fmt.Errorf("cert commands need an image path (the dir holding webhead.json)"))
	}
	im, err := image.Load(path)
	if err != nil {
		fatal(err)
	}
	if im.CertDirPath() == "" {
		fatal(fmt.Errorf("image %q has no https certDir configured", path))
	}
	return im
}

func readCert(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	for {
		var blk *pem.Block
		blk, b = pem.Decode(b)
		if blk == nil {
			return nil, fmt.Errorf("no CERTIFICATE block in %s", path)
		}
		if blk.Type == "CERTIFICATE" {
			return x509.ParseCertificate(blk.Bytes)
		}
	}
}

func certStatus(args []string) {
	imgArg, _ := splitImageArgs(args)
	im := certImage(imgArg)
	full := filepath.Join(im.CertDirPath(), "fullchain.pem")
	fmt.Printf("image  : %s\ndomain : %s\ncertdir: %s\n", im.Dir, im.Manifest.Domain, im.CertDirPath())
	c, err := readCert(full)
	if err != nil {
		fmt.Printf("cert   : none installed (%v)\n         HTTPS will use a self-signed fallback until you run `webhead cert refresh`.\n", err)
		return
	}
	days := int(time.Until(c.NotAfter).Hours() / 24)
	fmt.Printf("cert   : CN=%s  issuer=%s\n", c.Subject.CommonName, c.Issuer.Organization)
	fmt.Printf("expires: %s  (%d days left)\n", c.NotAfter.Format("2006-01-02"), days)
	if days < 21 {
		fmt.Println("         ⚠ within renewal window — run `webhead cert refresh`.")
	}
}

func findAcme() (string, error) {
	if p, err := exec.LookPath("acme.sh"); err == nil {
		return p, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".acme.sh", "acme.sh")
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("acme.sh not found (install it: https://acme.sh) — see CERT_REFRESH_RUNBOOK.md")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func certRefresh(args []string) {
	fs := flag.NewFlagSet("cert refresh", flag.ExitOnError)
	force := fs.Bool("force", false, "renew even if the cert is not near expiry")
	imgArg, rest := splitImageArgs(args)
	fs.Parse(rest)
	if imgArg == "" {
		imgArg = fs.Arg(0)
	}

	im := certImage(imgArg)
	domain := im.Manifest.Domain
	certDir := im.CertDirPath()
	if err := os.MkdirAll(certDir, 0755); err != nil {
		fatal(err)
	}
	acme, err := findAcme()
	if err != nil {
		fatal(err)
	}
	full := filepath.Join(certDir, "fullchain.pem")
	key := filepath.Join(certDir, "privkey.pem")

	fmt.Printf("==> refreshing %s cert into %s\n", domain, certDir)

	// 1) Renew (acme.sh skips if not due unless --force). A skip is not an error.
	renewArgs := []string{"--renew", "-d", domain, "--ecc"}
	if *force {
		renewArgs = append(renewArgs, "--force")
	}
	fmt.Printf("==> acme.sh %v\n", renewArgs)
	if err := run(acme, renewArgs...); err != nil {
		fmt.Println("    (renew reported nothing to do or a non-fatal status; continuing to install)")
	}

	// 2) Install the current cert into the image and register a reinstall hook
	//    so future auto-renewals land here too.
	installArgs := []string{"--install-cert", "-d", domain, "--ecc",
		"--fullchain-file", full, "--key-file", key}
	fmt.Printf("==> acme.sh --install-cert -> %s\n", certDir)
	if err := run(acme, installArgs...); err != nil {
		fatal(fmt.Errorf("acme.sh --install-cert failed: %w", err))
	}

	// 3) Report the result.
	if c, err := readCert(full); err == nil {
		days := int(time.Until(c.NotAfter).Hours() / 24)
		fmt.Printf("==> installed: CN=%s, expires %s (%d days left)\n", c.Subject.CommonName, c.NotAfter.Format("2006-01-02"), days)
		fmt.Println("    run `webhead run` to serve it, then verify the padlock at https://" + domain)
	}
}
