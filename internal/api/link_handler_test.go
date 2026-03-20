package api

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

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}
	svc := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)
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
