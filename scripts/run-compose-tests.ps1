param(
  [switch]$Integration
)

$ErrorActionPreference = "Stop"

$requiredServices = @("postgres")

$runningServices = & docker compose --profile test ps --status running --services 2>$null
if ($LASTEXITCODE -ne 0) {
  $runningServices = @()
}

$startedServices = @()
foreach ($service in $requiredServices) {
  if ($runningServices -notcontains $service) {
    $startedServices += $service
  }
}

$status = 0
$cleanupStatus = 0

& docker compose --profile test up -d --wait @requiredServices
if ($LASTEXITCODE -ne 0) {
  $status = $LASTEXITCODE
}

if ($status -eq 0) {
  if ($Integration) {
    & docker compose --profile test run --rm backend go test ./... -tags=integration -count=1 -v
  } else {
    & docker compose --profile test run --rm --no-deps backend go test ./... -count=1 -v
  }

  if ($LASTEXITCODE -ne 0) {
    $status = $LASTEXITCODE
  }
}

if ($startedServices.Count -gt 0) {
  & docker compose --profile test stop @startedServices
  if ($LASTEXITCODE -ne 0) {
    $cleanupStatus = $LASTEXITCODE
  }
}

if ($status -eq 0) {
  $status = $cleanupStatus
}

exit $status
