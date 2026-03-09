#!/bin/bash
set -e

chmod a+x /usr/local/bin/cfssl /usr/local/bin/cfssljson

# 写入生成证书所需的配置文件
cd /ar/scripts/gen-pem
cat etcd-csr.json 

BASE_HOSTNAMES="127.0.0.1,k8s-api-server,::1"
EXTRA_HOSTNAMES="$(echo "${1:-}" | tr -d '[:space:]')"
HOSTNAMES="$BASE_HOSTNAMES"

if [ -n "$EXTRA_HOSTNAMES" ]; then
  HOSTNAMES="${BASE_HOSTNAMES},${EXTRA_HOSTNAMES}"
fi

echo "BASE_HOSTNAMES: ${BASE_HOSTNAMES}"
echo "EXTRA_HOSTNAMES: ${EXTRA_HOSTNAMES}"
echo "HOSTNAMES: ${HOSTNAMES}"

cfssl gencert \
   -ca=/ar-data/pki/ca.pem \
   -ca-key=/ar-data/pki/ca-key.pem \
   -config=ca-config.json \
   -hostname="${HOSTNAMES}" \
   -profile=kubernetes \
   etcd-csr.json | cfssljson -bare /ar-data/pki/etcd

ls -l /ar-data/pki/