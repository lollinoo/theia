#!/usr/bin/env bash
set -euo pipefail

# Builds WinBox bridge binaries and deployment bundles and uploads them to GitHub Releases.

if [[ $# -ne 1 || -z "${1-}" ]]; then
  echo "Usage: release-assets.sh <tag>" >&2
  exit 1
fi

tag="$1"

: "${GH_TOKEN:?GH_TOKEN is required}"
: "${GH_REPO:?GH_REPO is required}"

for cmd in go make gh; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "release-assets.sh: required command '$cmd' not found" >&2
    exit 1
  fi
done

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"

cd "$repo_dir"

make bridge-build-all

bridge_files=(
  bridge_binaries/winbox-bridge-windows-amd64.exe
  bridge_binaries/winbox-bridge-windows-arm64.exe
  bridge_binaries/winbox-bridge-linux-amd64
  bridge_binaries/winbox-bridge-linux-arm64
)
for file in "${bridge_files[@]}"; do
  if [[ ! -f "$file" ]]; then
    echo "release-assets.sh: missing expected bridge binary: $file" >&2
    exit 1
  fi
done

gh release upload "$tag" "${bridge_files[@]}" --clobber

rm -rf dist/deployment
bash scripts/build-deployment-bundles.sh "$tag"
(
  cd dist/deployment
  sha256sum --check SHA256SUMS
  gh release upload "$tag" ./*.tar.gz SHA256SUMS --clobber
)
