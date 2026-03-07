#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

download_file "https://get.helm.sh/helm-v${HELM_VERSION}-linux-amd64.tar.gz" "${TARBALL_DIR}/helm-v${HELM_VERSION}-linux-amd64.tar.gz"
download_file "https://github.com/coredns/helm/archive/refs/tags/coredns-${COREDNS_CHART_VERSION}.tar.gz" "${TARBALL_DIR}/coredns-chart-${COREDNS_CHART_VERSION}.tar.gz"

mkdir -p "${MANIFEST_DIR}/coredns" "${MANIFEST_DIR}/metrics-server"
tar -xf "${TARBALL_DIR}/coredns-chart-${COREDNS_CHART_VERSION}.tar.gz" -C /tmp
rm -rf "${MANIFEST_DIR}/coredns/chart"
mkdir -p "${MANIFEST_DIR}/coredns/chart"
cp -r "/tmp/helm-coredns-${COREDNS_CHART_VERSION}/charts/coredns/." "${MANIFEST_DIR}/coredns/chart/"
rm -rf "/tmp/helm-coredns-${COREDNS_CHART_VERSION}"

cat > "${MANIFEST_DIR}/coredns/values.yaml" <<'EOF'
service:
  clusterIP: "10.96.0.10"
image:
  repository: coredns/coredns
  tag: 1.11.4
EOF

cat > "${MANIFEST_DIR}/metrics-server/components.yaml" <<'EOF'
# Placeholder metrics-server manifest for offline packaging.
apiVersion: v1
kind: List
items: []
EOF
