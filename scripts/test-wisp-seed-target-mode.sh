#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/wisp-lab-common.sh"

fail() {
  echo "$1" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"

  if [ "$expected" != "$actual" ]; then
    fail "$message Expected '$expected', got '$actual'."
  fi
}

assert_contains() {
  local needle="$1"
  local haystack="$2"
  local message="$3"

  if [[ "$haystack" != *"$needle"* ]]; then
    fail "$message Expected log to contain '$needle', got '$haystack'."
  fi
}

reset_wisp_seed_target_test_state() {
  MOCK_BACKEND_RUNNING="${1:-1}"
  MOCK_CONNECT_SUCCEEDS="${2:-1}"
  CONNECT_ATTEMPTS=0
  BACKEND_RUNNING_CHECKS=0
  LAST_PREFIX=""
  LAST_LOG=""
}

wisp_backend_running() {
  BACKEND_RUNNING_CHECKS=$((BACKEND_RUNNING_CHECKS + 1))
  [ "$MOCK_BACKEND_RUNNING" = "1" ]
}

wisp_connect_backend_to_lab_network() {
  CONNECT_ATTEMPTS=$((CONNECT_ATTEMPTS + 1))
  [ "$MOCK_CONNECT_SUCCEEDS" = "1" ]
}

run_wisp_seed_target_prefix() {
  local target_mode="$1"
  local api_base="$2"
  local stdout_file
  local stderr_file

  stdout_file="$(mktemp)"
  stderr_file="$(mktemp)"
  if ! wisp_seed_target_prefix "$target_mode" "$api_base" >"$stdout_file" 2>"$stderr_file"; then
    cat "$stderr_file" >&2
    rm -f "$stdout_file" "$stderr_file"
    fail "wisp_seed_target_prefix failed for mode '$target_mode' and API base '$api_base'."
  fi

  LAST_PREFIX="$(cat "$stdout_file")"
  LAST_LOG="$(cat "$stderr_file")"
  rm -f "$stdout_file" "$stderr_file"
}

reset_wisp_seed_target_test_state
run_wisp_seed_target_prefix "auto" "http://localhost:8080"
assert_eq "172.31.250." "$LAST_PREFIX" "auto mode should use Docker management targets for localhost API base when the backend container is running."
assert_eq "1" "$CONNECT_ATTEMPTS" "auto mode should connect the backend container before selecting Docker targets for localhost API base."
assert_contains "auto: backend container is running and connected" "$LAST_LOG" "auto mode should log the Docker decision for localhost API base."

reset_wisp_seed_target_test_state 0 0
run_wisp_seed_target_prefix "auto" "http://localhost:8080"
assert_eq "127.0.10." "$LAST_PREFIX" "auto mode should fall back to host loopback targets for localhost API base when Docker backend is unavailable."
assert_eq "0" "$CONNECT_ATTEMPTS" "auto mode should not attempt Docker connect when the backend container is not running."
assert_contains "auto: Docker backend unavailable for API host 'localhost'" "$LAST_LOG" "auto mode should log the localhost fallback decision."

reset_wisp_seed_target_test_state
run_wisp_seed_target_prefix "docker" "http://localhost:8080"
assert_eq "172.31.250." "$LAST_PREFIX" "docker mode should keep Docker management targets for localhost API base."
assert_eq "1" "$CONNECT_ATTEMPTS" "docker mode should still attempt to connect the backend container."
assert_contains "mode: docker" "$LAST_LOG" "docker mode should log the explicit docker decision."

reset_wisp_seed_target_test_state
run_wisp_seed_target_prefix "host" "http://localhost:8080"
assert_eq "127.0.10." "$LAST_PREFIX" "host mode should use host loopback targets."
assert_eq "0" "$CONNECT_ATTEMPTS" "host mode should not attempt to connect the backend container."
assert_eq "0" "$BACKEND_RUNNING_CHECKS" "host mode should not check backend container state."
assert_contains "mode: host" "$LAST_LOG" "host mode should log the explicit host decision."

reset_wisp_seed_target_test_state
run_wisp_seed_target_prefix "auto" "http://theia-backend:8080"
assert_eq "172.31.250." "$LAST_PREFIX" "auto mode should keep Docker targets for non-localhost API base when the backend can connect."
assert_eq "1" "$CONNECT_ATTEMPTS" "auto mode should attempt Docker connect for non-localhost API base."
assert_contains "auto: backend container is running and connected" "$LAST_LOG" "auto mode should log the Docker decision for non-local API base."

reset_wisp_seed_target_test_state 1 0
run_wisp_seed_target_prefix "auto" "http://theia-backend:8080"
assert_eq "127.0.10." "$LAST_PREFIX" "auto mode should fall back to host targets for non-localhost API base when Docker connect is unavailable."
assert_eq "1" "$CONNECT_ATTEMPTS" "auto mode should attempt Docker connect before falling back for non-localhost API base."
assert_contains "auto: Docker backend unavailable for API host 'theia-backend'" "$LAST_LOG" "auto mode should log the non-local fallback decision."

echo "WISP shell seed target mode behavior is valid"
