param()

$ErrorActionPreference = "Stop"

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )

  if (-not $Condition) {
    throw $Message
  }
}

function Quote-PowerShellLiteral {
  param([string]$Value)

  return "'" + $Value.Replace("'", "''") + "'"
}

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$sourceScript = Join-Path $repoRoot "scripts/build-winbox-bridge.ps1"
$tempRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("theia-winbox-bridge-safety-" + [guid]::NewGuid())

try {
  $fixtureRoot = Join-Path $tempRoot "repo"
  $fixtureScripts = Join-Path $fixtureRoot "scripts"
  New-Item -ItemType Directory -Force $fixtureScripts | Out-Null
  Copy-Item -LiteralPath $sourceScript -Destination (Join-Path $fixtureScripts "build-winbox-bridge.ps1")

  $scriptUnderTest = Join-Path $fixtureScripts "build-winbox-bridge.ps1"
  $unsafeOutDir = Join-Path $tempRoot "outside-output"
  New-Item -ItemType Directory -Force $unsafeOutDir | Out-Null
  $sentinel = Join-Path $unsafeOutDir "sentinel.txt"
  Set-Content -LiteralPath $sentinel -Value "keep"

  $command = "& $(Quote-PowerShellLiteral $scriptUnderTest) -OutDir $(Quote-PowerShellLiteral $unsafeOutDir) -Targets @()"
  try {
    $output = & powershell -NoProfile -ExecutionPolicy Bypass -Command $command 2>&1
    $exitCode = $LASTEXITCODE
  }
  catch {
    $output = @($_.Exception.Message)
    $exitCode = 1
  }
  $text = $output -join [Environment]::NewLine

  Assert-True ($exitCode -ne 0) "unsafe OutDir outside repository must be rejected"
  Assert-True (Test-Path -LiteralPath $sentinel) "unsafe OutDir must not be deleted"
  Assert-True ($text -match "OutDir") "unsafe OutDir rejection should mention OutDir"
}
finally {
  Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

Write-Output "WinBox bridge build output safety is valid"
