#!/bin/bash
set -euo pipefail

COMPOSE="docker compose -f docker-compose.wisp-lab.yml"

echo "=== wisp-ix-edge-01: BGP summary ==="
${COMPOSE} exec -T wisp-ix-edge-01 vtysh -c "show ip bgp summary" || true
echo ""

echo "=== wisp-transit-01: BGP summary ==="
${COMPOSE} exec -T wisp-transit-01 vtysh -c "show ip bgp summary" || true
echo ""

for service in wisp-ix-edge-01 wisp-core-01 wisp-pop-north-01 wisp-pop-south-01; do
  echo "=== ${service}: default route ==="
  ${COMPOSE} exec -T "${service}" vtysh -c "show ip route 0.0.0.0/0" || true
  echo ""
done
