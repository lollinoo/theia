theia_seed_interactive() {
  [ -t 0 ] && [ -t 1 ]
}

theia_login_payload() {
  local username="$1"
  local password="$2"
  printf '%s\0%s' "$username" "$password" | python3 -c '
import json
import sys

username, password = sys.stdin.buffer.read().decode().split("\0", 1)
print(json.dumps({"identifier": username, "password": password}))
'
}

theia_password_change_payload() {
  local current_password="$1"
  local new_password="$2"
  printf '%s\0%s' "$current_password" "$new_password" | python3 -c '
import json
import sys

current_password, new_password = sys.stdin.buffer.read().decode().split("\0", 1)
print(json.dumps({"current_password": current_password, "new_password": new_password}))
'
}

theia_cookie_value() {
  local name="$1"
  if [ -z "${THEIA_COOKIE_JAR:-}" ] || [ ! -f "$THEIA_COOKIE_JAR" ]; then
    return 1
  fi
  awk -v name="$name" '($0 !~ /^#/ || $0 ~ /^#HttpOnly_/) && $6 == name { value = $7 } END { if (value != "") print value; else exit 1 }' "$THEIA_COOKIE_JAR"
}

theia_read_username() {
  if [ -n "${THEIA_API_USERNAME:-}" ]; then
    printf '%s' "$THEIA_API_USERNAME"
    return
  fi
  if ! theia_seed_interactive; then
    printf '%s' "administrator"
    return
  fi

  local username
  read -r -p "Theia username [administrator]: " username
  if [ -z "$username" ]; then
    username="administrator"
  fi
  printf '%s' "$username"
}

theia_read_password() {
  local prompt="$1"
  local env_name="$2"
  local non_interactive_message="$3"
  local value="${!env_name:-}"
  if [ -n "$value" ]; then
    printf '%s' "$value"
    return
  fi
  if ! theia_seed_interactive; then
    echo "$non_interactive_message" >&2
    return 1
  fi

  local password
  read -r -s -p "$prompt: " password
  echo >&2
  printf '%s' "$password"
}

theia_seed_cleanup() {
  if [ -n "${THEIA_COOKIE_JAR:-}" ]; then
    rm -f "$THEIA_COOKIE_JAR"
  fi
}

ensure_theia_api_session() {
  if [ "${THEIA_API_SESSION_BASE:-}" = "$API_BASE" ] &&
    [ -n "${THEIA_COOKIE_JAR:-}" ] &&
    [ -n "$(theia_cookie_value theia_session 2>/dev/null || true)" ] &&
    [ -n "$(theia_cookie_value theia_csrf 2>/dev/null || true)" ]; then
    return
  fi

  local username
  local password
  local response
  username="$(theia_read_username)"
  password="$(theia_read_password \
    "Theia password" \
    "THEIA_API_PASSWORD" \
    "Theia seed scripts need a password-session login. Set THEIA_API_PASSWORD for local automation or run interactively.")"

  THEIA_COOKIE_JAR="$(mktemp)"
  trap theia_seed_cleanup EXIT

  response="$(theia_login_payload "$username" "$password" | curl -sf \
    -X POST "$API_BASE/api/v1/auth/login" \
    -H "Content-Type: application/json" \
    -c "$THEIA_COOKIE_JAR" \
    --data-binary @-)"

  THEIA_API_SESSION_BASE="$API_BASE"
  THEIA_CSRF_TOKEN="$(theia_cookie_value theia_csrf)"
  THEIA_CURL_AUTH_ARGS=(-b "$THEIA_COOKIE_JAR" -H "X-CSRF-Token: ${THEIA_CSRF_TOKEN}")

  if printf '%s' "$response" | python3 -c 'import json, sys; data=json.load(sys.stdin); raise SystemExit(0 if data.get("user", {}).get("must_change_password") is True else 1)'; then
    local new_password
    new_password="$(theia_read_password \
      "New Theia password" \
      "THEIA_API_NEW_PASSWORD" \
      "Theia login requires a password change. Sign in once and change the password before running seed scripts non-interactively.")"

    theia_password_change_payload "$password" "$new_password" | curl -sf \
      -X POST "$API_BASE/api/v1/auth/password/change" \
      "${THEIA_CURL_AUTH_ARGS[@]}" \
      -H "Content-Type: application/json" \
      --data-binary @- >/dev/null
  fi
}

primary_map_id() {
  local payload
  ensure_theia_api_session
  payload="$(curl -sf "${THEIA_CURL_AUTH_ARGS[@]}" "$API_BASE/api/v1/canvas/maps")"
  printf '%s' "$payload" | python3 -c 'import json, sys
payload = json.load(sys.stdin)
maps = payload.get("data", [])
for item in maps:
    if item.get("is_default") is True:
        print(item.get("id", ""))
        raise SystemExit(0)
if maps:
    print(maps[0].get("id", ""))'
}

device_id_by_ip() {
  local ip="$1"
  local payload
  ensure_theia_api_session
  payload="$(curl -sf "${THEIA_CURL_AUTH_ARGS[@]}" "$API_BASE/api/v1/devices")"
  printf '%s' "$payload" | python3 -c 'import json, sys
target_ip = sys.argv[1]
payload = json.load(sys.stdin)
for item in payload.get("data", []):
    attributes = item.get("attributes", {})
    if attributes.get("ip") == target_ip:
        print(item.get("id", ""))
        raise SystemExit(0)
' "$ip"
}

THEIA_CURL_AUTH_ARGS=()

add_device_to_primary_map() {
  local device_id="$1"
  local map_id
  local response
  local body
  local status

  map_id="$(primary_map_id)"
  if [ -z "$map_id" ] || [ -z "$device_id" ]; then
    return
  fi

  response="$(curl -sS -X POST "$API_BASE/api/v1/canvas/maps/${map_id}/devices/${device_id}" \
    "${THEIA_CURL_AUTH_ARGS[@]}" \
    -H "Content-Type: application/json" \
    -d '{"include_connected_links": true}' \
    -w $'\n%{http_code}' || true)"
  status="${response##*$'\n'}"
  body="${response%$'\n'"$status"}"

  if [ "$status" -ge 200 ] && [ "$status" -lt 300 ]; then
    return
  fi
  if [ "$status" = "409" ] && printf '%s' "$body" | grep -Fq "device already exists in this map"; then
    return
  fi

  if [ -n "$body" ]; then
    printf '%s\n' "$body" >&2
  fi
  return 1
}

run_topology_discovery() {
  local device_id="$1"
  local response
  local body
  local status

  if [ -z "$device_id" ]; then
    return
  fi

  ensure_theia_api_session
  response="$(curl -sS -X POST "$API_BASE/api/v1/devices/${device_id}/topology-discovery" \
    "${THEIA_CURL_AUTH_ARGS[@]}" \
    -w $'\n%{http_code}' || true)"
  status="${response##*$'\n'}"
  body="${response%$'\n'"$status"}"

  if [ "$status" -ge 200 ] && [ "$status" -lt 300 ]; then
    return
  fi

  if [ -n "$body" ]; then
    printf '%s\n' "$body" >&2
  fi
  return 1
}
