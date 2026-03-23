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

echo "lvs-care 已启动: VIP=${LVSCARE_VIP}, masters=${MASTER_IPS}"
