#!/bin/bash
set -euo pipefail

die() {
  echo "ERROR: $*" >&2
  exit 1
}

LVSCARE_ARCHIVE="/tmp/ar/images/lvs-care.tar"
LVSCARE_IMAGE="ghcr.io/labring/lvscare:v5.1.2-rc3"
LVSCARE_CONTAINER="lvscare"
# 第二个参数：VIP 地址（不含端口），默认 10.103.97.12
LVSCARE_VIP_HOST="${2:-10.103.97.12}"
LVSCARE_VIP="${LVSCARE_VIP_HOST}:6443"

# 第一个参数：逗号分隔的 master IP 列表
MASTER_IPS="${1:?用法: install-lvs-care.sh <master_ip1,master_ip2,...> [vip]}"

echo "安装 lvs-care (VIP=${LVSCARE_VIP}, masters=${MASTER_IPS})"

command -v ctr >/dev/null 2>&1 || die "ctr 未找到；请确保 containerd 已安装"

if [[ ! -s "${LVSCARE_ARCHIVE}" ]]; then
  die "缺少离线 lvs-care 镜像: ${LVSCARE_ARCHIVE}"
fi

# 确保 nerdctl 可用
if ! command -v nerdctl >/dev/null 2>&1; then
  nerdctl_tgz="$(ls -1 /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz 2>/dev/null | head -n1 || true)"
  [[ -n "${nerdctl_tgz}" ]] || die "nerdctl 未找到且 /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz 不存在"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT
  tar -xzf "${nerdctl_tgz}" -C "${tmpdir}"
  nerdctl_bin="$(find "${tmpdir}" -maxdepth 2 -type f -name nerdctl | head -n1 || true)"
  [[ -n "${nerdctl_bin}" ]] || die "nerdctl 二进制文件未在 tarball 中找到: ${nerdctl_tgz}"
  sudo install -m 0755 "${nerdctl_bin}" /usr/local/bin/nerdctl
  echo "已安装 nerdctl: $(nerdctl --version || true)"
  trap - EXIT
  rm -rf "${tmpdir}"
fi

# 导入 lvs-care 镜像到 containerd（幂等操作）
sudo ctr -n k8s.io images import "${LVSCARE_ARCHIVE}" >/dev/null

# 检测导入后的镜像引用
img=""
for cand in "${LVSCARE_IMAGE}" "ghcr.io/labring/lvscare:v5.1.2-rc3"; do
  if sudo ctr -n k8s.io images ls -q | grep -Fxq "${cand}"; then
    img="${cand}"
    break
  fi
done
if [[ -z "${img}" ]]; then
  img="$(sudo ctr -n k8s.io images ls -q | grep -E 'lvscare:v5\.1\.2-rc3$' | head -n1 || true)"
fi
[[ -n "${img}" ]] || die "导入后在 containerd 中未找到 lvs-care 镜像"

# 确保使用稳定的本地别名
if [[ "${img}" != "${LVSCARE_IMAGE}" ]]; then
  sudo ctr -n k8s.io images tag "${img}" "${LVSCARE_IMAGE}" >/dev/null 2>&1 || true
fi

# 解析逗号分隔的 master IP，构建 --rs 参数
RS_ARGS=""
IFS=',' read -ra MASTER_ARRAY <<< "${MASTER_IPS}"
for master_ip in "${MASTER_ARRAY[@]}"; do
  RS_ARGS="${RS_ARGS} --rs ${master_ip}:6443"
done

# 将系统 iptables 切换为 legacy 模式，确保与 lvscare 容器内行为一致
# lvscare 容器内使用 iptables-legacy 写规则，若宿主机内核走 nftables 则 MASQUERADE 不生效
if command -v update-alternatives >/dev/null 2>&1; then
  if update-alternatives --display iptables 2>/dev/null | grep -q 'iptables-nft'; then
    echo "将 iptables 切换为 legacy 模式（避免 lvscare MASQUERADE 规则失效）"
    sudo update-alternatives --set iptables /usr/sbin/iptables-legacy
    sudo update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy 2>/dev/null || true
  fi
fi

# 停止并删除已有的 lvscare 容器（如果存在）
sudo nerdctl -n k8s.io rm -f "${LVSCARE_CONTAINER}" >/dev/null 2>&1 || true

# 启动 lvs-care 容器（需要特权模式操作 IPVS 规则，挂载 /lib/modules 用于 modprobe）
sudo nerdctl -n k8s.io run -d \
  --name "${LVSCARE_CONTAINER}" \
  --restart=always \
  --privileged \
  --net host \
  -v /lib/modules:/lib/modules:ro \
  "${LVSCARE_IMAGE}" \
  care --mode link --iface lvscare --vs "${LVSCARE_VIP}" ${RS_ARGS} --interval 5 >/dev/null

# 等待 lvscare 完成初始化后确保网卡处于 UP 状态
# lvscare 创建 dummy 网卡后有时不会自动 UP，导致 VIP 无法联通
echo "等待 lvscare 网卡初始化..."
for i in $(seq 1 30); do
  if ip link show lvscare >/dev/null 2>&1; then
    sudo ip link set lvscare up 2>/dev/null || true
    echo "lvscare 网卡已 UP（等待 ${i}s）"
    break
  fi
  if [[ ${i} -eq 30 ]]; then
    echo "WARNING: lvscare 网卡在 30s 内未出现，请手动检查" >&2
  fi
  sleep 1
done

echo "lvs-care 已启动: VIP=${LVSCARE_VIP}, masters=${MASTER_IPS}"
