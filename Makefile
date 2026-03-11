.PHONY: dev test test-integration build clean seed verify stop logs help \
       prod prod-metrics prod-down prod-build prod-logs prod-clean \
       snmpwalk-router snmpwalk-switch snmpwalk-ap

# Default target
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

dev: ## Start full dev stack (backend + frontend + Prometheus + SNMP sims)
	@docker compose --profile dev --profile test down 2>/dev/null || true
	docker compose --profile dev up --build -d
	@echo ""
	@echo "MikroTik Theia dev stack is running:"
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

build: ## Build production Docker images (backend + frontend)
	docker compose -f docker-compose.prod.yml build

prod: ## Start production stack (backend + nginx frontend)
	docker compose -f docker-compose.prod.yml up --build -d
	@echo ""
	@echo "MikroTik Theia production stack is running:"
	@echo "  Frontend: http://localhost:80"
	@echo "  Backend:  http://localhost:8080"
	@echo ""
	@echo "Add devices via the API or the UI Settings panel."
	@echo "Run 'make prod-logs' to follow backend logs."

prod-metrics: ## Start production stack with Prometheus + SNMP exporter
	docker compose -f docker-compose.prod.yml --profile metrics up --build -d
	@echo ""
	@echo "MikroTik Theia production stack (with metrics) is running:"
	@echo "  Frontend:      http://localhost:80"
	@echo "  Backend:       http://localhost:8080"
	@echo "  Prometheus:    http://localhost:9090"
	@echo "  SNMP exporter: http://localhost:9116"
	@echo ""
	@echo "Edit docker/prometheus/prometheus.prod.yml to add your SNMP device IPs."

prod-down: ## Stop production stack
	docker compose -f docker-compose.prod.yml --profile metrics down

prod-build: ## Build production images without starting
	docker compose -f docker-compose.prod.yml build

prod-logs: ## Follow production backend logs
	docker compose -f docker-compose.prod.yml logs -f backend

prod-clean: ## Stop production stack and remove volumes (resets database)
	docker compose -f docker-compose.prod.yml --profile metrics down -v
	docker volume rm -f theia-data theia-prometheus-data 2>/dev/null || true
	@echo "Cleaned all production containers and volumes"

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
