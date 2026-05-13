param(
  [string]$OutDir = "bridge_binaries",
  [string]$Source = "./cmd/winbox-bridge/",
  [string[]]$Targets = @("windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64")
)

$ErrorActionPreference = "Stop"

Remove-Item -Recurse -Force $OutDir -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $OutDir | Out-Null

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

    $output = Join-Path $OutDir "winbox-bridge-$os-$arch$extension"
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
Write-Output "Bridge binaries built in ${OutDir}/:"
Get-ChildItem $OutDir
Write-Output ""
Write-Output "NOTE: macOS binaries (darwin/amd64, darwin/arm64) require CGO_ENABLED=1."
Write-Output "      Build natively on Mac: CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags=""-s -w"" -o $OutDir/winbox-bridge-darwin-arm64 $Source"
