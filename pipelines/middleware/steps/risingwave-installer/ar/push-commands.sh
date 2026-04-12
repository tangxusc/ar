#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-risingwavelabs-risingwave-v2.6.0.tar" "docker://${REGISTRY}:5000/risingwavelabs/risingwave:v2.6.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-bitnami-etcd-3.5.16.tar" "docker://${REGISTRY}:5000/bitnami/etcd:3.5.16"
