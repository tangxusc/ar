#!/bin/bash
set -e

# ============================================================================
# install-etcd.sh - 安装并启动 etcd 服务
# ============================================================================
#
# 用法:
#   sudo bash install-etcd.sh <node-ip>
#
# 参数:
#   <node-ip>      必填, 本节点的 IP 地址, 用于定位 /tmp/ar/<node-ip>/etcd.config.yml
#
# 前置条件:
#   1. gen-ca-pem.sh 已执行, 生成 CA 证书
#   2. gen-etcd-config.sh 已执行, 生成 etcd 证书、hosts 文件和各节点配置
#   3. copy-etcd-certs 步骤已将容器内 /ar/ 目录 rsync 到目标主机 /tmp/ar/
#
# 调用示例 (pipeline 中通过 ssh 远程执行):
#   sshpass -e ssh user@<node-ip> "sudo sh /tmp/ar/scripts/install-etcd.sh <node-ip>"
#
# 执行步骤:
#   1. 解压 etcd 二进制文件到 /usr/local/bin/
#   2. 将 /tmp/ar/hosts 中的 etcd 节点写入 /etc/hosts (幂等, 不重复写入)
#   3. 部署 etcd TLS 证书到 /etc/kubernetes/pki/etcd/
#   4. 部署本节点 etcd 配置到 /etc/etcd/etcd.config.yml
#   5. 创建 etcd 数据目录 /var/lib/etcd/
#   6. 安装并启动 etcd systemd 服务
#
# 目标主机上 /tmp/ar/ 目录结构 (由 rsync 从容器 /ar/ 同步而来):
#
# /tmp/ar/
# ├── confs/
# │   ├── containerd.conf          # containerd 配置
# │   ├── etcd.config.yml          # etcd 配置模板 (渲染前的原始模板)
# │   ├── hosts.toml               # containerd registry hosts 配置
# │   ├── ipvs.conf                # IPVS 内核模块配置
# │   ├── k8s.conf                 # K8s 内核参数配置
# │   └── limits.conf              # ulimit 配置
# ├── hosts                        # gen-etcd-config.sh 生成的 IP→节点名映射
# ├── pki/                         # gen-etcd-config.sh 生成的证书 (由 CA 签发)
# │   ├── ca.pem                   # CA 证书 → 部署为 etcd-ca.pem
# │   ├── etcd.pem                 # etcd 服务端证书
# │   ├── etcd-key.pem             # etcd 服务端私钥
# │   └── etcd.csr                 # etcd 证书签名请求
# ├── scripts/                     # 各安装脚本
# ├── service/
# │   └── etcd.service             # systemd 服务文件 (--config-file=/etc/etcd/etcd.config.yml)
# ├── tar/
# │   └── etcd-v*-linux-amd64.tar.gz      # etcd 二进制包 (仅允许一个版本)
# └── <node-ip>/                   # gen-etcd-config.sh 为本节点生成的配置
#     └── etcd.config.yml          # 已渲染的 etcd 配置 (含本节点 name/urls/cluster)
#
# 部署后目标主机关键路径:
#   /usr/local/bin/etcd             # etcd 二进制
#   /usr/local/bin/etcdctl          # etcdctl 二进制
#   /etc/etcd/etcd.config.yml      # etcd 配置文件
#   /etc/kubernetes/pki/etcd/      # TLS 证书目录
#     ├── etcd.pem                 # 服务端证书
#     ├── etcd-key.pem             # 服务端私钥
#     └── etcd-ca.pem              # CA 证书
#   /var/lib/etcd/                 # etcd 数据目录
#   /usr/lib/systemd/system/etcd.service  # systemd 服务文件
# ============================================================================

NODE_IP="${1:-}"
if [ -z "$NODE_IP" ]; then echo "ERROR: 用法: $0 <node-ip>"; exit 1; fi
echo "本节点 IP: $NODE_IP"

# 1. 解压 etcd 二进制文件 (自动检测 /tmp/ar/tar/ 下的 etcd 版本)
ETCD_TARBALL=$(ls /tmp/ar/tar/etcd-v*-linux-amd64.tar.gz 2>/dev/null)
TARBALL_COUNT=$(echo "$ETCD_TARBALL" | wc -l)
if [ -z "$ETCD_TARBALL" ]; then echo "ERROR: /tmp/ar/tar/ 下未找到 etcd tar 包"; exit 1; fi
if [ "$TARBALL_COUNT" -ne 1 ]; then echo "ERROR: /tmp/ar/tar/ 下存在多个 etcd tar 包, 仅允许一个版本"; ls /tmp/ar/tar/etcd-v*-linux-amd64.tar.gz; exit 1; fi
ETCD_DIR_NAME=$(tar -tzf "$ETCD_TARBALL" | head -1 | cut -d'/' -f1)
echo "检测到 etcd 包: $ETCD_TARBALL (目录: $ETCD_DIR_NAME)"
sudo tar -xf "$ETCD_TARBALL" -C /tmp/
sudo mv /tmp/${ETCD_DIR_NAME}/etcd /usr/local/bin/
sudo mv /tmp/${ETCD_DIR_NAME}/etcdctl /usr/local/bin/
sudo chmod +x /usr/local/bin/etcd /usr/local/bin/etcdctl
sudo rm -rf /tmp/${ETCD_DIR_NAME}
sudo etcd --version

# 2. 将 etcd 节点主机名写入 /etc/hosts (跳过已存在的条目)
while read -r ip name; do
  if ! grep -qE "^${ip}[[:space:]]" /etc/hosts; then
    echo "${ip} ${name}" | sudo tee -a /etc/hosts > /dev/null
    echo "已写入 /etc/hosts: ${ip} ${name}"
  else
    echo "已存在于 /etc/hosts, 跳过: ${ip}"
  fi
done < /tmp/ar/hosts

# 3. 部署 etcd 证书到 /etc/kubernetes/pki/etcd/
sudo mkdir -p /etc/kubernetes/pki/etcd
sudo cp /tmp/ar/pki/etcd.pem     /etc/kubernetes/pki/etcd/etcd.pem
sudo cp /tmp/ar/pki/etcd-key.pem /etc/kubernetes/pki/etcd/etcd-key.pem
sudo cp /tmp/ar/pki/ca.pem       /etc/kubernetes/pki/etcd/etcd-ca.pem
echo "已部署 etcd 证书到 /etc/kubernetes/pki/etcd/"
ls -l /etc/kubernetes/pki/etcd/

# 4. 部署本节点 etcd 配置到 /etc/etcd/
sudo mkdir -p /etc/etcd
sudo cp /tmp/ar/${NODE_IP}/etcd.config.yml /etc/etcd/etcd.config.yml
echo "已部署 etcd 配置: /etc/etcd/etcd.config.yml"
cat /etc/etcd/etcd.config.yml

# 5. 创建 etcd 数据目录
sudo mkdir -p /var/lib/etcd
sudo chmod 700 /var/lib/etcd

# 6. 安装 etcd systemd 服务
sudo cp /tmp/ar/service/etcd.service /usr/lib/systemd/system/etcd.service
sudo systemctl daemon-reload
sudo systemctl enable etcd
sudo systemctl start etcd
sudo systemctl status etcd
