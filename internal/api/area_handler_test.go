package api

// This file exercises area handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// mockAreaRepo implements domain.AreaRepository backed by an in-memory map.
type mockAreaRepo struct {
	areas map[uuid.UUID]*domain.Area
}

func newMockAreaRepo() *mockAreaRepo {
	return &mockAreaRepo{areas: make(map[uuid.UUID]*domain.Area)}
}

func (m *mockAreaRepo) Create(area *domain.Area) error {
	if area.ID == uuid.Nil {
		area.ID = uuid.New()
	}
	// Check unique name constraint
	for _, existing := range m.areas {
		if existing.Name == area.Name {
			return fmt.Errorf("UNIQUE constraint failed: areas.name")
		}
	}
	now := time.Now().UTC()
	area.CreatedAt = now
	area.UpdatedAt = now
	m.areas[area.ID] = area
	return nil
}

func (m *mockAreaRepo) GetByID(id uuid.UUID) (*domain.Area, error) {
	a, ok := m.areas[id]
	if !ok {
		return nil, fmt.Errorf("area not found: %s", id)
	}
	return a, nil
}

func (m *mockAreaRepo) GetAll() ([]domain.Area, error) {
	result := make([]domain.Area, 0, len(m.areas))
	for _, a := range m.areas {
		result = append(result, *a)
	}
	return result, nil
}

func (m *mockAreaRepo) GetAllWithDeviceCount() ([]domain.AreaWithCount, error) {
	result := make([]domain.AreaWithCount, 0, len(m.areas))
	for _, a := range m.areas {
		result = append(result, domain.AreaWithCount{Area: *a, DeviceCount: 0})
	}
	return result, nil
}

func (m *mockAreaRepo) Update(area *domain.Area) error {
	if _, ok := m.areas[area.ID]; !ok {
		return fmt.Errorf("area not found: %s", area.ID)
	}
	// Check unique name constraint (excluding self)
	for _, existing := range m.areas {
		if existing.Name == area.Name && existing.ID != area.ID {
			return fmt.Errorf("UNIQUE constraint failed: areas.name")
		}
	}
	area.UpdatedAt = time.Now().UTC()
	m.areas[area.ID] = area
	return nil
}

func (m *mockAreaRepo) Delete(id uuid.UUID) error {
	if _, ok := m.areas[id]; !ok {
		return fmt.Errorf("area not found: %s", id)
	}
	delete(m.areas, id)
	return nil
}

// seedAreaHelper adds an area to the mock repo and returns its ID.
func seedAreaHelper(t *testing.T, repo *mockAreaRepo, name, color string) uuid.UUID {
	t.Helper()
	a := &domain.Area{Name: name, Description: "test area", Color: color}
	if err := repo.Create(a); err != nil {
		t.Fatalf("failed to seed area %q: %v", name, err)
	}
	return a.ID
}

func TestAreaHandlerList(t *testing.T) {
	repo := newMockAreaRepo()
	seedAreaHelper(t, repo, "Backbone", "#2979FF")
	h := NewAreaHandler(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/areas", nil)
	rec := httptest.NewRecorder()

	h.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		Data []areaResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 area, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "Backbone" {
		t.Errorf("Name = %q, want %q", resp.Data[0].Name, "Backbone")
	}
}

func TestAreaHandlerCreate_HappyPath(t *testing.T) {
	repo := newMockAreaRepo()
	h := NewAreaHandler(repo)

	body := `{"name":"Edge","description":"Edge switches","color":"#FF6D00"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/areas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data areaResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.Name != "Edge" {
		t.Errorf("Name = %q, want %q", resp.Data.Name, "Edge")
	}
	if resp.Data.Color != "#FF6D00" {
		t.Errorf("Color = %q, want %q", resp.Data.Color, "#FF6D00")
	}
}

func TestAreaHandlerCreate_DefaultColor(t *testing.T) {
	repo := newMockAreaRepo()
	h := NewAreaHandler(repo)

	body := `{"name":"NoColor"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/areas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Data areaResponse `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.Color != "#00E676" {
		t.Errorf("Color = %q, want default %q", resp.Data.Color, "#00E676")
	}
}

func TestAreaHandlerCreate_DuplicateName_409(t *testing.T) {
	repo := newMockAreaRepo()
	seedAreaHelper(t, repo, "Backbone", "#2979FF")
	h := NewAreaHandler(repo)

	body := `{"name":"Backbone","color":"#FF1744"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/areas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAreaHandlerCreate_EmptyName_400(t *testing.T) {
	repo := newMockAreaRepo()
	h := NewAreaHandler(repo)

	body := `{"name":"","color":"#00E676"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/areas", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAreaHandlerDelete_204(t *testing.T) {
	repo := newMockAreaRepo()
	id := seedAreaHelper(t, repo, "ToDelete", "#FF1744")
	h := NewAreaHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/areas/"+id.String(), nil)
	rec := httptest.NewRecorder()

	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAreaHandlerDelete_NotFound_404(t *testing.T) {
	repo := newMockAreaRepo()
	h := NewAreaHandler(repo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/areas/"+uuid.New().String(), nil)
	rec := httptest.NewRecorder()

	h.HandleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body=%s", rec.Code, rec.Body.String())
	}
}
