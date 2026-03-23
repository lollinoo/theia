---
status: resolved
trigger: "Production Docker Compose deployment fails with 'Failed to load vendor registry from vendors: reading vendor directory vendors: open vendors: no such file or directory'. First deploy of v1.0."
created: 2026-03-23T00:00:00Z
updated: 2026-03-23T00:03:00Z
---

## Current Focus
<!-- OVERWRITE on each update - reflects NOW -->

hypothesis: CONFIRMED and FIXED — Production image had no vendors/ directory. Fixed by embedding YAML files in the binary via go:embed.
test: go build ./cmd/theia/ succeeds. go test ./internal/vendor/... — all 8 tests pass including TestLoadRealVendors which now uses LoadRegistryFromEmbedded().
expecting: Production container starts successfully without needing vendors/ on the filesystem
next_action: Awaiting human verification of production deploy

## Symptoms
<!-- Written during gathering, then IMMUTABLE -->

expected: Backend starts and loads vendor registry from the vendors directory
actual: Backend crashes/errors with "open vendors: no such file or directory"
errors: "Failed to load vendor registry from vendors: reading vendor directory vendors: open vendors: no such file or directory"
reproduction: Deploy via docker-compose in production
started: First production deploy of v1.0 — never worked in production before. Works in development.

## Eliminated
<!-- APPEND only - prevents re-investigating -->

- hypothesis: WORKDIR mismatch causing wrong CWD
  evidence: Production image has no explicit WORKDIR set (defaults to /), but the real issue is the vendors/ directory doesn't exist in the image at all — regardless of CWD
  timestamp: 2026-03-23T00:01:00Z

## Evidence
<!-- APPEND only - facts discovered -->

- timestamp: 2026-03-23T00:01:00Z
  checked: Dockerfile production stage (lines 58-70)
  found: Only copies /app/theia binary with COPY --from=builder /app/theia /usr/local/bin/theia. No COPY vendors or COPY config.yaml step.
  implication: vendors/ directory does not exist anywhere inside the production container image.

- timestamp: 2026-03-23T00:01:00Z
  checked: cmd/theia/main.go lines 81-88
  found: |
    vendorsDir := filepath.Join(filepath.Dir(cfgPath), "vendors")
    if envVendors := os.Getenv("THEIA_VENDORS_DIR"); envVendors != "" {
        vendorsDir = envVendors
    }
    yamlRegistry, err := vendor.LoadRegistryFromYAML(vendorsDir)
  implication: |
    vendors path is computed as: dir(cfgPath) + "/vendors"
    cfgPath defaults to "config.yaml" (relative), so vendors/ is looked up relative to CWD.
    In production container, CWD is / (no WORKDIR set), so it looks for /vendors — which doesn't exist.
    THEIA_VENDORS_DIR env var is an escape hatch but is not set in docker-compose.prod.yml.

- timestamp: 2026-03-23T00:01:00Z
  checked: docker-compose.prod.yml backend service environment block
  found: Only THEIA_DB_PATH, THEIA_LISTEN_ADDR, THEIA_LOG_LEVEL are set. THEIA_VENDORS_DIR is absent.
  implication: No override for vendors path — binary uses computed path which resolves to a non-existent directory.

- timestamp: 2026-03-23T00:01:00Z
  checked: internal/vendor/registry.go — LoadRegistryFromYAML function
  found: go:embed is NOT used. Function calls os.ReadDir(dir) directly on the filesystem path passed to it.
  implication: The vendor YAML files must be physically present on the filesystem at runtime. They are not embedded in the binary.

- timestamp: 2026-03-23T00:01:00Z
  checked: vendors/ directory in repo root
  found: Contains default.yaml and mikrotik.yaml — the required vendor configs.
  implication: These files exist on the developer's machine and are bind-mounted in dev (volume: ".:/app"), which is why dev works. They are never in the production image.

- timestamp: 2026-03-23T00:03:00Z
  checked: go build ./cmd/theia/ and go test ./internal/vendor/...
  found: Binary compiles successfully. All 8 vendor package tests pass including TestLoadRealVendors now using LoadRegistryFromEmbedded().
  implication: Fix is correct and does not break any existing tests.

## Resolution
<!-- OVERWRITE as understanding evolves -->

root_cause: |
  The production Docker image only copies the compiled binary into the image. The vendors/ directory containing default.yaml and mikrotik.yaml is never copied in. In dev, docker-compose.yml bind-mounts the entire repo (- ".:/app"), so vendors/ is accessible at runtime. In production, there is no bind-mount of source code — only the binary is present. The binary attempts to open vendors/ relative to CWD (which is / by default) and fails immediately with "no such file or directory".

fix: |
  Used go:embed to embed the vendor YAML files directly into the binary. Changes made:
  1. Created internal/vendor/data/ directory and placed default.yaml + mikrotik.yaml there
  2. Created internal/vendor/embedded.go with //go:embed data/*.yaml and LoadRegistryFromEmbedded() function
  3. Updated cmd/theia/main.go: now calls LoadRegistryFromEmbedded() by default; THEIA_VENDORS_DIR env var still works as an override for custom vendor configs
  4. Updated internal/vendor/registry_test.go: TestLoadRealVendors now tests LoadRegistryFromEmbedded() instead of reading from ../../vendors
  5. Added WORKDIR /app to Dockerfile production stage

verification: |
  - go build ./cmd/theia/ succeeds in golang:1.24-bookworm Docker container with CGO_ENABLED=1
  - go test ./internal/vendor/... — all 8 tests pass
  - Awaiting production deploy confirmation

files_changed:
  - internal/vendor/data/default.yaml (new — embedded vendor data)
  - internal/vendor/data/mikrotik.yaml (new — embedded vendor data)
  - internal/vendor/embedded.go (new — go:embed + LoadRegistryFromEmbedded)
  - cmd/theia/main.go (updated — use embedded loader by default)
  - internal/vendor/registry_test.go (updated — TestLoadRealVendors uses embedded loader)
  - Dockerfile (updated — added WORKDIR /app to production stage)
