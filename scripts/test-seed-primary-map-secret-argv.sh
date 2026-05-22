#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CAPTURED_ARGS="$(mktemp)"
trap 'rm -f "$CAPTURED_ARGS"' EXIT

python3() {
  printf '%s\n' "$*" >>"$CAPTURED_ARGS"
  cat >/dev/null
  printf '{}\n'
}

source "$SCRIPT_DIR/seed-primary-map.sh"

theia_login_payload administrator argv-leak-sentinel-login >/dev/null
theia_password_change_payload argv-leak-sentinel-current argv-leak-sentinel-new >/dev/null

if grep -Fq "argv-leak-sentinel-login" "$CAPTURED_ARGS"; then
  echo "login password leaked through python argv" >&2
  exit 1
fi
if grep -Fq "argv-leak-sentinel-current" "$CAPTURED_ARGS"; then
  echo "current password leaked through python argv" >&2
  exit 1
fi
if grep -Fq "argv-leak-sentinel-new" "$CAPTURED_ARGS"; then
  echo "new password leaked through python argv" >&2
  exit 1
fi

echo "seed primary map helpers do not pass secrets through python argv"
