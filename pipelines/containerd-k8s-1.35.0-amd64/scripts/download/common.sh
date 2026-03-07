#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/metadata/versions.lock"

ARTIFACTS_DIR="${ROOT_DIR}/steps/base/artifacts"
BIN_DIR="${ARTIFACTS_DIR}/bin"
TARBALL_DIR="${ARTIFACTS_DIR}/tarballs"
IMAGE_DIR="${ARTIFACTS_DIR}/images"
MANIFEST_DIR="${ARTIFACTS_DIR}/manifests"
CHECKSUM_DIR="${ARTIFACTS_DIR}/checksums"

mkdir -p "${BIN_DIR}" "${TARBALL_DIR}" "${IMAGE_DIR}" "${MANIFEST_DIR}" "${CHECKSUM_DIR}"

download_file() {
  local url="$1"
  local output="$2"
  if [[ -f "${output}" ]]; then
    echo "reuse ${output}"
    return 0
  fi
  curl -fL --progress-bar --retry 3 --retry-delay 2 -o "${output}" "${url}"
}

download_optional_file() {
  local url="$1"
  local output="$2"
  if [[ -f "${output}" ]]; then
    echo "reuse ${output}"
    return 0
  fi
  if ! curl -fL --progress-bar --retry 3 --retry-delay 2 -o "${output}" "${url}"; then
    echo "warning: failed to download ${url}" >&2
    rm -f "${output}"
  fi
}

record_download() {
  local target="$1"
  sha256sum "${target}" >> "${CHECKSUM_DIR}/downloaded.sha256"
}
