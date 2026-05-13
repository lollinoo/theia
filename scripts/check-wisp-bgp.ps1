param()

$ErrorActionPreference = "Stop"

Write-Output "=== wisp-ix-edge-01: BGP summary ==="
& docker compose -f docker-compose.wisp-lab.yml exec -T wisp-ix-edge-01 vtysh -c "show ip bgp summary"
Write-Output ""

Write-Output "=== wisp-transit-01: BGP summary ==="
& docker compose -f docker-compose.wisp-lab.yml exec -T wisp-transit-01 vtysh -c "show ip bgp summary"
Write-Output ""

foreach ($service in @("wisp-ix-edge-01", "wisp-core-01", "wisp-pop-north-01", "wisp-pop-south-01")) {
  Write-Output "=== ${service}: default route ==="
  & docker compose -f docker-compose.wisp-lab.yml exec -T $service vtysh -c "show ip route 0.0.0.0/0"
  Write-Output ""
}

exit 0
