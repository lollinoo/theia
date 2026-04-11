#!/bin/bash
set -euo pipefail

API_BASE="${1:-http://localhost:8080}"

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

create_router() {
  local ip="$1"
  local hostname="$2"
  local role="$3"
  local site="$4"
  local ospf_area="$5"

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
        \"vendor\": \"mikrotik\",
        \"role\": \"${role}\",
        \"site\": \"${site}\",
        \"lab\": \"wisp-ospf\",
        \"ospf_area\": \"${ospf_area}\"
      }
    }" | python3 -m json.tool 2>/dev/null || echo "(response above)"
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
