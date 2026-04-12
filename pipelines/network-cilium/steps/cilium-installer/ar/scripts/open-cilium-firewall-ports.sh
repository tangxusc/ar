#!/bin/bash
set -euo pipefail

# ============================================================================
# open-cilium-firewall-ports.sh - 为 Cilium 网络插件打开所需防火墙端口
# ============================================================================
# 用法: sudo bash open-cilium-firewall-ports.sh
# ============================================================================
# 需要开放的端口:
#   4240/tcp  - cilium-health 节点健康检查
#   4244/tcp  - Hubble server (gRPC)
#   4245/tcp  - Hubble Relay (gRPC)
#   9962/tcp  - Cilium agent Prometheus 指标
#   9963/tcp  - Cilium operator Prometheus 指标
#   9964/tcp  - Cilium Envoy proxy Prometheus 指标
#   8472/udp  - VXLAN overlay 网络流量（默认封装模式）
# ============================================================================
# 支持的防火墙类型（按优先级检测）:
#   1. firewalld (CentOS/RHEL 系)
#   2. ufw (Ubuntu/Debian 系)
#   3. iptables (通用回退)
#   4. 无防火墙 → 跳过，打印警告
# ============================================================================

echo "===== Cilium 防火墙端口配置开始 ====="

# Cilium 所需 TCP 端口
CILIUM_TCP_PORTS=(
  4240   # cilium-health 健康检查
  4244   # Hubble server
  4245   # Hubble Relay
  9962   # Cilium agent Prometheus 指标
  9963   # Cilium operator Prometheus 指标
  9964   # Cilium Envoy proxy Prometheus 指标
)

# Cilium 所需 UDP 端口
CILIUM_UDP_PORTS=(
  8472   # VXLAN overlay 网络流量
)

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

  for port in "${CILIUM_TCP_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp"
    firewall-cmd --zone="${zone}" --add-port="${port}/tcp" --permanent 2>/dev/null || true
  done

  for port in "${CILIUM_UDP_PORTS[@]}"; do
    echo "开放端口 ${port}/udp"
    firewall-cmd --zone="${zone}" --add-port="${port}/udp" --permanent 2>/dev/null || true
  done

  firewall-cmd --reload
  echo "firewalld 端口已配置:"
  firewall-cmd --zone="${zone}" --list-ports
}

# --- ufw 实现 ---
open_ufw() {
  for port in "${CILIUM_TCP_PORTS[@]}"; do
    echo "开放端口 ${port}/tcp"
    ufw allow "${port}/tcp" 2>/dev/null || true
  done

  for port in "${CILIUM_UDP_PORTS[@]}"; do
    echo "开放端口 ${port}/udp"
    ufw allow "${port}/udp" 2>/dev/null || true
  done

  echo "ufw 端口已配置:"
  ufw status verbose
}

# --- iptables 实现 ---
open_iptables() {
  for port in "${CILIUM_TCP_PORTS[@]}"; do
    if ! iptables -C INPUT -p tcp --dport "${port}" -j ACCEPT 2>/dev/null; then
      echo "开放端口 ${port}/tcp"
      iptables -I INPUT -p tcp --dport "${port}" -j ACCEPT
    fi
  done

  for port in "${CILIUM_UDP_PORTS[@]}"; do
    if ! iptables -C INPUT -p udp --dport "${port}" -j ACCEPT 2>/dev/null; then
      echo "开放端口 ${port}/udp"
      iptables -I INPUT -p udp --dport "${port}" -j ACCEPT
    fi
  done

  echo "iptables 端口已配置:"
  iptables -L INPUT -n --line-numbers | head -30

  # 尝试持久化 iptables 规则
  if command -v iptables-save >/dev/null 2>&1; then
    # Debian/Ubuntu: iptables-persistent
    if command -v netfilter-persistent >/dev/null 2>&1; then
      netfilter-persistent save 2>/dev/null || true
    fi
    # CentOS/RHEL: iptables-services
    if [ -f /etc/sysconfig/iptables ]; then
      iptables-save | tee /etc/sysconfig/iptables >/dev/null 2>&1 || true
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

echo "===== Cilium 防火墙端口配置完成 ====="
