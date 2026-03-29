#!/bin/bash
set -euo pipefail

# ============================================================================
# install-node.sh - 在 worker 节点上安装 K8s node 组件
# ============================================================================
# 用法: sudo bash install-node.sh <node_ip> <labels>
# 参数:
#   <node_ip>   本节点内网 IP
#   <labels>    本节点 labels，格式: key=value,key=value
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

NODE_IP="${1:?用法: $0 <node_ip> <labels>}"
NODE_LABELS="${2:-}"

echo "===== Worker 节点安装开始 ====="
echo "节点 IP: ${NODE_IP}"
echo "节点 Labels: ${NODE_LABELS}"

# --- 1. 解压 K8s 二进制文件 ---
echo "--- 步骤 1: 解压 K8s 二进制文件 ---"
K8S_TARBALL="/tmp/ar/tar/kubernetes-server-linux-amd64.tar.gz"
[[ -f "$K8S_TARBALL" ]] || die "K8s 包不存在: $K8S_TARBALL"

sudo tar -xf "$K8S_TARBALL" --strip-components=3 -C /usr/local/bin/ \
  kubernetes/server/bin/kubelet \
  kubernetes/server/bin/kube-proxy \
  kubernetes/server/bin/kubectl
sudo chmod +x /usr/local/bin/kubelet /usr/local/bin/kube-proxy /usr/local/bin/kubectl

# --- 2. 部署证书 ---
echo "--- 步骤 2: 部署证书 ---"
sudo mkdir -p /etc/kubernetes/pki
PKI_SRC="/tmp/ar/pki"

sudo cp "${PKI_SRC}/ca.pem" /etc/kubernetes/pki/
sudo cp "${PKI_SRC}/front-proxy-ca.pem" /etc/kubernetes/pki/
sudo cp "${PKI_SRC}/kube-proxy.pem" /etc/kubernetes/pki/
sudo cp "${PKI_SRC}/kube-proxy-key.pem" /etc/kubernetes/pki/

# --- 3. 部署 kubeconfig ---
echo "--- 步骤 3: 部署 kubeconfig ---"
KUBECONFIG_SRC="/tmp/ar/kubeconfig"
sudo cp "${KUBECONFIG_SRC}/bootstrap-kubelet.kubeconfig" /etc/kubernetes/bootstrap-kubelet.kubeconfig
sudo cp "${KUBECONFIG_SRC}/kube-proxy.kubeconfig" /etc/kubernetes/kube-proxy.kubeconfig

# --- 4. 安装 kubelet ---
echo "--- 步骤 4: 安装 kubelet ---"
sudo mkdir -p /etc/kubernetes/manifests /var/lib/kubelet

sudo cp /tmp/ar/confs/kubelet-conf.yml /etc/kubernetes/kubelet-conf.yml

KUBELET_RESOLV_CONF="/etc/resolv.conf"
if systemctl is-active --quiet systemd-resolved; then
  RESOLV_REALPATH="$(readlink -f /etc/resolv.conf 2>/dev/null || true)"
  if [[ "${RESOLV_REALPATH}" == *"/run/systemd/resolve/stub-resolv.conf"* ]] || grep -q '127\.0\.0\.53' /etc/resolv.conf 2>/dev/null; then
    KUBELET_RESOLV_CONF="/run/systemd/resolve/resolv.conf"
  fi
fi

sudo sed -i "s|^resolvConf: .*|resolvConf: ${KUBELET_RESOLV_CONF}|" /etc/kubernetes/kubelet-conf.yml
echo "kubelet resolvConf: ${KUBELET_RESOLV_CONF}"

sudo cp /tmp/ar/service/kubelet.service /usr/lib/systemd/system/kubelet.service
sudo sed -i "s|REPLACE_NODE_IP|${NODE_IP}|g" /usr/lib/systemd/system/kubelet.service

sudo systemctl daemon-reload
sudo systemctl enable kubelet
sudo systemctl start kubelet
echo "kubelet 已启动，等待 bootstrap 完成..."

# --- 5. 安装 kube-proxy ---
echo "--- 步骤 5: 安装 kube-proxy ---"
sudo cp /tmp/ar/confs/kube-proxy.yaml /etc/kubernetes/kube-proxy.yaml
sudo sed -i "s|REPLACE_NODE_IP|${NODE_IP}|g" /etc/kubernetes/kube-proxy.yaml
sudo cp /tmp/ar/service/kube-proxy.service /usr/lib/systemd/system/kube-proxy.service

sudo systemctl daemon-reload
sudo systemctl enable kube-proxy
sudo systemctl start kube-proxy
echo "kube-proxy 已启动"

echo "===== Worker 节点安装完成 ====="
sudo systemctl status kubelet --no-pager || true
sudo systemctl status kube-proxy --no-pager || true
