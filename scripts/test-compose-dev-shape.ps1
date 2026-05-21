param()

$ErrorActionPreference = "Stop"

$previousOperatorToken = $env:THEIA_OPERATOR_TOKEN
$env:THEIA_OPERATOR_TOKEN = "test-operator-token-not-secret-1234"

try {
  $composeJson = & docker compose --profile dev --profile test config --format json
}
finally {
  if ($null -eq $previousOperatorToken) {
    Remove-Item Env:\THEIA_OPERATOR_TOKEN -ErrorAction SilentlyContinue
  }
  else {
    $env:THEIA_OPERATOR_TOKEN = $previousOperatorToken
  }
}

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
Assert-True ($backendEnvironment.THEIA_OPERATOR_TOKEN -eq "test-operator-token-not-secret-1234") "backend must receive THEIA_OPERATOR_TOKEN in dev/test Compose profiles"

$composeSource = Get-Content -Raw -Path "docker-compose.yml"
Assert-True ($composeSource -match 'THEIA_OPERATOR_TOKEN=\$\{THEIA_OPERATOR_TOKEN:\?THEIA_OPERATOR_TOKEN must be set\}') "docker-compose.yml must fail closed when THEIA_OPERATOR_TOKEN is missing"
Assert-True ($composeSource -match 'Authorization: Bearer \$\$THEIA_OPERATOR_TOKEN') "backend healthcheck must authenticate with THEIA_OPERATOR_TOKEN"

$frontendEnvironment = Get-ServiceProperty $frontend "environment"
Assert-True ($frontendEnvironment.VITE_API_URL -eq "http://backend:8080") "frontend dev proxy must reach backend over the Compose network"

Write-Output "docker-compose.yml dev/test shape is valid"
