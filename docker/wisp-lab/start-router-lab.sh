#!/usr/bin/env bash
set -euo pipefail

: "${ROUTER_NAME:?ROUTER_NAME is required}"

mkdir -p /etc/frr /etc/snmp /var/log/frr /var/run/frr /run

python3 /usr/local/bin/render-router-lab.py

if [[ -f /run/wisp-router.env ]]; then
  # shellcheck disable=SC1091
  source /run/wisp-router.env
fi

ENABLE_OSPFD="${ENABLE_OSPFD:-0}"
ENABLE_BGPD="${ENABLE_BGPD:-0}"

if [[ -n "${LOOPBACK_CIDR:-}" ]]; then
  ip addr add "${LOOPBACK_CIDR}" dev lo 2>/dev/null || true
fi

# Docker injects a default route via the bridge gateway on one attached network.
# Remove it so the lab's routing behavior comes only from FRR.
while ip route show default | grep -q '^default'; do
  ip route del default 2>/dev/null || break
done

chown -R frr:frr /etc/frr /var/log/frr /var/run/frr

/usr/lib/frr/zebra -d -A 127.0.0.1 -f /etc/frr/frr.conf
if [[ "${ENABLE_OSPFD}" == "1" ]]; then
  /usr/lib/frr/ospfd -d -A 127.0.0.1 -f /etc/frr/frr.conf
fi
if [[ "${ENABLE_BGPD}" == "1" ]]; then
  /usr/lib/frr/bgpd -d -A 127.0.0.1 -f /etc/frr/frr.conf
fi

for _ in $(seq 1 20); do
  if vtysh -c "show version" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

if [[ "${ENABLE_OSPFD}" == "1" ]]; then
  for iface in $(ip -o link show | awk -F': ' '$2 ~ /^eth/ {print $2}' | cut -d@ -f1); do
    vtysh -c "configure terminal" \
          -c "interface ${iface}" \
          -c "ip ospf network point-to-point" >/dev/null 2>&1 || true
  done
fi

cleanup() {
  pkill -TERM -x bgpd >/dev/null 2>&1 || true
  pkill -TERM -x ospfd >/dev/null 2>&1 || true
  pkill -TERM -x zebra >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

exec snmpd -f -Lo -C -c /etc/snmp/snmpd.conf
