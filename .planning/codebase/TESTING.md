# Testing Patterns

**Analysis Date:** 2026-04-19

## Test Framework

**Runner:**
- Frontend: Vitest `^4.1.0` from `frontend/package.json`
- Frontend config: `frontend/vitest.config.ts`
- Backend: Go `testing` package driven by `go test` in `Makefile`

**Assertion Library:**
- Frontend: Vitest `expect` plus `@testing-library/jest-dom` from `frontend/src/test-setup.ts`
- Backend: standard `testing` assertions with `t.Fatal`, `t.Fatalf`, `t.Error`, and `t.Errorf`

**Run Commands:**
```bash
npm --prefix frontend test      # Run frontend tests via `frontend/package.json`
make test                       # Run backend unit tests in the backend container
make test-integration           # Run backend integration-tagged tests against simulators
```

Watch mode and a dedicated coverage command are not configured in `frontend/package.json` or `Makefile`.

## Test File Organization

**Location:**
- Frontend tests are mostly co-located with implementation files, e.g. `frontend/src/api/client.ts` with `frontend/src/api/client.test.ts`, `frontend/src/hooks/useWebSocket.ts` with `frontend/src/hooks/useWebSocket.test.ts`, and `frontend/src/components/Toolbar.tsx` with `frontend/src/components/Toolbar.test.tsx`.
- Additional audit/smoke suites live under `frontend/src/components/__tests__/`, such as `frontend/src/components/__tests__/theme05-smoke.test.tsx` and `frontend/src/components/__tests__/canvas-token-audit.test.ts`.
- Backend tests sit next to package files as `*_test.go`, e.g. `internal/config/config_test.go`, `internal/scheduler/scheduler_test.go`, and `internal/topology/observations_test.go`.

**Naming:**
- Use `.test.ts` and `.test.tsx` for frontend tests.
- Use `*_test.go` with `TestXxx` functions for backend tests.
- Backend test names often encode exact scenarios with underscores, such as `TestRefreshDevices_SchedulesOperationalOnlyForVirtualIPDevices` in `internal/scheduler/scheduler_test.go`.

**Structure:**
```text
frontend/src/<feature>/<module>.ts
frontend/src/<feature>/<module>.test.ts
frontend/src/components/__tests__/<audit>.test.ts[x]
internal/<package>/<module>.go
internal/<package>/<module>_test.go
```

## Test Structure

**Suite Organization:**
```typescript
describe('useWebSocket', () => {
  it('sets connected=true after open', () => {
    const { result } = renderHook(() => useWebSocket('ws://localhost:8080/ws'));

    act(() => {
      mockInstance.simulateOpen();
    });

    expect(result.current.connected).toBe(true);
  });
});
```

Pattern from `frontend/src/hooks/useWebSocket.test.ts`.

```go
func TestRefreshDevices_SkipsUnmanagedDevices(t *testing.T) {
	managed := domain.Device{Managed: true}
	unmanaged := domain.Device{Managed: false}
	// setup omitted
	if got := len(scheduler.items); got != 3 {
		t.Fatalf("len(items) = %d, want only 3 managed tasks", got)
	}
}
```

Pattern from `internal/scheduler/scheduler_test.go`.

**Patterns:**
- Frontend suites group behavior with `describe` blocks and keep most assertions in small single-behavior `it` cases, as in `frontend/src/api/client.test.ts`, `frontend/src/contexts/ThemeContext.test.tsx`, and `frontend/src/components/Dashboard.test.tsx`.
- Backend tests favor plain helper functions and direct setup over external test frameworks, as in `internal/topology/observations_test.go` and `internal/service/static_persistence_test.go`.
- Shared setup lives in local helpers inside the test file, not in a global fixtures package: see `mockDevice` in `frontend/src/components/Dashboard.test.tsx`, `mockResponse` in `frontend/src/api/client.test.ts`, `newMockDeviceRepo` in `internal/service/device_service_test.go`, and `newMaterializerRepos` in `internal/topology/observations_test.go`.
- Cleanup uses framework-native tools: `beforeEach` / `afterEach` in frontend files and `t.Cleanup` / `t.TempDir` in Go files.

## Mocking

**Framework:**
- Frontend: Vitest mocks and spies via `vi.mock`, `vi.fn`, `vi.stubGlobal`, and `vi.spyOn`
- Backend: hand-written fakes, in-memory repositories, temp directories, and real in-memory SQLite databases

**Patterns:**
```typescript
vi.mock('../api/client', () => ({
  fetchAreas: vi.fn().mockResolvedValue([]),
  updateDevice: vi.fn().mockResolvedValue({}),
  deleteDevice: vi.fn().mockResolvedValue(undefined),
}));
```

Pattern from `frontend/src/components/BulkEditPanel.test.tsx`.

```typescript
beforeEach(() => {
  vi.useFakeTimers();
  vi.stubGlobal('WebSocket', OriginalMock);
});
```

Pattern from `frontend/src/hooks/useWebSocket.test.ts`.

```go
type mockDialer struct {
	dialCalled bool
	addr       string
	err        error
}

func (m *mockDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	m.dialCalled = true
	m.addr = addr
	return nil, m.err
}
```

Pattern from `internal/ssh/client_test.go`.

**What to Mock:**
- Mock browser globals and network boundaries in frontend tests: `fetch` in `frontend/src/api/client.test.ts`, `WebSocket` in `frontend/src/hooks/useWebSocket.test.ts`, and `matchMedia` in `frontend/src/contexts/ThemeContext.test.tsx`.
- Mock complex child components when the parent test only cares about composition, as in `frontend/src/components/Dashboard.test.tsx`.
- Replace backend collaborators with small in-file fakes for repositories and dialers, as in `internal/service/device_service_test.go` and `internal/ssh/client_test.go`.

**What NOT to Mock:**
- Prefer real parser logic and real component state transitions in frontend tests. `frontend/src/api/client.test.ts` exercises actual response parsing instead of mocking `parseDevicesResponse`.
- Prefer real SQLite repositories when repository semantics matter, as in `internal/topology/observations_test.go` and parts of `internal/cache/cache_test.go`.
- Do not add heavyweight mocking libraries; the existing style is manual and file-local.

## Fixtures and Factories

**Test Data:**
```typescript
function mockDevice(overrides: Partial<Device> = {}): Device {
  return {
    id: 'dev-1',
    hostname: 'router-01',
    ip: '10.0.0.1',
    device_type: 'router',
    status: 'up',
    interfaces: [],
    backup_supported: true,
    metrics_source: 'prometheus',
    prometheus_label_name: 'instance',
    prometheus_label_value: '10.0.0.1:9100',
    area_ids: [],
    ...overrides,
  };
}
```

Pattern from `frontend/src/components/Dashboard.test.tsx`.

```go
func newMockSettingsRepo() *mockSettingsRepo {
	return &mockSettingsRepo{settings: domain.DefaultSettings()}
}
```

Pattern from `internal/service/device_service_test.go`.

**Location:**
- Fixtures are defined inline in each test file instead of centralized fixture modules.
- Temporary filesystem fixtures use `t.TempDir()` in backend tests like `internal/vendor/registry_test.go`.
- In-memory DB fixtures use `sql.Open("sqlite3", ":memory:?_foreign_keys=on")` plus migrations in `internal/topology/observations_test.go`.

## Coverage

**Requirements:** None enforced by configuration.

**View Coverage:**
```bash
Not detected in `frontend/package.json`, `frontend/vitest.config.ts`, or `Makefile`
```

## Test Types

**Unit Tests:**
- Frontend unit tests cover API clients, utility functions, hooks, and component rendering in files such as `frontend/src/api/client.test.ts`, `frontend/src/utils/validation.test.ts`, and `frontend/src/components/Toolbar.test.tsx`.
- Backend unit tests cover pure domain logic, config loading, encryption, and scheduler behavior in `internal/domain/poll_class_test.go`, `internal/config/config_test.go`, `internal/crypto/encrypt_test.go`, and `internal/scheduler/scheduler_test.go`.

**Integration Tests:**
- Backend tests blend unit and lightweight integration styles. Several packages use real SQLite migrations and repositories, notably `internal/topology/observations_test.go` and `internal/cache/cache_test.go`.
- Containerized integration execution is defined by `make test-integration` in `Makefile`, which runs `go test ./... -tags=integration` against SNMP simulators.

**E2E Tests:**
- A browser E2E framework is not detected. There is no Playwright or Cypress config at the repository root or in `frontend/`.
- UI confidence relies on component tests plus audit/smoke suites in `frontend/src/components/__tests__/`.

## Common Patterns

**Async Testing:**
```typescript
fireEvent.click(screen.getByText('Apply to 1 Devices'));

await waitFor(() => {
  expect(screen.getByText(/server error \(ref: bulk001\)/)).toBeInTheDocument();
});
```

Pattern from `frontend/src/components/BulkEditPanel.test.tsx`.

```go
if err := sqliterepo.RunMigrations(db); err != nil {
	t.Fatalf("RunMigrations failed: %v", err)
}
```

Pattern from `internal/topology/observations_test.go`.

**Error Testing:**
```typescript
await expect(createDevice(payload)).rejects.toThrow(ValidationError);
await expect(createDevice(payload)).rejects.toThrow('a device with IP/host "10.0.0.2" already exists');
```

Pattern from `frontend/src/api/client.test.ts`.

```go
_, err := Decrypt(ciphertext, key2)
if err == nil {
	t.Fatal("Decrypt with wrong key should fail")
}
```

Pattern from `internal/crypto/encrypt_test.go`.

## Prescriptive Guidance

- Add new frontend tests beside the file under test unless the suite is an audit-style rule check that belongs in `frontend/src/components/__tests__/`.
- Use Vitest globals, Testing Library render helpers, and local file-level factories, following `frontend/src/api/client.test.ts` and `frontend/src/components/Dashboard.test.tsx`.
- Mock only boundaries: browser APIs, HTTP, WebSocket, or heavyweight child components. Keep parsing and business logic real.
- For backend code, prefer table-driven tests for pure logic and explicit in-file mocks for services, matching `internal/domain/poll_class_test.go` and `internal/service/device_service_test.go`.
- Use `t.TempDir`, `t.Setenv`, and in-memory SQLite instead of bespoke test harnesses when filesystem, environment, or repository behavior matters.
- Name tests after the exact contract they protect, and keep requirement IDs in comments only when they already exist in the surrounding file.

---

*Testing analysis: 2026-04-19*
