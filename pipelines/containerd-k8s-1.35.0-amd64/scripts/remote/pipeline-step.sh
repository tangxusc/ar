#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"
source "${SCRIPT_DIR}/actions.sh"

main() {
  mkdir -p /tasks/cluster/
  mkdir -p /tasks/inventory/
  mkdir -p /tasks/rendered/
  mkdir -p /tasks/pki/
  mkdir -p /tasks/kubeconfig/
  mkdir -p /tasks/bootstrap/
  mkdir -p /tasks/reports/
  mkdir -p /tasks/diagnostics/
  parse_nodes
  require_command bash
  [[ -n "${ACTION:-}" ]] || {
    echo "ACTION is required" >&2
    exit 1
  }
  dispatch_action "${ACTION}"
}

main "$@"