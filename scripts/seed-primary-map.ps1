function Test-TheiaInteractiveShell {
  return (-not [Console]::IsInputRedirected) -and (-not [Console]::IsOutputRedirected)
}

function ConvertFrom-SecureStringToPlainText {
  param([System.Security.SecureString]$Value)

  $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Value)
  try {
    return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
  }
  finally {
    if ($bstr -ne [IntPtr]::Zero) {
      [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
    }
  }
}

function Read-TheiaSeedUsername {
  $username = $env:THEIA_API_USERNAME
  if (-not [string]::IsNullOrWhiteSpace($username)) {
    return $username.Trim()
  }

  if (-not (Test-TheiaInteractiveShell)) {
    return "administrator"
  }

  $typed = Read-Host "Theia username [administrator]"
  if ([string]::IsNullOrWhiteSpace($typed)) {
    return "administrator"
  }
  return $typed.Trim()
}

function Read-TheiaSeedPassword {
  param(
    [string]$Prompt,
    [string]$EnvName,
    [string]$NonInteractiveMessage
  )

  $fromEnv = [Environment]::GetEnvironmentVariable($EnvName)
  if (-not [string]::IsNullOrEmpty($fromEnv)) {
    return $fromEnv
  }

  if (-not (Test-TheiaInteractiveShell)) {
    throw $NonInteractiveMessage
  }

  $secure = Read-Host $Prompt -AsSecureString
  return ConvertFrom-SecureStringToPlainText -Value $secure
}

function Get-TheiaCookieHeader {
  param([string]$ApiBase)

  if ($null -eq $script:theiaApiSession) {
    return ""
  }

  $cookies = $script:theiaApiSession.Cookies.GetCookies([Uri]$ApiBase)
  $pairs = @()
  foreach ($cookie in $cookies) {
    $pairs += "$($cookie.Name)=$($cookie.Value)"
  }
  return ($pairs -join "; ")
}

function Get-TheiaCookieValue {
  param(
    [string]$ApiBase,
    [string]$Name
  )

  if ($null -eq $script:theiaApiSession) {
    return ""
  }

  $cookies = $script:theiaApiSession.Cookies.GetCookies([Uri]$ApiBase)
  foreach ($cookie in $cookies) {
    if ($cookie.Name -eq $Name) {
      return [string]$cookie.Value
    }
  }
  return ""
}

function Invoke-TheiaPasswordChangeIfRequired {
  param(
    [string]$ApiBase,
    [object]$LoginResponse,
    [string]$CurrentPassword
  )

  if ($null -eq $LoginResponse.user -or $LoginResponse.user.must_change_password -ne $true) {
    return
  }

  $newPassword = Read-TheiaSeedPassword `
    -Prompt "New Theia password" `
    -EnvName "THEIA_API_NEW_PASSWORD" `
    -NonInteractiveMessage "Theia login requires a password change. Sign in once and change the password before running seed scripts non-interactively."

  $headers = Get-TheiaApiHeaders -ApiBase $ApiBase -Mutating -SkipLogin
  $body = @{
    current_password = $CurrentPassword
    new_password = $newPassword
  } | ConvertTo-Json -Compress

  Invoke-RestMethod `
    -Method Post `
    -Uri "$ApiBase/api/v1/auth/password/change" `
    -ContentType "application/json" `
    -Headers $headers `
    -Body $body | Out-Null
}

function Ensure-TheiaApiSession {
  param([string]$ApiBase)

  if ($script:theiaApiSessionBase -eq $ApiBase -and
      -not [string]::IsNullOrWhiteSpace((Get-TheiaCookieValue -ApiBase $ApiBase -Name "theia_session")) -and
      -not [string]::IsNullOrWhiteSpace((Get-TheiaCookieValue -ApiBase $ApiBase -Name "theia_csrf"))) {
    return
  }

  $username = Read-TheiaSeedUsername
  $password = Read-TheiaSeedPassword `
    -Prompt "Theia password" `
    -EnvName "THEIA_API_PASSWORD" `
    -NonInteractiveMessage "Theia seed scripts need a password-session login. Set THEIA_API_PASSWORD for local automation or run interactively."

  $body = @{
    identifier = $username
    password = $password
  } | ConvertTo-Json -Compress

  $login = Invoke-WebRequest `
    -Method Post `
    -Uri "$ApiBase/api/v1/auth/login" `
    -ContentType "application/json" `
    -Body $body `
    -SessionVariable "theiaApiSession" `
    -TimeoutSec 10 `
    -UseBasicParsing

  if ($null -eq $script:theiaApiSession -and $null -ne $theiaApiSession) {
    $script:theiaApiSession = $theiaApiSession
  }
  $script:theiaApiSessionBase = $ApiBase
  $loginResponse = $login.Content | ConvertFrom-Json
  Invoke-TheiaPasswordChangeIfRequired -ApiBase $ApiBase -LoginResponse $loginResponse -CurrentPassword $password
}

function Get-TheiaApiHeaders {
  param(
    [string]$ApiBase,
    [switch]$Mutating,
    [switch]$SkipLogin
  )

  if ([string]::IsNullOrWhiteSpace($ApiBase)) {
    $ApiBase = $script:theiaApiSessionBase
  }
  if ([string]::IsNullOrWhiteSpace($ApiBase)) {
    throw "ApiBase is required to build Theia API session headers"
  }

  if (-not $SkipLogin) {
    Ensure-TheiaApiSession -ApiBase $ApiBase
  }

  $headers = @{}
  $cookieHeader = Get-TheiaCookieHeader -ApiBase $ApiBase
  if (-not [string]::IsNullOrWhiteSpace($cookieHeader)) {
    $headers["Cookie"] = $cookieHeader
  }
  if ($Mutating) {
    $csrf = Get-TheiaCookieValue -ApiBase $ApiBase -Name "theia_csrf"
    if ([string]::IsNullOrWhiteSpace($csrf)) {
      throw "Theia login did not return a CSRF cookie"
    }
    $headers["X-CSRF-Token"] = $csrf
  }
  return $headers
}

function Get-PrimaryMapId {
  param([string]$ApiBase)

  $payload = Invoke-RestMethod -Uri "$ApiBase/api/v1/canvas/maps" -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase) -TimeoutSec 10
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

  $payload = Invoke-RestMethod -Uri "$ApiBase/api/v1/devices" -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase) -TimeoutSec 10
  foreach ($item in (@($payload.data) | Where-Object { $null -ne $_ })) {
    if ($item.attributes.ip -eq $Ip) {
      return [string]$item.id
    }
  }

  return ""
}

function Get-DeviceIdByHostnameAndTag {
  param(
    [string]$ApiBase,
    [string]$Hostname,
    [string]$TagKey,
    [string]$TagValue
  )

  $payload = Invoke-RestMethod -Uri "$ApiBase/api/v1/devices" -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase) -TimeoutSec 10
  foreach ($item in (@($payload.data) | Where-Object { $null -ne $_ })) {
    $attributes = $item.attributes
    if ($null -eq $attributes -or $attributes.hostname -ne $Hostname) {
      continue
    }

    $tags = $attributes.tags
    if ($null -eq $tags) {
      continue
    }

    $tagProperty = $tags.PSObject.Properties[$TagKey]
    if ($null -ne $tagProperty -and [string]$tagProperty.Value -eq $TagValue) {
      return [string]$item.id
    }
  }

  return ""
}

function Update-DeviceIp {
  param(
    [string]$ApiBase,
    [string]$DeviceId,
    [string]$Ip
  )

  $body = @{
    ip = $Ip
  } | ConvertTo-Json -Compress

  Invoke-RestMethod `
    -Method Put `
    -Uri "$ApiBase/api/v1/devices/$DeviceId" `
    -ContentType "application/json" `
    -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase -Mutating) `
    -Body $body | Out-Null
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
      -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase -Mutating) `
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
    -Headers (Get-TheiaApiHeaders -ApiBase $ApiBase -Mutating) `
    -TimeoutSec 30 | Out-Null
}
