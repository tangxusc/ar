#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-nginx-proxy] $*"; }

# 清理 nginx apiserver 反向代理容器
if command -v nerdctl >/dev/null 2>&1; then
  sudo nerdctl -n k8s.io rm -f nginx-apiserver-proxy >/dev/null 2>&1 || true
fi

if command -v ctr >/dev/null 2>&1; then
  sudo ctr -n k8s.io task kill -s SIGKILL nginx-apiserver-proxy >/dev/null 2>&1 || true
  sudo ctr -n k8s.io task rm -f nginx-apiserver-proxy >/dev/null 2>&1 || true
  sudo ctr -n k8s.io container rm nginx-apiserver-proxy >/dev/null 2>&1 || true
fi

sudo rm -rf /etc/nginx-apiserver-proxy >/dev/null 2>&1 || true

for vip in 169.254.1.5; do
  while read -r line; do
    rule="${line#-A }"
    [[ -n "${rule}" ]] || continue
    sudo iptables -t nat -D ${rule} >/dev/null 2>&1 || true
  done < <(
    sudo iptables -t nat -S POSTROUTING | grep -- "-s ${vip}/32" | grep -- "-p tcp" | grep -- "--dport 6443" | grep -- "-j MASQUERADE" || true
  )
done

log "nginx proxy uninstall completed"
