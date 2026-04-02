#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-istio-proxyv2-1.29.1.tar" "docker://${REGISTRY}:5000/istio/proxyv2:1.29.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-istio-pilot-1.29.1.tar" "docker://${REGISTRY}:5000/istio/pilot:1.29.1"
