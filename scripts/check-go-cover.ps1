param(
  [Parameter(Mandatory = $true)]
  [string]$ProfilePath,

  [Parameter(Mandatory = $true)]
  [double]$MinPercent
)

$ErrorActionPreference = "Stop"

$coverageOutput = & go tool cover "-func=$ProfilePath"
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}

$totalLine = $coverageOutput | Select-Object -Last 1
if ($totalLine -notmatch '^total:\s+\(statements\)\s+([0-9.]+)%') {
  Write-Error "Unable to parse total coverage from: $totalLine"
  exit 1
}

$actual = [double]$Matches[1]
if ($actual -lt $MinPercent) {
  Write-Error ("backend coverage {0:N1}% is below required {1:N1}%" -f $actual, $MinPercent)
  exit 1
}

Write-Output ("backend coverage {0:N1}% meets required {1:N1}%" -f $actual, $MinPercent)
