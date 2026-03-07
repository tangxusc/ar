#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

download_file "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-amd64.tar.gz" "${TARBALL_DIR}/cilium-linux-amd64.tar.gz"
download_optional_file "https://github.com/cilium/cilium-cli/releases/download/${CILIUM_CLI_VERSION}/cilium-linux-amd64.tar.gz.sha256sum" "${TARBALL_DIR}/cilium-linux-amd64.tar.gz.sha256sum"
download_file "https://github.com/cilium/cilium/archive/refs/tags/v${CILIUM_VERSION}.tar.gz" "${TARBALL_DIR}/cilium-chart-${CILIUM_VERSION}.tar.gz"

mkdir -p "${MANIFEST_DIR}/cilium/chart"
tar -xf "${TARBALL_DIR}/cilium-chart-${CILIUM_VERSION}.tar.gz" -C /tmp
rm -rf "${MANIFEST_DIR}/cilium/chart"
mkdir -p "${MANIFEST_DIR}/cilium/chart"
cp -r "/tmp/cilium-${CILIUM_VERSION}/install/kubernetes/cilium/." "${MANIFEST_DIR}/cilium/chart/"
rm -rf "/tmp/cilium-${CILIUM_VERSION}"

cat > "${MANIFEST_DIR}/cilium/install.env" <<'EOF'
CILIUM_NAMESPACE=kube-system
CILIUM_COMPAT_PREPEND_IPTABLES_CHAINS=false
CILIUM_ENABLE_HUBBLE=true
EOF
