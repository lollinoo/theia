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
Assert-True ($backendEnvironment.THEIA_DB_DSN -like "*@postgres:5432/theia*") "backend must reach PostgreSQL over the Compose network"

$frontendEnvironment = Get-ServiceProperty $frontend "environment"
Assert-True ($frontendEnvironment.VITE_API_URL -eq "http://backend:8080") "frontend dev proxy must reach backend over the Compose network"

Write-Output "docker-compose.yml dev/test shape is valid"
