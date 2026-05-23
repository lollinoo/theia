$ErrorActionPreference = "Stop"

. "$PSScriptRoot/wisp-lab-common.ps1"

function Fail {
  param([string]$Message)
  Write-Error $Message
  exit 1
}

function Assert-Equal {
  param(
    [Parameter(Mandatory = $true)]$Expected,
    [Parameter(Mandatory = $true)]$Actual,
    [string]$Message
  )

  if ($Expected -ne $Actual) {
    Fail "$Message Expected '$Expected', got '$Actual'."
  }
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

function Assert-Match {
  param(
    [Parameter(Mandatory = $true)][string]$Pattern,
    [Parameter(Mandatory = $true)][string]$Value,
    [string]$Message
  )

  if ($Value -notmatch $Pattern) {
    Fail "$Message Pattern '$Pattern' was not found in '$Value'."
  }
}

$script:connectAttempts = 0
$script:backendRunningChecks = 0
$script:mockBackendRunning = $true
$script:mockConnectSucceeds = $true
$script:hostMessages = @()

function Reset-WispSeedTargetTestState {
  param(
    [bool]$BackendRunning = $true,
    [bool]$ConnectSucceeds = $true
  )

  $script:connectAttempts = 0
  $script:backendRunningChecks = 0
  $script:mockBackendRunning = $BackendRunning
  $script:mockConnectSucceeds = $ConnectSucceeds
  $script:hostMessages = @()
}

function Get-WispSeedTargetLog {
  return [string]::Join("`n", $script:hostMessages)
}

function Write-Host {
  param([Parameter(ValueFromRemainingArguments = $true)]$Object)
  $script:hostMessages += [string]::Join(" ", @($Object))
}

function Test-WispBackendRunning {
  $script:backendRunningChecks += 1
  return $script:mockBackendRunning
}

function Connect-WispBackendToLabNetwork {
  $script:connectAttempts += 1
  return $script:mockConnectSucceeds
}

$localApiBases = @(
  "http://localhost:8080",
  "http://127.0.0.1:8080",
  "http://[::1]:8080",
  "http://[0000:0000:0000:0000:0000:0000:0000:0001]:8080"
)

foreach ($apiBase in $localApiBases) {
  Reset-WispSeedTargetTestState
  $prefix = Get-WispSeedTargetPrefix -TargetMode "auto" -ApiBase $apiBase
  Assert-Equal "172.31.250." $prefix "auto mode should use Docker management targets for local API base $apiBase when the backend container is running."
  Assert-Equal 1 $script:connectAttempts "auto mode should connect the backend container before selecting Docker targets for local API base $apiBase."
  Assert-Match "auto: backend container is running and connected" (Get-WispSeedTargetLog) "auto mode should log the Docker decision for $apiBase."
}

Reset-WispSeedTargetTestState -BackendRunning $false -ConnectSucceeds $false
$prefix = Get-WispSeedTargetPrefix -TargetMode "auto" -ApiBase "http://localhost:8080"
Assert-Equal "127.0.10." $prefix "auto mode should fall back to host loopback targets for localhost API base when Docker backend is unavailable."
Assert-Equal 0 $script:connectAttempts "auto mode should not attempt Docker connect when the backend container is not running."
Assert-Match "auto: Docker backend unavailable for API host 'localhost'" (Get-WispSeedTargetLog) "auto mode should log the localhost fallback decision."

Reset-WispSeedTargetTestState
$prefix = Get-WispSeedTargetPrefix -TargetMode "host" -ApiBase "http://localhost:8080"
Assert-Equal "127.0.10." $prefix "host mode should use host loopback targets."
Assert-Equal 0 $script:connectAttempts "host mode should not attempt to connect the backend container."
Assert-Equal 0 $script:backendRunningChecks "host mode should not check backend container state."
Assert-Match "mode: host" (Get-WispSeedTargetLog) "host mode should log the explicit host decision."

Reset-WispSeedTargetTestState
$prefix = Get-WispSeedTargetPrefix -TargetMode "docker" -ApiBase "http://localhost:8080"
Assert-Equal "172.31.250." $prefix "docker mode should keep Docker management targets for localhost API base."
Assert-Equal 1 $script:connectAttempts "docker mode should still attempt to connect the backend container."
Assert-Match "mode: docker" (Get-WispSeedTargetLog) "docker mode should log the explicit docker decision."

Reset-WispSeedTargetTestState
$prefix = Get-WispSeedTargetPrefix -TargetMode "auto" -ApiBase "http://theia-backend:8080"
Assert-Equal "172.31.250." $prefix "auto mode should keep Docker targets for non-localhost API base when the backend can connect."
Assert-Equal 1 $script:connectAttempts "auto mode should attempt Docker connect for non-localhost API base."
Assert-Match "auto: backend container is running and connected" (Get-WispSeedTargetLog) "auto mode should log the Docker decision for non-local API base."

Reset-WispSeedTargetTestState -ConnectSucceeds $false
$prefix = Get-WispSeedTargetPrefix -TargetMode "auto" -ApiBase "http://theia-backend:8080"
Assert-Equal "127.0.10." $prefix "auto mode should fall back to host targets for non-localhost API base when Docker connect is unavailable."
Assert-Equal 1 $script:connectAttempts "auto mode should attempt Docker connect before falling back for non-localhost API base."
Assert-Match "auto: Docker backend unavailable for API host 'theia-backend'" (Get-WispSeedTargetLog) "auto mode should log the non-local fallback decision."

$psSeedFiles = @(
  "scripts/seed-wisp.ps1",
  "scripts/seed-wisp-radio.ps1"
)

foreach ($seedFile in $psSeedFiles) {
  $content = Get-Content -Raw -Path $seedFile
  Assert-True ($content -match 'Get-WispSeedTargetPrefix\s+-TargetMode\s+\$TargetMode\s+-ApiBase\s+\$ApiBase') "$seedFile must pass ApiBase into Get-WispSeedTargetPrefix."
}

$shSeedFiles = @(
  "scripts/seed-wisp.sh",
  "scripts/seed-wisp-radio.sh"
)

foreach ($seedFile in $shSeedFiles) {
  $content = Get-Content -Raw -Path $seedFile
  Assert-True ($content -match 'wisp_seed_target_prefix\s+"\$TARGET_MODE"\s+"\$API_BASE"') "$seedFile must pass API_BASE into wisp_seed_target_prefix."
}

$shCommon = Get-Content -Raw -Path "scripts/wisp-lab-common.sh"
Assert-True ($shCommon -match 'local api_base=') "shell WISP helper must accept an API base argument."
Assert-True ($shCommon -match 'localhost' -and $shCommon -match '127\.0\.0\.1' -and $shCommon -match '::1') "shell WISP helper must recognize localhost API base hosts."
Assert-True (Test-Path "scripts/test-wisp-seed-target-mode.sh") "shell WISP target-mode runtime test must exist."

Write-Output "WISP seed target mode behavior is valid"
