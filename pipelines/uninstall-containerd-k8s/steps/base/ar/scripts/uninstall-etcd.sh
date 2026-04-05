#!/usr/bin/env bash
set -euo pipefail

PURGE_ETCD_DATA="${1:-false}"

log() { echo "[uninstall-etcd] $*"; }

is_true() {
  case "${1,,}" in
    1|true|yes|y|on) return 0 ;;
    *) return 1 ;;
  esac
}

if command -v systemctl >/dev/null 2>&1; then
  sudo systemctl stop etcd >/dev/null 2>&1 || true
  sudo systemctl disable etcd >/dev/null 2>&1 || true
  sudo systemctl reset-failed etcd >/dev/null 2>&1 || true
fi

sudo rm -f /etc/systemd/system/etcd.service /usr/lib/systemd/system/etcd.service || true
if command -v systemctl >/dev/null 2>&1; then
  sudo systemctl daemon-reload >/dev/null 2>&1 || true
fi

sudo rm -f /usr/bin/etcd /usr/bin/etcdctl /usr/local/bin/etcd /usr/local/bin/etcdctl || true
sudo rm -rf /etc/etcd /etc/kubernetes/pki/etcd || true

if is_true "$PURGE_ETCD_DATA"; then
  sudo rm -rf /var/lib/etcd || true
  log "etcd data directory removed"
else
  log "etcd data directory preserved: /var/lib/etcd"
fi

log "etcd uninstall completed"
