#!/bin/bash
set -euo pipefail
# 此文件由 Makefile offline-images 自动生成，勿手动修改
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-library-mysql-5.6.tar" "docker://${REGISTRY}:5000/library/mysql:5.6"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cephcsi-ceph-csi-operator-v0.6.0.tar" "docker://${REGISTRY}:5000/cephcsi/ceph-csi-operator:v0.6.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-ceph-ceph-v19.2.3.tar" "docker://${REGISTRY}:5000/ceph/ceph:v19.2.3"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/docker.io-rook-ceph-v1.19.3.tar" "docker://${REGISTRY}:5000/rook/ceph:v1.19.3"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-ceph-ceph-v19.tar" "docker://${REGISTRY}:5000/ceph/ceph:v19"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-cephcsi-cephcsi-v3.16.2.tar" "docker://${REGISTRY}:5000/cephcsi/cephcsi:v3.16.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-sig-storage-csi-provisioner-v6.1.1.tar" "docker://${REGISTRY}:5000/sig-storage/csi-provisioner:v6.1.1"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-sig-storage-csi-resizer-v2.1.0.tar" "docker://${REGISTRY}:5000/sig-storage/csi-resizer:v2.1.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-sig-storage-csi-attacher-v4.11.0.tar" "docker://${REGISTRY}:5000/sig-storage/csi-attacher:v4.11.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-sig-storage-csi-snapshotter-v8.5.0.tar" "docker://${REGISTRY}:5000/sig-storage/csi-snapshotter:v8.5.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/registry.k8s.io-sig-storage-csi-node-driver-registrar-v2.16.0.tar" "docker://${REGISTRY}:5000/sig-storage/csi-node-driver-registrar:v2.16.0"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/gcr.io-k8s-staging-sig-storage-objectstorage-sidecar-v20240513-v0.1.0-35-gefb3255.tar" "docker://${REGISTRY}:5000/k8s-staging-sig-storage/objectstorage-sidecar:v20240513-v0.1.0-35-gefb3255"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-ceph-cosi-v0.1.2.tar" "docker://${REGISTRY}:5000/ceph/cosi:v0.1.2"
skopeo copy --dest-creds "${REGISTRY_USER}:${REGISTRY_PASSWORD}" --dest-tls-verify=false "oci-archive:/ar/images/quay.io-csiaddons-k8s-sidecar-v0.14.0.tar" "docker://${REGISTRY}:5000/csiaddons/k8s-sidecar:v0.14.0"
