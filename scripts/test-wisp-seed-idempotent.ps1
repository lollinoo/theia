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
$script:loginAttempts = 0

function Invoke-RestMethod {
  param(
    [string]$Uri,
    [string]$Method,
    [string]$ContentType,
    [string]$Body,
    [int]$TimeoutSec,
    [hashtable]$Headers
  )

  if ($Uri -eq "http://unit.test/api/v1/auth/login" -and $Method -eq "Post") {
    $script:loginAttempts += 1
    $loginBody = $Body | ConvertFrom-Json
    Assert-True ($loginBody.identifier -eq "administrator") "login must use configured API username"
    Assert-True ($loginBody.password -eq "unit-test-password") "login must send configured API password"
    return [pscustomobject]@{
      authenticated = $true
      user = [pscustomobject]@{
        username = "administrator"
        must_change_password = $false
      }
    }
  }

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

$script:webRequestCalls = @()

function Invoke-WebRequest {
  param(
    [string]$Uri,
    [string]$Method,
    [string]$ContentType,
    [string]$Body,
    [hashtable]$Headers,
    [int]$TimeoutSec,
    [switch]$UseBasicParsing,
    [string]$SessionVariable,
    [object]$WebSession
  )

  $script:webRequestCalls += [pscustomobject]@{
    Method = $Method
    Uri = $Uri
    Body = $Body
    Headers = $Headers
  }

  if ($Uri -eq "http://unit.test/api/v1/auth/login" -and $Method -eq "Post") {
    $script:loginAttempts += 1
    $loginBody = $Body | ConvertFrom-Json
    Assert-True ($loginBody.identifier -eq "administrator") "login must use configured API username"
    Assert-True ($loginBody.password -eq "unit-test-password") "login must send configured API password"

    $session = [Microsoft.PowerShell.Commands.WebRequestSession]::new()
    $session.Cookies.Add([Uri]"http://unit.test", [System.Net.Cookie]::new("theia_session", "unit-session-token", "/", "unit.test"))
    $session.Cookies.Add([Uri]"http://unit.test", [System.Net.Cookie]::new("theia_csrf", "unit-csrf-token", "/", "unit.test"))
    Set-Variable -Scope Script -Name $SessionVariable -Value $session

    return [pscustomobject]@{
      Content = '{"authenticated":true,"user":{"username":"administrator","must_change_password":false}}'
    }
  }

  throw "unexpected web request: $Method $Uri"
}

$script:postAttempts = 0
$script:topologyDiscoveryAttempts = 0
$previousApiUsername = $env:THEIA_API_USERNAME
$previousApiPassword = $env:THEIA_API_PASSWORD
$env:THEIA_API_USERNAME = "administrator"
$env:THEIA_API_PASSWORD = "unit-test-password"
try {
  Add-DeviceToPrimaryMap -ApiBase "http://unit.test" -DeviceId "device-1"
  Invoke-TopologyDiscovery -ApiBase "http://unit.test" -DeviceId "device-1"
}
finally {
  if ($null -eq $previousApiUsername) {
    Remove-Item Env:\THEIA_API_USERNAME -ErrorAction SilentlyContinue
  }
  else {
    $env:THEIA_API_USERNAME = $previousApiUsername
  }
  if ($null -eq $previousApiPassword) {
    Remove-Item Env:\THEIA_API_PASSWORD -ErrorAction SilentlyContinue
  }
  else {
    $env:THEIA_API_PASSWORD = $previousApiPassword
  }
}
Assert-True ($script:loginAttempts -eq 1) "seed helpers should login once and reuse the cookie session"
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
Assert-True ($script:restCalls[$membershipCallIndex].Headers.Cookie -match "theia_session=unit-session-token") "primary-map membership calls must send the session cookie"
Assert-True ($script:restCalls[$membershipCallIndex].Headers.Cookie -match "theia_csrf=unit-csrf-token") "primary-map membership calls must send the csrf cookie"
Assert-True ($script:restCalls[$membershipCallIndex].Headers."X-CSRF-Token" -eq "unit-csrf-token") "primary-map membership calls must send the csrf header"
Assert-True ($script:restCalls[$discoveryCallIndex].Headers.Cookie -match "theia_session=unit-session-token") "topology discovery calls must send the session cookie"
Assert-True ($script:restCalls[$discoveryCallIndex].Headers."X-CSRF-Token" -eq "unit-csrf-token") "topology discovery calls must send the csrf header"
Assert-True (-not $script:restCalls[$membershipCallIndex].Headers.ContainsKey("Authorization")) "primary-map membership calls must not send bearer Authorization"
Assert-True (-not $script:restCalls[$discoveryCallIndex].Headers.ContainsKey("Authorization")) "topology discovery calls must not send bearer Authorization"

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
    Assert-True ($content -match "Get-TheiaApiHeaders") "$seedFile must send configured session auth"
  }
  else {
    Assert-True ($content -match "THEIA_CURL_AUTH_ARGS") "$seedFile must send configured session auth"
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
