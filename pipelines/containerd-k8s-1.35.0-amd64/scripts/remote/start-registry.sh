#!/usr/bin/env bash
set -euo pipefail

REGISTRY_NAME="${REGISTRY_NAME:-local-registry}"
REGISTRY_IMAGE="${REGISTRY_IMAGE:-registry:2}"
REGISTRY_DATA_DIR="${REGISTRY_DATA_DIR:-/var/lib/registry}"

sudo mkdir -p "${REGISTRY_DATA_DIR}"
sudo nerdctl rm -f "${REGISTRY_NAME}" >/dev/null 2>&1 || true
sudo nerdctl run -d \
  --restart always \
  --net host \
  -v "${REGISTRY_DATA_DIR}:/var/lib/registry" \
  --name "${REGISTRY_NAME}" \
  "${REGISTRY_IMAGE}"
