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

$script:restCalls = @()

function Invoke-RestMethod {
  param(
    [string]$Uri,
    [string]$Method,
    [string]$ContentType,
    [string]$Body,
    [int]$TimeoutSec,
    [hashtable]$Headers
  )

  $script:restCalls += [pscustomobject]@{
    Method = $Method
    Uri = $Uri
    Body = $Body
    Headers = $Headers
  }

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

  if ($Uri -eq "http://unit.test/api/v1/devices/device-1/topology-discovery" -and $Method -eq "Post") {
    $script:topologyDiscoveryAttempts += 1
    return [pscustomobject]@{
      data = [pscustomobject]@{
        device_id = "device-1"
      }
    }
  }

  throw "unexpected REST call: $Method $Uri"
}

$script:postAttempts = 0
$script:topologyDiscoveryAttempts = 0
$previousOperatorToken = $env:THEIA_OPERATOR_TOKEN
$env:THEIA_OPERATOR_TOKEN = "test-operator-token-not-secret-1234"
try {
  Add-DeviceToPrimaryMap -ApiBase "http://unit.test" -DeviceId "device-1"
  Invoke-TopologyDiscovery -ApiBase "http://unit.test" -DeviceId "device-1"
}
finally {
  if ($null -eq $previousOperatorToken) {
    Remove-Item Env:\THEIA_OPERATOR_TOKEN -ErrorAction SilentlyContinue
  }
  else {
    $env:THEIA_OPERATOR_TOKEN = $previousOperatorToken
  }
}
Assert-True ($script:postAttempts -eq 1) "duplicate primary-map membership should be attempted once and treated as idempotent"
Assert-True ($script:topologyDiscoveryAttempts -eq 1) "topology discovery should be triggered once for an existing WISP device"

$membershipCallIndex = -1
$discoveryCallIndex = -1
for ($i = 0; $i -lt $script:restCalls.Count; $i++) {
  if ($script:restCalls[$i].Uri -eq "http://unit.test/api/v1/canvas/maps/primary-map/devices/device-1") {
    $membershipCallIndex = $i
  }
  if ($script:restCalls[$i].Uri -eq "http://unit.test/api/v1/devices/device-1/topology-discovery") {
    $discoveryCallIndex = $i
  }
}
Assert-True ($membershipCallIndex -ge 0) "primary-map membership call should be captured"
Assert-True ($discoveryCallIndex -ge 0) "topology discovery call should be captured"
Assert-True ($discoveryCallIndex -gt $membershipCallIndex) "topology discovery should run after primary-map membership"
Assert-True ($script:restCalls[$membershipCallIndex].Headers.Authorization -eq "Bearer test-operator-token-not-secret-1234") "primary-map membership calls must send the operator bearer token when configured"
Assert-True ($script:restCalls[$discoveryCallIndex].Headers.Authorization -eq "Bearer test-operator-token-not-secret-1234") "topology discovery calls must send the operator bearer token when configured"

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
  if ($seedFile.EndsWith(".ps1")) {
    Assert-True ($content -match "Get-TheiaApiHeaders") "$seedFile must send configured operator bearer auth"
  }
  else {
    Assert-True ($content -match "THEIA_CURL_AUTH_ARGS") "$seedFile must send configured operator bearer auth"
  }
}

$wispSeedFiles = @(
  "scripts/seed-wisp.ps1",
  "scripts/seed-wisp.sh",
  "scripts/seed-wisp-radio.ps1",
  "scripts/seed-wisp-radio.sh"
)

foreach ($seedFile in $wispSeedFiles) {
  $content = Get-Content -Raw -Path $seedFile
  if ($seedFile.EndsWith(".ps1")) {
    Assert-True ($content -match 'topology_discovery_mode\s*=\s*"lldp_cdp"') "$seedFile must force topology_discovery_mode to lldp_cdp"
    Assert-True (([regex]::Matches($content, 'Invoke-TopologyDiscovery\b')).Count -eq 1) "$seedFile existing-device path must call Invoke-TopologyDiscovery once"

    $mapCallIndex = $content.IndexOf("Add-DeviceToPrimaryMap")
    $discoveryCallIndex = $content.IndexOf("Invoke-TopologyDiscovery")
  }
  else {
    Assert-True ($content -match '\\?"topology_discovery_mode\\?"\s*:\s*\\?"lldp_cdp\\?"') "$seedFile must force topology_discovery_mode to lldp_cdp"
    Assert-True (([regex]::Matches($content, 'run_topology_discovery\b')).Count -eq 1) "$seedFile existing-device path must call run_topology_discovery once"

    $mapCallIndex = $content.IndexOf("add_device_to_primary_map")
    $discoveryCallIndex = $content.IndexOf("run_topology_discovery")
  }

  Assert-True ($mapCallIndex -ge 0) "$seedFile must still ensure primary-map membership"
  Assert-True ($discoveryCallIndex -ge 0) "$seedFile must trigger topology discovery for existing devices"
  Assert-True ($discoveryCallIndex -gt $mapCallIndex) "$seedFile must trigger topology discovery after primary-map membership"
}

$genericSeedFiles = @(
  "scripts/seed.ps1",
  "scripts/seed.sh"
)

foreach ($seedFile in $genericSeedFiles) {
  $content = Get-Content -Raw -Path $seedFile
  Assert-True ($content -notmatch "topology_discovery_mode") "$seedFile should not be forced to include WISP topology mode"
}

Write-Output "WISP seed idempotency behavior is valid"
