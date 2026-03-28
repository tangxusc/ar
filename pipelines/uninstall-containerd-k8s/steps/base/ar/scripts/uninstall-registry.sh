#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-registry] $*"; }

if command -v nerdctl >/dev/null 2>&1; then
  sudo nerdctl -n k8s.io rm -f registry >/dev/null 2>&1 || true
fi

if command -v ctr >/dev/null 2>&1; then
  sudo ctr -n k8s.io task kill -s SIGKILL registry >/dev/null 2>&1 || true
  sudo ctr -n k8s.io task rm -f registry >/dev/null 2>&1 || true
  sudo ctr -n k8s.io container rm registry >/dev/null 2>&1 || true
fi

sudo rm -rf /etc/registry /var/lib/registry || true

log "registry uninstall completed"
