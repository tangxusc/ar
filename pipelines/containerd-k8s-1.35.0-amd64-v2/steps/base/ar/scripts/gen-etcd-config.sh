#!/bin/bash
set -euo pipefail

usage() {
  echo "Usage: $0 \"<ip1>,<ip2>,...\""
}

die() {
  echo "ERROR: $*" >&2
  exit 1
}

is_ipv4() {
  local ip="$1"
  local IFS='.'
  local -a octets=()

  [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || return 1
  read -r -a octets <<< "$ip"
  [[ "${#octets[@]}" -eq 4 ]] || return 1

  local octet
  for octet in "${octets[@]}"; do
    [[ "$octet" =~ ^[0-9]+$ ]] || return 1
    ((octet >= 0 && octet <= 255)) || return 1
  done
  return 0
}

count_ipv6_groups() {
  local section="$1"
  if [[ -z "$section" ]]; then
    echo 0
    return 0
  fi

  local IFS=':'
  local -a groups=()
  local g
  read -r -a groups <<< "$section"
  for g in "${groups[@]}"; do
    [[ -n "$g" ]] || return 1
    [[ "$g" =~ ^[0-9a-fA-F]{1,4}$ ]] || return 1
  done
  echo "${#groups[@]}"
  return 0
}

is_ipv6() {
  local ip="$1"
  local head
  local tail
  local head_count
  local tail_count
  local total_count
  local double_colon_count=0
  local rest="$ip"

  [[ -n "$ip" ]] || return 1
  [[ "$ip" == *:* ]] || return 1
  [[ "$ip" != *%* ]] || return 1
  [[ "$ip" != *.* ]] || return 1
  [[ "$ip" =~ ^[0-9a-fA-F:]+$ ]] || return 1
  [[ "$ip" != *:::* ]] || return 1

  while [[ "$rest" == *"::"* ]]; do
    double_colon_count=$((double_colon_count + 1))
    rest="${rest#*::}"
  done
  ((double_colon_count <= 1)) || return 1

  if [[ "$ip" == *"::"* ]]; then
    head="${ip%%::*}"
    tail="${ip##*::}"
    head_count="$(count_ipv6_groups "$head")" || return 1
    tail_count="$(count_ipv6_groups "$tail")" || return 1
    total_count=$((head_count + tail_count))
    ((total_count < 8)) || return 1
  else
    head_count="$(count_ipv6_groups "$ip")" || return 1
    ((head_count == 8)) || return 1
  fi

  return 0
}

url_host() {
  local ip="$1"
  if is_ipv6 "$ip"; then
    printf '[%s]' "$ip"
  else
    printf '%s' "$ip"
  fi
}

cluster_endpoint() {
  local ip="$1"
  printf 'https://%s:2380' "$(url_host "$ip")"
}

escape_sed_replacement() {
  printf '%s' "$1" | sed -e 's/[&|]/\\&/g'
}

AR_ROOT="${AR_ROOT:-/ar}"
AR_DATA_ROOT="${AR_DATA_ROOT:-/ar-data}"
PEM_DIR="${AR_ROOT}/scripts/gen-pem"
TEMPLATE_ETCD_CONFIG="${AR_ROOT}/confs/etcd.config.yml"
HOSTS_FILE="${AR_DATA_ROOT}/hosts"
PKI_DIR="${AR_DATA_ROOT}/pki"
CA_PEM="${PKI_DIR}/ca.pem"
CA_KEY_PEM="${PKI_DIR}/ca-key.pem"
ETCD_CERT_PREFIX="${PKI_DIR}/etcd"
CFSSL_BIN="${CFSSL_BIN:-/usr/local/bin/cfssl}"
CFSSLJSON_BIN="${CFSSLJSON_BIN:-/usr/local/bin/cfssljson}"

raw_input="${1:-}"
input_ips="$(printf '%s' "$raw_input" | tr -d '[:space:]')"
[[ -n "$input_ips" ]] || {
  usage
  die "missing etcd node IP list"
}
[[ "$input_ips" != ,* ]] || die "invalid ip list: starts with comma"
[[ "$input_ips" != *, ]] || die "invalid ip list: ends with comma"
[[ "$input_ips" != *,,* ]] || die "invalid ip list: empty item found"

[[ -d "$PEM_DIR" ]] || die "pem directory not found: $PEM_DIR"
[[ -f "${PEM_DIR}/ca-config.json" ]] || die "missing file: ${PEM_DIR}/ca-config.json"
[[ -f "${PEM_DIR}/etcd-csr.json" ]] || die "missing file: ${PEM_DIR}/etcd-csr.json"
[[ -f "$TEMPLATE_ETCD_CONFIG" ]] || die "missing template file: $TEMPLATE_ETCD_CONFIG"
[[ -f "$CA_PEM" ]] || die "missing CA cert: $CA_PEM"
[[ -f "$CA_KEY_PEM" ]] || die "missing CA key: $CA_KEY_PEM"
[[ -f "$CFSSL_BIN" ]] || die "missing cfssl binary: $CFSSL_BIN"
[[ -f "$CFSSLJSON_BIN" ]] || die "missing cfssljson binary: $CFSSLJSON_BIN"
chmod a+x "$CFSSL_BIN" "$CFSSLJSON_BIN"

mkdir -p "$AR_DATA_ROOT" "$PKI_DIR"

declare -a ETCD_IPS=()
declare -a ETCD_NAMES=()
declare -a INITIAL_CLUSTER_PARTS=()
declare -a SAN_HOST_PARTS=()

declare -A SEEN_IPS=()

IFS=',' read -r -a ip_candidates <<< "$input_ips"
for ip in "${ip_candidates[@]}"; do
  [[ -n "$ip" ]] || die "invalid ip list: empty item found"

  family=""
  if is_ipv4 "$ip"; then
    family="4"
    key="$ip"
  elif is_ipv6 "$ip"; then
    family="6"
    key="${ip,,}"
  else
    die "invalid IP address: $ip"
  fi

  [[ -z "${SEEN_IPS[$key]:-}" ]] || die "duplicate IP address: $ip"
  SEEN_IPS["$key"]=1

  index=$(( ${#ETCD_IPS[@]} + 1 ))
  printf -v node_name 'etcd-%02d' "$index"
  ETCD_IPS+=("$ip")
  ETCD_NAMES+=("$node_name")
  SAN_HOST_PARTS+=("$node_name")
  INITIAL_CLUSTER_PARTS+=("${node_name}=$(cluster_endpoint "$ip")")
done

[[ "${#ETCD_IPS[@]}" -gt 0 ]] || die "no valid etcd node IP addresses provided"

initial_cluster="$(IFS=,; echo "${INITIAL_CLUSTER_PARTS[*]}")"
san_hosts="127.0.0.1,::1,$(IFS=,; echo "${SAN_HOST_PARTS[*]}"),$(IFS=,; echo "${ETCD_IPS[*]}")"

echo "ETCD_IPS: $(IFS=,; echo "${ETCD_IPS[*]}")"
echo "ETCD_NAMES: $(IFS=,; echo "${ETCD_NAMES[*]}")"
echo "INITIAL_CLUSTER: $initial_cluster"
echo "CERT_SAN_HOSTS: $san_hosts"

{
  for i in "${!ETCD_IPS[@]}"; do
    printf '%s %s\n' "${ETCD_IPS[$i]}" "${ETCD_NAMES[$i]}"
  done
} > "$HOSTS_FILE"

echo "Generated hosts file: $HOSTS_FILE"
cat "$HOSTS_FILE"

cd "$PEM_DIR"
"$CFSSL_BIN" gencert \
  -ca="$CA_PEM" \
  -ca-key="$CA_KEY_PEM" \
  -config=ca-config.json \
  -hostname="$san_hosts" \
  -profile=kubernetes \
  etcd-csr.json | "$CFSSLJSON_BIN" -bare "$ETCD_CERT_PREFIX"

echo "Generated certificates under: $PKI_DIR"
ls -l "$PKI_DIR"

for i in "${!ETCD_IPS[@]}"; do
  ip="${ETCD_IPS[$i]}"
  node_name="${ETCD_NAMES[$i]}"
  node_dir="${AR_DATA_ROOT}/${ip}"
  target_config="${node_dir}/etcd.config.yml"
  listen_peer_url="https://$(url_host "$ip"):2380"
  listen_client_urls="https://$(url_host "$ip"):2379,http://127.0.0.1:2379"
  initial_advertise_peer_url="https://$(url_host "$ip"):2380"
  advertise_client_urls="https://$(url_host "$ip"):2379"

  mkdir -p "$node_dir"
  cp "$TEMPLATE_ETCD_CONFIG" "$target_config"

  esc_node_name="$(escape_sed_replacement "$node_name")"
  esc_listen_peer_url="$(escape_sed_replacement "$listen_peer_url")"
  esc_listen_client_urls="$(escape_sed_replacement "$listen_client_urls")"
  esc_initial_advertise_peer_url="$(escape_sed_replacement "$initial_advertise_peer_url")"
  esc_advertise_client_urls="$(escape_sed_replacement "$advertise_client_urls")"
  esc_initial_cluster="$(escape_sed_replacement "$initial_cluster")"

  sed -i \
    -e "s|^name:.*$|name: '${esc_node_name}'|" \
    -e "s|^listen-peer-urls:.*$|listen-peer-urls: '${esc_listen_peer_url}'|" \
    -e "s|^listen-client-urls:.*$|listen-client-urls: '${esc_listen_client_urls}'|" \
    -e "s|^initial-advertise-peer-urls:.*$|initial-advertise-peer-urls: '${esc_initial_advertise_peer_url}'|" \
    -e "s|^advertise-client-urls:.*$|advertise-client-urls: '${esc_advertise_client_urls}'|" \
    -e "s|^initial-cluster:.*$|initial-cluster: '${esc_initial_cluster}'|" \
    "$target_config"

  echo "Generated config: $target_config"
done
