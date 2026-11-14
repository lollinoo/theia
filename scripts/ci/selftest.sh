#!/usr/bin/env bash
set -euo pipefail

# Fast offline self-test for CI helpers, path mapping, and GHA/Woodpecker parity.
# Runs with bash + grep/sed only (safe on alpine:3.24 + bash).

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"

# shellcheck source=scripts/ci/lib.sh
source "$script_dir/lib.sh"

failures=0

fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

pass() {
  echo "PASS: $*"
}

assert_changes() {
  local label="$1"
  local expected_backend="$2"
  local expected_frontend="$3"
  local expected_integration="$4"
  shift 4

  local actual expected
  expected="$(printf 'backend=%s\nfrontend=%s\nintegration=%s' "$expected_backend" "$expected_frontend" "$expected_integration")"
  if [[ "${1-}" == "--all" ]]; then
    actual="$("$script_dir/changes.sh" --all)"
  else
    actual="$(printf '%b' "${1-}" | "$script_dir/changes.sh")"
  fi

  if [[ "$actual" == "$expected" ]]; then
    pass "changes.sh ($label)"
  else
    fail "changes.sh ($label): expected [$expected], got [$actual]"
  fi
}

assert_sanitize_ok() {
  local raw="$1"
  local expected="$2"
  local actual

  if actual="$(sanitize_tag "$raw" 2>/dev/null)" && [[ "$actual" == "$expected" ]]; then
    pass "sanitize_tag ('$raw' -> '$actual')"
  else
    fail "sanitize_tag ('$raw'): expected '$expected', got '${actual:-<error>}'"
  fi
}

assert_sanitize_fail() {
  local label="$1"
  local raw="$2"
  local actual

  if actual="$(sanitize_tag "$raw" 2>/dev/null)"; then
    fail "sanitize_tag ($label): expected failure, got '$actual'"
  else
    pass "sanitize_tag ($label fails as expected)"
  fi
}

# --- 1. changes.sh mapping ---
assert_changes "backend-only" true false true "internal/api/router.go\n"
assert_changes "frontend-only" false true true "frontend/src/App.tsx\n"
assert_changes "docs-only" false false false "README.md\n"
assert_changes "ci-gha-file" true true true ".github/workflows/ci.yml\n"
assert_changes "ci-woodpecker-file" true true true ".woodpecker/woodpecker.yml\n"
assert_changes "ci-script-file" true true true "scripts/ci/lib.sh\n"
assert_changes "empty-stdin" true true true ""
assert_changes "mixed-paths" true true true "cmd/theia/main.go\nfrontend/package.json\n"
assert_changes "--all" true true true --all

# --- 2. sanitize_tag ---
assert_sanitize_ok "V1.8.0" "v1.8.0"
assert_sanitize_ok "feature/Foo/Bar" "feature-foo-bar"
assert_sanitize_ok "---v1.2.3---" "v1.2.3"
assert_sanitize_ok "--foo/bar--" "foo-bar"

printf -v tag_128 '%*s' 128 ''
tag_128="${tag_128// /a}"
assert_sanitize_ok "$tag_128" "$tag_128"

assert_sanitize_fail "empty" ""
assert_sanitize_fail "only-dashes" "---"
assert_sanitize_fail "only-slashes" "///"
assert_sanitize_fail "over-128-chars" "${tag_128}a"

# --- 3. Script reference parity ---
gha_yml="$repo_dir/.github/workflows/ci.yml"
wp_yml="$repo_dir/.woodpecker/woodpecker.yml"

check_ref_in_both() {
  local rel_path="$1"
  local ok=true

  if ! grep -v '^[[:space:]]*#' "$gha_yml" | grep -F "$rel_path" >/dev/null; then
    fail "parity: '$rel_path' not referenced in .github/workflows/ci.yml"
    ok=false
  fi
  if ! grep -v '^[[:space:]]*#' "$wp_yml" | grep -F "$rel_path" >/dev/null; then
    fail "parity: '$rel_path' not referenced in .woodpecker/woodpecker.yml"
    ok=false
  fi
  if [[ "$ok" == "true" ]]; then
    pass "parity: '$rel_path' referenced in both workflows"
  fi
}

for script in "$script_dir"/*.sh; do
  name="$(basename "$script")"
  if [[ "$name" == "lib.sh" || "$name" == "selftest.sh" ]]; then
    continue
  fi
  check_ref_in_both "scripts/ci/$name"
done
check_ref_in_both "scripts/test-managed-ci.sh"

# --- 4. Toolchain parity ---
go_mod="$repo_dir/go.mod"
deploy_df="$repo_dir/Dockerfile.deployment-test"

go_toolchain="$(sed -n -E 's/^toolchain[[:space:]]+go([0-9]+\.[0-9]+(\.[0-9]+)?)[[:space:]]*$/\1/p' "$go_mod" | head -n 1 || true)"
if [[ -z "$go_toolchain" ]]; then
  fail "toolchain: could not read 'toolchain go<ver>' from go.mod"
else
  pass "toolchain: go.mod specifies go${go_toolchain}"

  for target_file in "$wp_yml" "$deploy_df"; do
    rel_target="${target_file#"$repo_dir/"}"
    go_images="$(grep -v '^[[:space:]]*#' "$target_file" | grep -Eo 'golang:[^[:space:]"'\'']+' || true)"
    if [[ -z "$go_images" ]]; then
      fail "toolchain: no golang:<ver>-... image found in $rel_target"
      continue
    fi

    file_ok=true
    while IFS= read -r img; do
      [[ -z "$img" ]] && continue
      ver="${img#golang:}"
      ver="${ver%%-*}"
      if [[ "$ver" != "$go_toolchain" ]]; then
        fail "toolchain: $rel_target image '$img' ($ver) != go.mod toolchain ($go_toolchain)"
        file_ok=false
      fi
    done <<< "$go_images"

    if [[ "$file_ok" == "true" ]]; then
      pass "toolchain: all golang images in $rel_target match go${go_toolchain}"
    fi
  done
fi

gha_node_versions="$(grep -v '^[[:space:]]*#' "$gha_yml" | sed -n -E 's/^[[:space:]]*node-version:[[:space:]]*["'\'']?([^"'\''#[:space:]]+)["'\'']?.*$/\1/p' || true)"
wp_node_images="$(grep -v '^[[:space:]]*#' "$wp_yml" | grep -Eo 'node:[^[:space:]"'\'']+' || true)"

if [[ -z "$gha_node_versions" ]]; then
  fail "toolchain: no node-version found in .github/workflows/ci.yml"
elif [[ -z "$wp_node_images" ]]; then
  fail "toolchain: no node:<major>-... image found in .woodpecker/woodpecker.yml"
else
  node_ok=true
  while IFS= read -r gha_node; do
    [[ -z "$gha_node" ]] && continue
    while IFS= read -r wp_img; do
      [[ -z "$wp_img" ]] && continue
      wp_node="${wp_img#node:}"
      wp_node="${wp_node%%-*}"
      if [[ "$wp_node" != "$gha_node" ]]; then
        fail "toolchain: Woodpecker '$wp_img' ($wp_node) != GHA node-version '$gha_node'"
        node_ok=false
      fi
    done <<< "$wp_node_images"
  done <<< "$gha_node_versions"

  if [[ "$node_ok" == "true" ]]; then
    pass "toolchain: Woodpecker node images match GHA node-version"
  fi
fi

# --- 5. Fail-fast argument and env validation (no stubs) ---
assert_fails() {
  local label="$1"
  shift
  if "$@" >/dev/null 2>&1; then
    fail "$label: expected failure"
  else
    pass "$label (fails as expected)"
  fi
}

assert_fails "publish-images.sh (missing tags)" env GHCR_USERNAME=usr GHCR_TOKEN=tok bash "$script_dir/publish-images.sh" linux/amd64
assert_fails "publish-images.sh (missing GHCR_USERNAME)" env -u GHCR_USERNAME GHCR_TOKEN=tok bash "$script_dir/publish-images.sh" linux/amd64 master
assert_fails "publish-images.sh (missing GHCR_TOKEN)" env -u GHCR_TOKEN GHCR_USERNAME=usr bash "$script_dir/publish-images.sh" linux/amd64 master
assert_fails "release-assets.sh (no args)" env GH_TOKEN=tok GH_REPO=o/r bash "$script_dir/release-assets.sh"
assert_fails "release-assets.sh (extra arg)" env GH_TOKEN=tok GH_REPO=o/r bash "$script_dir/release-assets.sh" v1.0.0 false
assert_fails "release-assets.sh (missing GH_TOKEN)" env -u GH_TOKEN GH_REPO=o/r bash "$script_dir/release-assets.sh" v1.0.0
assert_fails "release-assets.sh (missing GH_REPO)" env -u GH_REPO GH_TOKEN=tok bash "$script_dir/release-assets.sh" v1.0.0
assert_fails "backend-fast.sh (missing THEIA_TEST_DB_DSN)" env -u THEIA_TEST_DB_DSN bash "$script_dir/backend-fast.sh"
assert_fails "browser-e2e.sh (missing THEIA_E2E_DB_DSN)" env -u THEIA_E2E_DB_DSN bash "$script_dir/browser-e2e.sh"

if (( failures > 0 )); then
  echo "selftest: $failures check(s) failed" >&2
  exit 1
fi

echo "selftest: all checks passed"
