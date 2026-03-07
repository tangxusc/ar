#!/usr/bin/env bash
set -euo pipefail

source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh"

download_file "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssl_${CFSSL_VERSION}_linux_amd64" "${BIN_DIR}/cfssl"
download_file "https://github.com/cloudflare/cfssl/releases/download/v${CFSSL_VERSION}/cfssljson_${CFSSL_VERSION}_linux_amd64" "${BIN_DIR}/cfssljson"
chmod +x "${BIN_DIR}/cfssl" "${BIN_DIR}/cfssljson"
