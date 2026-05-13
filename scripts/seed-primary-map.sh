primary_map_id() {
  python3 - "$API_BASE" <<'PY'
import json
import sys
import urllib.request

api_base = sys.argv[1]
with urllib.request.urlopen(f"{api_base}/api/v1/canvas/maps", timeout=10) as response:
    payload = json.load(response)

maps = payload.get("data", [])
for item in maps:
    if item.get("is_default") is True:
        print(item.get("id", ""))
        raise SystemExit(0)

if maps:
    print(maps[0].get("id", ""))
PY
}

device_id_by_ip() {
  local ip="$1"
  python3 - "$API_BASE" "$ip" <<'PY'
import json
import sys
import urllib.request

api_base, target_ip = sys.argv[1], sys.argv[2]
with urllib.request.urlopen(f"{api_base}/api/v1/devices", timeout=10) as response:
    payload = json.load(response)

for item in payload.get("data", []):
    attributes = item.get("attributes", {})
    if attributes.get("ip") == target_ip:
        print(item.get("id", ""))
        raise SystemExit(0)

raise SystemExit(1)
PY
}

add_device_to_primary_map() {
  local device_id="$1"
  local map_id
  local response
  local body
  local status

  map_id="$(primary_map_id || true)"
  if [ -z "$map_id" ] || [ -z "$device_id" ]; then
    return
  fi

  response="$(curl -sS -X POST "$API_BASE/api/v1/canvas/maps/${map_id}/devices/${device_id}" \
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
