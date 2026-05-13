#!/usr/bin/env bash
set -euo pipefail

test_pattern='Test(PrometheusCollectorContractCases|SNMPCollectorContractCases|MetricsCollectorAppliesContractNormalizedRuntimeOutcome)'
router_target="${THEIA_SNMP_ROUTER_TARGET:-}"
ap_target="${THEIA_SNMP_AP_TARGET:-}"

if { [ -n "$router_target" ] && [ -z "$ap_target" ]; } || { [ -z "$router_target" ] && [ -n "$ap_target" ]; }; then
  printf 'Set both THEIA_SNMP_ROUTER_TARGET and THEIA_SNMP_AP_TARGET, or leave both unset to skip live SNMP contract cases.\n' >&2
  exit 1
fi

if [ -n "$router_target" ]; then
  export THEIA_ENABLE_CONTRACT_TESTS=1
  printf 'Running collector contract tests with live SNMP targets.\n'
else
  unset THEIA_ENABLE_CONTRACT_TESTS
  printf 'Running collector contract tests without live SNMP targets; SNMP contract cases will be skipped.\n'
fi

go test ./internal/collector ./internal/worker -count=1 -run "$test_pattern"
