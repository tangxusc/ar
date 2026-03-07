#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source "${ROOT_DIR}/metadata/versions.lock"

declare -a NODE_IPS=()
declare -a NODE_PORTS=()
declare -a NODE_USERS=()
declare -a NODE_PASSWORDS=()
declare -a NODE_LABELS=()

PIPELINE_WORK_DIR="${PIPELINE_WORK_DIR:-/opt/ar}"

parse_nodes() {
  local raw="${NODES_MATRIX:-}"
  NODE_IPS=()
  NODE_PORTS=()
  NODE_USERS=()
  NODE_PASSWORDS=()
  NODE_LABELS=()
  IFS=';' read -r -a records <<< "${raw}"
  for record in "${records[@]}"; do
    [[ -z "${record}" ]] && continue
    IFS='|' read -r ip port user pass labels <<< "${record}"
    NODE_IPS+=("${ip}")
    NODE_PORTS+=("${port:-22}")
    NODE_USERS+=("${user}")
    NODE_PASSWORDS+=("${pass}")
    NODE_LABELS+=("${labels}")
  done
}

label_has() {
  local labels="$1"
  local key="$2"
  local value="${3:-}"
  if [[ -z "${value}" ]]; then
    [[ ",${labels}," == *",${key},"* || ",${labels}," == *",${key}=true,"* ]]
  else
    [[ ",${labels}," == *",${key}=${value},"* ]]
  fi
}

role_of() {
  local labels="$1"
  if label_has "${labels}" "role" "master"; then
    echo "master"
  elif label_has "${labels}" "role" "worker"; then
    echo "worker"
  else
    echo ""
  fi
}

first_master_index() {
  local i
  for i in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$i]}")" == "master" ]]; then
      echo "${i}"
      return 0
    fi
  done
  return 1
}

registry_indexes() {
  local i found=0
  for i in "${!NODE_IPS[@]}"; do
    if label_has "${NODE_LABELS[$i]}" "registry" "true"; then
      echo "${i}"
      found=1
    fi
  done
  return $(( ! found ))
}

registry_endpoints() {
  local idx first=1
  while read -r idx; do
    [[ -z "${idx}" ]] && continue
    if [[ "${first}" -eq 0 ]]; then
      printf ","
    fi
    printf "http://%s:5000" "${NODE_IPS[$idx]}"
    first=0
  done < <(registry_indexes || true)
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing command: $1" >&2
    exit 1
  }
}

remote_exec() {
  local idx="$1"
  shift
  local command="$*"
  local -a args=(
    -p "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "sudo bash -lc $(printf '%q' "${command}")"
}

remote_copy() {
  local idx="$1"
  local src="$2"
  local dst="$3"
  local src_base tmp_dir tmp_src remote_cmd
  local -a copy_args=()
  local -a scp_args=(
    -P "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  local -a ssh_args=(
    -p "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  src_base="$(basename "${src}")"
  tmp_dir="/tmp/ar-copy-${RANDOM}-$$"
  tmp_src="${tmp_dir}/${src_base}"
  remote_cmd="mkdir -p $(printf '%q' "${tmp_dir}")"
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${ssh_args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "${remote_cmd}"

  if [[ -d "${src}" ]]; then
    copy_args+=(-r)
  fi
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e scp "${copy_args[@]}" "${scp_args[@]}" "${src}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}:${tmp_dir}/"

  remote_cmd=$(
    cat <<EOF
if [[ ${dst@Q} == */ ]]; then
  sudo mkdir -p ${dst@Q}
  sudo mv ${tmp_src@Q} ${dst@Q}
else
  sudo mkdir -p \$(dirname ${dst@Q})
  sudo mv ${tmp_src@Q} ${dst@Q}
fi
rm -rf ${tmp_dir@Q}
EOF
  )
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${ssh_args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "bash -lc $(printf '%q' "${remote_cmd}")"
}

remote_fetch() {
  local idx="$1"
  local src="$2"
  local dst="$3"
  local src_base tmp_dir tmp_src remote_cmd
  local -a scp_args=(
    -P "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  local -a ssh_args=(
    -p "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  src_base="$(basename "${src}")"
  tmp_dir="/tmp/ar-fetch-${RANDOM}-$$"
  tmp_src="${tmp_dir}/${src_base}"
  mkdir -p "$(dirname "${dst}")"

  remote_cmd=$(
    cat <<EOF
set -e
sudo mkdir -p ${tmp_dir@Q}
sudo cp -a ${src@Q} ${tmp_src@Q}
sudo chmod a+r ${tmp_src@Q}
EOF
  )
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${ssh_args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "bash -lc $(printf '%q' "${remote_cmd}")"

  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e scp "${scp_args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}:${tmp_src}" "${dst}"

  remote_cmd="sudo rm -rf $(printf '%q' "${tmp_dir}")"
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${ssh_args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "${remote_cmd}"
}

remote_write() {
  local idx="$1"
  local dest="$2"
  local content="$3"
  local -a args=(
    -p "${NODE_PORTS[$idx]}"
    -o StrictHostKeyChecking=no
    -o UserKnownHostsFile=/dev/null
  )
  SSHPASS="${NODE_PASSWORDS[$idx]}" sshpass -e ssh "${args[@]}" "${NODE_USERS[$idx]}@${NODE_IPS[$idx]}" "sudo tee $(printf '%q' "${dest}") >/dev/null" <<< "${content}"
}

master_ips_csv() {
  local i first=1
  for i in "${!NODE_IPS[@]}"; do
    if [[ "$(role_of "${NODE_LABELS[$i]}")" != "master" ]]; then
      continue
    fi
    if [[ "${first}" -eq 0 ]]; then
      printf ","
    fi
    printf "%s" "${NODE_IPS[$i]}"
    first=0
  done
}

render_template() {
  local template_path="$1"
  envsubst < "${template_path}"
}

write_report() {
  local name="$1"
  shift
  mkdir -p /tasks/reports
  printf '%s\n' "$@" > "/tasks/reports/${name}"
}

write_current_summary() {
  local summary="$1"
  printf '%s\n' "${summary}" > /current-task/summary.txt
}

extract_registry_mirrors() {
  local endpoints
  endpoints="$(registry_endpoints)"
  IFS=',' read -r -a MIRRORS <<< "${endpoints}"
}
