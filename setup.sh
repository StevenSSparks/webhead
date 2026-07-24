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
#   ./setup.sh ~/dev/spiderverse-os          # Spider-Verse OS on wififun.net
#   SUDO="" ./setup.sh ~/dev/spiderverse-os  # no sudo (use high ports in the image)
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

echo "==> clearing any stale SSH host key for localhost:$SSH_PORT"
ssh-keygen -R "[localhost]:$SSH_PORT" >/dev/null 2>&1 || true

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
