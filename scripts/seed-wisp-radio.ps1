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
      Invoke-WebRequest -Uri "$ApiBase/api/v1/health" -UseBasicParsing -TimeoutSec 2 | Out-Null
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

function New-WispRadioDevice {
  param(
    [string]$Ip,
    [string]$Hostname,
    [string]$Role,
    [string]$Site,
    [string]$RfDomain,
    [string]$Segment
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
      vendor = "ubiquiti"
      role = $Role
      site = $Site
      lab = "wisp-ospf"
      overlay = "radio"
      rf_domain = $RfDomain
      segment = $Segment
    }
  } | ConvertTo-Json -Depth 5

  $response = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/devices" -ContentType "application/json" -Body $payload
  $response | ConvertTo-Json -Depth 10
  Write-Output ""
  Start-Sleep -Milliseconds 500
}

Write-Output "=== Seeding Theia with WISP radio access nodes ==="
Wait-ApiReady -ApiBase $ApiBase
$targetPrefix = Get-WispSeedTargetPrefix -TargetMode $TargetMode -ApiBase $ApiBase

New-WispRadioDevice "$($targetPrefix)31" "wisp-ap-north-a-01" "sector-ap" "tower-north-a" "north" "sector-a"
New-WispRadioDevice "$($targetPrefix)32" "wisp-ap-north-b-01" "sector-ap" "tower-north-b" "north" "sector-b"
New-WispRadioDevice "$($targetPrefix)33" "wisp-ap-south-a-01" "sector-ap" "tower-south-a" "south" "sector-a"
New-WispRadioDevice "$($targetPrefix)34" "wisp-ap-south-b-01" "sector-ap" "tower-south-b" "south" "sector-b"
New-WispRadioDevice "$($targetPrefix)35" "wisp-cpe-north-a-01" "subscriber-cpe" "subscriber-north-a-01" "north" "sector-a"
New-WispRadioDevice "$($targetPrefix)36" "wisp-cpe-north-a-02" "subscriber-cpe" "subscriber-north-a-02" "north" "sector-a"
New-WispRadioDevice "$($targetPrefix)37" "wisp-cpe-north-b-01" "subscriber-cpe" "subscriber-north-b-01" "north" "sector-b"
New-WispRadioDevice "$($targetPrefix)38" "wisp-cpe-north-b-02" "subscriber-cpe" "subscriber-north-b-02" "north" "sector-b"
New-WispRadioDevice "$($targetPrefix)39" "wisp-cpe-south-a-01" "subscriber-cpe" "subscriber-south-a-01" "south" "sector-a"
New-WispRadioDevice "$($targetPrefix)40" "wisp-cpe-south-a-02" "subscriber-cpe" "subscriber-south-a-02" "south" "sector-a"
New-WispRadioDevice "$($targetPrefix)41" "wisp-cpe-south-b-01" "subscriber-cpe" "subscriber-south-b-01" "south" "sector-b"
New-WispRadioDevice "$($targetPrefix)42" "wisp-cpe-south-b-02" "subscriber-cpe" "subscriber-south-b-02" "south" "sector-b"

Write-Output "=== WISP radio seed complete ==="
