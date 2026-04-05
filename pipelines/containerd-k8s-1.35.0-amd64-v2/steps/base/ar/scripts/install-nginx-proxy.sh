#!/bin/bash
set -euo pipefail

die() {
  echo "ERROR: $*" >&2
  exit 1
}

NGINX_ARCHIVE="/tmp/ar/images/nginx-1.27.5.tar"
NGINX_IMAGE="docker.io/library/nginx:1.27.5"
NGINX_CONTAINER="nginx-apiserver-proxy"
LISTEN_ADDR="127.0.0.1"
LISTEN_PORT="8443"

MASTER_IPS="${1:?用法: install-nginx-proxy.sh <master_ip1,master_ip2,...>}"

echo "安装 nginx apiserver 反向代理 (listen=${LISTEN_ADDR}:${LISTEN_PORT}, masters=${MASTER_IPS})"

command -v ctr >/dev/null 2>&1 || die "ctr 未找到；请确保 containerd 已安装"

if [[ ! -s "${NGINX_ARCHIVE}" ]]; then
  die "缺少离线 nginx 镜像: ${NGINX_ARCHIVE}"
fi

if ! command -v nerdctl >/dev/null 2>&1; then
  nerdctl_tgz="$(ls -1 /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz 2>/dev/null | head -n1 || true)"
  [[ -n "${nerdctl_tgz}" ]] || die "nerdctl 未找到且 /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz 不存在"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT
  tar -xzf "${nerdctl_tgz}" -C "${tmpdir}"
  nerdctl_bin="$(find "${tmpdir}" -maxdepth 2 -type f -name nerdctl | head -n1 || true)"
  [[ -n "${nerdctl_bin}" ]] || die "nerdctl 二进制文件未在 tarball 中找到: ${nerdctl_tgz}"
  sudo install -m 0755 "${nerdctl_bin}" /usr/bin/nerdctl
  echo "已安装 nerdctl: $(nerdctl --version || true)"
  trap - EXIT
  rm -rf "${tmpdir}"
fi

sudo ctr -n k8s.io images import "${NGINX_ARCHIVE}" >/dev/null

img=""
for cand in "${NGINX_IMAGE}" "nginx:1.27.5" "docker.io/nginx:1.27.5"; do
  if sudo ctr -n k8s.io images ls -q | grep -Fxq "${cand}"; then
    img="${cand}"
    break
  fi
done
if [[ -z "${img}" ]]; then
  img="$(sudo ctr -n k8s.io images ls -q | grep -E '(^|/)nginx:1\.27\.5$' | head -n1 || true)"
fi
[[ -n "${img}" ]] || die "导入后在 containerd 中未找到 nginx:1.27.5 镜像"

if [[ "${img}" != "${NGINX_IMAGE}" ]]; then
  sudo ctr -n k8s.io images tag "${img}" "${NGINX_IMAGE}" >/dev/null 2>&1 || true
fi

CONF_DIR="/etc/nginx-apiserver-proxy"
CONF_FILE="${CONF_DIR}/nginx.conf"
sudo mkdir -p "${CONF_DIR}"

IFS=',' read -ra MASTER_ARRAY <<< "${MASTER_IPS}"
TMP_CONF="$(mktemp)"
{
  cat <<'EOF_CONF'
user nginx;
worker_processes auto;

events {
  worker_connections 1024;
}

stream {
  upstream kube_apiserver {
EOF_CONF
  has_master="false"
  for master_ip in "${MASTER_ARRAY[@]}"; do
    [[ -n "${master_ip}" ]] || continue
    has_master="true"
    echo "    server ${master_ip}:6443 max_fails=3 fail_timeout=10s;"
  done
  if [[ "${has_master}" != "true" ]]; then
    rm -f "${TMP_CONF}"
    die "master IP 列表为空"
  fi
  cat <<EOF_CONF
  }

  server {
    listen ${LISTEN_ADDR}:${LISTEN_PORT};
    proxy_connect_timeout 3s;
    proxy_timeout 600s;
    proxy_pass kube_apiserver;
  }
}
EOF_CONF
} > "${TMP_CONF}"

sudo cp "${TMP_CONF}" "${CONF_FILE}"
rm -f "${TMP_CONF}"

sudo nerdctl -n k8s.io run --rm \
  --net host \
  -v "${CONF_FILE}:/etc/nginx/nginx.conf:ro" \
  "${NGINX_IMAGE}" nginx -t >/dev/null

sudo nerdctl -n k8s.io rm -f "${NGINX_CONTAINER}" >/dev/null 2>&1 || true

sudo nerdctl -n k8s.io run -d \
  --name "${NGINX_CONTAINER}" \
  --restart=always \
  --net host \
  -v "${CONF_FILE}:/etc/nginx/nginx.conf:ro" \
  "${NGINX_IMAGE}" >/dev/null

echo "等待 nginx 反向代理监听 ${LISTEN_ADDR}:${LISTEN_PORT}..."
for i in $(seq 1 20); do
  if sudo ss -lnt "( sport = :${LISTEN_PORT} )" | grep -q "${LISTEN_ADDR}:${LISTEN_PORT}"; then
    echo "nginx 反向代理已就绪"
    break
  fi
  if [[ ${i} -eq 20 ]]; then
    die "nginx 反向代理启动超时"
  fi
  sleep 1
done

echo "nginx apiserver 反向代理安装完成"
