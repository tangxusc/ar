#!/bin/bash
set -euo pipefail

# ============================================================================
# open-firewall-ports.sh - 为 K8s 集群打开所需防火墙端口
# ============================================================================
# 用法: sudo bash open-firewall-ports.sh <labels>
# 参数:
#   <labels>  本节点 labels，格式: key=value,key=value
#             用于判断是否需要开放 etcd/registry 等额外端口
# ============================================================================
# 支持的防火墙类型（按优先级检测）:
#   1. firewalld (CentOS/RHEL 系)
#   2. ufw (Ubuntu/Debian 系)
#   3. iptables (通用回退)
#   4. 无防火墙 → 跳过，打印警告
# ============================================================================

NODE_LABELS="${1:-}"

echo "===== 防火墙端口配置开始 ====="
echo "节点 Labels: ${NODE_LABELS}"

# --- Helper: 判断 label 是否包含指定 key=value ---
label_has() {
  local labels="$1" key="$2" value="$3"
  echo "${labels}" | tr ',' '\n' | grep -qx "${key}=${value}"
}

# --- 定义端口列表 ---

# 所有节点必须开放的 TCP 端口
BASE_TCP_PORTS=(
  6443    # kube-apiserver
  10250   # kubelet
  10255   # kubelet read-only
  10248   # kubelet health
  10249   # kube-proxy metrics
  10256   # kube-proxy health
  8443    # nginx apiserver proxy
)

# NodePort 范围
NODEPORT_RANGE="30000-32767"

# 按角色追加的 TCP 端口
EXTRA_TCP_PORTS=()

if label_has "${NODE_LABELS}" "etcd" "true"; then
  echo "检测到 etcd 节点，追加 etcd 端口 (2379, 2380)"
  EXTRA_TCP_PORTS+=(2379 2380)
fi

if label_has "${NODE_LABELS}" "registry" "true"; then
  echo "检测到 registry 节点，追加 registry 端口 (5000)"
  EXTRA_TCP_PORTS+=(5000)
fi

if label_has "${NODE_LABELS}" "role" "master"; then
  echo "检测到 master 节点，追加 controller-manager/scheduler 端口 (10257, 10259)"
  EXTRA_TCP_PORTS+=(10257 10259)
fi

# DNS 端口 (需要同时开放 TCP 和 UDP)
DNS_PORTS=(53 9153)

# --- 检测防火墙类型 ---
detect_firewall() {
  if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld 2>/dev/null; then
    echo "firewalld"
  elif command -v ufw >/dev/null 2>&1 && ufw status 2>/dev/null | grep -q "Status: active"; then
    echo "ufw"
  elif command -v iptables >/dev/null 2>&1; then
    echo "iptables"
  else
    echo "none"
  fi
}

FW_TYPE="$(detect_firewall)"
echo "检测到防火墙类型: ${FW_TYPE}"

# --- firewalld 实现 ---
open_firewalld() {
  local zone
  zone="$(firewall-cmd --get-default-zone 2>/dev/null || echo public)"
  echo "firewalld 默认 zone: ${zone}"

  for port in "${BASE_TCP_PORTS[@]}" "${EXTRA_TCP_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp"
    sudo firewall-cmd --zone="${zone}" --add-port="${port}/tcp" --permanent 2>/dev/null || true
  done

  # NodePort 范围
  echo "开放端口范围 ${NODEPORT_RANGE}/tcp"
  sudo firewall-cmd --zone="${zone}" --add-port="${NODEPORT_RANGE}/tcp" --permanent 2>/dev/null || true

  # DNS 端口 (tcp+udp)
  for port in "${DNS_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp+udp"
    sudo firewall-cmd --zone="${zone}" --add-port="${port}/tcp" --permanent 2>/dev/null || true
    sudo firewall-cmd --zone="${zone}" --add-port="${port}/udp" --permanent 2>/dev/null || true
  done

  sudo firewall-cmd --reload
  echo "firewalld 端口已配置:"
  sudo firewall-cmd --zone="${zone}" --list-ports
}

# --- ufw 实现 ---
open_ufw() {
  for port in "${BASE_TCP_PORTS[@]}" "${EXTRA_TCP_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp"
    sudo ufw allow "${port}/tcp" 2>/dev/null || true
  done

  # NodePort 范围
  echo "开放端口范围 ${NODEPORT_RANGE}/tcp"
  sudo ufw allow "${NODEPORT_RANGE}/tcp" 2>/dev/null || true

  # DNS 端口 (tcp+udp)
  for port in "${DNS_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp+udp"
    sudo ufw allow "${port}/tcp" 2>/dev/null || true
    sudo ufw allow "${port}/udp" 2>/dev/null || true
  done

  echo "ufw 端口已配置:"
  sudo ufw status verbose
}

# --- iptables 实现 ---
open_iptables() {
  # 仅添加 ACCEPT 规则到 INPUT 链，不 flush 现有规则
  for port in "${BASE_TCP_PORTS[@]}" "${EXTRA_TCP_PORTS[@]}"; do
    if ! sudo iptables -C INPUT -p tcp --dport "${port}" -j ACCEPT 2>/dev/null; then
      echo "开放端口 ${port}/tcp"
      sudo iptables -I INPUT -p tcp --dport "${port}" -j ACCEPT
    fi
  done

  # NodePort 范围
  if ! sudo iptables -C INPUT -p tcp -m multiport --dports 30000:32767 -j ACCEPT 2>/dev/null; then
    echo "开放端口范围 30000:32767/tcp"
    sudo iptables -I INPUT -p tcp -m multiport --dports 30000:32767 -j ACCEPT
  fi

  # DNS 端口 (tcp+udp)
  for port in "${DNS_PORTS[@]}"; do
    if ! sudo iptables -C INPUT -p tcp --dport "${port}" -j ACCEPT 2>/dev/null; then
      echo "开放端口 ${port}/tcp"
      sudo iptables -I INPUT -p tcp --dport "${port}" -j ACCEPT
    fi
    if ! sudo iptables -C INPUT -p udp --dport "${port}" -j ACCEPT 2>/dev/null; then
      echo "开放端口 ${port}/udp"
      sudo iptables -I INPUT -p udp --dport "${port}" -j ACCEPT
    fi
  done

  echo "iptables 端口已配置:"
  sudo iptables -L INPUT -n --line-numbers | head -30

  # 尝试持久化 iptables 规则
  if command -v iptables-save >/dev/null 2>&1; then
    # Debian/Ubuntu: iptables-persistent
    if command -v netfilter-persistent >/dev/null 2>&1; then
      sudo netfilter-persistent save 2>/dev/null || true
    fi
    # CentOS/RHEL: iptables-services
    if [ -f /etc/sysconfig/iptables ]; then
      sudo iptables-save | sudo tee /etc/sysconfig/iptables >/dev/null 2>&1 || true
    fi
  fi
}

# --- 执行 ---
case "${FW_TYPE}" in
  firewalld)
    open_firewalld
    ;;
  ufw)
    open_ufw
    ;;
  iptables)
    open_iptables
    ;;
  none)
    echo "WARNING: 未检测到活跃的防火墙 (firewalld/ufw/iptables)，跳过端口配置"
    echo "如果系统无防火墙，所有端口默认已开放"
    ;;
esac

echo "===== 防火墙端口配置完成 ====="
