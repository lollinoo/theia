#!/bin/bash
# =============================================================================
# Seed script: adds the 3 SNMP simulator devices via the REST API
# Usage: ./scripts/seed.sh [API_BASE_URL]
# =============================================================================
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"

echo "=== Seeding MikroTik Theia with SNMP simulator devices ==="
echo ""

# Wait for API to be ready
echo "Waiting for API at $API_BASE..."
for i in $(seq 1 30); do
  if curl -sf "$API_BASE/api/v1/health" > /dev/null 2>&1; then
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

# Add MikroTik Router (172.28.10.10)
echo "Adding MikroTik Router (gw-core-01 @ 172.28.10.10)..."
curl -sf -X POST "$API_BASE/api/v1/devices" \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "172.28.10.10",
    "hostname": "gw-core-01",
    "snmp": {
      "version": "2c",
      "community": "public"
    },
    "tags": {"vendor": "mikrotik", "role": "gateway", "site": "hq"}
  }' | python3 -m json.tool 2>/dev/null || echo "(response above)"

echo ""

# Add Cisco Switch (172.28.10.11)
echo "Adding Cisco Switch (sw-dist-01 @ 172.28.10.11)..."
curl -sf -X POST "$API_BASE/api/v1/devices" \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "172.28.10.11",
    "hostname": "sw-dist-01",
    "snmp": {
      "version": "2c",
      "community": "public"
    },
    "tags": {"vendor": "cisco", "role": "distribution", "site": "hq"}
  }' | python3 -m json.tool 2>/dev/null || echo "(response above)"

echo ""

# Add Ubiquiti AP (172.28.10.12)
echo "Adding Ubiquiti AP (ap-office-01 @ 172.28.10.12)..."
curl -sf -X POST "$API_BASE/api/v1/devices" \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "172.28.10.12",
    "hostname": "ap-office-01",
    "snmp": {
      "version": "2c",
      "community": "public"
    },
    "tags": {"vendor": "ubiquiti", "role": "access-point", "site": "hq"}
  }' | python3 -m json.tool 2>/dev/null || echo "(response above)"

echo ""
echo "=== Seed complete ==="
echo "Check devices: curl $API_BASE/api/v1/devices"
