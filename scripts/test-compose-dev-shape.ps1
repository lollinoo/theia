param()

$ErrorActionPreference = "Stop"

$composeJson = & docker compose --profile dev --profile test config --format json

if ($LASTEXITCODE -ne 0) {
  exit $LASTEXITCODE
}

$config = $composeJson | ConvertFrom-Json
$services = $config.services

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )

  if (-not $Condition) {
    throw $Message
  }
}

function Get-ServiceProperty {
  param(
    [object]$Service,
    [string]$Name
  )

  $property = $Service.PSObject.Properties[$Name]
  if ($null -eq $property) {
    return $null
  }

  return $property.Value
}

foreach ($removedService in @("snmp-router", "snmp-switch", "ssh-mock", "snmp-ap")) {
  Assert-True ($null -eq (Get-ServiceProperty $services $removedService)) "service '$removedService' must not be defined in docker-compose.yml"
}

$backend = Get-ServiceProperty $services "backend"
$frontend = Get-ServiceProperty $services "frontend"
Assert-True ($null -ne $backend) "backend service must be defined"
Assert-True ($null -ne $frontend) "frontend service must be defined"

foreach ($serviceProperty in $services.PSObject.Properties) {
  Assert-True ($null -eq (Get-ServiceProperty $serviceProperty.Value "network_mode")) "service '$($serviceProperty.Name)' must not use network_mode: host"
}

$backendPorts = @(Get-ServiceProperty $backend "ports")
$frontendPorts = @(Get-ServiceProperty $frontend "ports")
Assert-True ($backendPorts.Count -gt 0) "backend must publish a host port"
Assert-True ($frontendPorts.Count -gt 0) "frontend must publish a host port"

$backendPortMatches = @($backendPorts | Where-Object { $_.target -eq 8080 -and $_.published -eq "8080" -and $_.protocol -eq "tcp" })
$frontendPortMatches = @($frontendPorts | Where-Object { $_.target -eq 3000 -and $_.published -eq "3000" -and $_.protocol -eq "tcp" })
Assert-True ($backendPortMatches.Count -gt 0) "backend must publish 8080:8080/tcp"
Assert-True ($frontendPortMatches.Count -gt 0) "frontend must publish 3000:3000/tcp"

Assert-True ($backend.depends_on.PSObject.Properties.Name -notcontains "snmp-router") "backend must not depend on snmp-router"
Assert-True ($backend.depends_on.PSObject.Properties.Name -notcontains "snmp-switch") "backend must not depend on snmp-switch"
Assert-True ($backend.depends_on.PSObject.Properties.Name -notcontains "snmp-ap") "backend must not depend on snmp-ap"

$backendEnvironment = Get-ServiceProperty $backend "environment"
$legacyTokenName = "THEIA_" + "OPERATOR_TOKEN"
$bearerHeaderText = "Authorization: " + "Bearer"
Assert-True ($backendEnvironment.THEIA_DB_DSN -like "*@postgres:5432/theia*") "backend must reach PostgreSQL over the Compose network"
Assert-True ($null -ne $backendEnvironment.THEIA_SESSION_SECRET) "backend must receive THEIA_SESSION_SECRET in dev/test Compose profiles"
Assert-True ([string]::IsNullOrEmpty($backendEnvironment.$legacyTokenName)) "backend must not receive legacy auth token"

$composeSource = Get-Content -Raw -Path "docker-compose.yml"
Assert-True ($composeSource -notmatch $legacyTokenName) "docker-compose.yml must not reference legacy auth token"
Assert-True ($composeSource -match 'THEIA_SESSION_SECRET=\$\{THEIA_SESSION_SECRET:-dev-session-secret-change-me-32bytes\}') "docker-compose.yml must provide THEIA_SESSION_SECRET to backend"
Assert-True ($composeSource -match 'curl -sf http://localhost:8080/api/v1/auth/me') "backend healthcheck must use /api/v1/auth/me"
Assert-True ($composeSource -notmatch $bearerHeaderText) "backend healthcheck must not use bearer Authorization"

$frontendEnvironment = Get-ServiceProperty $frontend "environment"
Assert-True ($frontendEnvironment.VITE_API_URL -eq "http://backend:8080") "frontend dev proxy must reach backend over the Compose network"

foreach ($nginxPath in @("frontend/nginx.conf", "frontend/nginx.conf.template")) {
  $nginxSource = Get-Content -Raw -Path $nginxPath
  Assert-True ($nginxSource -notmatch 'proxy_set_header Host \$host;') "$nginxPath must preserve the browser Host header port with `$http_host"
  $httpHostMatches = [regex]::Matches($nginxSource, 'proxy_set_header Host \$http_host;')
  Assert-True ($httpHostMatches.Count -eq 3) "$nginxPath must set Host to `$http_host in all API proxy locations"
}

Write-Output "docker-compose.yml dev/test shape is valid"
