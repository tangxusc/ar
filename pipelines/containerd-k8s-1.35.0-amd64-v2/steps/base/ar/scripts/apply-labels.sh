#!/bin/bash
set -euo pipefail

# ============================================================================
# apply-labels.sh - 在 master 上为所有节点应用 labels
# ============================================================================
# 用法: bash apply-labels.sh <node_ip:labels> [node_ip:labels ...]
# 参数: 每个参数格式为 "内网IP:key=value,key=value"
# 例:   bash apply-labels.sh "172.19.16.11:role=master,etcd=true,env=dev" "172.19.16.13:role=worker,zone=shanghai"
# 环境变量: KUBECONFIG - kubeconfig 文件路径 (默认: /etc/kubernetes/admin.kubeconfig)
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

[[ $# -ge 1 ]] || die "用法: $0 <node_ip:labels> [node_ip:labels ...]"

# 使用环境变量 KUBECONFIG，如果未设置则使用默认值
KUBECONFIG="${KUBECONFIG:-/etc/kubernetes/admin.kubeconfig}"
[[ -f "$KUBECONFIG" ]] || die "admin kubeconfig 不存在: $KUBECONFIG"

echo "===== 应用节点 Labels ====="
echo "使用 KUBECONFIG: ${KUBECONFIG}"

# 测试 kubectl 连接
echo "测试 kubectl 连接..."
if ! kubectl --kubeconfig="$KUBECONFIG" version --short 2>&1; then
  echo "WARNING: kubectl 连接测试失败，但继续尝试..."
fi

# 等待所有节点注册
echo "等待所有节点注册..."
for arg in "$@"; do
  NODE_IP="${arg%%:*}"
  NODE_FOUND=false
  for i in $(seq 1 60); do
    if kubectl --kubeconfig="$KUBECONFIG" get node "$NODE_IP" >/dev/null 2>&1; then
      echo "节点 ${NODE_IP} 已注册"
      NODE_FOUND=true
      break
    fi
    if [ $i -eq 1 ] || [ $((i % 10)) -eq 0 ]; then
      echo "等待节点 ${NODE_IP} 注册... (${i}/60)"
      # 每10次显示一次详细错误
      kubectl --kubeconfig="$KUBECONFIG" get node "$NODE_IP" 2>&1 || true
    fi
    sleep 5
  done

  if [ "$NODE_FOUND" = false ]; then
    echo "WARNING: 节点 ${NODE_IP} 在 300 秒后仍未注册，尝试继续..."
    kubectl --kubeconfig="$KUBECONFIG" get nodes 2>&1 || true
  fi
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
    kubectl --kubeconfig="$KUBECONFIG" label node "$NODE_IP" "${key}=${value}" --overwrite || true
  done
done

echo "===== Labels 应用完成 ====="
kubectl --kubeconfig="$KUBECONFIG" get nodes --show-labels || true
