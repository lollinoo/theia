param(
  [string]$ApiBase = "http://localhost:8080",
  [string]$TargetMode = ""
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/seed-primary-map.ps1"
. "$PSScriptRoot/wisp-lab-common.ps1"

function Wait-ApiReady {
  param([string]$ApiBase)

  foreach ($attempt in 1..30) {
    try {
      Invoke-WebRequest -Uri "$ApiBase/api/v1/health" -Headers (Get-TheiaApiHeaders) -UseBasicParsing -TimeoutSec 2 | Out-Null
      return
    }
    catch {
      if ($attempt -eq 30) {
        throw "ERROR: API not ready after 30 seconds"
      }
      Start-Sleep -Seconds 1
    }
  }
}

function New-WispRouter {
  param(
    [string]$Ip,
    [string]$Hostname,
    [string]$Role,
    [string]$Site,
    [string]$OspfArea
  )

  $existingId = Get-DeviceIdByIp -ApiBase $ApiBase -Ip $Ip
  if (-not [string]::IsNullOrWhiteSpace($existingId)) {
    Write-Output "Skipping $Hostname ($Ip) - already present; ensuring primary map membership and rerunning topology discovery"
    Add-DeviceToPrimaryMap -ApiBase $ApiBase -DeviceId $existingId
    Invoke-TopologyDiscovery -ApiBase $ApiBase -DeviceId $existingId
    return
  }

  Write-Output "Adding $Hostname ($Ip)..."
  $payload = @{
    ip = $Ip
    hostname = $Hostname
    metrics_source = "snmp"
    topology_discovery_mode = "lldp_cdp"
    snmp = @{
      version = "2c"
      community = "public"
    }
    tags = @{
      vendor = "mikrotik"
      role = $Role
      site = $Site
      lab = "wisp-ospf"
      ospf_area = $OspfArea
    }
  } | ConvertTo-Json -Depth 5

  $response = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/devices" -Headers (Get-TheiaApiHeaders) -ContentType "application/json" -Body $payload
  $response | ConvertTo-Json -Depth 10
  Write-Output ""
  Start-Sleep -Milliseconds 500
}

Write-Output "=== Seeding Theia with WISP lab routers ==="
Wait-ApiReady -ApiBase $ApiBase
$targetPrefix = Get-WispSeedTargetPrefix -TargetMode $TargetMode -ApiBase $ApiBase

New-WispRouter "$($targetPrefix)21" "wisp-core-01" "core" "noc" "0.0.0.0"
New-WispRouter "$($targetPrefix)22" "wisp-core-02" "core" "noc" "0.0.0.0"
New-WispRouter "$($targetPrefix)23" "wisp-pop-north-01" "pop-abr" "pop-north" "0.0.0.0,0.0.0.10"
New-WispRouter "$($targetPrefix)24" "wisp-pop-south-01" "pop-abr" "pop-south" "0.0.0.0,0.0.0.20"
New-WispRouter "$($targetPrefix)25" "wisp-ix-edge-01" "edge" "ix" "0.0.0.0"
New-WispRouter "$($targetPrefix)26" "wisp-tower-north-01" "tower" "tower-north-a" "0.0.0.10"
New-WispRouter "$($targetPrefix)27" "wisp-tower-north-02" "tower" "tower-north-b" "0.0.0.10"
New-WispRouter "$($targetPrefix)28" "wisp-tower-south-01" "tower" "tower-south-a" "0.0.0.20"
New-WispRouter "$($targetPrefix)29" "wisp-tower-south-02" "tower" "tower-south-b" "0.0.0.20"
New-WispRouter "$($targetPrefix)30" "wisp-dc-agg-01" "datacenter-agg" "dc" "0.0.0.0"

Write-Output "=== WISP lab seed complete ==="
