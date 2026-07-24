# 🕸️ Webhead

**A captive-portal appliance in a box — DNS funnel, HTTPS, a live console, and a
real SSH mini-shell. Point it at any static site.**

Webhead is a single Go binary that **flashes a device image and boots it**: it
broadcasts a captive-portal welcome, funnels every DNS name to itself, serves the
image's site over HTTP + HTTPS, streams a live console, and lets you `ssh` into a
little unix-like shell — all on localhost, no hardware, no sudo.

It began as a way to test **[Spider-Verse OS](https://github.com/stevenssparks/spiderverse-os)**
(a Seeed XIAO ESP32-S3 appliance) on a laptop, and generalized into a reusable
tool: anything with a captive portal, a wildcard-DNS trick, and an admin shell.

```
$ webhead run ~/dev/spiderverse-os
🕸️  flashing image: Spider-Verse OS
  HTTP  (portal)  → http://localhost:8080
  HTTPS (secure)  → https://localhost:8443   (or https://wififun.net after --setup-hosts)
  DNS   (funnel)  → udp:5354  (every name → 127.0.0.1)
  SSH   (shell)   → ssh spider@localhost -p 2222   (pass: spider-verse)
  DASH  (console) → http://localhost:9090
```

## Install & run

```bash
go install github.com/stevenssparks/webhead@latest

webhead                       # boot the built-in demo image
webhead run ./my-image        # boot your own image
ssh webhead@localhost -p 2222 # log into the shell   (demo pass: webhead)
open http://localhost:9090    # watch the live console
```

Or from a clone: `go build -o webhead . && ./webhead`.

### One-shot setup script

`setup.sh` builds, clears any stale SSH host key for the image's port, and runs
with `--setup-hosts` (so the domain resolves locally):

```bash
./setup.sh ~/dev/spiderverse-os     # build + host-map + run on 80/443/53 (uses sudo)
SUDO="" ./setup.sh ~/dev/spiderverse-os   # no sudo (image must use unprivileged ports)
NO_HOSTS=1 ./setup.sh                # don't touch /etc/hosts; run the demo image
```

## Commands

| Command | What it does |
|---|---|
| `webhead run [image]` | Flash the image (or the embedded demo) and boot every enabled service. |
| `webhead flash [image]` | Load + validate an image and print the config it *would* boot (dry run). |
| `webhead init [dir]` | Scaffold a new image (`webhead.json` + `data/index.html`). |
| `webhead cert status <image>` | Show the image's installed TLS cert and days-to-expiry. |
| `webhead cert refresh <image>` | Renew via acme.sh (DNS-01) and install the cert into the image. |

Run-flag overrides (win over the manifest): `--http :8080` `--https :8443`
`--dns :5354` `--ssh :2222` `--dash :9090` `--ssh-user` `--ssh-pass`
`--answer-ip` `--extended` `--setup-hosts` `--lan`.

## What an image is

An image is just a folder — so **a GitHub repo can be an image**:

```
my-image/
  webhead.json     manifest: identity, ports, which services run
  data/            the site served over HTTP/HTTPS  (index.html, …)
  certs/           optional: fullchain.pem + privkey.pem
```

`webhead.json` (every field optional — omitted fields inherit built-in defaults):

```json
{
  "name": "Spider-Verse OS",
  "hostname": "spider-verse",
  "prompt": "spider-verse# ",
  "ssid": "Spider-Verse",
  "domain": "wififun.net",
  "dnsAnswer": "127.0.0.1",
  "extendedShell": false,
  "services": {
    "http":      { "enabled": true, "addr": ":8080" },
    "https":     { "enabled": true, "addr": ":8443", "certDir": "certs" },
    "dns":       { "enabled": true, "addr": ":5354" },
    "ssh":       { "enabled": true, "addr": ":2222", "user": "spider", "pass": "spider-verse" },
    "dashboard": { "enabled": true, "addr": ":9090" }
  }
}
```

## The shell

`ssh <user>@localhost -p 2222` drops you at the image's prompt. Faithful command
set (matches the ESP32 firmware — the honest emulation of a microcontroller):

```
help  status  clients  stats  log [n]  tail  ls [path]  cat <path>
rm <path>  free  uptime  wifi  reboot
```

`tail` live-streams web hits as you browse — press Enter to stop. With
`extendedShell` / `--extended` you also get `pwd whoami echo uname clear`, clearly
banner-marked as **emulator-only, beyond the ESP32** (a real ESP32-S3 has no MMU
and can't run a unix kernel — that layer is for Pi-class targets).

## HTTPS & real certs

Webhead serves HTTPS with the image's real cert if `<image>/<certDir>/fullchain.pem`
+ `privkey.pem` are present; otherwise it generates a self-signed
`CN=<domain>` so it always runs. To browse `https://<domain>` with a clean
padlock locally, run once with `--setup-hosts` (needs sudo) to map the domain to
`dnsAnswer` in `/etc/hosts`.

Manage the real cert with acme.sh from the CLI:

```bash
webhead cert status  ~/dev/spiderverse-os   # CN, issuer, days to expiry
webhead cert refresh ~/dev/spiderverse-os   # renew (if due) + install into the image
```

`cert refresh` renews the image domain's Let's Encrypt cert via DNS-01 and copies
`fullchain.pem` + `privkey.pem` into the image's cert dir (and registers an
acme.sh reinstall hook so future auto-renewals land there too). Add `--force` to
renew before the renewal window.

## How the captive portal works

A device joins the Wi-Fi and probes a URL like `/generate_204` to check for
internet. The DNS service answers **every** lookup with the board's address, so
the probe hits Webhead and gets a `302` redirect — the phone pops the portal.
Watch every hit and DNS lookup stream in the console at `:9090`.

## Roadmap — from emulator to board

The image is a portable bundle, so it's also the artifact you flash to real
hardware. Planned bridges (not yet implemented):

- **`webhead flash-board`** — push the same image's `data/` to an ESP32 over
  `arduino-cli`/`esptool` (LittleFS), so what you tested is what ships.
- `--lan` — bind real LAN ports so another device can join over Wi-Fi.

## Development

```bash
go test ./...   # unit + SSH integration tests
go vet ./...
```

Packages: `image` (manifest/flash), `device` (shared live state: log ring, stats,
virtual FS, DNS log), `services` (http, cert, dns, shell, ssh, dashboard),
`assets` (embedded demo image + console page).

## License

MIT © 2026 Steve Sparks
