#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib.sh"
source "${SCRIPT_DIR}/actions.sh"

main() {
  parse_nodes
  require_command bash
  [[ -n "${ACTION:-}" ]] || {
    echo "ACTION is required" >&2
    exit 1
  }
  dispatch_action "${ACTION}"
}

main "$@"