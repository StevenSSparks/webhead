#!/usr/bin/env bash
#
# webhead setup helper — build, clear any stale SSH host key, and run an image.
#
# usage:
#   ./setup.sh [image-dir]      run an image  (default: the embedded demo image)
#
# env overrides:
#   SUDO=""        run without sudo (skips /etc/hosts + privileged ports 80/443/53)
#   NO_HOSTS=1     don't pass --setup-hosts (don't touch /etc/hosts)
#
# examples:
#   ./setup.sh examples/friendlyportal-os          # FriendlyPortal demo
#   SUDO="" ./setup.sh examples/friendlyportal-os  # no sudo (unprivileged ports)
#
set -euo pipefail
cd "$(dirname "$0")"

IMAGE="${1:-}"
SUDO="${SUDO-sudo}"

echo "==> building webhead"
go build -o webhead .

# Discover the SSH port from the image manifest (fallback: 2222) so we clear the
# right known_hosts entry.
SSH_PORT=2222
if [ -n "$IMAGE" ] && [ -f "$IMAGE/webhead.json" ]; then
  p=$(grep -oE '"ssh"[^}]*' "$IMAGE/webhead.json" | grep -oE ':[0-9]+' | tr -d ':' | head -1 || true)
  [ -n "${p:-}" ] && SSH_PORT="$p"
fi

# Ensure a STABLE, user-owned SSH host key so the server key never changes
# between runs. Mixing sudo/non-sudo runs can leave <image>/.webhead owned by
# root (or empty), which makes webhead fall back to a fresh key each launch —
# the classic "host key mismatch / possible MITM" client error.
if [ -n "$IMAGE" ]; then
  KEYDIR="$IMAGE/.webhead"
  KEYFILE="$KEYDIR/ssh_host_key"
  if [ -e "$KEYDIR" ] && [ ! -O "$KEYDIR" ]; then
    echo "==> repairing ownership of $KEYDIR (was root-owned from a prior sudo run)"
    ${SUDO} chown -R "$(id -un)" "$KEYDIR" 2>/dev/null || true
  fi
  mkdir -p "$KEYDIR"
  if [ ! -f "$KEYFILE" ]; then
    echo "==> generating a stable SSH host key"
    ssh-keygen -t ed25519 -f "$KEYFILE" -N "" -q -C webhead
  fi
  echo "==> SSH host key fingerprint (stable across runs):"
  ssh-keygen -lf "$KEYFILE" 2>/dev/null | sed 's/^/    /'
fi

echo "==> clearing stale SSH host keys for localhost:$SSH_PORT (all clients)"
# OpenSSH plus every GUI-client known_hosts store (PounceTERM, etc.). These are
# standard known_hosts files, so ssh-keygen -R removes the entry from each.
KH_FILES=("$HOME/.ssh/known_hosts")
if [ -d "$HOME/Library/Application Support" ]; then
  while IFS= read -r f; do KH_FILES+=("$f"); done \
    < <(find "$HOME/Library/Application Support" -maxdepth 2 -name known_hosts 2>/dev/null)
fi
for kh in "${KH_FILES[@]}"; do
  [ -f "$kh" ] || continue
  if grep -q "\[localhost\]:$SSH_PORT" "$kh" 2>/dev/null; then
    ssh-keygen -R "[localhost]:$SSH_PORT" -f "$kh" >/dev/null 2>&1 && echo "    cleared: $kh"
  fi
done

echo "==> stopping any previous webhead instance"
${SUDO} pkill -f 'webhead run' 2>/dev/null || true
sleep 0.4

# Assemble the run command.
ARGS=(run)
[ -n "$IMAGE" ] && ARGS+=("$IMAGE")
[ -z "${NO_HOSTS:-}" ] && ARGS+=(--setup-hosts)

if [ -n "$SUDO" ]; then
  echo "==> starting webhead via sudo (needed for /etc/hosts + ports 80/443/53)"
else
  echo "==> starting webhead (no sudo; make sure the image uses unprivileged ports)"
fi
echo "    ./webhead ${ARGS[*]}"
echo
exec $SUDO ./webhead "${ARGS[@]}"
