param(
  [Parameter(Mandatory = $true)]
  [string]$Version
)

$ErrorActionPreference = "Stop"

function Invoke-GitChecked {
  param([string[]]$Arguments)

  & git @Arguments
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
}

$currentVersion = (& git describe --tags --always 2>$null)
if ([string]::IsNullOrWhiteSpace($currentVersion)) {
  $currentVersion = "dev"
}

if ([string]::IsNullOrWhiteSpace($Version) -or $Version -eq $currentVersion) {
  Write-Error "Usage: make release VERSION=1.5.1"
  exit 1
}

$status = & git status --porcelain
if (-not [string]::IsNullOrWhiteSpace(($status -join ""))) {
  Write-Error "Error: working tree is not clean"
  exit 1
}

$branch = (& git rev-parse --abbrev-ref HEAD).Trim()
if ($branch -ne "master") {
  Write-Error "Error: must be on master branch"
  exit 1
}

& git rev-parse "v$Version" *> $null
if ($LASTEXITCODE -eq 0) {
  Write-Error "Error: tag v$Version already exists"
  exit 1
}

if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') {
  Write-Error "Error: VERSION must be valid semver (e.g., 1.5.1)"
  exit 1
}

Invoke-GitChecked @("tag", "-a", "v$Version", "-m", "release: v$Version")
Invoke-GitChecked @("push", "origin", "v$Version")

Write-Output ""
Write-Output "Release v$Version tagged and pushed."
Write-Output "CI will build and push Docker images to GHCR."
