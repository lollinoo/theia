.PHONY: dev test test-integration build clean seed verify stop logs help \
       postgres-up postgres-down dev-postgres \
       prod-postgres prod-postgres-metrics staging-postgres \
       wisp-lab wisp-lab-down wisp-seed wisp-radio-seed wisp-seed-all wisp-ospf wisp-bgp \
       phase4-scale-lab phase4-validate \
       development prod production prod-metrics prod-down prod-build prod-logs prod-clean \
       staging staging-down staging-logs \
       backend-fast frontend-fast govulncheck \
       realtime-stress collector-contract browser-e2e \
       bridge-build-all

ifeq ($(OS),Windows_NT)
NULL := NUL
SHELL := powershell.exe
.SHELLFLAGS := -NoProfile -ExecutionPolicy Bypass -Command
IS_WINDOWS := 1
else
NULL := /dev/null
IS_WINDOWS := 0
endif

DEV_COMPOSE_PROFILES := --profile dev
TEST_COMPOSE_PROFILES := --profile test
PHASE4_API_BASE ?= http://localhost:8080
PHASE4_OUT ?= .planning/phases/04-scale-validation-and-hardening/evidence/synthetic
PHASE4_MODE ?= synthetic
WISP_SEED_TARGET_MODE ?= auto

# Default target
ifeq ($(IS_WINDOWS),1)
help: ## Show this help
	@$$seen = @{}; Get-Content $(MAKEFILE_LIST) | ForEach-Object { if ($$_ -match '^([a-zA-Z_-]+):.*?## (.*)$$' -and -not $$seen.ContainsKey($$matches[1])) { $$seen[$$matches[1]] = $$true; [PSCustomObject]@{Target=$$matches[1]; Description=$$matches[2]} } } | Sort-Object Target | ForEach-Object { '{0,-22} {1}' -f $$_.Target, $$_.Description }
else
help: ## Show this help
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z_-]+:.*## / && !seen[$$1]++ {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST) | sort
endif

ifeq ($(IS_WINDOWS),1)
dev: ## Start full dev stack (backend + frontend + PostgreSQL + Prometheus)
	@docker compose $(DEV_COMPOSE_PROFILES) down 2>$$null; exit 0
	@$$env:THEIA_DEPLOYMENT_ENV='development'; docker compose $(DEV_COMPOSE_PROFILES) up --build -d; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }
	@Write-Output ""
	@Write-Output "Theia dev stack is running:"
	@Write-Output "  Backend:  http://localhost:8080"
	@Write-Output "  Frontend: http://localhost:3000"
	@Write-Output "  PostgreSQL: postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable"
	@Write-Output "  Prometheus: http://localhost:9090"
	@Write-Output "  SNMP exporter: http://localhost:9116"
	@Write-Output ""
	@Write-Output "Login with administrator / theia, then change the password when prompted."
	@Write-Output "Run 'make wisp-lab' and 'make wisp-seed-all' to add lab devices"
	@Write-Output "Run 'make logs' to follow backend logs"
else
dev: ## Start full dev stack (backend + frontend + PostgreSQL + Prometheus)
	@docker compose $(DEV_COMPOSE_PROFILES) down 2>/dev/null || true
	THEIA_DEPLOYMENT_ENV=development docker compose $(DEV_COMPOSE_PROFILES) up --build -d
	@echo ""
	@echo "Theia dev stack is running:"
	@echo "  Backend:  http://localhost:8080"
	@echo "  Frontend: http://localhost:3000"
	@echo "  PostgreSQL: postgres://theia:theia@127.0.0.1:5432/theia?sslmode=disable"
	@echo "  Prometheus: http://localhost:9090"
	@echo "  SNMP exporter: http://localhost:9116"
	@echo ""
	@echo "Login with administrator / theia, then change the password when prompted."
	@echo "Run 'make wisp-lab' and 'make wisp-seed-all' to add lab devices"
	@echo "Run 'make logs' to follow backend logs"
endif

development: dev ## Start the development stack

ifeq ($(IS_WINDOWS),1)
postgres-up: ## Start local PostgreSQL for Theia
	@docker compose --profile postgres down 2>$$null; exit 0
	docker compose --profile postgres up -d --wait postgres
else
postgres-up: ## Start local PostgreSQL for Theia
	@docker compose --profile postgres down 2>/dev/null || true
	docker compose --profile postgres up -d --wait postgres
endif

postgres-down: ## Stop local PostgreSQL for Theia
	docker compose --profile postgres down

dev-postgres: ## Start dev stack on PostgreSQL (same as standard dev path)
	@$(MAKE) dev

stop: ## Stop all containers
	docker compose $(DEV_COMPOSE_PROFILES) down

ifeq ($(IS_WINDOWS),1)
test: ## Run unit tests inside backend container
	@& ./scripts/run-compose-tests.ps1

test-integration: ## Run integration tests inside backend container
	@& ./scripts/run-compose-tests.ps1 -Integration
else
test: ## Run unit tests inside backend container
	@status=0; cleanup_status=0; started_services=""; running_services="$$(docker compose $(TEST_COMPOSE_PROFILES) ps --status running --services 2>/dev/null || true)"; \
		case " $$running_services " in *" postgres "*) ;; *) started_services="postgres" ;; esac; \
		docker compose $(TEST_COMPOSE_PROFILES) up -d --wait postgres || status=$$?; \
		if [ $$status -eq 0 ]; then \
			docker compose $(TEST_COMPOSE_PROFILES) run --rm --no-deps backend go test ./... -count=1 -v || status=$$?; \
		fi; \
		if [ -n "$$started_services" ]; then \
			docker compose $(TEST_COMPOSE_PROFILES) stop $$started_services || cleanup_status=$$?; \
		fi; \
		if [ $$status -eq 0 ]; then status=$$cleanup_status; fi; \
		exit $$status

test-integration: ## Run integration tests inside backend container
	@status=0; cleanup_status=0; started_services=""; running_services="$$(docker compose $(TEST_COMPOSE_PROFILES) ps --status running --services 2>/dev/null || true)"; \
		case " $$running_services " in *" postgres "*) ;; *) started_services="postgres" ;; esac; \
		docker compose $(TEST_COMPOSE_PROFILES) up -d --wait postgres || status=$$?; \
		if [ $$status -eq 0 ]; then \
			docker compose $(TEST_COMPOSE_PROFILES) run --rm --no-deps backend go test ./... -tags=integration -count=1 -v || status=$$?; \
		fi; \
		if [ -n "$$started_services" ]; then \
			docker compose $(TEST_COMPOSE_PROFILES) stop $$started_services || cleanup_status=$$?; \
		fi; \
		if [ $$status -eq 0 ]; then status=$$cleanup_status; fi; \
		exit $$status
endif

# ---------------------------------------------------------------------------
# CI and focused quality gates
# ---------------------------------------------------------------------------
ifeq ($(IS_WINDOWS),1)
govulncheck: ## Run Go vulnerability scanning
	@if (-not (Get-Command govulncheck -ErrorAction SilentlyContinue)) { Write-Error "govulncheck is required. Install it with: go install golang.org/x/vuln/cmd/govulncheck@latest"; exit 1 }
	govulncheck ./...

backend-fast: ## Run the backend-fast quality gate locally
	@New-Item -ItemType Directory -Force coverage | Out-Null
	go vet ./...
	go build ./cmd/theia/
	$(MAKE) govulncheck
	go test ./... -count=1 -covermode=atomic -coverprofile=coverage/backend-fast.out
	@& ./scripts/check-go-cover.ps1 coverage/backend-fast.out 45
else
govulncheck: ## Run Go vulnerability scanning
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck is required. Install it with: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2; exit 1; }
	govulncheck ./...

backend-fast: ## Run the backend-fast quality gate locally
	mkdir -p coverage
	go vet ./...
	go build ./cmd/theia/
	$(MAKE) govulncheck
	go test ./... -count=1 -covermode=atomic -coverprofile=coverage/backend-fast.out
	bash scripts/check-go-cover.sh coverage/backend-fast.out 45
endif

realtime-stress: ## Run focused realtime stress tests locally
	go test ./internal/ws ./internal/worker ./internal/service ./internal/scalelab -count=1 -run 'Test(HubBroadcastMarksLegacyClientForResyncWhenMailboxIsFull|HubBroadcastAvoidsSnapshotForHTTPBootstrapClientWhenMailboxIsFull|HubRepeatedDetailSubscriptionsConvergeToSingleTarget|PipelineResyncRequiredSnapshotSequenceStaysStableAcrossBurstReplay|BurstReplayFixtureKeepsDeterministicLinkCountsAcrossPasses)'

ifeq ($(IS_WINDOWS),1)
collector-contract: ## Run focused collector contract tests locally
	@& ./scripts/run-collector-contract.ps1
else
collector-contract: ## Run focused collector contract tests locally
	bash scripts/run-collector-contract.sh
endif

frontend-fast: ## Run the frontend-fast quality gate locally
	npm --prefix frontend ci
	npm --prefix frontend run check
	npm --prefix frontend run test:coverage
	npm --prefix frontend run typecheck
	npm --prefix frontend run build

browser-e2e: ## Run the browser E2E gate locally
	npm --prefix frontend ci
	npm --prefix frontend run e2e:install
	npm --prefix frontend run e2e

# ---------------------------------------------------------------------------
# Production stack (GHCR pull -- no local builds)
# ---------------------------------------------------------------------------
ifeq ($(IS_WINDOWS),1)
prod: ## Start production stack (pulls from GHCR)
	docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
	@Write-Output ""
	@Write-Output "MikroTik Theia production stack is running:"
	@$$frontendPort = '80'; if (Test-Path '.env.prod') { $$line = Get-Content '.env.prod' | Where-Object { $$_ -match '^FRONTEND_PORT=' } | Select-Object -First 1; if ($$line) { $$frontendPort = ($$line -split '=', 2)[1] } }; Write-Output "  Frontend: http://localhost:$$frontendPort"; Write-Output "  API proxy: http://localhost:$$frontendPort/api/v1"
	@Write-Output ""
	@Write-Output "Run 'make prod-logs' to follow backend logs."
else
prod: ## Start production stack (pulls from GHCR)
	docker compose -f docker-compose.prod.yml --env-file .env.prod up -d
	@echo ""
	@echo "MikroTik Theia production stack is running:"
	@echo "  Frontend: http://localhost:$$(grep FRONTEND_PORT .env.prod 2>/dev/null | cut -d= -f2 || echo 80)"
	@echo "  API proxy: http://localhost:$$(grep FRONTEND_PORT .env.prod 2>/dev/null | cut -d= -f2 || echo 80)/api/v1"
	@echo ""
	@echo "Run 'make prod-logs' to follow backend logs."
endif

production: prod ## Start the production stack

prod-metrics: ## Start production stack with Prometheus + SNMP exporter
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics up -d
	@echo ""
	@echo "MikroTik Theia production stack (with metrics) is running."
	@echo "Edit docker/prometheus/prometheus.prod.yml to add your SNMP device IPs."

ifeq ($(IS_WINDOWS),1)
prod-postgres: ## Start production stack on PostgreSQL
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d postgres
	@docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }
	@Write-Output ""
	@Write-Output "MikroTik Theia production stack is running on PostgreSQL."

prod-postgres-metrics: ## Start production stack on PostgreSQL with metrics
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d postgres
	@docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres --profile metrics up -d; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }
	@Write-Output ""
	@Write-Output "MikroTik Theia production metrics stack is running on PostgreSQL."
else
prod-postgres: ## Start production stack on PostgreSQL
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d postgres
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d
	@echo ""
	@echo "MikroTik Theia production stack is running on PostgreSQL."

prod-postgres-metrics: ## Start production stack on PostgreSQL with metrics
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres up -d postgres
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile postgres --profile metrics up -d
	@echo ""
	@echo "MikroTik Theia production metrics stack is running on PostgreSQL."
endif

prod-down: ## Stop production stack
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics --profile postgres down

prod-logs: ## Follow production backend logs
	docker compose -f docker-compose.prod.yml --env-file .env.prod logs -f backend

ifeq ($(IS_WINDOWS),1)
prod-clean: ## Stop production stack and remove volumes (resets database)
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics --profile postgres down -v
	@docker volume rm -f theia-data theia-prometheus-data theia-prod-postgres-data 2>$$null; exit 0
	@Write-Output "Cleaned all production containers and volumes"
else
prod-clean: ## Stop production stack and remove volumes (resets database)
	docker compose -f docker-compose.prod.yml --env-file .env.prod --profile metrics --profile postgres down -v
	docker volume rm -f theia-data theia-prometheus-data theia-prod-postgres-data 2>/dev/null || true
	@echo "Cleaned all production containers and volumes"
endif

# ---------------------------------------------------------------------------
# Staging stack (GHCR pull + Watchtower auto-update)
# ---------------------------------------------------------------------------
staging: ## Start staging stack (auto-updates via Watchtower)
	docker compose -f docker-compose.staging.yml --env-file .env.staging up -d
	@echo ""
	@echo "MikroTik Theia staging stack is running:"
	@echo "  Frontend: http://localhost:3001"
	@echo "  API proxy: http://localhost:3001/api/v1"
	@echo "  Watchtower polls for new :staging images every 30s"
	@echo ""
	@echo "Run 'make staging-logs' to follow backend logs."

ifeq ($(IS_WINDOWS),1)
staging-postgres: ## Start staging stack on PostgreSQL
	docker compose -f docker-compose.staging.yml --env-file .env.staging --profile postgres up -d postgres
	@docker compose -f docker-compose.staging.yml --env-file .env.staging --profile postgres up -d; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE }
	@Write-Output ""
	@Write-Output "MikroTik Theia staging stack is running on PostgreSQL."
else
staging-postgres: ## Start staging stack on PostgreSQL
	docker compose -f docker-compose.staging.yml --env-file .env.staging --profile postgres up -d postgres
	docker compose -f docker-compose.staging.yml --env-file .env.staging --profile postgres up -d
	@echo ""
	@echo "MikroTik Theia staging stack is running on PostgreSQL."
endif

staging-down: ## Stop staging stack
	docker compose -f docker-compose.staging.yml --env-file .env.staging --profile postgres down

staging-logs: ## Follow staging backend logs
	docker compose -f docker-compose.staging.yml --env-file .env.staging logs -f backend

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------
ifeq ($(IS_WINDOWS),1)
clean: ## Stop containers, remove volumes, and prune build cache
	docker compose $(DEV_COMPOSE_PROFILES) down -v
	@docker volume rm -f theia-data 2>$$null; exit 0
	@Write-Output "Cleaned all containers and volumes"
else
clean: ## Stop containers, remove volumes, and prune build cache
	docker compose $(DEV_COMPOSE_PROFILES) down -v
	docker volume rm -f theia-data 2>/dev/null || true
	@echo "Cleaned all containers and volumes"
endif

ifeq ($(IS_WINDOWS),1)
seed: ## Add sample SNMP devices via the API (requires reachable devices)
	@& ./scripts/seed.ps1 http://localhost:8080
else
seed: ## Add sample SNMP devices via the API (requires reachable devices)
	@bash scripts/seed.sh http://localhost:8080
endif

ifeq ($(IS_WINDOWS),1)
phase4-scale-lab: ## Write Phase 4 synthetic scale-lab evidence files
	@New-Item -ItemType Directory -Force "$(PHASE4_OUT)" | Out-Null
	go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -out "$(PHASE4_OUT)/scale-300-baseline.json" >$$null
	go run ./cmd/theia-scale-lab -profile 300 -scenario burst-adds -out "$(PHASE4_OUT)/scale-300-burst-adds.json" >$$null
	@Write-Output "Wrote scale-lab evidence to $(PHASE4_OUT)"

phase4-validate: ## Run the Phase 4 validation workflow and capture evidence
	@& ./scripts/phase4-validate.ps1 "$(PHASE4_MODE)" "$(PHASE4_API_BASE)" "$(PHASE4_OUT)"
else
phase4-scale-lab: ## Write Phase 4 synthetic scale-lab evidence files
	@mkdir -p "$(PHASE4_OUT)"
	go run ./cmd/theia-scale-lab -profile 300 -scenario baseline -out "$(PHASE4_OUT)/scale-300-baseline.json" >/dev/null
	go run ./cmd/theia-scale-lab -profile 300 -scenario burst-adds -out "$(PHASE4_OUT)/scale-300-burst-adds.json" >/dev/null
	@echo "Wrote scale-lab evidence to $(PHASE4_OUT)"

phase4-validate: ## Run the Phase 4 validation workflow and capture evidence
	@bash scripts/phase4-validate.sh "$(PHASE4_MODE)" "$(PHASE4_API_BASE)" "$(PHASE4_OUT)"
endif

ifeq ($(IS_WINDOWS),1)
wisp-lab: ## Start WISP lab with 10 routers, radio access overlay, OSPF, and SNMP
	@& ./scripts/start-wisp-lab.ps1

wisp-lab-down: ## Stop the dedicated WISP lab
	@& ./scripts/stop-wisp-lab.ps1

wisp-seed: ## Add the 10 WISP lab routers via the API
	@& ./scripts/seed-wisp.ps1 -ApiBase http://localhost:8080 -TargetMode "$(WISP_SEED_TARGET_MODE)"

wisp-radio-seed: ## Add sector APs and CPE radio nodes via the API
	@& ./scripts/seed-wisp-radio.ps1 -ApiBase http://localhost:8080 -TargetMode "$(WISP_SEED_TARGET_MODE)"

wisp-seed-all: ## Add routers plus radio access nodes via the API
	@& ./scripts/seed-wisp.ps1 -ApiBase http://localhost:8080 -TargetMode "$(WISP_SEED_TARGET_MODE)"
	@& ./scripts/seed-wisp-radio.ps1 -ApiBase http://localhost:8080 -TargetMode "$(WISP_SEED_TARGET_MODE)"

wisp-ospf: ## Show OSPF neighbors for all WISP lab routers
	@& ./scripts/check-wisp-ospf.ps1

wisp-bgp: ## Show BGP and propagated default routes in the WISP lab
	@& ./scripts/check-wisp-bgp.ps1
else
wisp-lab: ## Start WISP lab with 10 routers, radio access overlay, OSPF, and SNMP
	@bash scripts/start-wisp-lab.sh

wisp-lab-down: ## Stop the dedicated WISP lab
	@bash scripts/stop-wisp-lab.sh

wisp-seed: ## Add the 10 WISP lab routers via the API
	@bash scripts/seed-wisp.sh http://localhost:8080 "$(WISP_SEED_TARGET_MODE)"

wisp-radio-seed: ## Add sector APs and CPE radio nodes via the API
	@bash scripts/seed-wisp-radio.sh http://localhost:8080 "$(WISP_SEED_TARGET_MODE)"

wisp-seed-all: ## Add routers plus radio access nodes via the API
	@bash scripts/seed-wisp.sh http://localhost:8080 "$(WISP_SEED_TARGET_MODE)"
	@bash scripts/seed-wisp-radio.sh http://localhost:8080 "$(WISP_SEED_TARGET_MODE)"

wisp-ospf: ## Show OSPF neighbors for all WISP lab routers
	@bash scripts/check-wisp-ospf.sh

wisp-bgp: ## Show BGP and propagated default routes in the WISP lab
	@bash scripts/check-wisp-bgp.sh
endif

verify: ## Run go vet, go build, and vulnerability scanning inside container
	docker compose --profile test run --build --rm --no-deps backend sh -c "go vet ./... && go build ./cmd/theia/ && govulncheck ./..."

logs: ## Follow backend container logs
	docker compose logs -f backend

# ---------------------------------------------------------------------------
# WinBox Bridge cross-compilation
# ---------------------------------------------------------------------------
BRIDGE_OUT := bridge_binaries
BRIDGE_SRC := ./cmd/winbox-bridge/

# Windows and Linux: CGO_ENABLED=0 (fyne.io/systray is pure Go on these platforms)
# macOS: requires CGO_ENABLED=1 (Cocoa via Objective-C) - build natively on Mac or via CI
BRIDGE_TARGETS_NOCGO := windows/amd64 windows/arm64 linux/amd64 linux/arm64

ifeq ($(IS_WINDOWS),1)
bridge-build-all: ## Cross-compile winbox-bridge for Windows + Linux (macOS requires native Mac - use CI)
	@& ./scripts/build-winbox-bridge.ps1 -OutDir "$(BRIDGE_OUT)" -Source "$(BRIDGE_SRC)" -Targets @('windows/amd64','windows/arm64','linux/amd64','linux/arm64')
else
bridge-build-all: ## Cross-compile winbox-bridge for Windows + Linux (macOS requires native Mac - use CI)
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
endif
