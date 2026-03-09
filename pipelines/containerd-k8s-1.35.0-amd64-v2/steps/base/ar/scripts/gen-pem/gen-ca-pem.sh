#!/bin/bash
set -e

chmod a+x /usr/local/bin/cfssl /usr/local/bin/cfssljson

# 写入生成证书所需的配置文件
cd /ar/scripts/gen-pem
cat ca-config.json
cat ca-csr.json

mkdir -p /ar-data/pki
cfssl gencert -initca ca-csr.json | cfssljson -bare /ar-data/pki/ca

ls -l /ar-data/pki/