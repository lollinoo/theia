#!/usr/bin/env bash
set -euo pipefail
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
bundle_release=${1:?Usage: build-deployment-bundles.sh v1.8.0 [output-directory]}
bundle_output=${2:-"$repo_dir/dist/deployment"}
[[ "$bundle_release" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || { echo 'Release must be a Docker-compatible semantic version' >&2; exit 1; }
mkdir -p "$bundle_output"
bundle_output=$(cd "$bundle_output" && pwd)
bundle_work=$(mktemp -d)
trap 'rm -rf "$bundle_work"' EXIT
cd "$repo_dir"
for bundle_arch in amd64 arm64; do
  bundle_name="theia-$bundle_release-linux-$bundle_arch"
  bundle_dir="$bundle_work/$bundle_name"
  mkdir -p "$bundle_dir"
  GOOS=linux GOARCH="$bundle_arch" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.Version=$bundle_release" -o "$bundle_dir/theia-admin" ./cmd/theia-admin
  cp OPERATIONS.md "$bundle_dir/OPERATIONS.md"
  cp -R internal/deployment/chart "$bundle_dir/chart"
  tar -C "$bundle_work" -czf "$bundle_output/$bundle_name.tar.gz" "$bundle_name"
done
cd "$bundle_output"
sha256sum "theia-$bundle_release-linux-amd64.tar.gz" "theia-$bundle_release-linux-arm64.tar.gz" > SHA256SUMS
