#!/bin/bash
set -euo pipefail

die() {
  echo "ERROR: $*" >&2
  exit 1
}

REGISTRY_ARCHIVE="/tmp/ar/images/registry-2.8.3.tar"
REGISTRY_PORT="5000"
REGISTRY_CONTAINER="registry"
REGISTRY_DATA_DIR="/var/lib/registry"

echo "Installing local image registry (port=${REGISTRY_PORT})"

command -v ctr >/dev/null 2>&1 || die "ctr not found; ensure containerd is installed"
command -v systemctl >/dev/null 2>&1 || die "systemctl not found"

if [[ ! -s "${REGISTRY_ARCHIVE}" ]]; then
  die "missing offline registry image archive: ${REGISTRY_ARCHIVE}"
fi

# Install nerdctl if missing (from the offline tarball shipped with the pipeline).
if ! command -v nerdctl >/dev/null 2>&1; then
  nerdctl_tgz="$(ls -1 /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz 2>/dev/null | head -n1 || true)"
  [[ -n "${nerdctl_tgz}" ]] || die "nerdctl not found and /tmp/ar/tar/nerdctl-*-linux-amd64.tar.gz missing"
  tmpdir="$(mktemp -d)"
  trap 'rm -rf "${tmpdir}"' EXIT
  tar -xzf "${nerdctl_tgz}" -C "${tmpdir}"
  nerdctl_bin="$(find "${tmpdir}" -maxdepth 2 -type f -name nerdctl | head -n1 || true)"
  [[ -n "${nerdctl_bin}" ]] || die "nerdctl binary not found in tarball: ${nerdctl_tgz}"
  sudo install -m 0755 "${nerdctl_bin}" /usr/local/bin/nerdctl
  echo "Installed nerdctl: $(nerdctl --version || true)"
  trap - EXIT
  rm -rf "${tmpdir}"
fi

# Import registry image to containerd (idempotent).
sudo ctr -n k8s.io images import "${REGISTRY_ARCHIVE}" >/dev/null

img=""
for cand in "registry:2.8.3" "docker.io/library/registry:2.8.3" "docker.io/registry:2.8.3"; do
  if sudo ctr -n k8s.io images ls -q | grep -Fxq "${cand}"; then
    img="${cand}"
    break
  fi
done
if [[ -z "${img}" ]]; then
  img="$(sudo ctr -n k8s.io images ls -q | grep -E '(^|/)registry:2\.8\.3$' | head -n1 || true)"
fi
[[ -n "${img}" ]] || die "registry image not found in containerd after import"

# Ensure a stable local alias for runtime.
if [[ "${img}" != "registry:2.8.3" ]]; then
  sudo ctr -n k8s.io images tag "${img}" "registry:2.8.3" >/dev/null 2>&1 || true
fi

# Deploy config/auth files.
sudo mkdir -p /etc/registry
sudo cp /tmp/ar/confs/image-registry/* /etc/registry/

sudo mkdir -p "${REGISTRY_DATA_DIR}"

# (Re)start registry using host networking (no CNI dependency).
sudo nerdctl -n k8s.io rm -f "${REGISTRY_CONTAINER}" >/dev/null 2>&1 || true
sudo nerdctl -n k8s.io run -d \
  --name "${REGISTRY_CONTAINER}" \
  --restart=always \
  --net host \
  -v /etc/registry/config.yml:/etc/docker/registry/config.yml \
  -v /etc/registry/htpasswd:/etc/registry/htpasswd \
  -v "${REGISTRY_DATA_DIR}":/var/lib/registry \
  registry:2.8.3 >/dev/null

echo "Registry started: http://127.0.0.1:${REGISTRY_PORT}"

