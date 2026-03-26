#!/bin/bash
set -euo pipefail

die() {
  echo "ERROR: $*" >&2
  exit 1
}

COREDNS_ARCHIVE="/tmp/ar/images/registry.k8s.io-coredns-coredns-v1.14.2.tar"
COREDNS_IMAGE="registry.k8s.io/coredns/coredns:v1.14.2"

command -v ctr >/dev/null 2>&1 || die "ctr not found; ensure containerd is installed"

if [[ ! -s "${COREDNS_ARCHIVE}" ]]; then
  die "missing offline CoreDNS image archive: ${COREDNS_ARCHIVE}"
fi

sudo ctr -n k8s.io images import "${COREDNS_ARCHIVE}" >/dev/null

img=""
for cand in "${COREDNS_IMAGE}" "coredns/coredns:v1.14.2"; do
  if sudo ctr -n k8s.io images ls -q | grep -Fxq "${cand}"; then
    img="${cand}"
    break
  fi
done
if [[ -z "${img}" ]]; then
  img="$(sudo ctr -n k8s.io images ls -q | grep -E '(^|/)coredns:v1\.14\.2$' | head -n1 || true)"
fi
[[ -n "${img}" ]] || die "CoreDNS image not found in containerd after import"

if [[ "${img}" != "${COREDNS_IMAGE}" ]]; then
  sudo ctr -n k8s.io images tag "${img}" "${COREDNS_IMAGE}" >/dev/null 2>&1 || true
fi

echo "CoreDNS image imported: ${COREDNS_IMAGE}"
