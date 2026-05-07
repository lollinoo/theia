package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/ws"
	_ "github.com/mattn/go-sqlite3"
)

type canvasMapIntegrationRouter struct {
	router          http.Handler
	db              *sql.DB
	deviceRepo      *sqlite.DeviceRepo
	linkRepo        *sqlite.LinkRepo
	positionRepo    *sqlite.PositionRepo
	mapRepo         *sqlite.CanvasMapRepo
	mapPositionRepo *sqlite.CanvasMapPositionRepo
	areaRepo        *sqlite.AreaRepo
}

type testCanvasMapResponse struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	SourceAreaID  *string                `json:"source_area_id"`
	Filter        domain.CanvasMapFilter `json:"filter"`
	IsDefault     bool                   `json:"is_default"`
	DeviceCount   int                    `json:"device_count"`
	LinkCount     int                    `json:"link_count"`
	PositionCount int                    `json:"position_count"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
}

func newCanvasMapIntegrationRouter(t *testing.T) canvasMapIntegrationRouter {
	t.Helper()

	dbName := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(t.Name())
	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=memory&cache=shared&_foreign_keys=on", dbName))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	sqlite.ConfigureSQLiteDB(db)
	t.Cleanup(func() { _ = db.Close() })

	encryptionKey := []byte("test-encryption-key-32-bytes!!!!")
	if err := sqlite.RunMigrations(db, encryptionKey); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}

	deviceRepo := sqlite.NewDeviceRepo(db, encryptionKey, nil)
	linkRepo := sqlite.NewLinkRepo(db, nil)
	positionRepo := sqlite.NewPositionRepo(db)
	settingsRepo := sqlite.NewSettingsRepo(db)
	areaRepo := sqlite.NewAreaRepo(db)
	snmpProfileRepo := sqlite.NewSNMPProfileRepo(db, encryptionKey)
	credentialProfileRepo := sqlite.NewCredentialProfileRepo(db)
	vendorConfigRepo := sqlite.NewVendorConfigRepo(db)
	mapRepo := sqlite.NewCanvasMapRepo(db)
	mapPositionRepo := sqlite.NewCanvasMapPositionRepo(db)

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}
	deviceService := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	runtimeSnapshotFunc := func() (*ws.SnapshotPayload, uint64) {
		return ws.EmptySnapshot(), 42
	}

	router := NewRouter(
		db,
		deviceService,
		linkRepo,
		positionRepo,
		mapRepo,
		mapPositionRepo,
		settingsRepo,
		snmpProfileRepo,
		credentialProfileRepo,
		areaRepo,
		nil,
		buildTestVendorRegistry(),
		vendorConfigRepo,
		nil,
		nil,
		func() {},
		"",
		runtimeSnapshotFunc,
		nil,
	)

	return canvasMapIntegrationRouter{
		router:          router,
		db:              db,
		deviceRepo:      deviceRepo,
		linkRepo:        linkRepo,
		positionRepo:    positionRepo,
		mapRepo:         mapRepo,
		mapPositionRepo: mapPositionRepo,
		areaRepo:        areaRepo,
	}
}

func TestCanvasMapHandlerCRUDRejectsDefaultDelete(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET maps: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp struct {
		Data []testCanvasMapResponse `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("expected one seeded default map, got %#v", listResp.Data)
	}
	defaultMap := listResp.Data[0]
	if !defaultMap.IsDefault || defaultMap.Name != "Default" {
		t.Fatalf("expected seeded default map, got %#v", defaultMap)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodDelete, "/api/v1/canvas/maps/"+defaultMap.ID, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("DELETE default map: expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCanvasMapHandlerCreatesDuplicatesAndUsesMapPositions(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", map[string]any{
		"name":        "Backbone",
		"description": "Backbone saved layout",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST map: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	created := decodeCanvasMapData(t, rec)
	if created.ID == "" || created.Name != "Backbone" || created.IsDefault {
		t.Fatalf("unexpected created map: %#v", created)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodPut, "/api/v1/canvas/maps/"+created.ID+"/positions", map[string]any{
		"positions": []any{},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT empty map positions: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps/"+created.ID+"/duplicate", map[string]any{
		"name": "Backbone Copy",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST duplicate: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	copyMap := decodeCanvasMapData(t, rec)
	if copyMap.ID == created.ID || copyMap.Name != "Backbone Copy" || copyMap.IsDefault {
		t.Fatalf("unexpected duplicate map: %#v, source %#v", copyMap, created)
	}
}

func TestCanvasMapHandlerMapScopedRoutesReturn404ForInvalidAndMissingMap(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "get invalid uuid", method: http.MethodGet, path: "/api/v1/canvas/maps/not-a-uuid"},
		{name: "patch invalid uuid", method: http.MethodPatch, path: "/api/v1/canvas/maps/not-a-uuid", body: map[string]any{"name": "x"}},
		{name: "delete invalid uuid", method: http.MethodDelete, path: "/api/v1/canvas/maps/not-a-uuid"},
		{name: "duplicate invalid uuid", method: http.MethodPost, path: "/api/v1/canvas/maps/not-a-uuid/duplicate", body: map[string]any{"name": "x"}},
		{name: "topology invalid uuid", method: http.MethodGet, path: "/api/v1/canvas/maps/not-a-uuid/topology"},
		{name: "bootstrap invalid uuid", method: http.MethodGet, path: "/api/v1/canvas/maps/not-a-uuid/bootstrap"},
		{name: "positions get invalid uuid", method: http.MethodGet, path: "/api/v1/canvas/maps/not-a-uuid/positions"},
		{name: "positions put invalid uuid", method: http.MethodPut, path: "/api/v1/canvas/maps/not-a-uuid/positions", body: map[string]any{"positions": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := canvasMapRequest(t, fixture.router, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
			}
		})
	}

	missingID := uuid.New().String()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{name: "get missing", method: http.MethodGet, path: "/api/v1/canvas/maps/" + missingID},
		{name: "topology missing", method: http.MethodGet, path: "/api/v1/canvas/maps/" + missingID + "/topology"},
		{name: "positions get missing", method: http.MethodGet, path: "/api/v1/canvas/maps/" + missingID + "/positions"},
		{name: "positions put missing", method: http.MethodPut, path: "/api/v1/canvas/maps/" + missingID + "/positions", body: map[string]any{"positions": []any{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := canvasMapRequest(t, fixture.router, tc.method, tc.path, tc.body)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCanvasMapHandlerRejectsUnknownSourceAreaOnCreateAndUpdate(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	unknownAreaID := uuid.New().String()

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", map[string]any{
		"name":           "Unknown Area",
		"source_area_id": unknownAreaID,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST unknown source_area_id: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}

	created := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Patch Target"})
	rec = canvasMapRequest(t, fixture.router, http.MethodPatch, "/api/v1/canvas/maps/"+created.ID, map[string]any{
		"source_area_id": unknownAreaID,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH unknown source_area_id: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCanvasMapHandlerPatchSourceAreaPersistsAndAffectsProjection(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Area A", "#2979FF")
	areaB := seedCanvasMapTestArea(t, fixture, "Area B", "#FF6D00")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-a", "10.65.0.1", []uuid.UUID{areaA})
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-b", "10.65.0.2", []uuid.UUID{areaB})
	seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)

	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Area Scoped"})
	rec := canvasMapRequest(t, fixture.router, http.MethodPatch, "/api/v1/canvas/maps/"+canvasMap.ID, map[string]any{
		"source_area_id": areaA.String(),
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH source_area_id: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	updated := decodeCanvasMapData(t, rec)
	if updated.SourceAreaID == nil || *updated.SourceAreaID != areaA.String() {
		t.Fatalf("PATCH response source_area_id = %#v, want %s", updated.SourceAreaID, areaA)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodPatch, "/api/v1/canvas/maps/"+canvasMap.ID, map[string]any{
		"description": "renamed without source area",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH without source_area_id: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	got := decodeCanvasMapData(t, rec)
	if got.SourceAreaID == nil || *got.SourceAreaID != areaA.String() {
		t.Fatalf("GET source_area_id = %#v, want %s", got.SourceAreaID, areaA)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET maps: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data []testCanvasMapResponse `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &listResp)
	listed, ok := findCanvasMapTestResponse(listResp.Data, canvasMap.ID)
	if !ok {
		t.Fatalf("map %s missing from list: %#v", canvasMap.ID, listResp.Data)
	}
	if listed.SourceAreaID == nil || *listed.SourceAreaID != areaA.String() {
		t.Fatalf("listed source_area_id = %#v, want %s", listed.SourceAreaID, areaA)
	}
	if listed.DeviceCount != 1 || listed.LinkCount != 0 {
		t.Fatalf("listed counts = devices:%d links:%d, want 1/0", listed.DeviceCount, listed.LinkCount)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if topology.Map == nil || topology.Map.SourceAreaID == nil || *topology.Map.SourceAreaID != areaA.String() {
		t.Fatalf("topology map source_area_id = %#v", topology.Map)
	}
	if len(topology.Devices) != 1 || topology.Devices[0].ID != deviceA.ID.String() {
		t.Fatalf("expected topology to include only %s, got %#v", deviceA.ID, topology.Devices)
	}
	if len(topology.Links) != 0 {
		t.Fatalf("expected area-scoped topology links to be filtered, got %#v", topology.Links)
	}
}

func TestCanvasMapHandlerPatchSourceAreaNullClearsExistingValue(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaID := seedCanvasMapTestArea(t, fixture, "Clearable", "#2979FF")
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name":           "Clear Source Area",
		"source_area_id": areaID.String(),
	})
	if canvasMap.SourceAreaID == nil || *canvasMap.SourceAreaID != areaID.String() {
		t.Fatalf("created source_area_id = %#v, want %s", canvasMap.SourceAreaID, areaID)
	}

	rec := canvasMapRawRequest(fixture.router, http.MethodPatch, "/api/v1/canvas/maps/"+canvasMap.ID, `{"source_area_id":null}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH source_area_id null: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	cleared := decodeCanvasMapData(t, rec)
	if cleared.SourceAreaID != nil {
		t.Fatalf("PATCH clear source_area_id = %#v, want nil", cleared.SourceAreaID)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	got := decodeCanvasMapData(t, rec)
	if got.SourceAreaID != nil {
		t.Fatalf("GET source_area_id = %#v, want nil", got.SourceAreaID)
	}
}

func TestCanvasMapHandlerSourceAreaUnexpectedLookupErrorReturns500(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	handler := NewCanvasMapHandler(
		fixture.mapRepo,
		fixture.mapPositionRepo,
		fixture.positionRepo,
		nil,
		nil,
		nil,
		errorAreaRepo{err: errMock},
		nil,
	)

	body := `{"name":"Broken Area Lookup","source_area_id":"` + uuid.New().String() + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/canvas/maps", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestCanvasMapHandlerFiltersPositionsToProjectedBaseDevices(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Position Area A", "#2979FF")
	areaB := seedCanvasMapTestArea(t, fixture, "Position Area B", "#FF6D00")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-pos-a", "10.66.0.1", []uuid.UUID{areaA})
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-pos-b", "10.66.0.2", []uuid.UUID{areaB})
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name":           "Projected Positions",
		"source_area_id": areaA.String(),
	})
	mapID := uuid.MustParse(canvasMap.ID)
	if err := fixture.mapPositionRepo.SaveAllForMap(mapID, []domain.DevicePosition{
		{DeviceID: deviceA.ID, X: 10, Y: 20, Pinned: true},
		{DeviceID: deviceB.ID, X: 30, Y: 40, Pinned: true},
	}); err != nil {
		t.Fatalf("save map positions: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET maps: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Data []testCanvasMapResponse `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &listResp)
	listed, ok := findCanvasMapTestResponse(listResp.Data, canvasMap.ID)
	if !ok {
		t.Fatalf("map %s missing from list: %#v", canvasMap.ID, listResp.Data)
	}
	if listed.PositionCount != 1 {
		t.Fatalf("listed position_count = %d, want 1", listed.PositionCount)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if topology.Map == nil || topology.Map.PositionCount != 1 {
		t.Fatalf("topology map position_count = %#v, want 1", topology.Map)
	}
	if len(topology.Positions) != 1 {
		t.Fatalf("topology positions = %#v, want one position", topology.Positions)
	}
	if _, ok := topology.Positions[deviceA.ID.String()]; !ok {
		t.Fatalf("expected position for projected device %s, got %#v", deviceA.ID, topology.Positions)
	}
	if _, ok := topology.Positions[deviceB.ID.String()]; ok {
		t.Fatalf("unexpected position for filtered device %s: %#v", deviceB.ID, topology.Positions)
	}
}

func TestCanvasMapHandlerTopologyIncludesGhostDevicesForCrossAreaLinks(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Ghost Area A", "#2979FF")
	areaB := seedCanvasMapTestArea(t, fixture, "Ghost Area B", "#FF6D00")
	baseDevice := seedCanvasMapTestDevice(t, fixture, "router-ghost-a", "10.67.0.1", []uuid.UUID{areaA})
	remoteDevice := seedCanvasMapTestDevice(t, fixture, "router-ghost-b", "10.67.0.2", []uuid.UUID{areaB})
	seedCanvasMapTestLink(t, fixture, baseDevice.ID, remoteDevice.ID)

	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name":           "Ghost Cross Area",
		"source_area_id": areaA.String(),
		"filter": map[string]any{
			"include_cross_area_links": true,
			"include_ghost_devices":    true,
		},
	})

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if topology.Map == nil {
		t.Fatal("expected map metadata")
	}
	if topology.Map.DeviceCount != 1 {
		t.Fatalf("map device_count = %d, want base count 1", topology.Map.DeviceCount)
	}
	if len(topology.Links) != 1 {
		t.Fatalf("links = %#v, want one cross-area link", topology.Links)
	}
	deviceIDs := canvasTopologyTestDeviceIDs(topology.Devices)
	if !deviceIDs[baseDevice.ID.String()] || !deviceIDs[remoteDevice.ID.String()] {
		t.Fatalf("expected base and ghost devices in response, got %#v", deviceIDs)
	}
}

func TestCanvasMapHandlerTopologyETagChangesForMapMetadataPatch(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	device := seedCanvasMapTestDevice(t, fixture, "router-etag", "10.68.0.1", nil)
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name": "Initial Metadata",
		"filter": map[string]any{
			"device_ids": []string{device.ID.String()},
		},
	})

	first := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("initial topology: expected 200, got %d; body: %s", first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected initial ETag")
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodPatch, "/api/v1/canvas/maps/"+canvasMap.ID, map[string]any{
		"name":        "Updated Metadata",
		"description": "metadata-only update",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH metadata: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	req.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	fixture.router.ServeHTTP(second, req)
	if second.Code != http.StatusOK {
		t.Fatalf("topology after metadata PATCH with stale ETag: expected 200, got %d; body: %s", second.Code, second.Body.String())
	}
	if second.Header().Get("ETag") == "" || second.Header().Get("ETag") == etag {
		t.Fatalf("expected changed ETag, got initial=%q next=%q", etag, second.Header().Get("ETag"))
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, second, &topology)
	if topology.Map == nil || topology.Map.Name != "Updated Metadata" || topology.Map.Description != "metadata-only update" {
		t.Fatalf("expected updated map metadata, got %#v", topology.Map)
	}
}

func TestCanvasMapHandlerListComputesCountsFromProjection(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-a", "10.60.0.1", nil)
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-b", "10.60.0.2", nil)
	seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)

	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	if err := fixture.mapPositionRepo.SaveAllForMap(defaultMap.ID, []domain.DevicePosition{
		{DeviceID: deviceA.ID, X: 10, Y: 20, Pinned: true},
	}); err != nil {
		t.Fatalf("save default map positions: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET maps: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var listResp struct {
		Data []testCanvasMapResponse `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("expected one default map, got %#v", listResp.Data)
	}
	got := listResp.Data[0]
	if got.DeviceCount != 2 || got.LinkCount != 1 || got.PositionCount != 1 {
		t.Fatalf("counts = devices:%d links:%d positions:%d, want 2/1/1", got.DeviceCount, got.LinkCount, got.PositionCount)
	}
}

func TestCanvasMapHandlerTopologyUsesMapProjectionFilter(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-a", "10.61.0.1", nil)
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-b", "10.61.0.2", nil)
	seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)

	filteredMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name": "Single Device",
		"filter": map[string]any{
			"device_ids": []string{deviceA.ID.String()},
		},
	})

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+filteredMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &resp)
	if resp.Map == nil {
		t.Fatal("expected map metadata on topology response")
	}
	if resp.Map.DeviceCount != 1 || resp.Map.LinkCount != 0 {
		t.Fatalf("map counts = devices:%d links:%d, want 1/0", resp.Map.DeviceCount, resp.Map.LinkCount)
	}
	if len(resp.Devices) != 1 || resp.Devices[0].ID != deviceA.ID.String() {
		t.Fatalf("expected only device %s, got %#v", deviceA.ID, resp.Devices)
	}
	if len(resp.Links) != 0 {
		t.Fatalf("expected filtered links, got %#v", resp.Links)
	}
}

func TestCanvasMapHandlerSavePositionsRejectsUnknownDevicesAndInvalidCoordinates(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Positions"})

	rec := canvasMapRequest(t, fixture.router, http.MethodPut, "/api/v1/canvas/maps/"+canvasMap.ID+"/positions", map[string]any{
		"positions": []map[string]any{
			{"device_id": uuid.New().String(), "x": 10, "y": 20},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown device position: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}

	device := seedCanvasMapTestDevice(t, fixture, "router-a", "10.62.0.1", nil)
	body := `{"positions":[{"device_id":"` + device.ID.String() + `","x":1e999,"y":20}]}`
	rec = canvasMapRawRequest(fixture.router, http.MethodPut, "/api/v1/canvas/maps/"+canvasMap.ID+"/positions", body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-finite coordinate: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPositionHandlerLegacySaveWritesDefaultMapAndLegacyPositions(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	device := seedCanvasMapTestDevice(t, fixture, "router-a", "10.63.0.1", nil)

	rec := canvasMapRequest(t, fixture.router, http.MethodPut, "/api/v1/positions", map[string]any{
		"positions": []map[string]any{
			{"device_id": device.ID.String(), "x": 100, "y": 200, "pinned": true},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT legacy positions: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	legacyPositions, err := fixture.positionRepo.GetAll()
	if err != nil {
		t.Fatalf("get legacy positions: %v", err)
	}
	if len(legacyPositions) != 1 || legacyPositions[0].DeviceID != device.ID || legacyPositions[0].X != 100 {
		t.Fatalf("unexpected legacy positions: %#v", legacyPositions)
	}

	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	mapPositions, err := fixture.mapPositionRepo.GetAllForMap(defaultMap.ID)
	if err != nil {
		t.Fatalf("get default map positions: %v", err)
	}
	if len(mapPositions) != 1 || mapPositions[0].DeviceID != device.ID || mapPositions[0].X != 100 {
		t.Fatalf("unexpected default map positions: %#v", mapPositions)
	}
}

func TestPositionHandlerLegacyListReadsDefaultMapPositionsWhenConfigured(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	device := seedCanvasMapTestDevice(t, fixture, "router-a", "10.64.0.1", nil)
	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	if err := fixture.positionRepo.SaveAll([]domain.DevicePosition{
		{DeviceID: device.ID, X: 1, Y: 2},
	}); err != nil {
		t.Fatalf("save legacy position: %v", err)
	}
	if err := fixture.mapPositionRepo.SaveAllForMap(defaultMap.ID, []domain.DevicePosition{
		{DeviceID: device.ID, X: 300, Y: 400, Pinned: true},
	}); err != nil {
		t.Fatalf("save default map position: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/positions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET legacy positions: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []domain.DevicePosition `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &resp)
	if len(resp.Data) != 1 || resp.Data[0].DeviceID != device.ID || resp.Data[0].X != 300 || !resp.Data[0].Pinned {
		t.Fatalf("expected default map positions, got %#v", resp.Data)
	}
}

func mustCreateCanvasMapForTest(t *testing.T, fixture canvasMapIntegrationRouter, payload map[string]any) testCanvasMapResponse {
	t.Helper()
	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", payload)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create map: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	return decodeCanvasMapData(t, rec)
}

func seedCanvasMapTestDevice(t *testing.T, fixture canvasMapIntegrationRouter, hostname string, ip string, areaIDs []uuid.UUID) domain.Device {
	t.Helper()
	device := &domain.Device{
		ID:            uuid.New(),
		Hostname:      hostname,
		IP:            ip,
		DeviceType:    domain.DeviceTypeRouter,
		Status:        domain.DeviceStatusUp,
		SysName:       hostname,
		Vendor:        "default",
		Managed:       true,
		Tags:          map[string]string{},
		AreaIDs:       areaIDs,
		MetricsSource: domain.MetricsSourceNone,
		Interfaces: []domain.Interface{
			{IfName: "ether1", Speed: 1000000000, OperStatus: "up"},
		},
	}
	if err := fixture.deviceRepo.Create(device); err != nil {
		t.Fatalf("seed device %s: %v", hostname, err)
	}
	return *device
}

func seedCanvasMapTestArea(t *testing.T, fixture canvasMapIntegrationRouter, name string, color string) uuid.UUID {
	t.Helper()
	area := &domain.Area{
		ID:          uuid.New(),
		Name:        name,
		Description: name + " test area",
		Color:       color,
	}
	if err := fixture.areaRepo.Create(area); err != nil {
		t.Fatalf("seed area %s: %v", name, err)
	}
	return area.ID
}

func seedCanvasMapTestLink(t *testing.T, fixture canvasMapIntegrationRouter, sourceID uuid.UUID, targetID uuid.UUID) domain.Link {
	t.Helper()
	link := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    sourceID,
		SourceIfName:      "ether1",
		TargetDeviceID:    targetID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := fixture.linkRepo.Create(link); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	return *link
}

func findCanvasMapTestResponse(maps []testCanvasMapResponse, id string) (testCanvasMapResponse, bool) {
	for _, canvasMap := range maps {
		if canvasMap.ID == id {
			return canvasMap, true
		}
	}
	return testCanvasMapResponse{}, false
}

func canvasTopologyTestDeviceIDs(devices []jsonAPIResource) map[string]bool {
	ids := make(map[string]bool, len(devices))
	for _, device := range devices {
		ids[device.ID] = true
	}
	return ids
}

func canvasMapRequest(t *testing.T, router http.Handler, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		return canvasMapRawRequest(router, method, path, "")
	}
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	return canvasMapRawRequest(router, method, path, string(payload))
}

func canvasMapRawRequest(router http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeCanvasMapData(t *testing.T, rec *httptest.ResponseRecorder) testCanvasMapResponse {
	t.Helper()
	var resp struct {
		Data testCanvasMapResponse `json:"data"`
	}
	decodeCanvasMapTestResponse(t, rec, &resp)
	return resp.Data
}

func decodeCanvasMapTestResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		t.Fatalf("decode response body %q: %v", rec.Body.String(), err)
	}
}

type errorAreaRepo struct {
	err error
}

func (r errorAreaRepo) Create(area *domain.Area) error {
	return r.err
}

func (r errorAreaRepo) GetByID(id uuid.UUID) (*domain.Area, error) {
	return nil, r.err
}

func (r errorAreaRepo) GetAll() ([]domain.Area, error) {
	return nil, r.err
}

func (r errorAreaRepo) GetAllWithDeviceCount() ([]domain.AreaWithCount, error) {
	return nil, r.err
}

func (r errorAreaRepo) Update(area *domain.Area) error {
	return r.err
}

func (r errorAreaRepo) Delete(id uuid.UUID) error {
	return r.err
}
