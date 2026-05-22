#!/bin/bash
# =============================================================================
# Seed script: adds sample SNMP devices via the REST API
# Usage: ./scripts/seed.sh [API_BASE_URL]
# =============================================================================
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/seed-primary-map.sh"

echo "=== Seeding Theia with sample SNMP devices ==="
echo "These devices must be reachable from the backend container."
echo ""

# Wait for API to be ready
echo "Waiting for API at $API_BASE..."
for i in $(seq 1 30); do
  if curl -sf "$API_BASE/api/v1/auth/me" > /dev/null 2>&1; then
    echo "API is ready."
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: API not ready after 30 seconds"
    exit 1
  fi
  sleep 1
done

echo ""

create_seed_device() {
  local ip="$1"
  local label="$2"
  local payload="$3"
  local existing_id
  existing_id="$(device_id_by_ip "$ip" || true)"

  if [ -n "$existing_id" ]; then
    echo "Skipping ${label} - already present; ensuring primary map membership"
    add_device_to_primary_map "$existing_id"
    echo ""
    return
  fi

  echo "Adding ${label}..."
  response="$(curl -sf -X POST "$API_BASE/api/v1/devices" \
    "${THEIA_CURL_AUTH_ARGS[@]}" \
    -H "Content-Type: application/json" \
    -d "$payload")"
  echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
  echo ""
}

create_seed_device "172.28.10.10" "Router (gw-core-01 @ 172.28.10.10)" '{
  "ip": "172.28.10.10",
  "hostname": "gw-core-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "mikrotik", "role": "gateway", "site": "hq"}
}'

create_seed_device "172.28.10.11" "Cisco Switch (sw-dist-01 @ 172.28.10.11)" '{
  "ip": "172.28.10.11",
  "hostname": "sw-dist-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "cisco", "role": "distribution", "site": "hq"}
}'

create_seed_device "172.28.10.12" "Ubiquiti AP (ap-office-01 @ 172.28.10.12)" '{
  "ip": "172.28.10.12",
  "hostname": "ap-office-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "ubiquiti", "role": "access-point", "site": "hq"}
}'

echo ""
echo "=== Seed complete ==="
echo "Check devices: curl $API_BASE/api/v1/devices"
