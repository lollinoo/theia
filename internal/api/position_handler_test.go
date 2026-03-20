package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// mockPositionRepo implements domain.PositionRepository backed by an in-memory slice.
type mockPositionRepo struct {
	positions []domain.DevicePosition
	err       error // if set, all operations return this error
}

func newMockPositionRepo() *mockPositionRepo {
	return &mockPositionRepo{}
}

func (m *mockPositionRepo) GetAll() ([]domain.DevicePosition, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.positions, nil
}

func (m *mockPositionRepo) SaveAll(positions []domain.DevicePosition) error {
	if m.err != nil {
		return m.err
	}
	m.positions = positions
	return nil
}

func (m *mockPositionRepo) DeleteByDeviceID(deviceID uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	filtered := m.positions[:0]
	for _, p := range m.positions {
		if p.DeviceID != deviceID {
			filtered = append(filtered, p)
		}
	}
	m.positions = filtered
	return nil
}

func TestPositionHandlerList(t *testing.T) {
	repo := newMockPositionRepo()
	id := uuid.New()
	repo.positions = []domain.DevicePosition{
		{DeviceID: id, X: 10, Y: 20, Pinned: true},
	}
	h := NewPositionHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/positions", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Data []domain.DevicePosition `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 position, got %d", len(resp.Data))
	}
	if resp.Data[0].DeviceID != id {
		t.Fatalf("expected device_id=%s, got %s", id, resp.Data[0].DeviceID)
	}
}

func TestPositionHandlerList_RepoError(t *testing.T) {
	repo := newMockPositionRepo()
	repo.err = errMock
	h := NewPositionHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/positions", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestPositionHandlerSaveAll_HappyPath(t *testing.T) {
	repo := newMockPositionRepo()
	h := NewPositionHandler(repo)

	id := uuid.New()
	body := `{"positions":[{"device_id":"` + id.String() + `","x":100,"y":200,"pinned":false}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/positions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSaveAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if len(repo.positions) != 1 {
		t.Fatalf("expected 1 saved position, got %d", len(repo.positions))
	}
	if repo.positions[0].DeviceID != id {
		t.Fatalf("expected device_id=%s, got %s", id, repo.positions[0].DeviceID)
	}
	if repo.positions[0].X != 100 || repo.positions[0].Y != 200 {
		t.Fatalf("expected x=100 y=200, got x=%f y=%f", repo.positions[0].X, repo.positions[0].Y)
	}
}

func TestPositionHandlerSaveAll_MalformedJSON(t *testing.T) {
	repo := newMockPositionRepo()
	h := NewPositionHandler(repo)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/positions", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()

	h.HandleSaveAll(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPositionHandlerSaveAll_InvalidDeviceID(t *testing.T) {
	repo := newMockPositionRepo()
	h := NewPositionHandler(repo)

	body := `{"positions":[{"device_id":"not-a-uuid","x":100,"y":200}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/positions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSaveAll(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestPositionHandlerSaveAll_RepoError(t *testing.T) {
	repo := newMockPositionRepo()
	repo.err = errMock
	h := NewPositionHandler(repo)

	id := uuid.New()
	body := `{"positions":[{"device_id":"` + id.String() + `","x":1,"y":2}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/positions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleSaveAll(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
