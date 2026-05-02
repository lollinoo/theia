package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/ws"
)

func newTestCanvasTopologyHandler(t *testing.T) (*CanvasTopologyHandler, *mockDeviceRepo, *mockLinkRepo, *mockPositionRepo, *mockAreaRepo) {
	t.Helper()

	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	positionRepo := newMockPositionRepo()
	areaRepo := newMockAreaRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}
	deviceService := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	return NewCanvasTopologyHandler(
		deviceService,
		linkRepo,
		positionRepo,
		areaRepo,
		buildTestVendorRegistry(),
	), deviceRepo, linkRepo, positionRepo, areaRepo
}

func TestCanvasTopologyHandlerHandleGet_ReturnsVersionedReadModel(t *testing.T) {
	handler, deviceRepo, linkRepo, positionRepo, areaRepo := newTestCanvasTopologyHandler(t)

	sourceID := uuid.New()
	targetID := uuid.New()
	areaID := seedAreaHelper(t, areaRepo, "Backbone", "#2979FF")
	now := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)

	if err := deviceRepo.Create(&domain.Device{
		ID:         sourceID,
		Hostname:   "router-01",
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-01",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
		AreaIDs:    []uuid.UUID{areaID},
		Interfaces: []domain.Interface{
			{IfName: "ether1", Speed: 1000000000, OperStatus: "up"},
		},
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed source device: %v", err)
	}
	if err := deviceRepo.Create(&domain.Device{
		ID:         targetID,
		Hostname:   "router-02",
		IP:         "10.0.0.2",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-02",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
		Interfaces: []domain.Interface{
			{IfName: "ether2", Speed: 100000000, OperStatus: "down"},
		},
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed target device: %v", err)
	}
	if err := linkRepo.Create(&domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    sourceID,
		SourceIfName:      "ether1",
		TargetDeviceID:    targetID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	positionRepo.positions = []domain.DevicePosition{
		{DeviceID: sourceID, X: 110, Y: 220, Pinned: true, UpdatedAt: now},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/topology/canvas", nil)
	rec := httptest.NewRecorder()

	handler.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("ETag") == "" {
		t.Fatal("expected ETag header")
	}

	var resp canvasTopologyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.SchemaVersion != 1 {
		t.Fatalf("expected schema_version=1, got %d", resp.SchemaVersion)
	}
	if resp.TopologyVersion == "" {
		t.Fatal("expected topology_version")
	}
	if resp.GeneratedAt == "" {
		t.Fatal("expected generated_at")
	}
	if len(resp.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(resp.Devices))
	}
	if len(resp.Links) != 1 {
		t.Fatalf("expected 1 link, got %d", len(resp.Links))
	}
	if resp.Links[0].SourceIfSpeed != 1000000000 {
		t.Fatalf("expected enriched source speed, got %d", resp.Links[0].SourceIfSpeed)
	}
	if resp.Links[0].TargetIfOperStatus != "down" {
		t.Fatalf("expected enriched target oper status, got %q", resp.Links[0].TargetIfOperStatus)
	}
	if _, ok := resp.Positions[sourceID.String()]; !ok {
		t.Fatalf("expected position keyed by %s", sourceID)
	}
	if len(resp.Areas) != 1 || resp.Areas[0].ID != areaID.String() {
		t.Fatalf("expected area %s, got %#v", areaID, resp.Areas)
	}
	if !resp.Capabilities.SupportsAreaFiltering || resp.Capabilities.SupportsPositionRevision {
		t.Fatalf("expected canvas capabilities, got %#v", resp.Capabilities)
	}
	if resp.Settings.Layout.Version != 1 {
		t.Fatalf("expected layout settings version 1, got %#v", resp.Settings)
	}
}

func TestCanvasTopologyHandlerHandleGet_ReturnsNotModifiedForMatchingETag(t *testing.T) {
	handler, deviceRepo, _, _, _ := newTestCanvasTopologyHandler(t)
	if err := deviceRepo.Create(&domain.Device{
		ID:         uuid.New(),
		Hostname:   "router-01",
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-01",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	firstReq := httptest.NewRequest(http.MethodGet, "/api/v1/topology/canvas", nil)
	firstRec := httptest.NewRecorder()
	handler.HandleGet(firstRec, firstReq)
	etag := firstRec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected initial ETag")
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/topology/canvas", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()

	handler.HandleGet(secondRec, secondReq)

	if secondRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d: %s", secondRec.Code, secondRec.Body.String())
	}
	if secondRec.Body.Len() != 0 {
		t.Fatalf("expected empty 304 body, got %q", secondRec.Body.String())
	}
}

func TestCanvasTopologyHandlerTopologyVersionIgnoresVolatileRuntimeFields(t *testing.T) {
	handler, deviceRepo, linkRepo, positionRepo, areaRepo := newTestCanvasTopologyHandler(t)

	sourceID := uuid.New()
	targetID := uuid.New()
	linkID := uuid.New()
	areaID := seedAreaHelper(t, areaRepo, "Backbone", "#2979FF")
	initialDiscoveryAt := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	initialPositionAt := time.Date(2026, 5, 1, 10, 5, 0, 0, time.UTC)

	source := &domain.Device{
		ID:         sourceID,
		Hostname:   "router-01",
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-01",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
		AreaIDs:    []uuid.UUID{areaID},
		Interfaces: []domain.Interface{
			{IfName: "ether1", Speed: 1000000000, OperStatus: "up"},
		},
		TopologyBootstrapState:      domain.TopologyBootstrapStatePending,
		LastTopologyDiscoveryAt:     &initialDiscoveryAt,
		LastTopologyDiscoveryResult: "scheduled",
	}
	if err := deviceRepo.Create(source); err != nil {
		t.Fatalf("seed source device: %v", err)
	}
	target := &domain.Device{
		ID:         targetID,
		Hostname:   "router-02",
		IP:         "10.0.0.2",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-02",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
		Interfaces: []domain.Interface{
			{IfName: "ether2", Speed: 100000000, OperStatus: "down"},
		},
	}
	if err := deviceRepo.Create(target); err != nil {
		t.Fatalf("seed target device: %v", err)
	}
	if err := linkRepo.Create(&domain.Link{
		ID:                linkID,
		SourceDeviceID:    sourceID,
		SourceIfName:      "ether1",
		TargetDeviceID:    targetID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("seed link: %v", err)
	}
	positionRepo.positions = []domain.DevicePosition{
		{DeviceID: sourceID, X: 110, Y: 220, Pinned: true, UpdatedAt: initialPositionAt},
	}

	first := handler.buildResponse(
		mustGetAllDevices(t, deviceRepo),
		mustGetAllLinks(t, linkRepo),
		positionRepo.positions,
		mustGetAllAreas(t, areaRepo),
	)

	updatedDiscoveryAt := initialDiscoveryAt.Add(5 * time.Minute)
	updatedSource, err := deviceRepo.GetByID(sourceID)
	if err != nil {
		t.Fatalf("load source device: %v", err)
	}
	updatedSource.Status = domain.DeviceStatusDown
	updatedSource.TopologyBootstrapState = domain.TopologyBootstrapStateCompleted
	updatedSource.LastTopologyDiscoveryAt = &updatedDiscoveryAt
	updatedSource.LastTopologyDiscoveryResult = "no new neighbors"
	updatedSource.Interfaces[0].OperStatus = "down"
	if err := deviceRepo.Update(updatedSource); err != nil {
		t.Fatalf("update source device: %v", err)
	}
	positionRepo.positions[0].UpdatedAt = initialPositionAt.Add(10 * time.Minute)

	second := handler.buildResponse(
		mustGetAllDevices(t, deviceRepo),
		mustGetAllLinks(t, linkRepo),
		positionRepo.positions,
		mustGetAllAreas(t, areaRepo),
	)

	if first.TopologyVersion != second.TopologyVersion {
		t.Fatalf("topology version changed for volatile runtime fields: first=%s second=%s", first.TopologyVersion, second.TopologyVersion)
	}
}

func TestCanvasTopologyHandlerHandleGetCanvas_ReturnsRuntimeBootstrap(t *testing.T) {
	handler, deviceRepo, _, _, _ := newTestCanvasTopologyHandler(t)
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID:         deviceID,
		Hostname:   "router-01",
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-01",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	runtimeSnapshot := ws.EmptySnapshot()
	runtimeSnapshot.Devices[deviceID.String()] = ws.DeviceRuntimeDTO{
		DeviceID:          deviceID.String(),
		OperationalStatus: "down",
		PrimaryHealth:     "unreachable",
		MetricsStatus:     "unavailable",
		MetricsReason:     "device_unreachable",
	}
	handler.runtimeSnapshotFunc = func() (*ws.SnapshotPayload, uint64) {
		return runtimeSnapshot, 42
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/canvas", nil)
	rec := httptest.NewRecorder()

	handler.HandleGetCanvas(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp canvasTopologyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RuntimeVersion == nil || *resp.RuntimeVersion != 42 {
		t.Fatalf("runtime_version = %#v, want 42", resp.RuntimeVersion)
	}
	if resp.RuntimeIdentity != ws.RuntimeIdentityForSnapshot(runtimeSnapshot) {
		t.Fatalf("runtime_identity = %q, want snapshot identity", resp.RuntimeIdentity)
	}
	if resp.RuntimeSnapshot == nil {
		t.Fatal("expected runtime_snapshot")
	}
	if got := resp.RuntimeSnapshot.Devices[deviceID.String()].OperationalStatus; got != "down" {
		t.Fatalf("runtime snapshot status = %q, want down", got)
	}
}

func TestCanvasTopologyHandlerHandleGetCanvas_DebugLogsCardinality(t *testing.T) {
	logs := captureAPIDebugLogs(t)
	handler, deviceRepo, _, positionRepo, _ := newTestCanvasTopologyHandler(t)
	deviceID := uuid.New()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	if err := deviceRepo.Create(&domain.Device{
		ID:         deviceID,
		Hostname:   "router-01",
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
		Status:     domain.DeviceStatusUp,
		SysName:    "router-01",
		Vendor:     "default",
		Managed:    true,
		Tags:       map[string]string{},
	}); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	positionRepo.positions = []domain.DevicePosition{
		{DeviceID: deviceID, X: 110, Y: 220, UpdatedAt: now},
	}

	runtimeSnapshot := ws.EmptySnapshot()
	runtimeSnapshot.Devices[deviceID.String()] = ws.DeviceRuntimeDTO{DeviceID: deviceID.String()}
	handler.runtimeSnapshotFunc = func() (*ws.SnapshotPayload, uint64) {
		return runtimeSnapshot, 42
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/canvas", nil)
	rec := httptest.NewRecorder()

	handler.HandleGetCanvas(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	output := logs.String()
	if !strings.Contains(output, "DEBUG canvas response endpoint=/api/v1/canvas status=200") {
		t.Fatalf("debug output missing canvas endpoint summary: %q", output)
	}
	for _, want := range []string{"devices=1", "links=0", "positions=1", "areas=0", "runtime_version=42", "runtime_devices=1", "runtime_links=0"} {
		if !strings.Contains(output, want) {
			t.Fatalf("debug output missing %s: %q", want, output)
		}
	}
}

func mustGetAllDevices(t *testing.T, repo *mockDeviceRepo) []domain.Device {
	t.Helper()
	devices, err := repo.GetAll()
	if err != nil {
		t.Fatalf("get all devices: %v", err)
	}
	return devices
}

func mustGetAllLinks(t *testing.T, repo *mockLinkRepo) []domain.Link {
	t.Helper()
	links, err := repo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	return links
}

func mustGetAllAreas(t *testing.T, repo *mockAreaRepo) []domain.AreaWithCount {
	t.Helper()
	areas, err := repo.GetAllWithDeviceCount()
	if err != nil {
		t.Fatalf("get all areas: %v", err)
	}
	return areas
}
