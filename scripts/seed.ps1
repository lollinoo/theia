param(
  [string]$ApiBase = "http://localhost:8080"
)

$ErrorActionPreference = "Stop"
. "$PSScriptRoot/seed-primary-map.ps1"

function Wait-ApiReady {
  param([string]$ApiBase)

  Write-Output "Waiting for API at $ApiBase..."
  foreach ($attempt in 1..30) {
    try {
      Invoke-WebRequest -Uri "$ApiBase/api/v1/health" -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase) -UseBasicParsing -TimeoutSec 2 | Out-Null
      Write-Output "API is ready."
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

function New-SeedDevice {
  param(
    [string]$Ip,
    [string]$Label,
    [string]$Payload
  )

  $existingId = Get-DeviceIdByIp -ApiBase $ApiBase -Ip $Ip
  if (-not [string]::IsNullOrWhiteSpace($existingId)) {
    Write-Output "Skipping $Label - already present; ensuring primary map membership"
    Add-DeviceToPrimaryMap -ApiBase $ApiBase -DeviceId $existingId
    Write-Output ""
    return
  }

  Write-Output "Adding $Label..."
  $response = Invoke-RestMethod -Method Post -Uri "$ApiBase/api/v1/devices" -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase -Mutating) -ContentType "application/json" -Body $Payload
  $response | ConvertTo-Json -Depth 10
  Write-Output ""
}

Write-Output "=== Seeding Theia with sample SNMP devices ==="
Write-Output "These devices must be reachable from the backend container."
Write-Output ""
Wait-ApiReady -ApiBase $ApiBase
Write-Output ""

New-SeedDevice -Ip "172.28.10.10" -Label "Router (gw-core-01 @ 172.28.10.10)" -Payload @'
{
  "ip": "172.28.10.10",
  "hostname": "gw-core-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "mikrotik", "role": "gateway", "site": "hq"}
}
'@

New-SeedDevice -Ip "172.28.10.11" -Label "Cisco Switch (sw-dist-01 @ 172.28.10.11)" -Payload @'
{
  "ip": "172.28.10.11",
  "hostname": "sw-dist-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "cisco", "role": "distribution", "site": "hq"}
}
'@

New-SeedDevice -Ip "172.28.10.12" -Label "Ubiquiti AP (ap-office-01 @ 172.28.10.12)" -Payload @'
{
  "ip": "172.28.10.12",
  "hostname": "ap-office-01",
  "snmp": {
    "version": "2c",
    "community": "public"
  },
  "tags": {"vendor": "ubiquiti", "role": "access-point", "site": "hq"}
}
'@

Write-Output ""
Write-Output "=== Seed complete ==="
Write-Output "Check devices: curl $ApiBase/api/v1/devices"
