#!/usr/bin/env bash
# Deploy lastmile-lab ke mode demo produksi (profile `sim` + `chaos`).
#
# Dua cara pakai:
#   1. Di VPS langsung:   ./scripts/deploy.sh
#   2. Dari mesin lain:   DEPLOY_SSH_HOST=user@vps ./scripts/deploy.sh
#      (SSH key harus sudah ada di agent / ~/.ssh — dipakai GitHub Actions
#       job `deploy` dengan secret DEPLOY_SSH_KEY, deploy/README.md §4.)
#
# Urutan: cek load host → pull image GHCR → up -d → verifikasi /healthz.
# Build TIDAK pernah dijalankan di sini (aturan keras: build = GitHub Actions).

set -euo pipefail

REPO_DIR="${DEPLOY_REPO_DIR:-/home/rico/portfolio/lastmile-lab}"
LOAD_MAX="${DEPLOY_LOAD_MAX:-8}"

run() {
  if [ -n "${DEPLOY_SSH_HOST:-}" ]; then
    ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "$DEPLOY_SSH_HOST" "$1"
  else
    bash -c "$1"
  fi
}

echo "==> [1/4] Cek load host (batas ${LOAD_MAX})"
load="$(run "cat /proc/loadavg")"
load1="$(printf '%s' "$load" | awk '{print $1}')"
echo "    loadavg: ${load}"
awk -v l="$load1" -v m="$LOAD_MAX" 'BEGIN { exit !(l <= m) }' || {
  echo "!!  Load ${load1} > ${LOAD_MAX} — deploy ditunda (deploy/README.md §7)."; exit 1;
}

echo "==> [2/4] Pull image GHCR (tag: \${LASTMILE_IMAGE_TAG:-latest})"
run "cd '${REPO_DIR}' && docker compose -p lastmile --profile sim --profile chaos pull --quiet"

echo "==> [3/4] up -d (profile sim + chaos — mode demo aman 24/7)"
run "cd '${REPO_DIR}' && docker compose -p lastmile --profile sim --profile chaos up -d"

echo "==> [4/4] Verifikasi /healthz (domain publik dulu, fallback gateway/loopback)"
if run "curl -fsS -m 8 https://api.lastmile-lab.ricothen.com/healthz"; then
  echo ""
elif run "curl -fsS -m 8 http://172.19.0.1:3010/healthz"; then
  echo ""
elif run "curl -fsS -m 8 http://127.0.0.1:3010/healthz"; then
  echo ""
else
  echo "!!  /healthz gagal — periksa: docker compose -p lastmile ps && docker compose -p lastmile logs --tail=50 api-gateway"
  exit 1
fi

run "cd '${REPO_DIR}' && docker compose -p lastmile ps --format '{{.Name}}\t{{.Status}}'"
echo "==> Deploy selesai."
