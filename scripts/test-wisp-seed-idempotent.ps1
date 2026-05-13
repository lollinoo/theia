$ErrorActionPreference = "Stop"

. "$PSScriptRoot/seed-primary-map.ps1"

function Fail {
  param([string]$Message)
  Write-Error $Message
  exit 1
}

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )

  if (-not $Condition) {
    Fail $Message
  }
}

function Invoke-RestMethod {
  param(
    [string]$Uri,
    [string]$Method,
    [string]$ContentType,
    [string]$Body,
    [int]$TimeoutSec
  )

  if ($Uri -eq "http://unit.test/api/v1/canvas/maps") {
    return [pscustomobject]@{
      data = @(
        [pscustomobject]@{
          id = "primary-map"
          is_default = $true
        }
      )
    }
  }

  if ($Uri -eq "http://unit.test/api/v1/canvas/maps/primary-map/devices/device-1" -and $Method -eq "Post") {
    $script:postAttempts += 1
    throw '{"error":"device already exists in this map"}'
  }

  throw "unexpected REST call: $Method $Uri"
}

$script:postAttempts = 0
Add-DeviceToPrimaryMap -ApiBase "http://unit.test" -DeviceId "device-1"
Assert-True ($script:postAttempts -eq 1) "duplicate primary-map membership should be attempted once and treated as idempotent"

$seedFiles = @(
  "scripts/seed.ps1",
  "scripts/seed.sh",
  "scripts/seed-wisp.ps1",
  "scripts/seed-wisp.sh",
  "scripts/seed-wisp-radio.ps1",
  "scripts/seed-wisp-radio.sh"
)

foreach ($seedFile in $seedFiles) {
  $content = Get-Content -Raw -Path $seedFile
  Assert-True ($content -notmatch "Get-CreatedDeviceIdFromResponse|created_device_id_from_response") "$seedFile must not re-add newly created devices to the primary map"
}

Write-Output "WISP seed idempotency behavior is valid"
