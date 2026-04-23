# Testing Patterns

**Analysis Date:** 2026-04-23

## Test Framework

**Runner:**
- Backend: Go `testing` package from Go 1.24, run with `go test ./...`; configuration is `go.mod` and `Makefile`.
- Frontend unit/component: Vitest 4.1 with jsdom, configured in `frontend/vitest.config.ts`.
- Browser E2E: Playwright 1.54, configured in `frontend/playwright.config.ts`.
- CI gates are defined in `.github/workflows/ci.yml` and `Makefile`.

**Assertion Library:**
- Backend uses standard `testing` assertions with `t.Fatal` / `t.Fatalf`; no `testify` dependency is present in `go.mod`.
- Frontend uses Vitest `expect` plus `@testing-library/jest-dom/vitest` from `frontend/src/test-setup.ts`.
- Component tests use Testing Library queries from `@testing-library/react` and `@testing-library/user-event` where needed.
- E2E tests use Playwright `expect` from `@playwright/test` in `frontend/e2e/realtime.spec.ts`.

**Run Commands:**
```bash
go test ./... -count=1                         # Run all backend tests locally
make backend-fast                              # Vet, build, backend tests, backend coverage gate
npm --prefix frontend run test                 # Run frontend Vitest tests
npm --prefix frontend run test:coverage        # Run frontend coverage with thresholds
npm --prefix frontend run e2e                  # Run Playwright browser E2E tests
make frontend-fast                             # Frontend check, coverage, typecheck, build
make browser-e2e                               # Install browser deps and run E2E
```

## Test File Organization

**Location:**
- Backend tests are co-located with packages under `cmd/` and `internal/`: `internal/api/device_handler_test.go`, `internal/ws/hub_test.go`, `cmd/theia/main_test.go`.
- Backend repository tests use package-local helpers in the same directory: `internal/repository/sqlite/test_helpers_test.go`.
- Frontend tests are mostly co-located beside source files: `frontend/src/components/DeviceCard.test.tsx`, `frontend/src/hooks/useWebSocket.test.ts`, `frontend/src/api/client.test.ts`.
- Frontend cross-file/audit tests live in `frontend/src/components/__tests__/`.
- E2E tests live under `frontend/e2e/`, currently `frontend/e2e/realtime.spec.ts`.

**Naming:**
- Backend: `TestName_Scenario` or descriptive `TestName` functions in `_test.go`, e.g. `TestDeviceRepoGetBySysName_NormalizedLookup` in `internal/repository/sqlite/device_repo_test.go`.
- Frontend: `describe('module/component', ...)` with `it('expected behavior', ...)`, e.g. `frontend/src/components/DeviceCard.test.tsx`.
- Playwright: `test('user-visible behavior', ...)`, e.g. `renders topology after bootstrap` in `frontend/e2e/realtime.spec.ts`.

**Structure:**
```
internal/<package>/*_test.go                 # Backend package tests
frontend/src/<area>/<unit>.test.ts[x]        # Frontend unit/component tests
frontend/src/components/__tests__/*.test.ts  # Frontend audit/contract tests
frontend/e2e/*.spec.ts                       # Browser E2E tests
```

## Test Structure

**Suite Organization:**
```typescript
// frontend/src/components/DeviceCard.test.tsx
function mockDevice(overrides: Partial<Device> = {}): Device {
  return { id: 'dev-1', hostname: 'router-01', ip: '10.0.0.1', ...overrides };
}

function renderDeviceCard(data: Partial<DeviceNodeData> = {}) {
  return render(
    <ReactFlowProvider>
      <DeviceCard {...makeNodeProps({ device: mockDevice(), pinned: false, ...data })} />
    </ReactFlowProvider>,
  );
}

describe('DeviceCard', () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('renders compact overview name, type, and IP chip', () => {
    renderDeviceCard();
    expect(screen.getByText('router-01')).toBeInTheDocument();
  });
});
```

```go
// internal/repository/sqlite/device_repo_test.go
func TestDeviceRepoGetBySysName_NormalizedLookup(t *testing.T) {
    db := newTestDB(t)
    repo := NewDeviceRepo(db, testKey, nil)

    tests := []struct {
        name string
        lookup string
        expectedID uuid.UUID
    }{ /* cases */ }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            device, err := repo.GetBySysName(tc.lookup)
            if err != nil { t.Fatalf("GetBySysName failed: %v", err) }
            if device.ID != tc.expectedID { t.Fatalf("expected device %s, got %s", tc.expectedID, device.ID) }
        })
    }
}
```

**Patterns:**
- Use local factory helpers for DTOs and props: `mockDevice`, `mockMetrics`, `mockLink`, `makeNodeProps` in `frontend/src/components/DeviceCard.test.tsx`.
- Use `beforeEach`/`afterEach` for fake timers and globals: `frontend/src/components/DeviceCard.test.tsx`, `frontend/src/hooks/useWebSocket.test.ts`.
- Use table-driven subtests in Go for variants: `internal/repository/sqlite/device_repo_test.go`.
- Use `t.Helper()` in backend test helpers: `setupTestDB` and `newTestDB` in `internal/repository/sqlite/test_helpers_test.go`.
- Prefer user-visible assertions in component tests: `screen.getByText`, `screen.queryByText`, and `toBeInTheDocument` in `frontend/src/components/DeviceCard.test.tsx`.

## Mocking

**Framework:**
- Backend uses hand-written fakes/mocks and `httptest`; no generated mocks are detected.
- Frontend uses Vitest `vi.fn`, `vi.stubGlobal`, `vi.spyOn`, fake timers, and Testing Library helpers.
- E2E uses Playwright `page.addInitScript` and browser APIs to observe WebSocket behavior.

**Patterns:**
```typescript
// frontend/src/hooks/useWebSocket.test.ts
class MockWebSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  send = vi.fn();
  close = vi.fn();
  readyState = MockWebSocket.CONNECTING;

  simulateOpen() { this.readyState = MockWebSocket.OPEN; this.onopen?.(); }
  simulateMessage(data: unknown) { this.onmessage?.({ data: JSON.stringify(data) } as MessageEvent); }
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal('WebSocket', OriginalMock);
});
```

```go
// internal/api/device_handler_test.go
type mockDeviceRepo struct {
    mu sync.Mutex
    devices map[uuid.UUID]*domain.Device
}

func newMockDeviceRepo() *mockDeviceRepo {
    return &mockDeviceRepo{devices: make(map[uuid.UUID]*domain.Device)}
}
```

**What to Mock:**
- Mock browser globals and network boundaries in frontend unit tests: `fetch` in `frontend/src/api/client.test.ts`, `WebSocket` in `frontend/src/hooks/useWebSocket.test.ts`.
- Mock repositories/services in backend handler tests with in-memory structs: `mockDeviceRepo`, `mockLinkRepo` in `internal/api/device_handler_test.go`.
- Use `httptest.NewServer` for external HTTP integrations such as Prometheus and WebSocket handler tests: `internal/metrics/prometheus_test.go`, `internal/ws/handler_test.go`.
- Use temporary/in-memory SQLite DBs for repository behavior instead of mocking SQL: `internal/repository/sqlite/test_helpers_test.go`.

**What NOT to Mock:**
- Do not mock parser/normalizer functions when testing API client behavior; assert parsed DTO results from payload fixtures as in `frontend/src/api/client.test.ts`.
- Do not mock React component internals; render components and assert DOM-visible output as in `frontend/src/components/DeviceCard.test.tsx`.
- Do not mock database migrations in repository tests; `setupTestDB` runs `RunMigrations(db)` in `internal/repository/sqlite/test_helpers_test.go`.
- Do not mock full browser/server behavior in E2E; `frontend/playwright.config.ts` starts the Go backend and Vite frontend.

## Fixtures and Factories

**Test Data:**
```typescript
// frontend/src/api/client.test.ts
function deviceResource(id: string, hostname: string, ip: string) {
  return {
    id,
    attributes: {
      hostname,
      ip,
      device_type: 'router',
      status: 'up',
      vendor: 'mikrotik',
    },
    relationships: { interfaces: { data: [] } },
  };
}
```

```go
// internal/repository/sqlite/device_repo_test.go
device := &domain.Device{
    ID: uuid.New(),
    Hostname: "edge-sw-01",
    IP: "10.0.0.3",
    SysName: "edge-sw-01",
    Managed: true,
    Status: domain.DeviceStatusUp,
    Tags: map[string]string{},
}
```

**Location:**
- Keep small factories local to each test file: `frontend/src/components/DeviceCard.test.tsx`, `frontend/src/api/client.test.ts`.
- Keep reusable package DB helpers in package test helper files: `internal/repository/sqlite/test_helpers_test.go`.
- Use test-only constants and helpers inside package tests for WebSocket clients: `registerTestClient` in `internal/ws/hub_test.go`.
- Playwright bootstrapping uses `frontend/playwright.config.ts` and `frontend/e2e/global.setup.ts` is excluded from Biome in `frontend/biome.json`.

## Coverage

**Requirements:**
- Backend `make backend-fast` runs `go test ./... -covermode=atomic -coverprofile=coverage/backend-fast.out` and `scripts/check-go-cover.sh coverage/backend-fast.out 60` from `Makefile`.
- Frontend Vitest coverage thresholds are lines 60%, functions 50%, branches 55%, statements 60% in `frontend/vitest.config.ts`.
- Coverage output uses V8 provider with `text` and `lcov` reporters into `frontend/coverage` per `frontend/vitest.config.ts`.

**View Coverage:**
```bash
make backend-fast                              # Writes coverage/backend-fast.out and enforces 60%
npm --prefix frontend run test:coverage        # Writes frontend/coverage with text and lcov reports
```

## Test Types

**Unit Tests:**
- Pure frontend utilities are tested directly: `frontend/src/utils/validation.test.ts`, `frontend/src/utils/freshness.test.ts`, `frontend/src/components/canvas/topologyComposer.test.ts`.
- Frontend parsers/types are tested with payload fixtures: `frontend/src/types/api.test.ts`, `frontend/src/types/metrics.test.ts`.
- Backend domain/service units use direct constructors and table cases: `internal/domain/poll_class_test.go`, `internal/scheduler/jitter_test.go`.

**Integration Tests:**
- Backend repository tests exercise SQLite migrations and SQL behavior: `internal/repository/sqlite/device_repo_test.go`, `internal/repository/sqlite/migrations_test.go`.
- Backend HTTP handler tests use `httptest.NewRequest` / `httptest.NewRecorder`: `internal/api/settings_handler_test.go`, `internal/api/vendor_handler_test.go`.
- WebSocket and Prometheus tests use `httptest.NewServer`: `internal/ws/handler_test.go`, `internal/metrics/prometheus_test.go`.
- `make test-integration` runs `go test ./... -tags=integration -count=1 -v` against Docker Compose services from `Makefile`.

**E2E Tests:**
- Playwright E2E is used in `frontend/e2e/realtime.spec.ts`.
- `frontend/playwright.config.ts` starts the backend with SQLite test settings and Vite frontend, then tests against `http://127.0.0.1:3300`.
- E2E tests assert seeded topology rendering, dashboard rows, websocket reconnect recovery, and device detail panel subscription behavior.

## Common Patterns

**Async Testing:**
```typescript
// frontend/src/api/client.test.ts
vi.stubGlobal('fetch', vi.fn().mockResolvedValue(mockResponse(payload)));
await expect(createDevice(payload)).rejects.toThrow(ValidationError);
```

```typescript
// frontend/e2e/realtime.spec.ts
await page.waitForFunction(() => {
  return Boolean((window as Window & { __playwrightBackendReconnected?: boolean }).__playwrightBackendReconnected);
}, null, { timeout: 15_000 });
```

```go
// internal/ws/hub_test.go
deadline := time.Now().Add(time.Second)
for time.Now().Before(deadline) {
    metrics := string(registry.MarshalPrometheus())
    if strings.Contains(metrics, `theia_ws_backpressure_total`) { break }
    time.Sleep(10 * time.Millisecond)
}
```

**Error Testing:**
```typescript
// frontend/src/api/client.test.ts
vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
  mockResponse({ error: 'a device already exists' }, { ok: false, status: 409, statusText: 'Conflict' }),
));
await expect(createDevice(payload)).rejects.toThrow(ValidationError);
```

```go
// internal/repository/sqlite/device_repo_test.go
result, err := repo.GetBySysName("unknown-host.example.net")
if err != nil {
    t.Fatalf("GetBySysName failed: %v", err)
}
if result != nil {
    t.Fatalf("expected nil for unknown lookup, got %s", result.ID)
}
```

---

*Testing analysis: 2026-04-23*
