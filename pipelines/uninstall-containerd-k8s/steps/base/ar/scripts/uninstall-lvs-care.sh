#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-lvscare] $*"; }

if command -v nerdctl >/dev/null 2>&1; then
  sudo nerdctl -n k8s.io rm -f lvscare >/dev/null 2>&1 || true
fi

if command -v ctr >/dev/null 2>&1; then
  sudo ctr -n k8s.io task kill -s SIGKILL lvscare >/dev/null 2>&1 || true
  sudo ctr -n k8s.io task rm -f lvscare >/dev/null 2>&1 || true
  sudo ctr -n k8s.io container rm lvscare >/dev/null 2>&1 || true
fi

if ip link show lvscare >/dev/null 2>&1; then
  sudo ip link delete lvscare >/dev/null 2>&1 || true
fi

log "lvs-care uninstall completed"
