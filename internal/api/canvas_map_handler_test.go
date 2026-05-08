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

func TestCanvasMapHandlerCreateMaterializesAreaMembershipOnce(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Materialized Area A", "#2979FF")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-materialized-a", "10.69.0.1", []uuid.UUID{areaA})

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", map[string]any{
		"name":           "Materialized Area Map",
		"source_area_id": areaA.String(),
		"filter": map[string]any{
			"area_id": areaA.String(),
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST map from area: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	canvasMap := decodeCanvasMapData(t, rec)

	seedCanvasMapTestDevice(t, fixture, "router-materialized-late", "10.69.0.2", []uuid.UUID{areaA})

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Devices) != 1 || topology.Devices[0].ID != deviceA.ID.String() {
		t.Fatalf("expected only initially materialized device %s, got %#v", deviceA.ID, topology.Devices)
	}
}

func TestCanvasMapHandlerCreateBlankMapDoesNotAutoSyncFutureDevices(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Empty Snapshot"})

	seedCanvasMapTestDevice(t, fixture, "router-created-after-map", "10.69.1.1", nil)

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Devices) != 0 {
		t.Fatalf("expected empty materialized map to stay empty, got %#v", topology.Devices)
	}
}

func TestCanvasMapHandlerCreateBlankMapDoesNotImportExistingGlobalTopology(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-existing-a", "10.69.1.10", nil)
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-existing-b", "10.69.1.11", nil)
	seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)
	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("load default map: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(defaultMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA.ID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: deviceB.ID, Role: domain.CanvasMapDeviceRoleBase},
		},
	}); err != nil {
		t.Fatalf("seed default map membership: %v", err)
	}
	if err := fixture.mapPositionRepo.SaveAllForMap(defaultMap.ID, []domain.DevicePosition{
		{DeviceID: deviceA.ID, X: 100, Y: 200, Pinned: true},
		{DeviceID: deviceB.ID, X: 300, Y: 400, Pinned: true},
	}); err != nil {
		t.Fatalf("seed default map positions: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", map[string]any{
		"name":           "Blank Map",
		"source_area_id": nil,
		"filter":         map[string]any{},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST blank map: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	canvasMap := decodeCanvasMapData(t, rec)
	if canvasMap.DeviceCount != 0 || canvasMap.LinkCount != 0 || canvasMap.PositionCount != 0 {
		t.Fatalf("created blank map counts = devices:%d links:%d positions:%d, want all zero", canvasMap.DeviceCount, canvasMap.LinkCount, canvasMap.PositionCount)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET blank map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Devices) != 0 || len(topology.Links) != 0 || len(topology.Positions) != 0 {
		t.Fatalf("blank map imported topology: devices=%#v links=%#v positions=%#v", topology.Devices, topology.Links, topology.Positions)
	}
	if topology.Map == nil {
		t.Fatal("blank map topology response omitted map metadata")
	}
	if topology.Map.DeviceCount != 0 || topology.Map.LinkCount != 0 || topology.Map.PositionCount != 0 {
		t.Fatalf("blank topology map counts = devices:%d links:%d positions:%d, want all zero", topology.Map.DeviceCount, topology.Map.LinkCount, topology.Map.PositionCount)
	}
}

func TestCanvasMapHandlerCreateMapFromSourceMapAreaCopiesScopedMembershipAndPositions(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaID := seedCanvasMapTestArea(t, fixture, "Map Area", "#2979FF")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-source-area-a", "10.73.0.1", []uuid.UUID{areaID})
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-source-area-b", "10.73.0.2", []uuid.UUID{areaID})
	deviceOutsideSource := seedCanvasMapTestDevice(t, fixture, "router-outside-source", "10.73.0.3", []uuid.UUID{areaID})
	linkAB := seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)
	seedCanvasMapTestLink(t, fixture, deviceB.ID, deviceOutsideSource.ID)

	sourceMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Source Map"})
	sourceMapID := uuid.MustParse(sourceMap.ID)
	if err := fixture.mapRepo.ReplaceMembership(sourceMapID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA.ID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaID}},
			{DeviceID: deviceB.ID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaID}},
		},
		LinkIDs: []uuid.UUID{linkAB.ID},
		Areas: []domain.CanvasMapAreaMembership{
			{
				AreaID:      areaID,
				Name:        "Map Area",
				Description: "Map Area test area",
				Color:       "#2979FF",
			},
		},
	}); err != nil {
		t.Fatalf("replace source membership: %v", err)
	}
	if err := fixture.mapPositionRepo.SaveAllForMap(sourceMapID, []domain.DevicePosition{
		{DeviceID: deviceA.ID, X: 120, Y: 240, Pinned: true},
		{DeviceID: deviceB.ID, X: 360, Y: 480, Pinned: true},
	}); err != nil {
		t.Fatalf("save source positions: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps", map[string]any{
		"name":           "Source Area Copy",
		"source_area_id": areaID.String(),
		"source_map_id":  sourceMap.ID,
		"filter": map[string]any{
			"area_id":                  areaID.String(),
			"include_cross_area_links": true,
			"include_ghost_devices":    true,
		},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST source-map area copy: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	created := decodeCanvasMapData(t, rec)

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+created.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET source-map area copy topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	gotDeviceIDs := make(map[string]struct{}, len(topology.Devices))
	for _, device := range topology.Devices {
		gotDeviceIDs[device.ID] = struct{}{}
	}
	for _, wantID := range []string{deviceA.ID.String(), deviceB.ID.String()} {
		if _, ok := gotDeviceIDs[wantID]; !ok {
			t.Fatalf("missing source-map area device %s in %#v", wantID, topology.Devices)
		}
	}
	if _, ok := gotDeviceIDs[deviceOutsideSource.ID.String()]; ok {
		t.Fatalf("copied device outside source map: %#v", topology.Devices)
	}
	if len(topology.Links) != 1 || topology.Links[0].ID != linkAB.ID.String() {
		t.Fatalf("copied links = %#v, want only source-map link %s", topology.Links, linkAB.ID)
	}
	if len(topology.Areas) != 1 || topology.Areas[0].ID != areaID.String() {
		t.Fatalf("copied areas = %#v, want source-map area %s", topology.Areas, areaID)
	}
	if len(topology.Positions) != 2 {
		t.Fatalf("copied positions = %#v, want source positions for two devices", topology.Positions)
	}
	if topology.Positions[deviceA.ID.String()].X != 120 || topology.Positions[deviceB.ID.String()].Y != 480 {
		t.Fatalf("copied positions = %#v, want source coordinates", topology.Positions)
	}
}

func TestCanvasMapHandlerDuplicateBackfillsMissingAreasAndKeepsMembershipIndependent(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaID := seedCanvasMapTestArea(t, fixture, "Default Area", "#00E676")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-default-a", "10.74.0.1", []uuid.UUID{areaID})
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-default-b", "10.74.0.2", []uuid.UUID{areaID})
	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("load default map: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(defaultMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA.ID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: deviceB.ID, Role: domain.CanvasMapDeviceRoleBase},
		},
	}); err != nil {
		t.Fatalf("seed default map membership without area snapshots: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodPost, "/api/v1/canvas/maps/"+defaultMap.ID.String()+"/duplicate", map[string]any{
		"name": "Default Copy",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST duplicate default map: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
	duplicated := decodeCanvasMapData(t, rec)

	rec = canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+duplicated.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET duplicated map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Areas) != 1 || topology.Areas[0].ID != areaID.String() {
		t.Fatalf("duplicated map areas = %#v, want inferred area %s", topology.Areas, areaID)
	}

	rec = canvasMapRequest(t, fixture.router, http.MethodDelete, "/api/v1/canvas/maps/"+duplicated.ID+"/devices/"+deviceA.ID.String(), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE duplicated map device: expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
	sourceMembership, err := fixture.mapRepo.GetMembership(defaultMap.ID)
	if err != nil {
		t.Fatalf("get default membership: %v", err)
	}
	if len(sourceMembership.Devices) != 2 {
		t.Fatalf("default membership changed after duplicate edit: %#v", sourceMembership.Devices)
	}
	copyMembership, err := fixture.mapRepo.GetMembership(uuid.MustParse(duplicated.ID))
	if err != nil {
		t.Fatalf("get copy membership: %v", err)
	}
	if len(copyMembership.Devices) != 1 || copyMembership.Devices[0].DeviceID != deviceB.ID {
		t.Fatalf("copy membership after local delete = %#v, want only %s", copyMembership.Devices, deviceB.ID)
	}
}

func TestCanvasMapHandlerBulkUpdateMapDeviceAreasDoesNotMutateSourceMapOrGlobalDevice(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Original Area", "#2979FF")
	areaB := seedCanvasMapTestArea(t, fixture, "Duplicated Map Area", "#FF6D00")
	device := seedCanvasMapTestDevice(t, fixture, "router-bulk-area", "10.75.0.1", []uuid.UUID{areaA})
	sourceMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Source Bulk Map"})
	copyMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Copied Bulk Map"})

	initial := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: device.ID, Role: domain.CanvasMapDeviceRoleBase, AreaIDs: []uuid.UUID{areaA}},
		},
		Areas: []domain.CanvasMapAreaMembership{
			{AreaID: areaA, Name: "Original Area", Description: "Original Area test area", Color: "#2979FF"},
			{AreaID: areaB, Name: "Duplicated Map Area", Description: "Duplicated Map Area test area", Color: "#FF6D00"},
		},
	}
	if err := fixture.mapRepo.ReplaceMembership(uuid.MustParse(sourceMap.ID), initial); err != nil {
		t.Fatalf("replace source membership: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(uuid.MustParse(copyMap.ID), initial); err != nil {
		t.Fatalf("replace copy membership: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodPut, "/api/v1/canvas/maps/"+copyMap.ID+"/device-areas", map[string]any{
		"device_ids": []string{device.ID.String()},
		"area_ids":   []string{areaB.String()},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT map device areas: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	copyTopologyRec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+copyMap.ID+"/topology", nil)
	if copyTopologyRec.Code != http.StatusOK {
		t.Fatalf("GET copy map topology: expected 200, got %d; body: %s", copyTopologyRec.Code, copyTopologyRec.Body.String())
	}
	var copyTopology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, copyTopologyRec, &copyTopology)
	assertCanvasTopologyDeviceAreaIDs(t, copyTopology, device.ID.String(), []string{areaB.String()})

	sourceTopologyRec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+sourceMap.ID+"/topology", nil)
	if sourceTopologyRec.Code != http.StatusOK {
		t.Fatalf("GET source map topology: expected 200, got %d; body: %s", sourceTopologyRec.Code, sourceTopologyRec.Body.String())
	}
	var sourceTopology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, sourceTopologyRec, &sourceTopology)
	assertCanvasTopologyDeviceAreaIDs(t, sourceTopology, device.ID.String(), []string{areaA.String()})

	globalDevice, err := fixture.deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("get global device: %v", err)
	}
	if len(globalDevice.AreaIDs) != 1 || globalDevice.AreaIDs[0] != areaA {
		t.Fatalf("global device area_ids = %#v, want only %s", globalDevice.AreaIDs, areaA)
	}
}

func TestCanvasMapHandlerUnmaterializedMapDoesNotUseLiveFilterFallback(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	device := seedCanvasMapTestDevice(t, fixture, "router-no-fallback", "10.69.1.2", nil)
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name": "Unmaterialized Legacy",
		"filter": map[string]any{
			"device_ids": []string{device.ID.String()},
		},
	})
	if _, err := fixture.db.Exec(
		`UPDATE canvas_maps
		 SET membership_materialized = 0
		 WHERE id = ?`,
		canvasMap.ID,
	); err != nil {
		t.Fatalf("force unmaterialized map: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Devices) != 0 {
		t.Fatalf("expected unmaterialized map to avoid live filter fallback, got %#v", topology.Devices)
	}
}

func TestCanvasMapHandlerCreateDeviceSubsetMaterializesOnlyMemberAreas(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaA := seedCanvasMapTestArea(t, fixture, "Subset Area A", "#2979FF")
	areaB := seedCanvasMapTestArea(t, fixture, "Subset Area B", "#FF6D00")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-subset-a", "10.69.2.1", []uuid.UUID{areaA})
	seedCanvasMapTestDevice(t, fixture, "router-subset-b", "10.69.2.2", []uuid.UUID{areaB})

	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name": "Device Subset Areas",
		"filter": map[string]any{
			"device_ids": []string{deviceA.ID.String()},
		},
	})

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Areas) != 1 || topology.Areas[0].ID != areaA.String() {
		t.Fatalf("expected only member area %s, got %#v", areaA, topology.Areas)
	}
}

func TestCanvasMapHandlerTopologyUsesMaterializedMembership(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-filtered-out", "10.70.0.1", nil)
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-member", "10.70.0.2", nil)
	canvasMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{
		"name": "Membership Wins",
		"filter": map[string]any{
			"device_ids": []string{deviceA.ID.String()},
		},
	})
	mapID := uuid.MustParse(canvasMap.ID)
	if err := fixture.mapRepo.ReplaceMembership(mapID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: deviceB.ID, Role: domain.CanvasMapDeviceRoleBase}},
	}); err != nil {
		t.Fatalf("replace membership: %v", err)
	}

	rec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+canvasMap.ID+"/topology", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET map topology: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, rec, &topology)
	if len(topology.Devices) != 1 || topology.Devices[0].ID != deviceB.ID.String() {
		t.Fatalf("expected materialized member %s, got %#v", deviceB.ID, topology.Devices)
	}
}

func TestCanvasMapHandlerRemoveDeviceFromMapDoesNotDeleteGlobalDeviceOrOtherMaps(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	device := seedCanvasMapTestDevice(t, fixture, "router-map-local-remove", "10.71.0.1", nil)
	firstMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "First Map"})
	secondMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Second Map"})
	for _, canvasMap := range []testCanvasMapResponse{firstMap, secondMap} {
		if err := fixture.mapRepo.ReplaceMembership(uuid.MustParse(canvasMap.ID), domain.CanvasMapMembership{
			Devices: []domain.CanvasMapDeviceMembership{{DeviceID: device.ID, Role: domain.CanvasMapDeviceRoleBase}},
		}); err != nil {
			t.Fatalf("replace membership for %s: %v", canvasMap.ID, err)
		}
	}

	rec := canvasMapRequest(
		t,
		fixture.router,
		http.MethodDelete,
		"/api/v1/canvas/maps/"+firstMap.ID+"/devices/"+device.ID.String(),
		nil,
	)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE map device: expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}

	if _, err := fixture.deviceRepo.GetByID(device.ID); err != nil {
		t.Fatalf("global device was deleted: %v", err)
	}
	firstMembership, err := fixture.mapRepo.GetMembership(uuid.MustParse(firstMap.ID))
	if err != nil {
		t.Fatalf("get first membership: %v", err)
	}
	if len(firstMembership.Devices) != 0 {
		t.Fatalf("first map devices = %#v, want empty", firstMembership.Devices)
	}
	secondMembership, err := fixture.mapRepo.GetMembership(uuid.MustParse(secondMap.ID))
	if err != nil {
		t.Fatalf("get second membership: %v", err)
	}
	if len(secondMembership.Devices) != 1 || secondMembership.Devices[0].DeviceID != device.ID {
		t.Fatalf("second map devices = %#v, want device %s", secondMembership.Devices, device.ID)
	}
}

func TestCanvasMapHandlerAddDeviceToMapAddsLocalMembershipWithoutTouchingOtherMaps(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	areaID := seedCanvasMapTestArea(t, fixture, "Backbone", "#00AEEF")
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-map-add-a", "10.72.0.1", []uuid.UUID{areaID})
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-map-add-b", "10.72.0.2", []uuid.UUID{areaID})
	deviceC := seedCanvasMapTestDevice(t, fixture, "router-map-add-c", "10.72.0.3", nil)
	linkAB := seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)
	seedCanvasMapTestLink(t, fixture, deviceB.ID, deviceC.ID)

	firstMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "First Add Map"})
	secondMap := mustCreateCanvasMapForTest(t, fixture, map[string]any{"name": "Second Add Map"})
	initial := domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: deviceA.ID, Role: domain.CanvasMapDeviceRoleBase}},
	}
	if err := fixture.mapRepo.ReplaceMembership(uuid.MustParse(firstMap.ID), initial); err != nil {
		t.Fatalf("replace first membership: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(uuid.MustParse(secondMap.ID), initial); err != nil {
		t.Fatalf("replace second membership: %v", err)
	}

	rec := canvasMapRequest(
		t,
		fixture.router,
		http.MethodPost,
		"/api/v1/canvas/maps/"+firstMap.ID+"/devices/"+deviceB.ID.String(),
		map[string]any{"include_connected_links": true},
	)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST map device: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	added := decodeCanvasMapData(t, rec)
	if added.DeviceCount != 2 || added.LinkCount != 1 {
		t.Fatalf("updated map counts = devices:%d links:%d, want 2/1", added.DeviceCount, added.LinkCount)
	}

	firstMembership, err := fixture.mapRepo.GetMembership(uuid.MustParse(firstMap.ID))
	if err != nil {
		t.Fatalf("get first membership: %v", err)
	}
	if len(firstMembership.Devices) != 2 {
		t.Fatalf("first map devices = %#v, want 2 base devices", firstMembership.Devices)
	}
	if len(firstMembership.LinkIDs) != 1 || firstMembership.LinkIDs[0] != linkAB.ID {
		t.Fatalf("first map links = %#v, want only %s", firstMembership.LinkIDs, linkAB.ID)
	}
	if len(firstMembership.Areas) != 1 || firstMembership.Areas[0].AreaID != areaID || firstMembership.Areas[0].Name != "Backbone" {
		t.Fatalf("first map areas = %#v, want map-local Backbone area", firstMembership.Areas)
	}

	secondMembership, err := fixture.mapRepo.GetMembership(uuid.MustParse(secondMap.ID))
	if err != nil {
		t.Fatalf("get second membership: %v", err)
	}
	if len(secondMembership.Devices) != 1 || secondMembership.Devices[0].DeviceID != deviceA.ID {
		t.Fatalf("second map devices = %#v, want original device only", secondMembership.Devices)
	}

	topologyRec := canvasMapRequest(t, fixture.router, http.MethodGet, "/api/v1/canvas/maps/"+firstMap.ID+"/topology", nil)
	if topologyRec.Code != http.StatusOK {
		t.Fatalf("GET first map topology: expected 200, got %d; body: %s", topologyRec.Code, topologyRec.Body.String())
	}
	var topology canvasTopologyResponse
	decodeCanvasMapTestResponse(t, topologyRec, &topology)
	if len(topology.Devices) != 2 || len(topology.Links) != 1 || topology.Links[0].ID != linkAB.ID.String() {
		t.Fatalf("topology devices/links = %d/%#v, want two devices and link %s", len(topology.Devices), topology.Links, linkAB.ID)
	}
	if _, err := fixture.deviceRepo.GetByID(deviceB.ID); err != nil {
		t.Fatalf("global device missing after map add: %v", err)
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

func TestCanvasMapHandlerListComputesCountsFromMaterializedMembership(t *testing.T) {
	fixture := newCanvasMapIntegrationRouter(t)
	deviceA := seedCanvasMapTestDevice(t, fixture, "router-a", "10.60.0.1", nil)
	deviceB := seedCanvasMapTestDevice(t, fixture, "router-b", "10.60.0.2", nil)
	link := seedCanvasMapTestLink(t, fixture, deviceA.ID, deviceB.ID)

	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(defaultMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{
			{DeviceID: deviceA.ID, Role: domain.CanvasMapDeviceRoleBase},
			{DeviceID: deviceB.ID, Role: domain.CanvasMapDeviceRoleBase},
		},
		LinkIDs: []uuid.UUID{link.ID},
	}); err != nil {
		t.Fatalf("replace default map membership: %v", err)
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
	defaultMap, err := fixture.mapRepo.GetDefault()
	if err != nil {
		t.Fatalf("get default map: %v", err)
	}
	if err := fixture.mapRepo.ReplaceMembership(defaultMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: device.ID, Role: domain.CanvasMapDeviceRoleBase}},
	}); err != nil {
		t.Fatalf("replace default map membership: %v", err)
	}

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
	if err := fixture.mapRepo.ReplaceMembership(defaultMap.ID, domain.CanvasMapMembership{
		Devices: []domain.CanvasMapDeviceMembership{{DeviceID: device.ID, Role: domain.CanvasMapDeviceRoleBase}},
	}); err != nil {
		t.Fatalf("replace default map membership: %v", err)
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

func assertCanvasTopologyDeviceAreaIDs(t *testing.T, topology canvasTopologyResponse, deviceID string, want []string) {
	t.Helper()
	for _, device := range topology.Devices {
		if device.ID != deviceID {
			continue
		}
		raw, ok := device.Attributes["area_ids"].([]interface{})
		if !ok {
			if areaIDs, ok := device.Attributes["area_ids"].([]string); ok {
				if !stringSlicesEqual(areaIDs, want) {
					t.Fatalf("device %s area_ids = %#v, want %#v", deviceID, areaIDs, want)
				}
				return
			}
			t.Fatalf("device %s area_ids missing or invalid: %#v", deviceID, device.Attributes["area_ids"])
		}
		got := make([]string, 0, len(raw))
		for _, value := range raw {
			areaID, ok := value.(string)
			if !ok {
				t.Fatalf("device %s area_ids contains non-string value %#v", deviceID, value)
			}
			got = append(got, areaID)
		}
		if !stringSlicesEqual(got, want) {
			t.Fatalf("device %s area_ids = %#v, want %#v", deviceID, got, want)
		}
		return
	}
	t.Fatalf("device %s not found in topology: %#v", deviceID, topology.Devices)
}

func stringSlicesEqual(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
