#!/bin/bash
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/seed-primary-map.sh"

echo "=== Seeding Theia with WISP lab routers ==="

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

create_router() {
  local ip="$1"
  local hostname="$2"
  local role="$3"
  local site="$4"
  local ospf_area="$5"
  local existing_id
  existing_id="$(device_id_by_ip "$ip" || true)"

  if [ -n "$existing_id" ]; then
    echo "Skipping ${hostname} (${ip}) - already present; ensuring primary map membership"
    add_device_to_primary_map "$existing_id"
    return
  fi

  echo "Adding ${hostname} (${ip})..."
  response="$(curl -sf -X POST "$API_BASE/api/v1/devices" \
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
        \"vendor\": \"mikrotik\",
        \"role\": \"${role}\",
        \"site\": \"${site}\",
        \"lab\": \"wisp-ospf\",
        \"ospf_area\": \"${ospf_area}\"
      }
    }")"
  echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
  add_device_to_primary_map "$(echo "$response" | created_device_id_from_response)"
  echo ""
  sleep 0.5
}

create_router "127.0.10.21" "wisp-core-01" "core" "noc" "0.0.0.0"
create_router "127.0.10.22" "wisp-core-02" "core" "noc" "0.0.0.0"
create_router "127.0.10.23" "wisp-pop-north-01" "pop-abr" "pop-north" "0.0.0.0,0.0.0.10"
create_router "127.0.10.24" "wisp-pop-south-01" "pop-abr" "pop-south" "0.0.0.0,0.0.0.20"
create_router "127.0.10.25" "wisp-ix-edge-01" "edge" "ix" "0.0.0.0"
create_router "127.0.10.26" "wisp-tower-north-01" "tower" "tower-north-a" "0.0.0.10"
create_router "127.0.10.27" "wisp-tower-north-02" "tower" "tower-north-b" "0.0.0.10"
create_router "127.0.10.28" "wisp-tower-south-01" "tower" "tower-south-a" "0.0.0.20"
create_router "127.0.10.29" "wisp-tower-south-02" "tower" "tower-south-b" "0.0.0.20"
create_router "127.0.10.30" "wisp-dc-agg-01" "datacenter-agg" "dc" "0.0.0.0"

echo "=== WISP lab seed complete ==="
