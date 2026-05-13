param(
  [Parameter(Mandatory = $true)]
  [ValidateSet("synthetic", "wisp")]
  [string]$Mode,

  [Parameter(Mandatory = $true)]
  [string]$ApiBase,

  [Parameter(Mandatory = $true)]
  [string]$OutputDir
)

$ErrorActionPreference = "Stop"

New-Item -ItemType Directory -Force $OutputDir | Out-Null

$scaleFiles = if ($Mode -eq "synthetic") {
  "- ``scale-300-baseline.json```n- ``scale-300-burst-adds.json``"
} else {
  "- ``scale-wisp-hybrid.json``"
}

@"
# Phase 4 Validation Evidence

- Mode: $Mode
- API base: $ApiBase
- Output directory: $OutputDir

## Evidence Files

$scaleFiles
- ``metrics.prom``

## Required Evidence Surfaces

- ``theia_refresh_snapshot_build_seconds``
- ``theia_refresh_topology_reload_total``
- ``theia_state_changes_dropped_total``
- ``window.__THEIA_CANVAS_METRICS__``
"@ | Set-Content -Encoding UTF8 (Join-Path $OutputDir "README.md")

if ($Mode -eq "synthetic") {
  & go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -out (Join-Path $OutputDir "scale-300-baseline.json") > $null
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }

  & go run ./cmd/theia-scale-lab -profile 300 -scenario burst-adds -out (Join-Path $OutputDir "scale-300-burst-adds.json") > $null
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
} else {
  & go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -fixture internal/scalelab/testdata/wisp-hybrid.json -out (Join-Path $OutputDir "scale-wisp-hybrid.json") > $null
  if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
  }
}

$metricsPath = Join-Path $OutputDir "metrics.prom"
Invoke-WebRequest -Uri "$ApiBase/metrics" -OutFile $metricsPath -UseBasicParsing

foreach ($metric in @("theia_refresh_snapshot_build_seconds", "theia_refresh_topology_reload_total", "theia_state_changes_dropped_total")) {
  if (-not (Select-String -Path $metricsPath -Pattern "^$metric" -Quiet)) {
    Write-Error "Missing required metric family: $metric"
    exit 1
  }
}

Write-Output "Saved Phase 4 $Mode evidence to $OutputDir"
