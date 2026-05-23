#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CALL_LOG="$(mktemp)"
STDOUT_LOG="$(mktemp)"
STDERR_LOG="$(mktemp)"
MOCK_BIN="$(mktemp -d)"
trap 'rm -rf "$MOCK_BIN"; rm -f "$CALL_LOG" "$STDOUT_LOG" "$STDERR_LOG"' EXIT

cat >"$MOCK_BIN/curl" <<'MOCK_CURL'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$THEIA_TEST_CALL_LOG"

case " $* " in
  *"/api/v1/auth/me"*)
    printf '{"authenticated":false}\n'
    exit 0
    ;;
  *)
    exit 22
    ;;
esac
MOCK_CURL
chmod +x "$MOCK_BIN/curl"

unset THEIA_API_PASSWORD
unset THEIA_API_USERNAME
unset THEIA_API_NEW_PASSWORD
export THEIA_TEST_CALL_LOG="$CALL_LOG"

set +e
PATH="$MOCK_BIN:$PATH" bash "$ROOT_DIR/scripts/seed-wisp.sh" http://localhost:8080 host \
  >"$STDOUT_LOG" 2>"$STDERR_LOG" </dev/null
status=$?
set -e

stdout="$(cat "$STDOUT_LOG")"
stderr="$(cat "$STDERR_LOG")"
calls="$(cat "$CALL_LOG")"

if [ "$status" -eq 0 ]; then
  echo "seed-wisp.sh should fail when THEIA_API_PASSWORD is missing in non-interactive mode" >&2
  exit 1
fi

if [[ "$stderr" != *"Theia seed scripts need a password-session login. Set THEIA_API_PASSWORD for local automation or run interactively."* ]]; then
  echo "missing-password failure should explain how to provide credentials" >&2
  printf 'stderr:\n%s\n' "$stderr" >&2
  exit 1
fi

if [[ "$stdout" == *"Adding wisp-core-01"* ]]; then
  echo "seed-wisp.sh should fail before starting device creation when credentials are missing" >&2
  printf 'stdout:\n%s\n' "$stdout" >&2
  exit 1
fi

if [[ "$stderr" == *"JSONDecodeError"* ]] || [[ "$stderr" == *"Traceback"* ]]; then
  echo "missing-password failure should not continue into JSON parsing" >&2
  printf 'stderr:\n%s\n' "$stderr" >&2
  exit 1
fi

if [[ "$calls" == *"/api/v1/auth/login"* ]] || [[ "$calls" == *"/api/v1/devices"* ]]; then
  echo "seed-wisp.sh should not call authenticated API endpoints without credentials" >&2
  printf 'calls:\n%s\n' "$calls" >&2
  exit 1
fi

echo "WISP shell seed auth failure is clean"
