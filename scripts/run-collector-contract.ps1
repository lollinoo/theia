param()

$ErrorActionPreference = "Stop"

$testPattern = "Test(PrometheusCollectorContractCases|SNMPCollectorContractCases|MetricsCollectorAppliesContractNormalizedRuntimeOutcome)"
$routerTarget = $env:THEIA_SNMP_ROUTER_TARGET
$apTarget = $env:THEIA_SNMP_AP_TARGET

if ([string]::IsNullOrWhiteSpace($routerTarget) -xor [string]::IsNullOrWhiteSpace($apTarget)) {
  Write-Error "Set both THEIA_SNMP_ROUTER_TARGET and THEIA_SNMP_AP_TARGET, or leave both unset to skip live SNMP contract cases."
  exit 1
}

if (-not [string]::IsNullOrWhiteSpace($routerTarget)) {
  $env:THEIA_ENABLE_CONTRACT_TESTS = "1"
  Write-Output "Running collector contract tests with live SNMP targets."
} else {
  $env:THEIA_ENABLE_CONTRACT_TESTS = ""
  Write-Output "Running collector contract tests without live SNMP targets; SNMP contract cases will be skipped."
}

& go test ./internal/collector ./internal/worker -count=1 -run $testPattern
exit $LASTEXITCODE
