# Make your own portal OS

This guide walks you from zero to your **own captive-portal appliance** — a little
box that greets people on its Wi-Fi, serves your website with a real HTTPS
padlock, and lets you `ssh` in. You'll start on your laptop (no hardware needed)
and finish by flashing it onto a real ESP32 board.

No prior embedded experience required. Every step is copy-paste.

**Contents**
1. [Install webhead](#1-install-webhead)
2. [Make your image](#2-make-your-image)
3. [Customize it](#3-customize-it) — the part most people care about
4. [Run & test locally](#4-run--test-locally)
5. [Add a real domain + free HTTPS cert](#5-add-a-real-domain--free-https-cert)
6. [Flash it onto an ESP32](#6-flash-it-onto-an-esp32)
7. [Troubleshooting](#troubleshooting)

---

## 1. Install webhead

You need [Go](https://go.dev/dl/) 1.22+.

```bash
go install github.com/stevenssparks/webhead@latest
# …or clone and build:
git clone https://github.com/StevenSSparks/webhead
cd webhead && go build -o webhead .
```

Check it runs — this boots the built-in **FriendlyPortal** demo:

```bash
webhead                 # Ctrl-C to stop
```

Open <http://localhost:8080> (the portal) and <http://localhost:9090> (the live
console). SSH in with `ssh friendly@localhost -p 2222` (password `friendly`).

---

## 2. Make your image

An **image** is just a folder. That's the whole idea:

```
my-portal/
  webhead.json     ← settings: name, domain, ports, which services run
  data/            ← your website (index.html and friends)
  certs/           ← optional: your HTTPS certificate goes here later
```

Start from the FriendlyPortal example (it's a complete, working portal):

```bash
cp -R webhead/examples/friendlyportal-os my-portal
# or scaffold an empty one:
webhead init my-portal
```

---

## 3. Customize it

### 3a. Settings — `webhead.json`

Open `my-portal/webhead.json`. Every field is optional; here's what each does:

| Field | What it is | Example |
|---|---|---|
| `name` | Shown in the SSH login banner and startup | `"Block Party OS"` |
| `hostname` | The device's name | `"blockparty"` |
| `prompt` | Your SSH shell prompt | `"blockparty# "` |
| `ssid` | The Wi-Fi network name it advertises | `"Block Party"` |
| `domain` | The address people browse to | `"blockparty.net"` |
| `dnsAnswer` | IP every name resolves to (leave `127.0.0.1` for local) | `"127.0.0.1"` |
| `extendedShell` | `true` adds unix-flavor shell commands (`cd`, `date`, …) | `true` |
| `motd` | A welcome message shown at SSH login | `"Welcome! Grab a game."` |
| `services` | Ports for http/https/dns/ssh/dashboard, and the ssh user/pass | see below |

```json
{
  "name": "Block Party OS",
  "ssid": "Block Party",
  "domain": "blockparty.net",
  "extendedShell": true,
  "motd": "Welcome to the block party! Games below, no wifi bill.",
  "services": {
    "http":      { "enabled": true, "addr": ":8080" },
    "https":     { "enabled": true, "addr": ":8443", "certDir": "certs" },
    "dns":       { "enabled": true, "addr": ":5354" },
    "ssh":       { "enabled": true, "addr": ":2222", "user": "guest", "pass": "guest" },
    "dashboard": { "enabled": true, "addr": ":9090" }
  }
}
```

> Tip: `webhead flash my-portal` prints the resolved settings without starting
> anything — handy for checking your JSON.

### 3b. Content — the `data/` folder

Everything under `data/` is served as-is.

- **`data/index.html`** is your home page (what the captive portal shows). Edit it.
- Add any pages: `data/about.html` → served at `/about.html`.
- A folder with an `index.html` works as a directory: `data/menu/index.html` → `/menu/`.
- **Games / apps:** drop self-contained `.html` files in `data/games/`. Any
  `.html` under a `games/` folder is counted in `stats` and the dashboard.
- **Keep it self-contained.** The box has no internet, so pages can't load
  anything from a CDN. Inline your CSS/JS and use emoji or inline SVG for art.

The FriendlyPortal example is a good template to copy from: it has a landing
page, a couple of docs pages, and four tiny original games.

---

## 4. Run & test locally

```bash
webhead run my-portal
```

You'll see the five services and their URLs. Then:

- **Browse** <http://localhost:8080> — your portal.
- **Watch it live** at <http://localhost:9090> — every web hit and DNS lookup.
- **SSH in:** `ssh guest@localhost -p 2222`. Try `help`, `top`, `tail`, `ls`,
  `stats`. Type `tail`, then reload a page in the browser and watch the hit
  stream in. `exit` to leave.

Flags override the manifest, e.g. `webhead run my-portal --extended`.

---

## 5. Add a real domain + free HTTPS cert

This makes `https://yourdomain` show a genuine padlock.

1. **Get a domain** at any registrar (Cloudflare Registrar, Namecheap, Porkbun…).
2. **Put its DNS on Cloudflare** (free plan). Create an API token scoped to
   *Edit DNS* for the zone.
3. **Issue a certificate** with [acme.sh](https://acme.sh) (DNS-01 — no open ports):

   ```bash
   curl https://get.acme.sh | sh
   export CF_Token="your-cloudflare-api-token"
   acme.sh --issue --dns dns_cf -d yourdomain.net -d '*.yourdomain.net' --keylength ec-256
   ```
4. **Install it into your image** (set `"domain": "yourdomain.net"` first):

   ```bash
   webhead cert refresh my-portal      # copies fullchain.pem + privkey.pem into my-portal/certs/
   webhead cert status  my-portal      # check the expiry
   ```
5. **Run with the domain mapped locally** (first time needs sudo to edit `/etc/hosts`
   and bind ports 80/443):

   ```bash
   sudo webhead run my-portal --setup-hosts
   ```
   Open `https://yourdomain.net` — clean padlock. 🎉

> Certificates last ~90 days. `acme.sh` auto-renews; the reinstall hook drops
> fresh files back into your image, so `webhead cert refresh` is your one command.

---

## 6. Flash it onto an ESP32

This puts your portal on a real pocket-sized board — a
[Seeed XIAO ESP32-S3](https://www.seeedstudio.com/XIAO-ESP32S3-p-5627.html) here.

### 6a. One-time tool setup (macOS shown; Linux is the same idea)

```bash
brew install arduino-cli esptool mklittlefs
arduino-cli config init
arduino-cli config add board_manager.additional_urls \
  https://raw.githubusercontent.com/espressif/arduino-esp32/gh-pages/package_esp32_index.json
arduino-cli core update-index
arduino-cli core install esp32:esp32
```

### 6b. Flash the firmware (the AP + DNS + web server)

The board needs firmware that runs the access point, DNS, and web server. (The
webhead binary is the *emulator*; on the board you run a small Arduino sketch —
see the project's `firmware/` for a starting point.) Compile and upload:

```bash
# plug the board in; find its port:
arduino-cli board list

arduino-cli compile --fqbn esp32:esp32:XIAO_ESP32S3 ./firmware
arduino-cli upload  --fqbn esp32:esp32:XIAO_ESP32S3 -p /dev/cu.usbmodemXXXX ./firmware
```

### 6c. Flash your website (the `data/` folder → LittleFS)

`webhead flash-board` builds a LittleFS image from `data/` and writes it. It
**plans first** (nothing is written) and figures out the flash offset for you:

```bash
webhead flash-board my-portal                    # prints the plan + fit check
webhead flash-board my-portal --confirm          # actually writes it
```

- It auto-detects the serial port, and reports whether your site **fits** the
  filesystem partition.
- The default (`default_8MB`) gives ~1.5 MB of space. If your site is bigger,
  use a larger partition — webhead looks up the offset for you:

  ```bash
  webhead flash-board my-portal --partition large_spiffs_8MB --confirm   # ~5.5 MB FS
  ```

  If you use `large_spiffs_8MB`, compile the firmware with the matching table so
  the board reads the file system at the right place:

  ```bash
  arduino-cli compile --fqbn esp32:esp32:XIAO_ESP32S3 \
    --build-property build.partitions=large_spiffs_8MB ./firmware
  ```

Reboot the board and join its Wi-Fi — your portal is live, no laptop needed.

---

## Troubleshooting

- **`ERR_NAME_NOT_RESOLVED` for your domain (local testing):** run once with
  `--setup-hosts` (needs sudo) so the domain maps to the box in `/etc/hosts`.
- **Browser cert warning on `localhost`:** expected — the cert is for your
  domain, not `localhost`. Browse the domain instead.
- **SSH "host key mismatch / possible MITM":** your client cached an old key.
  Clear it: `ssh-keygen -R '[localhost]:2222'` (GUI clients have their own
  known-hosts list to clear once). webhead keeps a stable key per image after that.
- **`payload is LARGER than the FS partition`:** your `data/` is bigger than the
  filesystem. Use `--partition large_spiffs_8MB`, or trim/compress content.
- **`webhead run` says a port is in use:** another copy is running
  (`pkill -f 'webhead run'`), or override the port, e.g. `--http :8081`.

---

Made something fun? The FriendlyPortal example is MIT-licensed — fork it, remix
it, and share your image folder so others can run your portal too.
