#!/bin/bash
# 列出 OCI Image Layout 目录中镜像的所有文件（来自各层）
# 用法: ./list-oci-image-files.sh /var/lib/ar/images/test

set -e
LAYOUT_DIR="${1:-.}"
cd "$LAYOUT_DIR"

if [[ ! -f index.json || ! -f oci-layout ]]; then
  echo "错误: 请在 OCI image layout 目录下执行（需有 index.json 和 oci-layout）"
  exit 1
fi

# 取 manifest digest（多 manifest 时取第一个）
MANIFEST_DIGEST=$(jq -r '.manifests[0].digest' index.json)
if [[ -z "$MANIFEST_DIGEST" || "$MANIFEST_DIGEST" == "null" ]]; then
  echo "错误: 无法从 index.json 读取 manifest digest"
  exit 1
fi

# digest 格式为 sha256:xxx，blob 路径只要 xxx
BLOB_DIR="blobs/sha256"
MANIFEST_SHA="${MANIFEST_DIGEST#sha256:}"
MANIFEST_PATH="$BLOB_DIR/$MANIFEST_SHA"

if [[ ! -f "$MANIFEST_PATH" ]]; then
  echo "错误: 找不到 manifest 文件 $MANIFEST_PATH"
  exit 1
fi

echo "=== 镜像层与文件列表 ==="
LAYER_NUM=0
for digest in $(jq -r '.layers[].digest' "$MANIFEST_PATH"); do
  LAYER_NUM=$((LAYER_NUM + 1))
  sha="${digest#sha256:}"
  layer_path="$BLOB_DIR/$sha"
  if [[ ! -f "$layer_path" ]]; then
    echo "警告: 找不到层 $layer_path，跳过"
    continue
  fi
  echo ""
  echo "--- 第 ${LAYER_NUM} 层 (${digest}) ---"
  tar -tvf "$layer_path"
done

echo ""
echo "=== 完成 ==="

# 若只想看某一层的文件，可手动执行（把 DIGEST 换成 jq -r '.layers[0].digest' 等输出）:
#   tar -tvf blobs/sha256/<digest去掉sha256:的部分>
