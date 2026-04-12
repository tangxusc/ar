#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/ghcr.io-apache-flink-kubernetes-operator-1.11.0.tar" "docker://${REGISTRY}:5000/apache/flink-kubernetes-operator:1.11.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-library-flink-2.0-java17.tar" "docker://${REGISTRY}:5000/library/flink:2.0-java17"
