#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/wisp-lab-common.sh"

docker compose -f "$WISP_LAB_COMPOSE_FILE" up --build -d

backend_connected=0
if wisp_connect_backend_to_lab_network; then
  backend_connected=1
fi

echo ""
echo "WISP lab is running:"
echo "  SNMP management targets: 172.31.250.21-172.31.250.42"
echo "  Host loopback SNMP ports: 127.0.10.21-127.0.10.42:161/udp"
echo "  SNMP exporter: http://localhost:9117"
echo "  Prometheus:    http://localhost:9091"
if [ "$backend_connected" -eq 1 ]; then
  echo "  Backend container '$WISP_BACKEND_CONTAINER' is connected to $WISP_LAB_NETWORK"
else
  echo "  Backend container '$WISP_BACKEND_CONTAINER' is not running; seed will use host loopback unless WISP_SEED_TARGET_MODE is set."
fi
echo ""
echo "Run 'make wisp-seed-all' to add routers plus radio access nodes to Theia."
