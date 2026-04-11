#!/bin/bash
set -euo pipefail

COMPOSE="docker compose -f docker-compose.wisp-lab.yml"
SERVICES=(
  wisp-core-01
  wisp-core-02
  wisp-pop-north-01
  wisp-pop-south-01
  wisp-ix-edge-01
  wisp-tower-north-01
  wisp-tower-north-02
  wisp-tower-south-01
  wisp-tower-south-02
  wisp-dc-agg-01
)

for service in "${SERVICES[@]}"; do
  echo "=== ${service} ==="
  ${COMPOSE} exec -T "${service}" vtysh -c "show ip ospf neighbor" || true
  echo ""
done
