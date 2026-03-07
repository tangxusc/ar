#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ARTIFACTS_DIR="${ROOT_DIR}/steps/base/artifacts"
MODE="${1:-verify}"

verify() {
  test -f "${ROOT_DIR}/metadata/versions.lock"
  test -d "${ARTIFACTS_DIR}"
  test -d "${ROOT_DIR}/scripts/remote"
  test -d "${ROOT_DIR}/templates"
}

package() {
  ROOT_DIR_ENV="${ROOT_DIR}" python3 - <<'PY'
import json
import os
from pathlib import Path

root = Path(os.environ["ROOT_DIR_ENV"])
artifacts = root / "steps" / "base" / "artifacts"
manifest_path = root / "metadata" / "manifest.json"
if manifest_path.exists():
    manifest = json.loads(manifest_path.read_text())
else:
    manifest = {"version": 1, "artifacts": [], "images": []}
known_artifacts = {item.get("path"): item for item in manifest.get("artifacts", []) if item.get("path")}
known_images = {item.get("path"): item for item in manifest.get("images", []) if item.get("path")}
artifacts_out = []
images_out = []
for path in sorted(artifacts.rglob("*")):
    if not path.is_file():
        continue
    rel = path.relative_to(root).as_posix()
    entry = {"path": rel}
    if "images/" in rel:
        merged = dict(known_images.get(rel, {}))
        merged.update(entry)
        images_out.append(merged)
    else:
        merged = dict(known_artifacts.get(rel, {}))
        merged.update(entry)
        artifacts_out.append(merged)
manifest["artifacts"] = artifacts_out
manifest["images"] = images_out
manifest_path.write_text(json.dumps(manifest, indent=2) + "\n")
PY
}

case "${MODE}" in
  verify)
    verify
    ;;
  package)
    verify
    package
    ;;
  *)
    echo "usage: $0 [verify|package]" >&2
    exit 1
    ;;
esac
