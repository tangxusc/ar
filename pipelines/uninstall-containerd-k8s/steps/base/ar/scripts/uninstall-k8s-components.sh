#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-k8s] $*"; }

stop_disable_service() {
  local svc="$1"
  if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl stop "$svc" >/dev/null 2>&1 || true
    sudo systemctl disable "$svc" >/dev/null 2>&1 || true
    sudo systemctl reset-failed "$svc" >/dev/null 2>&1 || true
  fi
}

remove_service_file() {
  local svc="$1"
  sudo rm -f "/etc/systemd/system/${svc}" "/usr/lib/systemd/system/${svc}" || true
}

for svc in \
  kube-apiserver.service \
  kube-controller-manager.service \
  kube-scheduler.service \
  kubelet.service \
  kube-proxy.service; do
  stop_disable_service "$svc"
  remove_service_file "$svc"
done

if command -v systemctl >/dev/null 2>&1; then
  sudo systemctl daemon-reload >/dev/null 2>&1 || true
fi

sudo rm -f \
  /usr/bin/kube-apiserver \
  /usr/bin/kube-controller-manager \
  /usr/bin/kube-scheduler \
  /usr/bin/kubelet \
  /usr/bin/kube-proxy \
  /usr/bin/kubectl \
  /usr/local/bin/kube-apiserver \
  /usr/local/bin/kube-controller-manager \
  /usr/local/bin/kube-scheduler \
  /usr/local/bin/kubelet \
  /usr/local/bin/kube-proxy \
  /usr/local/bin/kubectl || true

sudo rm -rf \
  /etc/kubernetes \
  /var/lib/kubelet \
  /var/lib/kube-proxy \
  /var/lib/kubernetes \
  /etc/cni/net.d/* \
  /root/.kube || true

log "k8s components uninstall completed"
