#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

download_file "https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz" "${TARBALL_DIR}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz"
download_file "https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz" "${TARBALL_DIR}/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz"
download_file "https://github.com/kubernetes-sigs/cri-tools/releases/download/${CRICTL_VERSION}/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz" "${TARBALL_DIR}/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz"
download_file "https://github.com/opencontainers/runc/releases/download/v${RUNC_VERSION}/runc.amd64" "${BIN_DIR}/runc"
chmod +x "${BIN_DIR}/runc"
download_file "https://github.com/containerd/nerdctl/releases/download/v${NERDCTL_VERSION}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz" "${TARBALL_DIR}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz"
download_file "https://github.com/lework/skopeo-binary/releases/download/v${SKOPEO_VERSION}/skopeo-linux-amd64" "${BIN_DIR}/skopeo"
chmod +x "${BIN_DIR}/skopeo"

cat > "${CHECKSUM_DIR}/runtime.sha256" <<EOF
$(sha256sum "${TARBALL_DIR}/containerd-${CONTAINERD_VERSION}-linux-amd64.tar.gz")
$(sha256sum "${TARBALL_DIR}/cni-plugins-linux-amd64-${CNI_PLUGINS_VERSION}.tgz")
$(sha256sum "${TARBALL_DIR}/crictl-${CRICTL_VERSION}-linux-amd64.tar.gz")
$(sha256sum "${BIN_DIR}/runc")
$(sha256sum "${TARBALL_DIR}/nerdctl-${NERDCTL_VERSION}-linux-amd64.tar.gz")
$(sha256sum "${BIN_DIR}/skopeo")
EOF
