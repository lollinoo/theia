#!/usr/bin/env bash
set -euo pipefail

# Builds and pushes backend and frontend production images to GHCR with registry buildcache.

if [[ $# -lt 2 || -z "${1-}" ]]; then
  echo "Usage: publish-images.sh <platforms> <tag> [<tag>...]" >&2
  exit 1
fi

: "${GHCR_USERNAME:?GHCR_USERNAME is required}"
: "${GHCR_TOKEN:?GHCR_TOKEN is required}"

platforms="$1"
shift

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"

# shellcheck source=scripts/ci/lib.sh
source "$script_dir/lib.sh"

cd "$repo_dir"

sanitized_tags=()
for raw_tag in "$@"; do
  sanitized_tags+=("$(sanitize_tag "$raw_tag")")
done

printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USERNAME" --password-stdin

if ! docker buildx inspect theia >/dev/null 2>&1; then
  docker buildx create --name theia --driver docker-container --use
fi
docker buildx use theia

if [[ "$platforms" == *arm64* ]]; then
  docker run --privileged --rm tonistiigi/binfmt --install arm64
fi

build_and_push_image() {
  local name="$1"
  local dockerfile="$2"
  local context="$3"
  local cache_ref="ghcr.io/lollinoo/theia-${name}:buildcache"
  local args=(
    --push
    --platform "$platforms"
    --provenance=false
    --sbom=false
    --file "$dockerfile"
    --target production
    --cache-from "type=registry,ref=${cache_ref}"
    --cache-to "type=registry,ref=${cache_ref},mode=max"
  )
  local tag

  for tag in "${sanitized_tags[@]}"; do
    args+=(--tag "ghcr.io/lollinoo/theia-${name}:${tag}")
  done
  if [[ -n "${IMAGE_SOURCE:-}" ]]; then
    args+=(--label "org.opencontainers.image.source=${IMAGE_SOURCE}")
  fi
  if [[ -n "${IMAGE_REVISION:-}" ]]; then
    args+=(--label "org.opencontainers.image.revision=${IMAGE_REVISION}")
  fi

  docker buildx build "${args[@]}" "$context"
}

build_and_push_image backend Dockerfile .
build_and_push_image frontend Dockerfile.frontend ./frontend
