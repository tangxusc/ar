#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-envoy-v1.35.9-1773656288-7b052e66eb2cfc5ac130ce0a5be66202a10d83be.tar" "docker://${REGISTRY}:5000/cilium/cilium-envoy:v1.35.9-1773656288-7b052e66eb2cfc5ac130ce0a5be66202a10d83be"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-v1.19.2.tar" "docker://${REGISTRY}:5000/cilium/cilium:v1.19.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-operator-generic-v1.19.2.tar" "docker://${REGISTRY}:5000/cilium/operator-generic:v1.19.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-pause-3.10.1.tar" "docker://${REGISTRY}:5000/pause:3.10.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-hubble-relay-v1.19.2.tar" "docker://${REGISTRY}:5000/cilium/hubble-relay:v1.19.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-hubble-ui-v0.13.3.tar" "docker://${REGISTRY}:5000/cilium/hubble-ui:v0.13.3"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-hubble-ui-backend-v0.13.3.tar" "docker://${REGISTRY}:5000/cilium/hubble-ui-backend:v0.13.3"
