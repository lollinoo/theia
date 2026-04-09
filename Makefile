.PHONY: dev test test-integration build clean seed verify stop logs help \
       prod prod-metrics prod-down prod-build prod-logs prod-clean \
       staging staging-down staging-logs \
       snmpwalk-router snmpwalk-switch snmpwalk-ap \
       version release bridge-build-all

# ---------------------------------------------------------------------------
# Version management
# ---------------------------------------------------------------------------
VERSION    := $(shell git describe --tags --always 2>/dev/null || echo dev)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
BUILD_DATE := $(shell date -u +%FT%TZ)

# Default target
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

dev: ## Start full dev stack (backend + frontend + Prometheus + SNMP sims)
	@docker compose --profile dev --profile test down 2>/dev/null || true
	THEIA_VERSION=$(VERSION) GIT_COMMIT=$(GIT_COMMIT) BUILD_DATE=$(BUILD_DATE) \
		docker compose --profile dev up --build -d
	@echo ""
	@echo "Theia dev stack is running:"
	@echo "  Backend:  http://localhost:8080"
	@echo "  Frontend: http://localhost:3000"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  SNMP exporter: http://localhost:9116"
	@echo ""
	@echo "Run 'make seed' to add SNMP simulator devices"
	@echo "Run 'make logs' to follow backend logs"

stop: ## Stop all containers
	docker compose --profile dev --profile test down

test: ## Run unit tests inside backend container
	docker compose --profile test run --rm --no-deps backend go test ./... -count=1 -v

test-integration: ## Run integration tests against SNMP simulators
	docker compose --profile test up -d snmp-router snmp-switch snmp-ap
	@echo "Waiting for SNMP simulators to be healthy..."
	docker compose --profile test up -d --wait snmp-router snmp-switch snmp-ap
	docker compose --profile test run --rm backend go test ./... -tags=integration -count=1 -v
	docker compose --profile test down

# ---------------------------------------------------------------------------
# Production stack (GHCR pull -- no local builds)
# ---------------------------------------------------------------------------
prod: ## Start production stack (pulls from GHCR)
	docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
	@echo ""
	@echo "MikroTik Theia production stack is running:"
	@echo "  Frontend: http://localhost:$$(grep FRONTEND_PORT .env.prod 2>/dev/null | cut -d= -f2 || echo 80)"
	@echo "  Backend:  http://localhost:$$(grep BACKEND_PORT .env.prod 2>/dev/null | cut -d= -f2 || echo 8080)"
	@echo ""
	@echo "Run 'make prod-logs' to follow backend logs."

prod-metrics: ## Start production stack with Prometheus + SNMP exporter
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics up -d
	@echo ""
	@echo "MikroTik Theia production stack (with metrics) is running."
	@echo "Edit docker/prometheus/prometheus.prod.yml to add your SNMP device IPs."

prod-down: ## Stop production stack
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics down

prod-logs: ## Follow production backend logs
	docker compose -f docker-compose.prod.yml --env-file .env.prod logs -f backend

prod-clean: ## Stop production stack and remove volumes (resets database)
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics down -v
	docker volume rm -f theia-data theia-prometheus-data 2>/dev/null || true
	@echo "Cleaned all production containers and volumes"

# ---------------------------------------------------------------------------
# Staging stack (GHCR pull + Watchtower auto-update)
# ---------------------------------------------------------------------------
staging: ## Start staging stack (auto-updates via Watchtower)
	docker compose -f docker-compose.staging.yml --env-file .env.staging up -d
	@echo ""
	@echo "MikroTik Theia staging stack is running:"
	@echo "  Frontend: http://localhost:3001"
	@echo "  Backend:  http://localhost:8081"
	@echo "  Watchtower polls for new :staging images every 30s"
	@echo ""
	@echo "Run 'make staging-logs' to follow backend logs."

staging-down: ## Stop staging stack
	docker compose -f docker-compose.staging.yml --env-file .env.staging down

staging-logs: ## Follow staging backend logs
	docker compose -f docker-compose.staging.yml --env-file .env.staging logs -f backend

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
clean: ## Stop containers, remove volumes, and prune build cache
	docker compose --profile dev --profile test down -v
	docker volume rm -f theia-data 2>/dev/null || true
	@echo "Cleaned all containers and volumes"

seed: ## Add SNMP simulator devices via the API
	@bash scripts/seed.sh http://localhost:8080

verify: ## Run go vet and go build inside container
	docker compose --profile test run --rm --no-deps backend sh -c "go vet ./... && go build ./cmd/theia/"

logs: ## Follow backend container logs
	docker compose logs -f backend

snmpwalk-router: ## Run snmpwalk against router simulator (debug)
	snmpwalk -v2c -c public localhost:10161 1.3.6.1.2.1.1

snmpwalk-switch: ## Run snmpwalk against switch simulator (debug)
	snmpwalk -v2c -c public localhost:10162 1.3.6.1.2.1.1

snmpwalk-ap: ## Run snmpwalk against AP simulator (debug)
	snmpwalk -v2c -c public localhost:10163 1.3.6.1.2.1.1

# ---------------------------------------------------------------------------
# Release workflow
# ---------------------------------------------------------------------------
version: ## Show current version
	@echo "Version:    $(VERSION)"
	@echo "Git commit: $(GIT_COMMIT)"
	@echo "Build date: $(BUILD_DATE)"

release: ## Create release tag and push (Usage: make release VERSION=1.3.8)
	@if [ -z "$(VERSION)" ] || [ "$(VERSION)" = "$$(git describe --tags --always 2>/dev/null || echo dev)" ]; then \
		echo "Usage: make release VERSION=1.3.8"; exit 1; fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is not clean"; exit 1; fi
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != "master" ]; then \
		echo "Error: must be on master branch"; exit 1; fi
	@if git rev-parse "v$(VERSION)" >/dev/null 2>&1; then \
		echo "Error: tag v$(VERSION) already exists"; exit 1; fi
	@if ! echo "$(VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$$'; then \
		echo "Error: VERSION must be valid semver (e.g., 1.3.8)"; exit 1; fi
	@git tag -a "v$(VERSION)" -m "release: v$(VERSION)"
	@git push origin "v$(VERSION)"
	@echo ""
	@echo "Release v$(VERSION) tagged and pushed."
	@echo "CI will build and push Docker images to GHCR."

# ---------------------------------------------------------------------------
# WinBox Bridge cross-compilation
# ---------------------------------------------------------------------------
BRIDGE_OUT := bridge_binaries
BRIDGE_SRC := ./cmd/winbox-bridge/

# Windows and Linux: CGO_ENABLED=0 (fyne.io/systray is pure Go on these platforms)
# macOS: requires CGO_ENABLED=1 (Cocoa via Objective-C) — build natively on Mac or via CI
BRIDGE_TARGETS_NOCGO := windows/amd64 windows/arm64 linux/amd64 linux/arm64

bridge-build-all: ## Cross-compile winbox-bridge for Windows + Linux (macOS requires native Mac — use CI)
	@rm -rf $(BRIDGE_OUT)
	@mkdir -p $(BRIDGE_OUT)
	@for target in $(BRIDGE_TARGETS_NOCGO); do \
		os=$$(echo $$target | cut -d/ -f1); \
		arch=$$(echo $$target | cut -d/ -f2); \
		ext=""; \
		ldextra=""; \
		if [ "$$os" = "windows" ]; then ext=".exe"; ldextra="-H=windowsgui"; fi; \
		output="$(BRIDGE_OUT)/winbox-bridge-$${os}-$${arch}$${ext}"; \
		echo "Building $$output ..."; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags="-s -w $${ldextra}" -o "$$output" $(BRIDGE_SRC) || exit 1; \
	done
	@echo ""
	@echo "Bridge binaries built in $(BRIDGE_OUT)/:"
	@ls -la $(BRIDGE_OUT)/
	@echo ""
	@echo "NOTE: macOS binaries (darwin/amd64, darwin/arm64) require CGO_ENABLED=1."
	@echo "      Build natively on Mac: CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags=\"-s -w\" -o $(BRIDGE_OUT)/winbox-bridge-darwin-arm64 $(BRIDGE_SRC)"
