#!/bin/bash
set -e

sudo mkdir -p /etc/cni/net.d /opt/cni/bin &&sudo tar xf /tmp/ar/tar/cni-plugins-linux-amd64-v*.tgz -C /opt/cni/bin/ 
sudo tar -xzf /tmp/ar/tar/containerd-*-linux-amd64.tar.gz -C /usr/local/
# 安装runc
if [ -f /tmp/ar/tar/runc.amd64 ]; then
  sudo install -m 0755 /tmp/ar/tar/runc.amd64 /usr/local/sbin/runc
  echo "Installed runc: $(/usr/local/sbin/runc --version || true)"
elif ! command -v runc >/dev/null 2>&1; then
  echo "WARNING: runc binary not found, containerd may not work correctly"
fi
sudo cp /tmp/ar/service/containerd.service /etc/systemd/system/containerd.service 
sudo mkdir -p /etc/containerd
sudo containerd config default | sudo tee /etc/containerd/config.toml
sudo mkdir -p /etc/containerd/certs.d/_default && sudo cp /tmp/ar/confs/hosts.toml /etc/containerd/certs.d/_default/hosts.toml

cat <<EOF | sudo tee /etc/modules-load.d/containerd.conf
overlay
br_netfilter
EOF
sudo systemctl restart systemd-modules-load.service

cat <<EOF | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF
sudo sysctl -p

sudo systemctl daemon-reload 
sudo systemctl enable --now containerd
sudo systemctl restart containerd
sudo systemctl status containerd

sudo tar xf /tmp/ar/tar/crictl-v*-linux-amd64.tar.gz -C /usr/bin/
#生成配置文件
sudo cat > /etc/crictl.yaml <<EOF
runtime-endpoint: unix:///run/containerd/containerd.sock
image-endpoint: unix:///run/containerd/containerd.sock
timeout: 10
debug: false
EOF

sudo crictl info