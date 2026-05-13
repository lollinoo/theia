$script:WispLabComposeFile = "docker-compose.wisp-lab.yml"
$script:WispLabNetwork = "theia-wisp-lab_wisp-access-mgmt"
$script:WispBackendContainer = "theia-backend"
$script:WispDockerTargetPrefix = "172.31.250."
$script:WispHostTargetPrefix = "127.0.10."

function Test-WispDockerObject {
  param(
    [Parameter(Mandatory = $true)][string]$Kind,
    [Parameter(Mandatory = $true)][string]$Name
  )

  $previousErrorActionPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $null = & docker $Kind inspect $Name 2>$null
    return $LASTEXITCODE -eq 0
  }
  finally {
    $ErrorActionPreference = $previousErrorActionPreference
  }
}

function Test-WispBackendRunning {
  if (-not (Test-WispBackendExists)) {
    return $false
  }

  $previousErrorActionPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $running = & docker inspect -f '{{.State.Running}}' $script:WispBackendContainer 2>$null
    return ($LASTEXITCODE -eq 0 -and [string]$running -eq "true")
  }
  finally {
    $ErrorActionPreference = $previousErrorActionPreference
  }
}

function Test-WispBackendExists {
  return (Test-WispDockerObject -Kind "container" -Name $script:WispBackendContainer)
}

function Test-WispContainerNetwork {
  param(
    [Parameter(Mandatory = $true)][string]$ContainerName,
    [Parameter(Mandatory = $true)][string]$NetworkName
  )

  if (-not (Test-WispDockerObject -Kind "container" -Name $ContainerName)) {
    return $false
  }

  $previousErrorActionPreference = $ErrorActionPreference
  try {
    $ErrorActionPreference = "Continue"
    $networks = & docker inspect -f '{{range $name, $config := .NetworkSettings.Networks}}{{println $name}}{{end}}' $ContainerName 2>$null
    if ($LASTEXITCODE -ne 0) {
      return $false
    }

    return @($networks) -contains $NetworkName
  }
  finally {
    $ErrorActionPreference = $previousErrorActionPreference
  }
}

function Connect-WispBackendToLabNetwork {
  if (-not (Test-WispBackendRunning)) {
    return $false
  }

  if (-not (Test-WispDockerObject -Kind "network" -Name $script:WispLabNetwork)) {
    Write-Warning "WISP lab network '$script:WispLabNetwork' does not exist yet."
    return $false
  }

  if (Test-WispContainerNetwork -ContainerName $script:WispBackendContainer -NetworkName $script:WispLabNetwork) {
    return $true
  }

  $connectOutput = & docker network connect $script:WispLabNetwork $script:WispBackendContainer 2>&1
  if ($LASTEXITCODE -ne 0) {
    $message = [string]::Join("`n", @($connectOutput))
    if ($message -notmatch "already exists|is already connected") {
      throw "Failed to connect $script:WispBackendContainer to $script:WispLabNetwork. $message"
    }
  }

  Write-Host "Connected $script:WispBackendContainer to $script:WispLabNetwork"
  return $true
}

function Disconnect-WispBackendFromLabNetwork {
  if (-not (Test-WispBackendExists)) {
    return $false
  }

  if (-not (Test-WispDockerObject -Kind "network" -Name $script:WispLabNetwork)) {
    return $false
  }

  if (-not (Test-WispContainerNetwork -ContainerName $script:WispBackendContainer -NetworkName $script:WispLabNetwork)) {
    return $false
  }

  $disconnectOutput = & docker network disconnect $script:WispLabNetwork $script:WispBackendContainer 2>&1
  if ($LASTEXITCODE -ne 0) {
    $message = [string]::Join("`n", @($disconnectOutput))
    if ($message -notmatch "is not connected|No such container|No such network") {
      throw "Failed to disconnect $script:WispBackendContainer from $script:WispLabNetwork. $message"
    }
  }

  Write-Host "Disconnected $script:WispBackendContainer from $script:WispLabNetwork"
  return $true
}

function Get-WispSeedTargetPrefix {
  param([string]$TargetMode = "")

  if ([string]::IsNullOrWhiteSpace($TargetMode)) {
    $TargetMode = $env:WISP_SEED_TARGET_MODE
  }
  if ([string]::IsNullOrWhiteSpace($TargetMode)) {
    $TargetMode = "auto"
  }

  $normalizedMode = $TargetMode.Trim().ToLowerInvariant()
  switch ($normalizedMode) {
    "docker" {
      if (-not (Connect-WispBackendToLabNetwork)) {
        throw "WISP_SEED_TARGET_MODE=docker requires the '$script:WispBackendContainer' container and '$script:WispLabNetwork' network to be running."
      }
      Write-Host "Using WISP Docker management targets ${script:WispDockerTargetPrefix}21-${script:WispDockerTargetPrefix}42"
      return $script:WispDockerTargetPrefix
    }
    "host" {
      Write-Host "Using WISP host loopback targets ${script:WispHostTargetPrefix}21-${script:WispHostTargetPrefix}42"
      return $script:WispHostTargetPrefix
    }
    "auto" {
      if (Test-WispBackendRunning) {
        if (Connect-WispBackendToLabNetwork) {
          Write-Host "Using WISP Docker management targets ${script:WispDockerTargetPrefix}21-${script:WispDockerTargetPrefix}42"
          return $script:WispDockerTargetPrefix
        }
      }

      Write-Host "Using WISP host loopback targets ${script:WispHostTargetPrefix}21-${script:WispHostTargetPrefix}42"
      return $script:WispHostTargetPrefix
    }
    default {
      throw "Invalid WISP seed target mode '$TargetMode'. Use auto, docker, or host."
    }
  }
}
