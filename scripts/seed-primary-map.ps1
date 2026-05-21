function Get-TheiaApiHeaders {
  $headers = @{}
  $token = $env:THEIA_OPERATOR_TOKEN
  if (-not [string]::IsNullOrWhiteSpace($token)) {
    $headers["Authorization"] = "Bearer $token"
  }
  return $headers
}

function Get-PrimaryMapId {
  param([string]$ApiBase)

  $payload = Invoke-RestMethod -Uri "$ApiBase/api/v1/canvas/maps" -Headers (Get-TheiaApiHeaders) -TimeoutSec 10
  $maps = @($payload.data) | Where-Object { $null -ne $_ }

  foreach ($item in $maps) {
    if ($item.is_default -eq $true) {
      return [string]$item.id
    }
  }

  if ($maps.Count -gt 0) {
    return [string]$maps[0].id
  }

  return ""
}

function Get-DeviceIdByIp {
  param(
    [string]$ApiBase,
    [string]$Ip
  )

  $payload = Invoke-RestMethod -Uri "$ApiBase/api/v1/devices" -Headers (Get-TheiaApiHeaders) -TimeoutSec 10
  foreach ($item in (@($payload.data) | Where-Object { $null -ne $_ })) {
    if ($item.attributes.ip -eq $Ip) {
      return [string]$item.id
    }
  }

  return ""
}

function Add-DeviceToPrimaryMap {
  param(
    [string]$ApiBase,
    [string]$DeviceId
  )

  $mapId = Get-PrimaryMapId -ApiBase $ApiBase
  if ([string]::IsNullOrWhiteSpace($mapId) -or [string]::IsNullOrWhiteSpace($DeviceId)) {
    return
  }

  try {
    Invoke-RestMethod `
      -Method Post `
      -Uri "$ApiBase/api/v1/canvas/maps/$mapId/devices/$DeviceId" `
      -ContentType "application/json" `
      -Headers (Get-TheiaApiHeaders) `
      -Body '{"include_connected_links": true}' | Out-Null
  }
  catch {
    $errorText = @(
      $_.Exception.Message
      $_.ErrorDetails.Message
      ($_ | Out-String)
    ) -join " "

    if ($errorText -match "device already exists in this map") {
      return
    }

    throw
  }
}

function Invoke-TopologyDiscovery {
  param(
    [string]$ApiBase,
    [string]$DeviceId
  )

  if ([string]::IsNullOrWhiteSpace($DeviceId)) {
    return
  }

  Invoke-RestMethod `
    -Method Post `
    -Uri "$ApiBase/api/v1/devices/$DeviceId/topology-discovery" `
    -Headers (Get-TheiaApiHeaders) `
    -TimeoutSec 30 | Out-Null
}
