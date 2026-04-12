#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.cn-beijing.aliyuncs.com-deepflow-ce-deepflow-server-latest.tar" "docker://${REGISTRY}:5000/deepflow-ce/deepflow-server:latest"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.cn-beijing.aliyuncs.com-deepflow-ce-deepflow-agent-latest.tar" "docker://${REGISTRY}:5000/deepflow-ce/deepflow-agent:latest"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.cn-beijing.aliyuncs.com-deepflow-ce-clickhouse-23.8.9.54.tar" "docker://${REGISTRY}:5000/deepflow-ce/clickhouse:23.8.9.54"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.cn-beijing.aliyuncs.com-deepflow-ce-grafana-latest.tar" "docker://${REGISTRY}:5000/deepflow-ce/grafana:latest"
