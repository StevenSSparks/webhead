# FriendlyPortal (example image)

A complete, working **roost** image you can copy and make your own. It's also
the binary's built-in default, so `roost run` (no args) boots it.

```bash
cp -R examples/friendlyportal my-portal
roost run my-portal
```

## What's inside

```
friendlyportal/
  roost.json              settings (name, ssid, domain, ports, motd, ssh login)
  data/
    index.html              the landing page (what the captive portal shows)
    how-it-works.html       explains the captive portal / DNS / HTTPS / SSH
    setup-guide.html        get-a-domain + cert walkthrough
    games/                  four tiny, self-contained games
```

## Make it yours (2 minutes)

1. **Rename it.** In `roost.json` change `name`, `ssid`, `hostname`, `prompt`,
   and `motd`. Set `domain` to your own if you have one.
2. **Change the home page.** Edit `data/index.html` — the text, the cards, the
   links. It's plain HTML/CSS, no build step, no dependencies.
3. **Add a page.** Drop `data/whatever.html`; it's served at `/whatever.html`.
   A folder with `index.html` becomes a directory (`/menu/`).
4. **Add a game.** Put a self-contained `.html` in `data/games/` and add a tile
   to the grid in `index.html`. Anything under `games/` is counted in `stats`.
5. **Test.** `roost run my-portal`, browse <http://localhost:8080>, and
   `ssh guest@localhost -p 2222`.

Keep pages **self-contained** — the box has no internet, so inline your CSS/JS
and use emoji or inline SVG instead of loading images from the web.

Full walkthrough (domain, HTTPS cert, flashing to an ESP32):
[`../../docs/MAKE-YOUR-OWN.md`](../../docs/MAKE-YOUR-OWN.md).
