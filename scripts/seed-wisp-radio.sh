#!/bin/bash
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"

echo "=== Seeding Theia with WISP radio access nodes ==="

for i in $(seq 1 30); do
  if curl -sf "$API_BASE/api/v1/health" >/dev/null 2>&1; then
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: API not ready after 30 seconds"
    exit 1
  fi
  sleep 1
done

device_exists() {
  local ip="$1"
  python3 - "$API_BASE" "$ip" <<'PY'
import json, sys, urllib.request
api_base, target_ip = sys.argv[1], sys.argv[2]
with urllib.request.urlopen(f"{api_base}/api/v1/devices", timeout=10) as response:
    payload = json.load(response)
for item in payload.get("data", []):
    attributes = item.get("attributes", {})
    if attributes.get("ip") == target_ip:
        raise SystemExit(0)
raise SystemExit(1)
PY
}

create_device() {
  local ip="$1"
  local hostname="$2"
  local role="$3"
  local site="$4"
  local rf_domain="$5"
  local segment="$6"

  if device_exists "$ip"; then
    echo "Skipping ${hostname} (${ip}) - already present"
    return
  fi

  echo "Adding ${hostname} (${ip})..."
  curl -sf -X POST "$API_BASE/api/v1/devices" \
    -H "Content-Type: application/json" \
    -d "{
      \"ip\": \"${ip}\",
      \"hostname\": \"${hostname}\",
      \"metrics_source\": \"snmp\",
      \"snmp\": {
        \"version\": \"2c\",
        \"community\": \"public\"
      },
      \"tags\": {
        \"vendor\": \"ubiquiti\",
        \"role\": \"${role}\",
        \"site\": \"${site}\",
        \"lab\": \"wisp-ospf\",
        \"overlay\": \"radio\",
        \"rf_domain\": \"${rf_domain}\",
        \"segment\": \"${segment}\"
      }
    }" | python3 -m json.tool 2>/dev/null || echo "(response above)"
  echo ""
  sleep 0.5
}

create_device "127.0.10.31" "wisp-ap-north-a-01" "sector-ap" "tower-north-a" "north" "sector-a"
create_device "127.0.10.32" "wisp-ap-north-b-01" "sector-ap" "tower-north-b" "north" "sector-b"
create_device "127.0.10.33" "wisp-ap-south-a-01" "sector-ap" "tower-south-a" "south" "sector-a"
create_device "127.0.10.34" "wisp-ap-south-b-01" "sector-ap" "tower-south-b" "south" "sector-b"
create_device "127.0.10.35" "wisp-cpe-north-a-01" "subscriber-cpe" "subscriber-north-a-01" "north" "sector-a"
create_device "127.0.10.36" "wisp-cpe-north-a-02" "subscriber-cpe" "subscriber-north-a-02" "north" "sector-a"
create_device "127.0.10.37" "wisp-cpe-north-b-01" "subscriber-cpe" "subscriber-north-b-01" "north" "sector-b"
create_device "127.0.10.38" "wisp-cpe-north-b-02" "subscriber-cpe" "subscriber-north-b-02" "north" "sector-b"
create_device "127.0.10.39" "wisp-cpe-south-a-01" "subscriber-cpe" "subscriber-south-a-01" "south" "sector-a"
create_device "127.0.10.40" "wisp-cpe-south-a-02" "subscriber-cpe" "subscriber-south-a-02" "south" "sector-a"
create_device "127.0.10.41" "wisp-cpe-south-b-01" "subscriber-cpe" "subscriber-south-b-01" "south" "sector-b"
create_device "127.0.10.42" "wisp-cpe-south-b-02" "subscriber-cpe" "subscriber-south-b-02" "south" "sector-b"

echo "=== WISP radio seed complete ==="
