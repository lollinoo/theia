#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CALL_LOG="$(mktemp)"
BODY_LOG="$(mktemp)"
MOCK_BIN="$(mktemp -d)"
trap 'rm -rf "$MOCK_BIN"; rm -f "$CALL_LOG" "$BODY_LOG"' EXIT

cat >"$MOCK_BIN/curl" <<'MOCK_CURL'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$THEIA_TEST_CALL_LOG"

method="GET"
url=""
body_file=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -X)
      method="$2"
      shift 2
      ;;
    -d|--data|--data-binary)
      body_file="$2"
      shift 2
      ;;
    -H|-b|-c|-w)
      shift 2
      ;;
    -s|-S|-f|-sS|-sf)
      shift
      ;;
    *)
      url="$1"
      shift
      ;;
  esac
done

case "$method $url" in
  "GET http://unit.test/api/v1/devices")
    cat <<'JSON'
{"data":[
  {"id":"device-loopback","attributes":{"hostname":"wisp-core-01","ip":"127.0.10.21","tags":{"lab":"wisp-ospf","role":"core"}}},
  {"id":"device-unrelated","attributes":{"hostname":"wisp-core-01","ip":"192.0.2.10","tags":{"lab":"customer"}}}
]}
JSON
    ;;
  "PUT http://unit.test/api/v1/devices/device-loopback")
    if [ -n "$body_file" ]; then
      printf '%s\n' "$body_file" >"$THEIA_TEST_BODY_LOG"
    fi
    printf '{"data":{"id":"device-loopback"}}\n'
    ;;
  *)
    printf 'unexpected curl call: %s %s\n' "$method" "$url" >&2
    exit 22
    ;;
esac
MOCK_CURL
chmod +x "$MOCK_BIN/curl"

source "$ROOT_DIR/scripts/seed-primary-map.sh"

ensure_theia_api_session() {
  return 0
}

API_BASE="http://unit.test"
THEIA_CURL_AUTH_ARGS=(-H "X-CSRF-Token: unit-token" -b "unit-cookie")
export THEIA_TEST_CALL_LOG="$CALL_LOG"
export THEIA_TEST_BODY_LOG="$BODY_LOG"

device_id="$(PATH="$MOCK_BIN:$PATH" device_id_by_hostname_and_tag "wisp-core-01" "lab" "wisp-ospf")"
if [ "$device_id" != "device-loopback" ]; then
  echo "expected WISP lab device id from hostname/tag lookup, got '$device_id'" >&2
  exit 1
fi

PATH="$MOCK_BIN:$PATH" update_device_ip "device-loopback" "172.31.250.21"

if ! python3 - "$BODY_LOG" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    payload = json.load(handle)

raise SystemExit(0 if payload == {"ip": "172.31.250.21"} else 1)
PY
then
  echo "update_device_ip must send only the target IP payload" >&2
  cat "$BODY_LOG" >&2
  exit 1
fi

calls="$(cat "$CALL_LOG")"
if [[ "$calls" != *"PUT http://unit.test/api/v1/devices/device-loopback"* ]]; then
  echo "update_device_ip must call the device update endpoint" >&2
  printf '%s\n' "$calls" >&2
  exit 1
fi

for seed_file in "$ROOT_DIR/scripts/seed-wisp.sh" "$ROOT_DIR/scripts/seed-wisp-radio.sh"; do
  content="$(cat "$seed_file")"
  if [[ "$content" != *'device_id_by_hostname_and_tag "$hostname" "lab" "wisp-ospf"'* ]]; then
    echo "$seed_file must look up existing WISP lab devices by hostname and lab tag before creating duplicates" >&2
    exit 1
  fi
  if [[ "$content" != *'update_device_ip "$existing_id" "$ip"'* ]]; then
    echo "$seed_file must migrate existing WISP lab devices to the selected target IP" >&2
    exit 1
  fi
done

echo "WISP seed IP migration helpers are valid"
