param()

$ErrorActionPreference = "Stop"

$services = @(
  "wisp-core-01",
  "wisp-core-02",
  "wisp-pop-north-01",
  "wisp-pop-south-01",
  "wisp-ix-edge-01",
  "wisp-tower-north-01",
  "wisp-tower-north-02",
  "wisp-tower-south-01",
  "wisp-tower-south-02",
  "wisp-dc-agg-01"
)

foreach ($service in $services) {
  Write-Output "=== $service ==="
  & docker compose -f docker-compose.wisp-lab.yml exec -T $service vtysh -c "show ip ospf neighbor"
  Write-Output ""
}

exit 0
