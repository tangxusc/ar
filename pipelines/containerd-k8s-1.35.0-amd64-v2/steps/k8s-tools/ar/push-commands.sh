#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-envoy-v1.35.9-1773656288-7b052e66eb2cfc5ac130ce0a5be66202a10d83be.tar" "docker://${REGISTRY}/cilium/cilium-envoy:v1.35.9-1773656288-7b052e66eb2cfc5ac130ce0a5be66202a10d83be"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-v1.19.2.tar" "docker://${REGISTRY}/cilium/cilium:v1.19.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-operator-generic-v1.19.2.tar" "docker://${REGISTRY}/cilium/operator-generic:v1.19.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-pause-3.10.1.tar" "docker://${REGISTRY}/pause:3.10.1"
