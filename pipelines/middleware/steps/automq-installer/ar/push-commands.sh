#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-strimzi-operator-0.47.0.tar" "docker://${REGISTRY}:5000/strimzi/operator:0.47.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-strimzi-kafka-0.47.0-kafka-3.9.0.tar" "docker://${REGISTRY}:5000/strimzi/kafka:0.47.0-kafka-3.9.0"
