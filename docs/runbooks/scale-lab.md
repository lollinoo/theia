# Scale Lab Runbook

## Built-in Profiles
- `100`
- `500`
- `1000`

## Built-in Scenarios
- `baseline`
- `db-slowdown`
- `snmp-timeout-spike`
- `burst-adds`
- `burst-unresolved-neighbors`
- `soak-24h`

## Run The Synthetic Lab
```bash
go run ./cmd/theia-scale-lab -profile 1000 -scenario soak-24h
```

## Replay A Recorded Fixture
```bash
go run ./cmd/theia-scale-lab \
  -profile 100 \
  -scenario baseline \
  -fixture internal/scalelab/testdata/lldp-sample.json
```

## Persist A Report
```bash
go run ./cmd/theia-scale-lab \
  -profile 500 \
  -scenario db-slowdown \
  -out /tmp/theia-scale-report.json
```

## What The Report Contains
- expected performance, operational, and static task rates per minute
- replay observation counts
- resolved vs unresolved neighbor counts
- self-neighbor counts
- link upsert event totals (`created`, `enriched`, `reoriented`, `updated`, `noop`)
- replay latency summary (`p50`, `p95`, `p99`, `max`)
