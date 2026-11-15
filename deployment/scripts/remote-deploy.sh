#!/usr/bin/env bash
# Runs on the server. Backs up the current release, installs the new one from
# .incoming/, rebuilds the backend container, health-checks it and rolls back
# to the previous release if the check fails.
set -euo pipefail

VERSION="${1:-unknown}"
APP="${APP:-/root/myzhihu2}"
IN="$APP/.incoming"
BK="$APP/.previous"

log() { echo "[$(date '+%F %T')] $*"; }

# install_release DIR: DIR contains server, seed and html/
install_release() {
  install -m 755 "$1/server" "$APP/backend/server"
  install -m 755 "$1/seed" "$APP/backend/seed"
  mkdir -p "$APP/web/html"
  rsync -a --delete --checksum "$1/html/" "$APP/web/html/"
  chmod -R a+rX "$APP/web"
}

healthy() {
  for _ in $(seq 1 30); do
    if docker compose exec -T backend wget -qO- http://127.0.0.1:8080/api/v1/meta >/dev/null 2>&1; then
      return 0
    fi
    sleep "${HEALTH_INTERVAL:-2}"
  done
  return 1
}

cd "$APP"

for f in "$IN/server" "$IN/seed" "$IN/html/index.html"; do
  if [ ! -f "$f" ]; then
    log "missing $f, abort without touching the running release"
    exit 1
  fi
done

log "back up current release"
rm -rf "$BK"
mkdir -p "$BK"
cp -a backend/server backend/seed "$BK/"
cp -a web/html "$BK/html"

log "install release $VERSION"
install_release "$IN"
docker compose up -d --build backend

if healthy; then
  log "release $VERSION is healthy"
  echo "$VERSION $(date '+%F %T')" >> "$APP/releases.log"
  exit 0
fi

log "release $VERSION failed the health check, rolling back"
docker compose logs --tail=30 backend || true
install_release "$BK"
docker compose up -d --build backend
if healthy; then
  log "rolled back to the previous release"
else
  log "rollback is unhealthy too, manual action required"
fi
exit 1
