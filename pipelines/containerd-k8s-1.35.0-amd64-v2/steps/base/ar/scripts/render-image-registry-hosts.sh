#!/bin/bash
set -euo pipefail

# Render containerd registry hosts.toml from registry node IPs.
# Output path is fixed to /ar-data/confs/registry/hosts.toml to match pipeline conventions.
#
# Usage:
#   ./render-image-registry-hosts.sh "<ip1>,<ip2>,..."

ips_csv="${1:-}"
ips_csv="$(printf '%s' "${ips_csv}" | tr -d '[:space:]')"

if [[ -z "${ips_csv}" ]]; then
  echo "ERROR: missing registry node ip list, usage: $0 \"<ip1>,<ip2>,...\"" >&2
  exit 1
fi

out_dir="/ar-data/confs/registry"
out_file="${out_dir}/hosts.toml"
mkdir -p "${out_dir}"

auth_username="tanxtanx"
auth_password="tanxtanx"

tmp="$(mktemp)"
trap 'rm -f "${tmp}"' EXIT

IFS=',' read -r -a ips <<< "${ips_csv}"
for ip in "${ips[@]}"; do
  [[ -n "${ip}" ]] || continue
  cat >> "${tmp}" <<EOF
[host."http://${ip}:5000"]
  capabilities = ["pull", "resolve"]
  auth = { username = "${auth_username}", password = "${auth_password}" }
  skip_verify = true

EOF
done

if [[ ! -s "${tmp}" ]]; then
  echo "ERROR: no valid registry ip found in: ${ips_csv}" >&2
  exit 1
fi

mv "${tmp}" "${out_file}"
echo "Rendered: ${out_file}"
cat "${out_file}"
