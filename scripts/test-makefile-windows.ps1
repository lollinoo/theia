param()

$ErrorActionPreference = "Stop"

function Invoke-Checked {
  param(
    [string]$Name,
    [string[]]$Command,
    [string[]]$ExpectedSubstrings
  )

  $previousErrorActionPreference = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  $output = & $Command[0] @($Command[1..($Command.Count - 1)]) 2>&1
  $exitCode = $LASTEXITCODE
  $ErrorActionPreference = $previousErrorActionPreference
  if ($exitCode -ne 0) {
    throw "$Name failed with exit code ${exitCode}: $($output -join [Environment]::NewLine)"
  }

  $text = $output -join [Environment]::NewLine
  foreach ($substring in $ExpectedSubstrings) {
    if (-not $text.Contains($substring)) {
      throw "$Name output did not contain '$substring'. Output: $text"
    }
  }
}

Invoke-Checked -Name "make help" -Command @("make", "help") -ExpectedSubstrings @("dev", "test", "bridge-build-all")
Invoke-Checked -Name "make version" -Command @("make", "version") -ExpectedSubstrings @("Version:", "Git commit:", "Build date:")
Invoke-Checked -Name "make dry-run test" -Command @("make", "-n", "test") -ExpectedSubstrings @("run-compose-tests.ps1")
Invoke-Checked -Name "make dry-run bridge-build-all" -Command @("make", "-n", "bridge-build-all") -ExpectedSubstrings @("build-winbox-bridge.ps1")
