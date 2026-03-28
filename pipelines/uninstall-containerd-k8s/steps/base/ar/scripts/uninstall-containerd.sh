#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-containerd] $*"; }

if command -v systemctl >/dev/null 2>&1; then
  sudo systemctl stop containerd >/dev/null 2>&1 || true
  sudo systemctl disable containerd >/dev/null 2>&1 || true
  sudo systemctl reset-failed containerd >/dev/null 2>&1 || true
fi

sudo pkill -9 -x containerd >/dev/null 2>&1 || true
sudo pkill -9 -f containerd-shim >/dev/null 2>&1 || true

sudo rm -f /etc/systemd/system/containerd.service /usr/lib/systemd/system/containerd.service || true
if command -v systemctl >/dev/null 2>&1; then
  sudo systemctl daemon-reload >/dev/null 2>&1 || true
fi

sudo rm -f \
  /usr/local/bin/containerd \
  /usr/local/bin/containerd-stress \
  /usr/local/bin/containerd-shim \
  /usr/local/bin/containerd-shim-runc-v1 \
  /usr/local/bin/containerd-shim-runc-v2 \
  /usr/local/bin/ctr \
  /usr/local/bin/nerdctl \
  /usr/local/sbin/runc \
  /usr/bin/crictl || true

sudo rm -f /etc/crictl.yaml || true
sudo rm -rf \
  /etc/containerd \
  /var/lib/containerd \
  /run/containerd \
  /opt/cni/bin \
  /etc/cni/net.d \
  /var/lib/cni \
  /var/lib/calico || true

log "containerd uninstall completed"
