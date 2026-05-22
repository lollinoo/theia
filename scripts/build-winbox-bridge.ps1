param(
  [string]$OutDir = "bridge_binaries",
  [string]$Source = "./cmd/winbox-bridge/",
  [string[]]$Targets = @("windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64")
)

$ErrorActionPreference = "Stop"

function Resolve-SafeBridgeOutDir {
  param([Parameter(Mandatory = $true)][string]$Path)

  if ([string]::IsNullOrWhiteSpace($Path)) {
    throw "OutDir is required."
  }

  $repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
  $basePath = (Get-Location).ProviderPath
  $candidate = if ([System.IO.Path]::IsPathRooted($Path)) { $Path } else { Join-Path $basePath $Path }
  $resolved = [System.IO.Path]::GetFullPath($candidate)
  $allowedRoot = [System.IO.Path]::GetFullPath((Join-Path $repoRoot "bridge_binaries"))
  $separators = [char[]]@([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar)
  $resolved = $resolved.TrimEnd($separators)
  $allowedRoot = $allowedRoot.TrimEnd($separators)
  $allowedPrefix = $allowedRoot + [System.IO.Path]::DirectorySeparatorChar

  if (-not ($resolved.Equals($allowedRoot, [System.StringComparison]::OrdinalIgnoreCase) -or $resolved.StartsWith($allowedPrefix, [System.StringComparison]::OrdinalIgnoreCase))) {
    throw "OutDir must resolve inside '$allowedRoot'. Got '$resolved'."
  }

  return $resolved
}

$resolvedOutDir = Resolve-SafeBridgeOutDir -Path $OutDir

Remove-Item -LiteralPath $resolvedOutDir -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $resolvedOutDir | Out-Null

$oldCgo = $env:CGO_ENABLED
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH

try {
  foreach ($target in $Targets) {
    $parts = $target -split "/", 2
    $os = $parts[0]
    $arch = $parts[1]
    $extension = ""
    $ldExtra = ""

    if ($os -eq "windows") {
      $extension = ".exe"
      $ldExtra = "-H=windowsgui"
    }

    $output = Join-Path $resolvedOutDir "winbox-bridge-$os-$arch$extension"
    $ldFlags = "-s -w $ldExtra".Trim()

    Write-Output "Building $output ..."
    $env:CGO_ENABLED = "0"
    $env:GOOS = $os
    $env:GOARCH = $arch
    & go build -ldflags "$ldFlags" -o "$output" $Source
    if ($LASTEXITCODE -ne 0) {
      exit $LASTEXITCODE
    }
  }
}
finally {
  $env:CGO_ENABLED = $oldCgo
  $env:GOOS = $oldGoos
  $env:GOARCH = $oldGoarch
}

Write-Output ""
Write-Output "Bridge binaries built in ${resolvedOutDir}/:"
Get-ChildItem -LiteralPath $resolvedOutDir
Write-Output ""
Write-Output "NOTE: macOS binaries (darwin/amd64, darwin/arm64) require CGO_ENABLED=1."
Write-Output "      Build natively on Mac: CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags=""-s -w"" -o $resolvedOutDir/winbox-bridge-darwin-arm64 $Source"
