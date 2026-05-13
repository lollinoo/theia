$ErrorActionPreference = "Stop"

function Fail {
  param([string]$Message)
  Write-Error $Message
  exit 1
}

function Assert-True {
  param(
    [bool]$Condition,
    [string]$Message
  )

  if (-not $Condition) {
    Fail $Message
  }
}

function Get-ObjectProperty {
  param(
    [Parameter(Mandatory = $true)]$Object,
    [Parameter(Mandatory = $true)][string]$Name
  )

  $property = $Object.PSObject.Properties[$Name]
  if ($null -eq $property) {
    return $null
  }
  return $property.Value
}

function Get-ServiceConfig {
  param(
    [Parameter(Mandatory = $true)]$Services,
    [Parameter(Mandatory = $true)][string]$Name
  )

  $service = Get-ObjectProperty $Services $Name
  Assert-True ($null -ne $service) "service '$Name' must be defined"
  return $service
}

function Assert-ServiceOnWispManagementNetwork {
  param(
    [Parameter(Mandatory = $true)]$Services,
    [Parameter(Mandatory = $true)][string]$Name,
    [Parameter(Mandatory = $true)][int]$Octet
  )

  $service = Get-ServiceConfig -Services $Services -Name $Name
  $networks = Get-ObjectProperty $service "networks"
  Assert-True ($null -ne $networks) "service '$Name' must define networks"

  $management = Get-ObjectProperty $networks "wisp-access-mgmt"
  Assert-True ($null -ne $management) "service '$Name' must join wisp-access-mgmt"
  Assert-True ($management.ipv4_address -eq "172.31.250.$Octet") "service '$Name' must use management IP 172.31.250.$Octet"
}

function Assert-ServicePublishesPort {
  param(
    [Parameter(Mandatory = $true)]$Service,
    [Parameter(Mandatory = $true)][int]$Target,
    [Parameter(Mandatory = $true)][string]$Published,
    [Parameter(Mandatory = $true)][string]$Protocol,
    [string]$HostIp = "127.0.0.1"
  )

  $ports = @($Service.ports | Where-Object { $null -ne $_ })
  $match = $ports | Where-Object {
    ([int]$_.target -eq $Target) -and
    ([string]$_.published -eq $Published) -and
    ([string]$_.protocol -eq $Protocol) -and
    ([string]$_.host_ip -eq $HostIp)
  }

  Assert-True (($match | Measure-Object).Count -gt 0) "service must publish $HostIp`:$Published->$Target/$Protocol"
}

$composeOutput = & docker compose -f docker-compose.wisp-lab.yml config --format json
if ($LASTEXITCODE -ne 0) {
  Fail "docker compose config failed for docker-compose.wisp-lab.yml"
}

$config = $composeOutput | ConvertFrom-Json
$services = Get-ObjectProperty $config "services"
Assert-True ($null -ne $services) "compose config must contain services"

foreach ($serviceProperty in $services.PSObject.Properties) {
  Assert-True ($null -eq (Get-ObjectProperty $serviceProperty.Value "network_mode")) "service '$($serviceProperty.Name)' must not use network_mode"
}

$managedTargets = @(
  @{ Name = "wisp-core-01"; Octet = 21 },
  @{ Name = "wisp-core-02"; Octet = 22 },
  @{ Name = "wisp-pop-north-01"; Octet = 23 },
  @{ Name = "wisp-pop-south-01"; Octet = 24 },
  @{ Name = "wisp-ix-edge-01"; Octet = 25 },
  @{ Name = "wisp-tower-north-01"; Octet = 26 },
  @{ Name = "wisp-tower-north-02"; Octet = 27 },
  @{ Name = "wisp-tower-south-01"; Octet = 28 },
  @{ Name = "wisp-tower-south-02"; Octet = 29 },
  @{ Name = "wisp-dc-agg-01"; Octet = 30 },
  @{ Name = "wisp-ap-north-a-01"; Octet = 31 },
  @{ Name = "wisp-ap-north-b-01"; Octet = 32 },
  @{ Name = "wisp-ap-south-a-01"; Octet = 33 },
  @{ Name = "wisp-ap-south-b-01"; Octet = 34 },
  @{ Name = "wisp-cpe-north-a-01"; Octet = 35 },
  @{ Name = "wisp-cpe-north-a-02"; Octet = 36 },
  @{ Name = "wisp-cpe-north-b-01"; Octet = 37 },
  @{ Name = "wisp-cpe-north-b-02"; Octet = 38 },
  @{ Name = "wisp-cpe-south-a-01"; Octet = 39 },
  @{ Name = "wisp-cpe-south-a-02"; Octet = 40 },
  @{ Name = "wisp-cpe-south-b-01"; Octet = 41 },
  @{ Name = "wisp-cpe-south-b-02"; Octet = 42 }
)

foreach ($target in $managedTargets) {
  Assert-ServiceOnWispManagementNetwork -Services $services -Name $target.Name -Octet $target.Octet
}

$snmpExporter = Get-ServiceConfig -Services $services -Name "wisp-snmp-exporter"
$prometheus = Get-ServiceConfig -Services $services -Name "wisp-prometheus"
Assert-ServiceOnWispManagementNetwork -Services $services -Name "wisp-snmp-exporter" -Octet 250
Assert-ServiceOnWispManagementNetwork -Services $services -Name "wisp-prometheus" -Octet 251
Assert-ServicePublishesPort -Service $snmpExporter -Target 9117 -Published "9117" -Protocol "tcp"
Assert-ServicePublishesPort -Service $prometheus -Target 9091 -Published "9091" -Protocol "tcp"

$prometheusConfig = Get-Content -Raw -Path "docker/prometheus/prometheus.wisp.yml"
Assert-True ($prometheusConfig -notmatch "127\.0\.10\.") "WISP Prometheus config must not scrape host loopback SNMP targets"
Assert-True ($prometheusConfig -match "172\.31\.250\.21") "WISP Prometheus config must include management target 172.31.250.21"
Assert-True ($prometheusConfig -match "172\.31\.250\.42") "WISP Prometheus config must include management target 172.31.250.42"
Assert-True ($prometheusConfig -match "replacement:\s+wisp-snmp-exporter:9117") "WISP Prometheus must reach snmp-exporter through the Docker network"

Assert-True (Test-Path ".gitattributes") ".gitattributes must define LF rules for files copied into Linux containers"
$attributes = Get-Content -Raw -Path ".gitattributes"
Assert-True ($attributes -match "(?m)^\*\.sh\s+text\s+eol=lf$") ".gitattributes must keep shell scripts LF-only"
Assert-True ($attributes -match "(?m)^docker/wisp-lab/\*\.py\s+text\s+eol=lf$") ".gitattributes must keep WISP Python scripts LF-only"
Assert-True ($attributes -match "(?m)^docker/wisp-lab/Dockerfile\s+text\s+eol=lf$") ".gitattributes must keep the WISP Dockerfile LF-only"

$dockerfile = Get-Content -Raw -Path "docker/wisp-lab/Dockerfile"
Assert-True (($dockerfile -match "sed -i") -and ($dockerfile -match "start-router-lab\.sh") -and ($dockerfile -match "render-router-lab\.py")) "WISP Dockerfile must strip CRLF from copied runtime files"

Assert-True (Test-Path "scripts/wisp-lab-common.ps1") "PowerShell WISP helper must exist"
Assert-True (Test-Path "scripts/wisp-lab-common.sh") "POSIX WISP helper must exist"
Assert-True (Test-Path "scripts/start-wisp-lab.ps1") "PowerShell WISP launcher must exist"
Assert-True (Test-Path "scripts/start-wisp-lab.sh") "POSIX WISP launcher must exist"
Assert-True (Test-Path "scripts/stop-wisp-lab.ps1") "PowerShell WISP stop helper must exist"
Assert-True (Test-Path "scripts/stop-wisp-lab.sh") "POSIX WISP stop helper must exist"

$makefile = Get-Content -Raw -Path "Makefile"
Assert-True ($makefile -match "WISP_SEED_TARGET_MODE") "Makefile must expose WISP_SEED_TARGET_MODE"
Assert-True ($makefile -match "start-wisp-lab\.ps1") "Windows wisp-lab target must use the PowerShell launcher"
Assert-True ($makefile -match "start-wisp-lab\.sh") "POSIX wisp-lab target must use the shell launcher"
Assert-True ($makefile -match "stop-wisp-lab\.ps1") "Windows wisp-lab-down target must use the PowerShell stop helper"
Assert-True ($makefile -match "stop-wisp-lab\.sh") "POSIX wisp-lab-down target must use the shell stop helper"

Write-Output "WISP lab cross-platform shape is valid"
