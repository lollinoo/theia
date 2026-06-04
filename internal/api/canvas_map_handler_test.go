package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// TestCanvasMapHandlerDeleteDefaultMapReturnsConflict characterizes HTTP conflict mapping for default maps.
func TestCanvasMapHandlerDeleteDefaultMapReturnsConflict(t *testing.T) {
	mapID := uuid.New()
	mapRepo := &fakeCanvasMapHandlerMapRepo{
		maps: map[uuid.UUID]domain.CanvasMap{
			mapID: {ID: mapID, Name: "Default", IsDefault: true},
		},
	}
	handler := NewCanvasMapHandler(
		mapRepo,
		&fakeCanvasMapHandlerPositionRepo{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/canvas/maps/"+mapID.String(), nil)
	rec := httptest.NewRecorder()

	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
	if mapRepo.deleted {
		t.Fatal("default map delete reached repository Delete")
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "cannot delete default canvas map" {
		t.Fatalf("error = %q, want default-map delete conflict", body["error"])
	}
}

// fakeCanvasMapHandlerMapRepo provides the CanvasMapRepository surface needed by handler tests.
type fakeCanvasMapHandlerMapRepo struct {
	maps    map[uuid.UUID]domain.CanvasMap
	deleted bool
}

// Create records no maps because delete tests do not exercise creation.
func (r *fakeCanvasMapHandlerMapRepo) Create(domain.CanvasMapCreate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// GetByID returns the seeded map for handler path lookups.
func (r *fakeCanvasMapHandlerMapRepo) GetByID(id uuid.UUID) (domain.CanvasMap, error) {
	if canvasMap, ok := r.maps[id]; ok {
		return canvasMap, nil
	}
	return domain.CanvasMap{}, errMock
}

// GetDefault returns an error because default lookup is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) GetDefault() (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// List returns no maps because list behavior is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) List() ([]domain.CanvasMap, error) {
	return nil, errMock
}

// Update returns an error because update behavior is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) Update(uuid.UUID, domain.CanvasMapUpdate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// SetPrimary returns an error because primary selection is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) SetPrimary(uuid.UUID) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// Delete records whether persistence deletion was reached.
func (r *fakeCanvasMapHandlerMapRepo) Delete(uuid.UUID) error {
	r.deleted = true
	return nil
}

// Duplicate returns an error because duplication is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) Duplicate(uuid.UUID, string) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// GetMembership returns an error because membership is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	return domain.CanvasMapMembership{}, errMock
}

// ReplaceMembership returns an error because membership replacement is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error {
	return errMock
}

// UpdateDeviceVisualColor returns an error because visual metadata is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) UpdateDeviceVisualColor(uuid.UUID, uuid.UUID, *string) error {
	return errMock
}

// RemoveDevice returns an error because device membership removal is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) RemoveDevice(uuid.UUID, uuid.UUID) error {
	return errMock
}

// RemoveLink returns an error because link removal is outside this handler test.
func (r *fakeCanvasMapHandlerMapRepo) RemoveLink(uuid.UUID, uuid.UUID) error {
	return errMock
}

// fakeCanvasMapHandlerPositionRepo satisfies the handler's required position repository dependency.
type fakeCanvasMapHandlerPositionRepo struct{}

// GetAllForMap returns no positions because delete behavior does not read positions.
func (r *fakeCanvasMapHandlerPositionRepo) GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error) {
	return nil, errMock
}

// SaveAllForMap returns an error because delete behavior does not save positions.
func (r *fakeCanvasMapHandlerPositionRepo) SaveAllForMap(uuid.UUID, []domain.DevicePosition) error {
	return errMock
}

// DeleteByDeviceID returns an error because delete behavior does not prune positions by device.
func (r *fakeCanvasMapHandlerPositionRepo) DeleteByDeviceID(uuid.UUID) error {
	return errMock
}
