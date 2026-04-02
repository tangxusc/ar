#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-metrics-server-metrics-server-v0.8.1.tar" "docker://${REGISTRY}:5000/metrics-server/metrics-server:v0.8.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-coredns-coredns-v1.14.2.tar" "docker://${REGISTRY}:5000/coredns/coredns:v1.14.2"
