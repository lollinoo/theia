$ErrorActionPreference = "Stop"

. "$PSScriptRoot/wisp-lab-common.ps1"

Disconnect-WispBackendFromLabNetwork | Out-Null

& docker compose -f $script:WispLabComposeFile down
if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}
