package api

// This file exercises canvas map handler behavior so refactors preserve the documented contract.

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
		nil,
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

func TestCanvasMapHandlerSaveLinkRoute(t *testing.T) {
	fixture := newCanvasMapLinkRouteHandlerFixture(t)
	request := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/canvas/maps/"+fixture.mapID.String()+"/link-routes/"+fixture.linkID.String(),
		strings.NewReader(`{"version":1,"waypoints":[{"x":120,"y":80}]}`),
	)
	response := httptest.NewRecorder()

	fixture.handler.HandleSaveLinkRoute(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	stored, ok := fixture.routeRepo.routes[fixture.mapID][fixture.linkID]
	if !ok {
		t.Fatal("route was not persisted")
	}
	if stored.LinkID != fixture.linkID || stored.Version != domain.CanvasMapLinkRouteVersion {
		t.Fatalf("stored route = %#v, want link %s version %d", stored, fixture.linkID, domain.CanvasMapLinkRouteVersion)
	}
	if len(stored.Waypoints) != 1 || stored.Waypoints[0] != (domain.CanvasPoint{X: 120, Y: 80}) {
		t.Fatalf("stored waypoints = %#v, want [{120 80}]", stored.Waypoints)
	}

	var body struct {
		Data canvasLinkRouteResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Version != stored.Version || len(body.Data.Waypoints) != 1 || body.Data.Waypoints[0] != stored.Waypoints[0] {
		t.Fatalf("response route = %#v, want stored route %#v", body.Data, stored)
	}
	if body.Data.UpdatedAt != formatCanvasMapTimestamp(stored.UpdatedAt) {
		t.Fatalf("updated_at = %q, want %q", body.Data.UpdatedAt, formatCanvasMapTimestamp(stored.UpdatedAt))
	}
}

func TestCanvasMapHandlerSaveLinkRouteRejectsInvalidInputAndRepositoryFailures(t *testing.T) {
	tests := []struct {
		name    string
		path    func(canvasMapLinkRouteHandlerFixture) string
		body    string
		prepare func(*testing.T, canvasMapLinkRouteHandlerFixture)
		want    int
	}{
		{
			name: "invalid map UUID",
			path: func(f canvasMapLinkRouteHandlerFixture) string {
				return "/api/v1/canvas/maps/not-a-uuid/link-routes/" + f.linkID.String()
			},
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			want: http.StatusNotFound,
		},
		{
			name: "invalid link UUID",
			path: func(f canvasMapLinkRouteHandlerFixture) string {
				return "/api/v1/canvas/maps/" + f.mapID.String() + "/link-routes/not-a-uuid"
			},
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "unsupported version",
			body: `{"version":2,"waypoints":[{"x":1,"y":2}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "zero waypoints",
			body: `{"version":1,"waypoints":[]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "seventeen waypoints",
			body: mustCanvasLinkRoutePayload(t, domain.CanvasMapLinkRouteMaxWaypoints+1),
			want: http.StatusBadRequest,
		},
		{
			name: "nonnumeric coordinate",
			body: `{"version":1,"waypoints":[{"x":"right","y":2}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "overflowing coordinate",
			body: `{"version":1,"waypoints":[{"x":1e1000,"y":2}]}`,
			want: http.StatusBadRequest,
		},
		{
			name: "non-member link",
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			prepare: func(_ *testing.T, f canvasMapLinkRouteHandlerFixture) {
				f.routeRepo.upsertErr = fmt.Errorf("membership rejected: %w", domain.ErrCanvasMapLinkRouteNotMember)
			},
			want: http.StatusBadRequest,
		},
		{
			name: "missing map",
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			prepare: func(_ *testing.T, f canvasMapLinkRouteHandlerFixture) {
				delete(f.mapRepo.maps, f.mapID)
			},
			want: http.StatusNotFound,
		},
		{
			name: "missing canonical link",
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			prepare: func(t *testing.T, f canvasMapLinkRouteHandlerFixture) {
				if err := f.linkRepo.Delete(f.linkID); err != nil {
					t.Fatalf("remove canonical link: %v", err)
				}
			},
			want: http.StatusNotFound,
		},
		{
			name: "persistence failure",
			body: `{"version":1,"waypoints":[{"x":1,"y":2}]}`,
			prepare: func(_ *testing.T, f canvasMapLinkRouteHandlerFixture) {
				f.routeRepo.upsertErr = errMock
			},
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCanvasMapLinkRouteHandlerFixture(t)
			if tt.prepare != nil {
				tt.prepare(t, fixture)
			}
			path := "/api/v1/canvas/maps/" + fixture.mapID.String() + "/link-routes/" + fixture.linkID.String()
			if tt.path != nil {
				path = tt.path(fixture)
			}
			request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(tt.body))
			response := httptest.NewRecorder()

			fixture.handler.HandleSaveLinkRoute(response, request)

			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.want, response.Body.String())
			}
			assertCanvasMapHandlerErrorEnvelope(t, response)
		})
	}
}

func TestCanvasMapHandlerSaveLinkRouteRejectsMissingRepositories(t *testing.T) {
	tests := []struct {
		name  string
		clear func(*CanvasMapHandler)
	}{
		{name: "map repository", clear: func(handler *CanvasMapHandler) { handler.mapRepo = nil }},
		{name: "canonical link repository", clear: func(handler *CanvasMapHandler) { handler.linkRepo = nil }},
		{name: "link route repository", clear: func(handler *CanvasMapHandler) { handler.linkRouteRepo = nil }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCanvasMapLinkRouteHandlerFixture(t)
			tt.clear(fixture.handler)
			request := httptest.NewRequest(
				http.MethodPut,
				"/api/v1/canvas/maps/"+fixture.mapID.String()+"/link-routes/"+fixture.linkID.String(),
				strings.NewReader(`{"version":1,"waypoints":[{"x":1,"y":2}]}`),
			)
			response := httptest.NewRecorder()

			fixture.handler.HandleSaveLinkRoute(response, request)

			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body=%s", response.Code, response.Body.String())
			}
			assertCanvasMapHandlerErrorEnvelope(t, response)
		})
	}
}

func TestCanvasMapHandlerDeleteLinkRoute(t *testing.T) {
	fixture := newCanvasMapLinkRouteHandlerFixture(t)
	fixture.routeRepo.routes[fixture.mapID] = map[uuid.UUID]domain.CanvasMapLinkRoute{
		fixture.linkID: {
			LinkID:    fixture.linkID,
			Version:   domain.CanvasMapLinkRouteVersion,
			Waypoints: []domain.CanvasPoint{{X: 120, Y: 80}},
		},
	}
	request := httptest.NewRequest(
		http.MethodDelete,
		"/api/v1/canvas/maps/"+fixture.mapID.String()+"/link-routes/"+fixture.linkID.String(),
		nil,
	)
	response := httptest.NewRecorder()

	fixture.handler.HandleDeleteLinkRoute(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", response.Code, response.Body.String())
	}
	if response.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", response.Body.String())
	}
	if _, ok := fixture.routeRepo.routes[fixture.mapID][fixture.linkID]; ok {
		t.Fatal("route still exists after delete")
	}
}

func TestCanvasMapHandlerDeleteLinkRouteValidatesIDsAndFailures(t *testing.T) {
	tests := []struct {
		name    string
		path    func(canvasMapLinkRouteHandlerFixture) string
		prepare func(canvasMapLinkRouteHandlerFixture)
		want    int
	}{
		{
			name: "invalid map UUID",
			path: func(f canvasMapLinkRouteHandlerFixture) string {
				return "/api/v1/canvas/maps/not-a-uuid/link-routes/" + f.linkID.String()
			},
			want: http.StatusNotFound,
		},
		{
			name: "invalid link UUID",
			path: func(f canvasMapLinkRouteHandlerFixture) string {
				return "/api/v1/canvas/maps/" + f.mapID.String() + "/link-routes/not-a-uuid"
			},
			want: http.StatusBadRequest,
		},
		{
			name: "missing map",
			prepare: func(f canvasMapLinkRouteHandlerFixture) {
				delete(f.mapRepo.maps, f.mapID)
			},
			want: http.StatusNotFound,
		},
		{
			name: "persistence failure",
			prepare: func(f canvasMapLinkRouteHandlerFixture) {
				f.routeRepo.deleteErr = errMock
			},
			want: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCanvasMapLinkRouteHandlerFixture(t)
			if tt.prepare != nil {
				tt.prepare(fixture)
			}
			path := "/api/v1/canvas/maps/" + fixture.mapID.String() + "/link-routes/" + fixture.linkID.String()
			if tt.path != nil {
				path = tt.path(fixture)
			}
			request := httptest.NewRequest(http.MethodDelete, path, nil)
			response := httptest.NewRecorder()

			fixture.handler.HandleDeleteLinkRoute(response, request)

			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.want, response.Body.String())
			}
			assertCanvasMapHandlerErrorEnvelope(t, response)
		})
	}
}

func TestCanvasMapHandlerTopologyVersionChangesForLinkRoute(t *testing.T) {
	fixture := newCanvasMapLinkRouteHandlerFixture(t)
	fixture.routeRepo.routes[fixture.mapID] = map[uuid.UUID]domain.CanvasMapLinkRoute{
		fixture.linkID: {
			LinkID:    fixture.linkID,
			Version:   domain.CanvasMapLinkRouteVersion,
			Waypoints: []domain.CanvasPoint{{X: 120, Y: 80}},
			UpdatedAt: time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC),
		},
	}
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/canvas/maps/"+fixture.mapID.String()+"/topology",
		nil,
	)
	firstRecorder := httptest.NewRecorder()

	first, ok := fixture.handler.buildMapTopologyResponse(firstRecorder, request)
	if !ok {
		t.Fatalf("first topology failed: status=%d body=%s", firstRecorder.Code, firstRecorder.Body.String())
	}
	firstRoute, ok := first.LinkRoutes[fixture.linkID.String()]
	if !ok {
		t.Fatalf("link_routes = %#v, want route %s", first.LinkRoutes, fixture.linkID)
	}
	if len(firstRoute.Waypoints) != 1 || firstRoute.Waypoints[0] != (domain.CanvasPoint{X: 120, Y: 80}) {
		t.Fatalf("first route = %#v, want initial waypoint", firstRoute)
	}

	fixture.routeRepo.routes[fixture.mapID][fixture.linkID] = domain.CanvasMapLinkRoute{
		LinkID:    fixture.linkID,
		Version:   domain.CanvasMapLinkRouteVersion,
		Waypoints: []domain.CanvasPoint{{X: 220, Y: 180}},
		UpdatedAt: time.Date(2026, 7, 21, 10, 1, 0, 0, time.UTC),
	}
	secondRecorder := httptest.NewRecorder()
	second, ok := fixture.handler.buildMapTopologyResponse(secondRecorder, request)
	if !ok {
		t.Fatalf("second topology failed: status=%d body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if first.TopologyVersion == second.TopologyVersion {
		t.Fatalf("topology version did not change for route update: %s", first.TopologyVersion)
	}
}

type canvasMapLinkRouteHandlerFixture struct {
	handler   *CanvasMapHandler
	mapID     uuid.UUID
	linkID    uuid.UUID
	mapRepo   *fakeCanvasMapHandlerMapRepo
	linkRepo  *mockLinkRepo
	routeRepo *fakeCanvasMapHandlerLinkRouteRepo
}

func newCanvasMapLinkRouteHandlerFixture(t *testing.T) canvasMapLinkRouteHandlerFixture {
	t.Helper()

	mapID := uuid.New()
	linkID := uuid.New()
	mapRepo := &fakeCanvasMapHandlerMapRepo{
		maps: map[uuid.UUID]domain.CanvasMap{
			mapID: {ID: mapID, Name: "Operations"},
		},
	}
	mapPositionRepo := &fakeCanvasMapHandlerPositionRepo{}
	canvasTopology, _, linkRepo, positionRepo, areaRepo := newTestCanvasTopologyHandler(t)
	if err := linkRepo.Create(&domain.Link{ID: linkID}); err != nil {
		t.Fatalf("seed canonical link: %v", err)
	}
	routeRepo := &fakeCanvasMapHandlerLinkRouteRepo{routes: make(map[uuid.UUID]map[uuid.UUID]domain.CanvasMapLinkRoute)}
	handler := NewCanvasMapHandler(
		mapRepo,
		mapPositionRepo,
		positionRepo,
		canvasTopology,
		canvasTopology.deviceService,
		linkRepo,
		routeRepo,
		areaRepo,
		nil,
	)
	return canvasMapLinkRouteHandlerFixture{
		handler:   handler,
		mapID:     mapID,
		linkID:    linkID,
		mapRepo:   mapRepo,
		linkRepo:  linkRepo,
		routeRepo: routeRepo,
	}
}

func mustCanvasLinkRoutePayload(t *testing.T, waypointCount int) string {
	t.Helper()
	waypoints := make([]domain.CanvasPoint, waypointCount)
	for i := range waypoints {
		waypoints[i] = domain.CanvasPoint{X: float64(i + 1), Y: float64(i + 2)}
	}
	payload, err := json.Marshal(map[string]interface{}{
		"version":   domain.CanvasMapLinkRouteVersion,
		"waypoints": waypoints,
	})
	if err != nil {
		t.Fatalf("marshal route payload: %v", err)
	}
	return string(payload)
}

func assertCanvasMapHandlerErrorEnvelope(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if strings.TrimSpace(body["error"]) == "" {
		t.Fatalf("error response = %#v, want non-empty error", body)
	}
}

// fakeCanvasMapHandlerMapRepo provides the CanvasMapRepository surface needed by handler tests.
type fakeCanvasMapHandlerMapRepo struct {
	maps       map[uuid.UUID]domain.CanvasMap
	membership domain.CanvasMapMembership
	getErr     error
	deleted    bool
}

// Create records no maps because delete tests do not exercise creation.
func (r *fakeCanvasMapHandlerMapRepo) Create(domain.CanvasMapCreate) (domain.CanvasMap, error) {
	return domain.CanvasMap{}, errMock
}

func (r *fakeCanvasMapHandlerMapRepo) GetByID(id uuid.UUID) (domain.CanvasMap, error) {
	if r.getErr != nil {
		return domain.CanvasMap{}, r.getErr
	}
	if canvasMap, ok := r.maps[id]; ok {
		return canvasMap, nil
	}
	return domain.CanvasMap{}, fmt.Errorf("canvas map not found: %s", id)
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

// fakeCanvasMapHandlerLinkRouteRepo records map-local route mutations and topology reads.
type fakeCanvasMapHandlerLinkRouteRepo struct {
	routes    map[uuid.UUID]map[uuid.UUID]domain.CanvasMapLinkRoute
	getAllErr error
	upsertErr error
	deleteErr error
}

func (r *fakeCanvasMapHandlerLinkRouteRepo) GetAllForMap(mapID uuid.UUID) ([]domain.CanvasMapLinkRoute, error) {
	if r.getAllErr != nil {
		return nil, r.getAllErr
	}
	stored := r.routes[mapID]
	routes := make([]domain.CanvasMapLinkRoute, 0, len(stored))
	for _, route := range stored {
		route.Waypoints = append([]domain.CanvasPoint(nil), route.Waypoints...)
		routes = append(routes, route)
	}
	return routes, nil
}

func (r *fakeCanvasMapHandlerLinkRouteRepo) UpsertForMap(
	mapID uuid.UUID,
	route domain.CanvasMapLinkRoute,
) (domain.CanvasMapLinkRoute, error) {
	if r.upsertErr != nil {
		return domain.CanvasMapLinkRoute{}, r.upsertErr
	}
	if r.routes == nil {
		r.routes = make(map[uuid.UUID]map[uuid.UUID]domain.CanvasMapLinkRoute)
	}
	if r.routes[mapID] == nil {
		r.routes[mapID] = make(map[uuid.UUID]domain.CanvasMapLinkRoute)
	}
	route.Waypoints = append([]domain.CanvasPoint(nil), route.Waypoints...)
	if route.UpdatedAt.IsZero() {
		route.UpdatedAt = time.Date(2026, 7, 21, 10, 0, 0, 0, time.UTC)
	}
	r.routes[mapID][route.LinkID] = route
	return route, nil
}

func (r *fakeCanvasMapHandlerLinkRouteRepo) DeleteForMap(mapID uuid.UUID, linkID uuid.UUID) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	delete(r.routes[mapID], linkID)
	return nil
}
