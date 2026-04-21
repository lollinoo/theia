#!/usr/bin/env bash
set -euo pipefail

router_was_running=0
ap_was_running=0
need_services=()

if docker container inspect theia-snmp-router >/dev/null 2>&1; then
  if [ "$(docker container inspect -f '{{.State.Running}}' theia-snmp-router)" = "true" ]; then
    router_was_running=1
  else
    need_services+=(snmp-router)
  fi
else
  need_services+=(snmp-router)
fi

if docker container inspect theia-snmp-ap >/dev/null 2>&1; then
  if [ "$(docker container inspect -f '{{.State.Running}}' theia-snmp-ap)" = "true" ]; then
    ap_was_running=1
  else
    need_services+=(snmp-ap)
  fi
else
  need_services+=(snmp-ap)
fi

if [ ${#need_services[@]} -gt 0 ]; then
  docker compose --profile test up -d --wait "${need_services[@]}"
fi

cleanup() {
  if [ "$router_was_running" = "0" ]; then
    docker stop theia-snmp-router >/dev/null 2>&1 || true
  fi
  if [ "$ap_was_running" = "0" ]; then
    docker stop theia-snmp-ap >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

router_ip=$(docker container inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' theia-snmp-router)
ap_ip=$(docker container inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' theia-snmp-ap)

if [ -z "$router_ip" ] || [ -z "$ap_ip" ]; then
  printf 'failed to resolve simulator IPs: router=%q ap=%q\n' "$router_ip" "$ap_ip" >&2
  exit 1
fi

THEIA_ENABLE_CONTRACT_TESTS=1 \
THEIA_SNMP_ROUTER_TARGET="$router_ip" \
THEIA_SNMP_AP_TARGET="$ap_ip" \
go test ./internal/collector ./internal/worker -count=1 -run 'Test(PrometheusCollectorContractCases|SNMPCollectorContractCases|MetricsCollectorAppliesContractNormalizedRuntimeOutcome)'
