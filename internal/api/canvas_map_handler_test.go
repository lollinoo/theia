package api

// This file exercises canvas map handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ws"
)

func TestCanvasMapHandlerBootstrapReturnsRuntimeStream(t *testing.T) {
	mapID := uuid.New()
	mapRepo := &fakeCanvasMapHandlerMapRepo{
		maps: map[uuid.UUID]domain.CanvasMap{
			mapID: {ID: mapID, Name: "Operations"},
		},
	}
	mapPositionRepo := &fakeCanvasMapHandlerPositionRepo{}
	canvasTopology, _, linkRepo, positionRepo, areaRepo := newTestCanvasTopologyHandler(t)
	runtimeSnapshot := ws.EmptySnapshot()
	runtimeState := ws.RuntimeOverviewState{
		Snapshot: runtimeSnapshot,
		Version:  13,
		StreamID: "runtime-stream-13",
	}
	handler := NewCanvasMapHandler(
		mapRepo,
		mapPositionRepo,
		positionRepo,
		canvasTopology,
		canvasTopology.deviceService,
		linkRepo,
		areaRepo,
		func() ws.RuntimeOverviewState { return runtimeState },
	)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/canvas/maps/"+mapID.String()+"/bootstrap", nil)
	response := httptest.NewRecorder()

	handler.HandleBootstrap(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var body canvasTopologyResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RuntimeStreamID != runtimeState.StreamID {
		t.Fatalf("runtime_stream_id = %q, want %q", body.RuntimeStreamID, runtimeState.StreamID)
	}
	if body.RuntimeVersion == nil || *body.RuntimeVersion != runtimeState.Version {
		t.Fatalf("runtime_version = %#v, want %d", body.RuntimeVersion, runtimeState.Version)
	}
	if body.RuntimeIdentity != ws.RuntimeIdentityForSnapshot(runtimeSnapshot) {
		t.Fatalf("runtime_identity = %q, want exact snapshot identity", body.RuntimeIdentity)
	}
}

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
	maps       map[uuid.UUID]domain.CanvasMap
	membership domain.CanvasMapMembership
	deleted    bool
}

// Create records no maps because delete tests do not exercise creation.
func (r *fakeCanvasMapHandlerMapRepo) Create(domain.CanvasMapCreate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) GetByID(id uuid.UUID) (domain.CanvasMap, error) {
	if canvasMap, ok := r.maps[id]; ok {
		return canvasMap, nil
	}
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) GetDefault() (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) List() ([]domain.CanvasMap, error) {
	return nil, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) Update(uuid.UUID, domain.CanvasMapUpdate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) SetPrimary(uuid.UUID) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

// Delete records whether persistence deletion was reached.
func (r *fakeCanvasMapHandlerMapRepo) Delete(uuid.UUID) error {
	r.deleted = true
	return nil
}

func (r *fakeCanvasMapHandlerMapRepo) Duplicate(uuid.UUID, string) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) GetMembership(uuid.UUID) (domain.CanvasMapMembership, error) {
	return r.membership, nil
}

func (r *fakeCanvasMapHandlerMapRepo) ReplaceMembership(uuid.UUID, domain.CanvasMapMembership) error {
	return errMock
}

func (r *fakeCanvasMapHandlerMapRepo) UpdateDeviceVisualColor(uuid.UUID, uuid.UUID, *string) error {
	return errMock
}

func (r *fakeCanvasMapHandlerMapRepo) RemoveDevice(uuid.UUID, uuid.UUID) error {
	return errMock
}

func (r *fakeCanvasMapHandlerMapRepo) RemoveLink(uuid.UUID, uuid.UUID) error {
	return errMock
}

// fakeCanvasMapHandlerPositionRepo satisfies the handler's required position repository dependency.
type fakeCanvasMapHandlerPositionRepo struct {
	positions []domain.DevicePosition
	err       error
}

func (r *fakeCanvasMapHandlerPositionRepo) GetAllForMap(uuid.UUID) ([]domain.DevicePosition, error) {
	return append([]domain.DevicePosition(nil), r.positions...), r.err
}

func (r *fakeCanvasMapHandlerPositionRepo) SaveAllForMap(uuid.UUID, []domain.DevicePosition) error {
	return errMock
}

func (r *fakeCanvasMapHandlerPositionRepo) DeleteByDeviceID(uuid.UUID) error {
	return errMock
}
