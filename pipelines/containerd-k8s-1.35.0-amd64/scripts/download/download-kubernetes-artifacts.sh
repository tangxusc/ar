#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

download_file "https://dl.k8s.io/release/v${KUBERNETES_SERVER_VERSION}/kubernetes-server-linux-amd64.tar.gz" "${TARBALL_DIR}/kubernetes-server-linux-amd64.tar.gz"
download_file "https://github.com/etcd-io/etcd/releases/download/${ETCD_VERSION}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz" "${TARBALL_DIR}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz"

cat > "${CHECKSUM_DIR}/kubernetes.sha256" <<EOF
$(sha256sum "${TARBALL_DIR}/kubernetes-server-linux-amd64.tar.gz")
EOF

cat > "${CHECKSUM_DIR}/etcd.sha256" <<EOF
$(sha256sum "${TARBALL_DIR}/etcd-${ETCD_VERSION}-linux-amd64.tar.gz")
EOF
