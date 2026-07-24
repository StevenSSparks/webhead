// Command webhead flashes a device image and boots its emulated services:
// captive-portal HTTP, HTTPS, a DNS funnel, a live console, and an SSH mini-shell.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/stevenssparks/webhead/assets"
	"github.com/stevenssparks/webhead/device"
	"github.com/stevenssparks/webhead/image"
	"github.com/stevenssparks/webhead/services"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		cmdRun(nil)
		return
	}
	switch args[0] {
	case "run":
		cmdRun(args[1:])
	case "flash":
		cmdFlash(args[1:])
	case "init":
		cmdInit(args[1:])
	case "cert":
		cmdCert(args[1:])
	case "flash-board":
		cmdFlashBoard(args[1:])
	case "version", "ver", "--version", "-v":
		fmt.Println("webhead " + services.Version)
	case "-h", "--help", "help":
		usage()
	default:
		// `webhead ./image` or `webhead --extended` → treat as run.
		cmdRun(args)
	}
}

func usage() {
	fmt.Print(`webhead — a captive-portal appliance emulator in a box

usage:
  webhead run [image] [flags]   flash an image (or the embedded demo) and boot it
  webhead flash [image]         validate an image and print the config it would boot
  webhead init [dir]            scaffold a new image (webhead.json + data/)
  webhead cert status <image>   show the image's installed TLS cert + expiry
  webhead cert refresh <image>  renew (acme.sh) and install the cert into the image
  webhead flash-board <image>   build a LittleFS image and flash it to an ESP32 (--confirm to write)

run flags (override the image manifest):
  --http :8080     --https :8443    --dns :5354     --ssh :2222    --dash :9090
  --ssh-user U     --ssh-pass P     --answer-ip IP
  --extended       enable emulator-only extended shell commands
  --setup-hosts    map the image domain -> answer-ip in /etc/hosts (needs sudo)
  --lan            future: bind real LAN ports (not implemented)

examples:
  webhead                                   # run the embedded demo image
  webhead run examples/friendlyportal-os    # run the FriendlyPortal demo image
  webhead run . --setup-hosts               # run this dir's image, map its domain
`)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func loadImage(path string) (*image.Image, error) {
	if path == "" {
		return image.LoadFS(defaultImageFS())
	}
	return image.Load(path)
}

func portFree(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

func cmdFlash(args []string) {
	fs := flag.NewFlagSet("flash", flag.ExitOnError)
	fs.Parse(args)
	im, err := loadImage(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	fmt.Println("[flash] image loaded OK")
	fmt.Println()
	fmt.Println(im.Summary())
	if unknown := im.Manifest.UnknownServices(); len(unknown) > 0 {
		fmt.Printf("\n[warn] ignoring unknown service(s): %s\n", strings.Join(unknown, ", "))
	}
}

func cmdInit(args []string) {
	dir := "."
	if len(args) > 0 && args[0] != "" {
		dir = args[0]
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0755); err != nil {
		fatal(err)
	}
	cfg := filepath.Join(dir, "webhead.json")
	if _, err := os.Stat(cfg); os.IsNotExist(err) {
		b, _ := json.MarshalIndent(image.Default(), "", "  ")
		if err := os.WriteFile(cfg, append(b, '\n'), 0644); err != nil {
			fatal(err)
		}
	}
	idx := filepath.Join(dir, "data", "index.html")
	if _, err := os.Stat(idx); os.IsNotExist(err) {
		os.WriteFile(idx, []byte("<!doctype html><meta charset=utf-8><title>My image</title><h1>Hello from Webhead</h1><p>Edit data/index.html.</p>\n"), 0644)
	}
	fmt.Printf("scaffolded image in %s\n  edit webhead.json + data/, then:  webhead run %s\n", dir, dir)
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	httpAddr := fs.String("http", "", "override HTTP addr")
	httpsAddr := fs.String("https", "", "override HTTPS addr")
	dnsAddr := fs.String("dns", "", "override DNS addr")
	sshAddr := fs.String("ssh", "", "override SSH addr")
	dashAddr := fs.String("dash", "", "override dashboard addr")
	sshUser := fs.String("ssh-user", "", "override SSH user")
	sshPass := fs.String("ssh-pass", "", "override SSH pass")
	answerIP := fs.String("answer-ip", "", "override DNS answer IP")
	extended := fs.Bool("extended", false, "enable extended shell commands")
	setupHosts := fs.Bool("setup-hosts", false, "map image domain in /etc/hosts")
	lan := fs.Bool("lan", false, "future LAN mode (stub)")

	// The image path is an optional positional arg. Go's flag package stops at
	// the first non-flag token, so `run <image> --setup-hosts` would drop the
	// flag. Pull a leading positional out first, then parse the rest — this lets
	// flags appear before or after the image path.
	image, rest := splitImageArgs(args)
	fs.Parse(rest)
	if image == "" {
		image = fs.Arg(0)
	}

	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })

	im, err := loadImage(image)
	if err != nil {
		fatal(err)
	}
	m := &im.Manifest

	setAddr := func(key string, on bool, v string) {
		if !on {
			return
		}
		s := m.Services[key]
		s.Addr = v
		m.Services[key] = s
	}
	setAddr("http", set["http"], *httpAddr)
	setAddr("https", set["https"], *httpsAddr)
	setAddr("dns", set["dns"], *dnsAddr)
	setAddr("ssh", set["ssh"], *sshAddr)
	setAddr("dashboard", set["dash"], *dashAddr)
	if set["ssh-user"] {
		s := m.Services["ssh"]
		s.User = *sshUser
		m.Services["ssh"] = s
	}
	if set["ssh-pass"] {
		s := m.Services["ssh"]
		s.Pass = *sshPass
		m.Services["ssh"] = s
	}
	if set["answer-ip"] {
		m.DNSAnswer = *answerIP
	}
	if set["extended"] {
		m.ExtendedShell = *extended
	}
	if err := m.Validate(); err != nil {
		fatal(err)
	}

	if *lan {
		fmt.Println("--lan: real Mac WiFi/LAN mode is not implemented yet; continuing in localhost mode.")
	}
	if unknown := m.UnknownServices(); len(unknown) > 0 {
		fmt.Printf("[warn] ignoring unknown service(s): %s\n", strings.Join(unknown, ", "))
	}

	st := device.New(im.Payload)
	st.System("     0s  boot — device up")

	shellProto := services.Shell{
		Extended:  m.ExtendedShell,
		Prompt:    m.Prompt,
		SSID:      m.SSID,
		Title:     m.Name,
		Hostname:  m.Hostname,
		Motd:      m.Motd,
		DNSAnswer: m.DNSAnswer,
	}

	fmt.Printf("\n🕸️  flashing image: %s\n", m.Name)
	if im.Dir != "" {
		fmt.Printf("    source: %s\n", im.Dir)
	} else {
		fmt.Printf("    source: (embedded demo image)\n")
	}
	fmt.Println()

	if *setupHosts {
		fmt.Println("[hosts]", ensureHostsEntry(m.Domain, m.DNSAnswer))
	}

	startWeb := func(key, tag string, tls bool) {
		svc, ok := m.Svc(key)
		if !ok || !svc.Enabled {
			return
		}
		if !portFree(svc.Addr) {
			fmt.Printf("  %-5s port %s in use — skipped (override with --%s)\n", tag, svc.Addr, flagFor(key))
			return
		}
		if tls {
			startHTTPS(st, svc.Addr, im, m)
		} else {
			go func() {
				if err := http.ListenAndServe(svc.Addr, services.NewHandler(st, "HTTP")); err != nil {
					fmt.Println("[http] stopped:", err)
				}
			}()
			fmt.Printf("  HTTP  (portal)  → http://localhost%s\n", svc.Addr)
		}
	}
	startWeb("http", "HTTP", false)
	startWeb("https", "HTTPS", true)

	// DNS
	if svc, ok := m.Svc("dns"); ok && svc.Enabled {
		if _, err := services.StartDNS(st, svc.Addr, m.DNSAnswer); err != nil {
			fmt.Printf("  DNS   %s failed: %v (override with --dns)\n", svc.Addr, err)
		} else {
			fmt.Printf("  DNS   (funnel)  → udp%s  (every name → %s)\n", svc.Addr, m.DNSAnswer)
		}
	}

	// SSH
	if svc, ok := m.Svc("ssh"); ok && svc.Enabled {
		if !portFree(svc.Addr) {
			fmt.Printf("  SSH   port %s in use — skipped (override with --ssh)\n", svc.Addr)
		} else {
			hkPath := hostKeyPath(im)
			shellProto.User = svc.User
			go func() {
				if err := services.StartSSH(st, svc.Addr, svc.User, svc.Pass, hkPath, shellProto); err != nil {
					fmt.Println("[ssh] stopped:", err)
				}
			}()
			fmt.Printf("  SSH   (shell)   → ssh %s@localhost -p %s   (pass: %s)\n", svc.User, portOf(svc.Addr), svc.Pass)
		}
	}

	// Dashboard
	if svc, ok := m.Svc("dashboard"); ok && svc.Enabled {
		if !portFree(svc.Addr) {
			fmt.Printf("  DASH  port %s in use — skipped (override with --dash)\n", svc.Addr)
		} else {
			go func() {
				if err := http.ListenAndServe(svc.Addr, services.NewDashboard(st, assets.DashboardHTML())); err != nil {
					fmt.Println("[dash] stopped:", err)
				}
			}()
			fmt.Printf("  DASH  (console) → http://localhost%s\n", svc.Addr)
		}
	}

	fmt.Printf("\n=== %s running.  Ctrl-C to stop. ===\n", m.Name)
	if m.ExtendedShell {
		fmt.Println("(extended shell mode ON — emulator-only commands available)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	fmt.Println("\nshutting down.")
}

func startHTTPS(st *device.State, addr string, im *image.Image, m *image.Manifest) {
	cert, source, err := services.SelectCertificate(im.CertDirPath(), m.Domain)
	if err != nil {
		fmt.Println("  [https] cert error:", err)
		return
	}
	if source == "self-signed" {
		where := im.CertDirPath()
		if where == "" {
			where = "the image's certs/ dir"
		}
		fmt.Printf("  [https] using SELF-SIGNED cert (CN=%s). Drop real certs in %s to use them.\n", m.Domain, where)
	} else {
		fmt.Printf("  [https] using real %s cert from %s\n", m.Domain, im.CertDirPath())
	}
	srv := &http.Server{
		Addr:      addr,
		Handler:   services.NewHandler(st, "HTTPS"),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
	}
	go func() {
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			fmt.Println("[https] stopped:", err)
		}
	}()
	fmt.Printf("  HTTPS (secure)  → https://localhost%s   (or https://%s after --setup-hosts)\n", addr, m.Domain)
}

func flagFor(key string) string {
	if key == "dashboard" {
		return "dash"
	}
	return key
}

// hostKeyPath returns where to persist the SSH host key: inside the image dir
// for on-disk images, or the user config dir for the embedded demo.
func hostKeyPath(im *image.Image) string {
	if im.Dir != "" {
		return filepath.Join(im.Dir, ".webhead", "ssh_host_key")
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		return filepath.Join(cfg, "webhead", "ssh_host_key")
	}
	return ""
}

// splitImageArgs pulls a leading positional image path out of args so run flags
// can appear after the image path (Go's flag package otherwise stops parsing at
// the first non-flag token). A leading token that starts with "-" is treated as
// a flag, not the image.
func splitImageArgs(args []string) (image string, rest []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func portOf(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return port
	}
	return strings.TrimPrefix(addr, ":")
}
