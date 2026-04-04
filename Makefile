.PHONY: dev test test-integration build clean seed verify stop logs help \
       prod prod-metrics prod-down prod-build prod-logs prod-clean \
       staging staging-down staging-logs \
       branch-staging branch-staging-down branch-staging-logs branch-staging-pull branch-staging-clean branch-staging-prune \
       snmpwalk-router snmpwalk-switch snmpwalk-ap \
       version release

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
# Branch staging (GHCR pull, branch-specific images)
# ---------------------------------------------------------------------------

# Sanitize branch name: feature/foo → feature-foo
BRANCH_TAG = $(shell echo "$(BRANCH)" | sed 's|/|-|g' | sed 's|^-||' | tr '[:upper:]' '[:lower:]')

branch-staging: ## Start branch staging stack (Usage: make branch-staging BRANCH=fix/foo)
	@if [ -z "$(BRANCH)" ]; then \
		echo "Usage: make branch-staging BRANCH=fix/login-timeout"; \
		exit 1; fi
	BRANCH_TAG=$(BRANCH_TAG) docker compose -f docker-compose.branch-staging.yml up -d
	@echo ""
	@echo "Branch staging stack for '$(BRANCH)' is running:"
	@echo "  Frontend: http://localhost:$${FRONTEND_PORT:-3002}"
	@echo "  Backend:  http://localhost:$${BACKEND_PORT:-8082}"
	@echo "  Image tag: $(BRANCH_TAG)"
	@echo "  Watchtower polls for new images every 30s"
	@echo ""
	@echo "Run 'make branch-staging-logs BRANCH=$(BRANCH)' to follow logs."

branch-staging-down: ## Stop branch staging stack
	@if [ -z "$(BRANCH)" ]; then echo "Usage: make branch-staging-down BRANCH=fix/foo"; exit 1; fi
	BRANCH_TAG=$(BRANCH_TAG) docker compose -f docker-compose.branch-staging.yml down

branch-staging-logs: ## Follow branch staging backend logs
	@if [ -z "$(BRANCH)" ]; then echo "Usage: make branch-staging-logs BRANCH=fix/foo"; exit 1; fi
	BRANCH_TAG=$(BRANCH_TAG) docker compose -f docker-compose.branch-staging.yml logs -f backend

branch-staging-pull: ## Pull latest branch images manually
	@if [ -z "$(BRANCH)" ]; then echo "Usage: make branch-staging-pull BRANCH=fix/foo"; exit 1; fi
	docker pull ghcr.io/lollinoo/theia-backend:$(BRANCH_TAG)
	docker pull ghcr.io/lollinoo/theia-frontend:$(BRANCH_TAG)

branch-staging-clean: ## Stop branch stack and remove its volume
	@if [ -z "$(BRANCH)" ]; then echo "Usage: make branch-staging-clean BRANCH=fix/foo"; exit 1; fi
	BRANCH_TAG=$(BRANCH_TAG) docker compose -f docker-compose.branch-staging.yml down -v
	@echo "Cleaned branch staging stack for '$(BRANCH)'"

branch-staging-prune: ## Remove all branch-tagged images (keeps :staging and semver)
	@echo "Removing branch-tagged images..."
	@docker image ls --format '{{.Repository}}:{{.Tag}}' | grep 'lollinoo/theia-' | grep -vE ':(staging|[0-9]+\.[0-9]+\.[0-9]+)$$' | xargs -r docker rmi
	@echo "Done. Run 'docker image prune -f' to reclaim space."

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
