#!/bin/bash
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"
TARGET_MODE="${2:-${WISP_SEED_TARGET_MODE:-auto}}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/seed-primary-map.sh"
source "$SCRIPT_DIR/wisp-lab-common.sh"

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

TARGET_PREFIX="$(wisp_seed_target_prefix "$TARGET_MODE")"

create_device() {
  local ip="$1"
  local hostname="$2"
  local role="$3"
  local site="$4"
  local rf_domain="$5"
  local segment="$6"
  local existing_id
  existing_id="$(device_id_by_ip "$ip" || true)"

  if [ -n "$existing_id" ]; then
    echo "Skipping ${hostname} (${ip}) - already present; ensuring primary map membership and rerunning topology discovery"
    add_device_to_primary_map "$existing_id"
    run_topology_discovery "$existing_id"
    return
  fi

  echo "Adding ${hostname} (${ip})..."
  response="$(curl -sf -X POST "$API_BASE/api/v1/devices" \
    -H "Content-Type: application/json" \
    -d "{
      \"ip\": \"${ip}\",
      \"hostname\": \"${hostname}\",
      \"metrics_source\": \"snmp\",
      \"topology_discovery_mode\": \"lldp_cdp\",
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
  }")"
  echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
  echo ""
  sleep 0.5
}

create_device "${TARGET_PREFIX}31" "wisp-ap-north-a-01" "sector-ap" "tower-north-a" "north" "sector-a"
create_device "${TARGET_PREFIX}32" "wisp-ap-north-b-01" "sector-ap" "tower-north-b" "north" "sector-b"
create_device "${TARGET_PREFIX}33" "wisp-ap-south-a-01" "sector-ap" "tower-south-a" "south" "sector-a"
create_device "${TARGET_PREFIX}34" "wisp-ap-south-b-01" "sector-ap" "tower-south-b" "south" "sector-b"
create_device "${TARGET_PREFIX}35" "wisp-cpe-north-a-01" "subscriber-cpe" "subscriber-north-a-01" "north" "sector-a"
create_device "${TARGET_PREFIX}36" "wisp-cpe-north-a-02" "subscriber-cpe" "subscriber-north-a-02" "north" "sector-a"
create_device "${TARGET_PREFIX}37" "wisp-cpe-north-b-01" "subscriber-cpe" "subscriber-north-b-01" "north" "sector-b"
create_device "${TARGET_PREFIX}38" "wisp-cpe-north-b-02" "subscriber-cpe" "subscriber-north-b-02" "north" "sector-b"
create_device "${TARGET_PREFIX}39" "wisp-cpe-south-a-01" "subscriber-cpe" "subscriber-south-a-01" "south" "sector-a"
create_device "${TARGET_PREFIX}40" "wisp-cpe-south-a-02" "subscriber-cpe" "subscriber-south-a-02" "south" "sector-a"
create_device "${TARGET_PREFIX}41" "wisp-cpe-south-b-01" "subscriber-cpe" "subscriber-south-b-01" "south" "sector-b"
create_device "${TARGET_PREFIX}42" "wisp-cpe-south-b-02" "subscriber-cpe" "subscriber-south-b-02" "south" "sector-b"

echo "=== WISP radio seed complete ==="
