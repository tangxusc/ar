#!/bin/bash
set -euo pipefail

# ============================================================================
# apply-labels.sh - 在 master 上为所有节点应用 labels
# ============================================================================
# 用法: sudo bash apply-labels.sh <node_ip:labels> [node_ip:labels ...]
# 参数: 每个参数格式为 "内网IP:key=value,key=value"
# 例:   sudo bash apply-labels.sh "172.19.16.11:role=master,etcd=true,env=dev" "172.19.16.13:role=worker,zone=shanghai"
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

[[ $# -ge 1 ]] || die "用法: $0 <node_ip:labels> [node_ip:labels ...]"

KUBECONFIG="/etc/kubernetes/admin.kubeconfig"
[[ -f "$KUBECONFIG" ]] || die "admin kubeconfig 不存在: $KUBECONFIG"

echo "===== 应用节点 Labels ====="

# 等待所有节点注册
echo "等待所有节点注册..."
for arg in "$@"; do
  NODE_IP="${arg%%:*}"
  for i in $(seq 1 60); do
    if sudo kubectl --kubeconfig="$KUBECONFIG" get node "$NODE_IP" >/dev/null 2>&1; then
      echo "节点 ${NODE_IP} 已注册"
      break
    fi
    echo "等待节点 ${NODE_IP} 注册... (${i}/60)"
    sleep 5
  done
done

# 应用 labels
for arg in "$@"; do
  NODE_IP="${arg%%:*}"
  LABELS="${arg#*:}"

  if [ -z "$LABELS" ] || [ "$LABELS" = "$arg" ]; then
    echo "节点 ${NODE_IP} 无 labels，跳过"
    continue
  fi

  echo "为节点 ${NODE_IP} 应用 labels: ${LABELS}"
  IFS=',' read -ra LABEL_PAIRS <<< "${LABELS}"
  for pair in "${LABEL_PAIRS[@]}"; do
    key="${pair%%=*}"
    value="${pair#*=}"
    echo "  应用: ${key}=${value}"
    sudo kubectl --kubeconfig="$KUBECONFIG" label node "$NODE_IP" "${key}=${value}" --overwrite || true
  done
done

echo "===== Labels 应用完成 ====="
sudo kubectl --kubeconfig="$KUBECONFIG" get nodes --show-labels || true
