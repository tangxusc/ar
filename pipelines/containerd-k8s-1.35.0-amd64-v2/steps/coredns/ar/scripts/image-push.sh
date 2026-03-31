#!/bin/bash
set -euo pipefail

# ============================================================================
# image-push.sh - 推送离线镜像到私有仓库
# ============================================================================
# 用法: bash image-push.sh <registry_urls> <registry_user> <registry_password>
# 例:   bash image-push.sh "172.19.16.11:5000,172.19.16.12:5000" "admin" "admin123"
# ============================================================================

die() { echo "ERROR: $*" >&2; exit 1; }

REGISTRY_URLS="${1:?用法: $0 <registry_urls>}"
REGISTRY_USER="${2:?用法: $0 <registry_user>}"
REGISTRY_PASSWORD="${3:?用法: $0 <registry_password>}"

PUSH_COMMANDS="/ar/push-commands.sh"

[[ -f "$PUSH_COMMANDS" ]] || die "推送命令文件不存在: $PUSH_COMMANDS"
command -v skopeo >/dev/null 2>&1 || die "skopeo 未安装"

IFS=',' read -ra REGISTRIES <<< "$REGISTRY_URLS"

# skopeo 默认对非 localhost 地址使用 HTTPS，需将目标仓库配置为 insecure 以使用 HTTP
mkdir -p /etc/containers
: > /etc/containers/registries.conf
for r in "${REGISTRIES[@]}"; do
  r="${r// /}"
  [[ -z "$r" ]] && continue
  printf '[[registry]]\nlocation = "%s"\ninsecure = true\n\n' "$r" >> /etc/containers/registries.conf
done

for registry in "${REGISTRIES[@]}"; do
  registry="${registry// /}"
  [[ -z "$registry" ]] && continue
  echo "---- 推送到仓库: $registry ----"
  export REGISTRY="$registry"
  export REGISTRY_USER
  export REGISTRY_PASSWORD
  bash "$PUSH_COMMANDS"
  echo "---- $registry 推送完成 ----"
done

echo "所有离线镜像推送完成！"
