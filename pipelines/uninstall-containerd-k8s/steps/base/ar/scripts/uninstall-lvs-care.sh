#!/usr/bin/env bash
set -euo pipefail

log() { echo "[uninstall-lvscare] $*"; }
VIP_HOST="${1:-10.103.97.12}"

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

log "cleanup SNAT rules for VIP ${VIP_HOST} ..."
while read -r line; do
  rule="${line#-A }"
  if [[ -z "${rule}" ]]; then
    continue
  fi
  sudo iptables -t nat -D ${rule} >/dev/null 2>&1 || true
done < <(
  sudo iptables -t nat -S POSTROUTING | grep -- "-s ${VIP_HOST}/32" | grep -- "-p tcp" | grep -- "--dport 6443" | grep -- "-j MASQUERADE" || true
)

log "lvs-care uninstall completed"
