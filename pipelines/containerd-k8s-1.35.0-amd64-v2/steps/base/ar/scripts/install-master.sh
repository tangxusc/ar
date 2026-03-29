#!/bin/bash
set -euo pipefail

# ============================================================================
# install-master.sh - 在 master 节点上安装 K8s control plane
# ============================================================================
# 用法: sudo bash install-master.sh <node_ip> <etcd_ips> <labels> [is_first_master]
# 参数:
#   <node_ip>         本节点内网 IP
#   <etcd_ips>        逗号分隔的 etcd 节点内网 IP 列表
#   <labels>          本节点 labels，格式: key=value,key=value
#   [is_first_master] 可选，true 表示是第一个 master（负责应用 bootstrap RBAC）
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

NODE_IP="${1:?用法: $0 <node_ip> <etcd_ips> <labels> [is_first_master]}"
ETCD_IPS="${2:?缺少 etcd IP 列表}"
NODE_LABELS="${3:-}"
IS_FIRST_MASTER="${4:-false}"

echo "===== Master 节点安装开始 ====="
echo "节点 IP: ${NODE_IP}"
echo "etcd 节点: ${ETCD_IPS}"
echo "节点 Labels: ${NODE_LABELS}"
echo "是否首个 Master: ${IS_FIRST_MASTER}"

# --- 1. 解压 K8s 二进制文件 ---
echo "--- 步骤 1: 解压 K8s 二进制文件 ---"
K8S_TARBALL="/tmp/ar/tar/kubernetes-server-linux-amd64.tar.gz"
[[ -f "$K8S_TARBALL" ]] || die "K8s 包不存在: $K8S_TARBALL"

sudo tar -xf "$K8S_TARBALL" --strip-components=3 -C /usr/local/bin/ \
  kubernetes/server/bin/kube-apiserver \
  kubernetes/server/bin/kube-controller-manager \
  kubernetes/server/bin/kube-scheduler \
  kubernetes/server/bin/kubelet \
  kubernetes/server/bin/kube-proxy \
  kubernetes/server/bin/kubectl
sudo chmod +x /usr/local/bin/kube-apiserver \
  /usr/local/bin/kube-controller-manager \
  /usr/local/bin/kube-scheduler \
  /usr/local/bin/kubelet \
  /usr/local/bin/kube-proxy \
  /usr/local/bin/kubectl

echo "K8s 二进制版本:"
kube-apiserver --version || true
kubectl version --client || true

# --- 2. 部署证书 ---
echo "--- 步骤 2: 部署证书到 /etc/kubernetes/pki/ ---"
sudo mkdir -p /etc/kubernetes/pki
PKI_SRC="/tmp/ar/pki"

for f in ca.pem ca-key.pem apiserver.pem apiserver-key.pem \
         front-proxy-ca.pem front-proxy-ca-key.pem \
         front-proxy-client.pem front-proxy-client-key.pem \
         controller-manager.pem controller-manager-key.pem \
         scheduler.pem scheduler-key.pem \
         admin.pem admin-key.pem \
         kube-proxy.pem kube-proxy-key.pem \
         sa.key sa.pub; do
  sudo cp "${PKI_SRC}/${f}" /etc/kubernetes/pki/
done

echo "已部署证书:"
ls -l /etc/kubernetes/pki/

# --- 3. 部署 kubeconfig ---
echo "--- 步骤 3: 部署 kubeconfig ---"
KUBECONFIG_SRC="/tmp/ar/kubeconfig"
sudo cp "${KUBECONFIG_SRC}/admin.kubeconfig" /etc/kubernetes/admin.kubeconfig
sudo cp "${KUBECONFIG_SRC}/controller-manager.kubeconfig" /etc/kubernetes/controller-manager.kubeconfig
sudo cp "${KUBECONFIG_SRC}/scheduler.kubeconfig" /etc/kubernetes/scheduler.kubeconfig
sudo cp "${KUBECONFIG_SRC}/kube-proxy.kubeconfig" /etc/kubernetes/kube-proxy.kubeconfig
sudo cp "${KUBECONFIG_SRC}/bootstrap-kubelet.kubeconfig" /etc/kubernetes/bootstrap-kubelet.kubeconfig

# 设置 kubectl 默认 kubeconfig
sudo mkdir -p /root/.kube
sudo cp /etc/kubernetes/admin.kubeconfig /root/.kube/config

# --- 4. 构建 etcd 连接串 ---
echo "--- 步骤 4: 构建 etcd 连接串 ---"
ETCD_SERVERS=""
IFS=',' read -ra ETCD_ARRAY <<< "${ETCD_IPS}"
for ip in "${ETCD_ARRAY[@]}"; do
  if [ -n "$ETCD_SERVERS" ]; then
    ETCD_SERVERS="${ETCD_SERVERS},"
  fi
  ETCD_SERVERS="${ETCD_SERVERS}https://${ip}:2379"
done
echo "ETCD_SERVERS: ${ETCD_SERVERS}"

# --- 5. 安装 kube-apiserver ---
echo "--- 步骤 5: 安装 kube-apiserver ---"
sudo cp /tmp/ar/service/kube-apiserver.service /usr/lib/systemd/system/kube-apiserver.service
sudo sed -i "s|REPLACE_NODE_IP|${NODE_IP}|g" /usr/lib/systemd/system/kube-apiserver.service
sudo sed -i "s|REPLACE_ETCD_SERVERS|${ETCD_SERVERS}|g" /usr/lib/systemd/system/kube-apiserver.service

sudo systemctl daemon-reload
sudo systemctl enable kube-apiserver
sudo systemctl start kube-apiserver

echo "等待 apiserver 就绪..."
for i in $(seq 1 60); do
  if curl -k --max-time 5 https://127.0.0.1:6443/readyz >/dev/null 2>&1; then
    echo "apiserver 已就绪"
    break
  fi
  echo "等待中... (${i}/60)"
  sleep 5
done
sudo systemctl status kube-apiserver --no-pager || true

# --- 6. 安装 kube-controller-manager ---
echo "--- 步骤 6: 安装 kube-controller-manager ---"
sudo cp /tmp/ar/service/kube-controller-manager.service /usr/lib/systemd/system/kube-controller-manager.service
sudo systemctl daemon-reload
sudo systemctl enable kube-controller-manager
sudo systemctl start kube-controller-manager
sudo systemctl status kube-controller-manager --no-pager || true

# --- 7. 安装 kube-scheduler ---
echo "--- 步骤 7: 安装 kube-scheduler ---"
sudo cp /tmp/ar/service/kube-scheduler.service /usr/lib/systemd/system/kube-scheduler.service
sudo systemctl daemon-reload
sudo systemctl enable kube-scheduler
sudo systemctl start kube-scheduler
sudo systemctl status kube-scheduler --no-pager || true

# --- 8. 应用 bootstrap RBAC (仅第一个 master) ---
if [ "${IS_FIRST_MASTER}" = "true" ]; then
  echo "--- 步骤 8: 应用 bootstrap RBAC ---"
  for i in $(seq 1 10); do
    if sudo kubectl --kubeconfig=/etc/kubernetes/admin.kubeconfig apply -f /tmp/ar/bootstrap-secret.yaml; then
      echo "Bootstrap RBAC 已应用"
      break
    fi
    echo "apply 失败，1秒后重试... (${i}/10)"
    sleep 1
    if [ "${i}" -eq 10 ]; then
      die "Bootstrap RBAC 应用失败，已重试 10 次"
    fi
  done
fi

# --- 9. 安装 kubelet ---
echo "--- 步骤 9: 安装 kubelet ---"
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
echo "kubelet 已启动"

# --- 10. 安装 kube-proxy ---
echo "--- 步骤 10: 安装 kube-proxy ---"
sudo cp /tmp/ar/confs/kube-proxy.yaml /etc/kubernetes/kube-proxy.yaml
sudo sed -i "s|REPLACE_NODE_IP|${NODE_IP}|g" /etc/kubernetes/kube-proxy.yaml
sudo cp /tmp/ar/service/kube-proxy.service /usr/lib/systemd/system/kube-proxy.service

sudo systemctl daemon-reload
sudo systemctl enable kube-proxy
sudo systemctl start kube-proxy
echo "kube-proxy 已启动"

# --- 10.1. 配置 kube-ipvs0 接口自动启动 ---
echo "--- 步骤 10.1: 配置 kube-ipvs0 接口自动启动 ---"
sudo tee /etc/systemd/system/kube-ipvs0-up.service > /dev/null << 'EOF'
[Unit]
Description=Bring up kube-ipvs0 interface
After=kube-proxy.service
Requires=kube-proxy.service

[Service]
Type=oneshot
ExecStartPre=/bin/sleep 5
ExecStart=/usr/sbin/ip link set kube-ipvs0 up
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable kube-ipvs0-up.service
sudo systemctl start kube-ipvs0-up.service
echo "kube-ipvs0 接口自动启动已配置"

# --- 11. 等待节点注册并应用 labels ---
echo "--- 步骤 11: 等待节点注册 ---"
for i in $(seq 1 60); do
  if sudo kubectl --kubeconfig=/etc/kubernetes/admin.kubeconfig get node "${NODE_IP}" >/dev/null 2>&1; then
    echo "节点 ${NODE_IP} 已注册"
    break
  fi
  echo "等待节点注册... (${i}/60)"
  sleep 5
done

echo "===== Master 节点安装完成 ====="
sudo systemctl status kube-apiserver --no-pager || true
sudo systemctl status kube-controller-manager --no-pager || true
sudo systemctl status kube-scheduler --no-pager || true
sudo systemctl status kubelet --no-pager || true
sudo systemctl status kube-proxy --no-pager || true
sudo kubectl --kubeconfig=/etc/kubernetes/admin.kubeconfig get nodes || true
