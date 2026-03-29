#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-envoy-v1.35.9-1770979049-232ed4a26881e4ab4f766f251f258ed424fff663.tar" "docker://${REGISTRY}:5000/cilium/cilium-envoy:v1.35.9-1770979049-232ed4a26881e4ab4f766f251f258ed424fff663"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-cilium-v1.19.1.tar" "docker://${REGISTRY}:5000/cilium/cilium:v1.19.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cilium-operator-generic-v1.19.1.tar" "docker://${REGISTRY}:5000/cilium/operator-generic:v1.19.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-pause-3.10.1.tar" "docker://${REGISTRY}:5000/pause:3.10.1"
