$ErrorActionPreference = "Stop"

. "$PSScriptRoot/wisp-lab-common.ps1"

& docker compose -f $script:WispLabComposeFile up --build -d
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}

$backendConnected = Connect-WispBackendToLabNetwork

Write-Output ""
Write-Output "WISP lab is running:"
Write-Output "  SNMP management targets: 172.31.250.21-172.31.250.42"
Write-Output "  Host loopback SNMP ports: 127.0.10.21-127.0.10.42:161/udp"
Write-Output "  SNMP exporter: http://localhost:9117"
Write-Output "  Prometheus:    http://localhost:9091"
if ($backendConnected) {
  Write-Output "  Backend container '$script:WispBackendContainer' is connected to $script:WispLabNetwork"
}
else {
  Write-Output "  Backend container '$script:WispBackendContainer' is not running; seed will use host loopback unless WISP_SEED_TARGET_MODE is set."
}
Write-Output ""
Write-Output "Run 'make wisp-seed-all' to add routers plus radio access nodes to Theia."
