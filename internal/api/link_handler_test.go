package api

// This file exercises link handler behavior so refactors preserve the documented contract.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
)

// newTestLinkHandler builds a LinkHandler backed by mock repos.
// It also returns the mock repos so tests can seed data.
func newTestLinkHandler(t *testing.T) (*LinkHandler, *mockLinkRepo, *mockDeviceRepo) {
	t.Helper()
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}
	svc := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	handler := NewLinkHandler(linkRepo, svc)
	return handler, linkRepo, deviceRepo
}

func TestLinkHandlerList(t *testing.T) {
	handler, linkRepo, _ := newTestLinkHandler(t)

	// Seed a link
	linkRepo.Create(&domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    uuid.New(),
		SourceIfName:      "ether1",
		TargetDeviceID:    uuid.New(),
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

func TestLinkHandlerCreate_HappyPath(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)

	// Create two devices that the link handler will validate
	srcDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.1", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	deviceRepo.Create(srcDevice)
	deviceRepo.Create(tgtDevice)

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether2"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerCreate_DuplicateManualLinkIsIdempotent(t *testing.T) {
	handler, linkRepo, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether2"
	}`

	firstReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	firstRec := httptest.NewRecorder()
	handler.HandleCreate(firstRec, firstReq)
	if firstRec.Code != http.StatusCreated {
		t.Fatalf("expected first request status 201, got %d; body: %s", firstRec.Code, firstRec.Body.String())
	}
	firstLink := decodeLinkCreateResponse(t, firstRec)

	secondReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	secondRec := httptest.NewRecorder()
	handler.HandleCreate(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second request status 200, got %d; body: %s", secondRec.Code, secondRec.Body.String())
	}
	secondLink := decodeLinkCreateResponse(t, secondRec)

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 stored link, got %d", len(links))
	}
	if secondLink.ID != firstLink.ID {
		t.Fatalf("expected duplicate response ID %s, got %s", firstLink.ID, secondLink.ID)
	}
}

func TestLinkHandlerCreate_DuplicateManualPreservesDiscoveredSameDirection(t *testing.T) {
	handler, linkRepo, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}
	existing := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    srcDevice.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    tgtDevice.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("seed discovered link: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether2"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for duplicate discovered link, got %d; body: %s", rec.Code, rec.Body.String())
	}
	respLink := decodeLinkCreateResponse(t, rec)
	if respLink.ID != existing.ID {
		t.Fatalf("expected response ID %s, got %s", existing.ID, respLink.ID)
	}
	if respLink.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected response to preserve LLDP protocol, got %q", respLink.DiscoveryProtocol)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 stored link, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected stored link to preserve LLDP protocol, got %q", links[0].DiscoveryProtocol)
	}
}

func TestLinkHandlerCreate_DuplicateManualPreservesDiscoveredReverseDirection(t *testing.T) {
	handler, linkRepo, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}
	existing := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    tgtDevice.ID,
		SourceIfName:      "ether2",
		TargetDeviceID:    srcDevice.ID,
		TargetIfName:      "ether1",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("seed discovered link: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether2"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for reverse duplicate discovered link, got %d; body: %s", rec.Code, rec.Body.String())
	}
	respLink := decodeLinkCreateResponse(t, rec)
	if respLink.ID != existing.ID {
		t.Fatalf("expected response ID %s, got %s", existing.ID, respLink.ID)
	}
	if respLink.SourceDeviceID != existing.SourceDeviceID || respLink.SourceIfName != existing.SourceIfName ||
		respLink.TargetDeviceID != existing.TargetDeviceID || respLink.TargetIfName != existing.TargetIfName {
		t.Fatalf("expected response to use stored orientation %+v, got %+v", *existing, respLink)
	}
	if respLink.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected response to preserve LLDP protocol, got %q", respLink.DiscoveryProtocol)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 stored link, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected stored link to preserve LLDP protocol, got %q", links[0].DiscoveryProtocol)
	}
}

func TestLinkHandlerCreate_BrowserLocalStorageMigrationUsesExistingUnorderedDevicePair(t *testing.T) {
	handler, linkRepo, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}
	existing := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    srcDevice.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    tgtDevice.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	if err := linkRepo.Create(existing); err != nil {
		t.Fatalf("seed discovered link: %v", err)
	}

	body := `{
		"source_device_id":"` + tgtDevice.ID.String() + `",
		"source_if_name":"",
		"target_device_id":"` + srcDevice.ID.String() + `",
		"target_if_name":"",
		"migration_source":"browser_localstorage"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for localStorage duplicate device pair, got %d; body: %s", rec.Code, rec.Body.String())
	}
	respLink := decodeLinkCreateResponse(t, rec)
	if respLink.ID != existing.ID {
		t.Fatalf("expected response ID %s, got %s", existing.ID, respLink.ID)
	}
	if respLink.SourceIfName != existing.SourceIfName || respLink.TargetIfName != existing.TargetIfName {
		t.Fatalf("expected response to use stored interface names %q/%q, got %q/%q",
			existing.SourceIfName, existing.TargetIfName, respLink.SourceIfName, respLink.TargetIfName)
	}
	if respLink.DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected response to preserve LLDP protocol, got %q", respLink.DiscoveryProtocol)
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 stored link, got %d", len(links))
	}
	if links[0].SourceIfName == "" || links[0].TargetIfName == "" {
		t.Fatalf("expected no empty-interface duplicate, stored link: %+v", links[0])
	}
}

func TestLinkHandlerCreate_UnknownMigrationSourceRejected(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether2",
		"migration_source":"unknown"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown migration source, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerCreate_BrowserLocalStorageMigrationRejectsInterfaceNames(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"",
		"migration_source":"browser_localstorage"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for browser_localstorage with interface names, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func decodeLinkCreateResponse(t *testing.T, rec *httptest.ResponseRecorder) domain.Link {
	t.Helper()
	var resp struct {
		Data domain.Link `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode create link response: %v", err)
	}
	return resp.Data
}

func TestLinkHandlerCreate_MalformedJSON(t *testing.T) {
	handler, _, _ := newTestLinkHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLinkHandlerCreate_MissingFields(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)

	srcDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.1", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	deviceRepo.Create(srcDevice)

	// Missing target_device_id
	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"",
		"target_if_name":"ether2"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerDelete_HappyPath(t *testing.T) {
	handler, linkRepo, _ := newTestLinkHandler(t)

	link := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    uuid.New(),
		SourceIfName:      "ether1",
		TargetDeviceID:    uuid.New(),
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolManual,
	}
	linkRepo.Create(link)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/links/"+link.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerDelete_NotFound(t *testing.T) {
	handler, _, _ := newTestLinkHandler(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/links/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// --- Virtual link validation tests (D-12, D-13) ---

// seedVirtualDevice inserts a virtual device into the mock repo.
func seedVirtualDevice(t *testing.T, repo *mockDeviceRepo, name string) *domain.Device {
	t.Helper()
	d := &domain.Device{
		ID:         uuid.New(),
		IP:         "",
		Hostname:   "",
		DeviceType: domain.DeviceTypeVirtual,
		Managed:    true,
		Status:     domain.DeviceStatusUnknown,
		Tags:       map[string]string{"display_name": name, "virtual_subtype": "internet"},
	}
	if err := repo.Create(d); err != nil {
		t.Fatalf("seedVirtualDevice: %v", err)
	}
	return d
}

func TestLinkHandlerCreate_VirtualSourceEmptyIfName(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedVirtualDevice(t, deviceRepo, "Internet")
	tgtDevice := seedDevice(t, deviceRepo)

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether1"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerCreate_VirtualTargetEmptyIfName(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := seedVirtualDevice(t, deviceRepo, "Cloud")

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":""
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestLinkHandlerCreate_BothVirtualRejected(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedVirtualDevice(t, deviceRepo, "Internet")
	tgtDevice := seedVirtualDevice(t, deviceRepo, "Cloud")

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":""
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "at least one device must be non-virtual") {
		t.Errorf("expected 'at least one device must be non-virtual' error, got: %s", rec.Body.String())
	}
}

func TestLinkHandlerCreate_BothPhysicalRequiresBothIfNames(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	deviceRepo.Create(tgtDevice)

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether1"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "source_if_name is required") {
		t.Errorf("expected 'source_if_name is required' error, got: %s", rec.Body.String())
	}
}

func TestLinkHandlerCreate_BrowserLocalStorageMigrationAllowsEmptyPhysicalInterfaces(t *testing.T) {
	handler, linkRepo, deviceRepo := newTestLinkHandler(t)
	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	if err := deviceRepo.Create(tgtDevice); err != nil {
		t.Fatalf("seed target device: %v", err)
	}

	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"",
		"migration_source":"browser_localstorage"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("get all links: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected exactly 1 stored link, got %d", len(links))
	}
	if links[0].SourceIfName != "" {
		t.Fatalf("expected empty source interface name, got %q", links[0].SourceIfName)
	}
	if links[0].TargetIfName != "" {
		t.Fatalf("expected empty target interface name, got %q", links[0].TargetIfName)
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolManual {
		t.Fatalf("expected manual discovery protocol, got %q", links[0].DiscoveryProtocol)
	}
}

// =============================================================================
// D-08: Link interface name length validation
// =============================================================================

func TestLinkCreate_SourceIfNameTooLong_400(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)

	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	deviceRepo.Create(tgtDevice)

	longName := strings.Repeat("e", 256)
	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"` + longName + `",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"ether1"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for source_if_name > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "source_if_name too long") {
		t.Errorf("expected error about source_if_name length, got: %s", rec.Body.String())
	}
}

func TestLinkCreate_TargetIfNameTooLong_400(t *testing.T) {
	handler, _, deviceRepo := newTestLinkHandler(t)

	srcDevice := seedDevice(t, deviceRepo)
	tgtDevice := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
		Tags: map[string]string{},
	}
	deviceRepo.Create(tgtDevice)

	longName := strings.Repeat("e", 256)
	body := `{
		"source_device_id":"` + srcDevice.ID.String() + `",
		"source_if_name":"ether1",
		"target_device_id":"` + tgtDevice.ID.String() + `",
		"target_if_name":"` + longName + `"
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for target_if_name > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "target_if_name too long") {
		t.Errorf("expected error about target_if_name length, got: %s", rec.Body.String())
	}
}
