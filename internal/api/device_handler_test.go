package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/vendor"
)

// --- Mock repositories for DeviceHandler tests ---

type mockDeviceRepo struct {
	mu      sync.Mutex
	devices map[uuid.UUID]*domain.Device
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{devices: make(map[uuid.UUID]*domain.Device)}
}

func (r *mockDeviceRepo) Create(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if device.ID == uuid.Nil {
		device.ID = uuid.New()
	}
	now := time.Now().UTC()
	device.CreatedAt = now
	device.UpdatedAt = now
	r.devices[device.ID] = device
	return nil
}

func (r *mockDeviceRepo) GetByID(id uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[id]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", id)
	}
	cp := *d
	return &cp, nil
}

func (r *mockDeviceRepo) GetByIP(ip string) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.IP == ip {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) GetAll() ([]domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Device
	for _, d := range r.devices {
		result = append(result, *d)
	}
	return result, nil
}

func (r *mockDeviceRepo) Update(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[device.ID]; !ok {
		return fmt.Errorf("device not found: %s", device.ID)
	}
	device.UpdatedAt = time.Now().UTC()
	r.devices[device.ID] = device
	return nil
}

func (r *mockDeviceRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[id]; !ok {
		return fmt.Errorf("device not found: %s", id)
	}
	delete(r.devices, id)
	return nil
}

func (r *mockDeviceRepo) GetBySysName(sysName string) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.devices {
		if d.SysName == sysName {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

// --- Mock LinkRepo ---

type mockLinkRepo struct {
	mu    sync.Mutex
	links map[uuid.UUID]*domain.Link
}

func newMockLinkRepo() *mockLinkRepo {
	return &mockLinkRepo{links: make(map[uuid.UUID]*domain.Link)}
}

func (r *mockLinkRepo) Create(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return nil
}

func (r *mockLinkRepo) GetByID(id uuid.UUID) (*domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[id]
	if !ok {
		return nil, fmt.Errorf("link not found: %s", id)
	}
	cp := *l
	return &cp, nil
}

func (r *mockLinkRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Link
	for _, l := range r.links {
		if l.SourceDeviceID == deviceID || l.TargetDeviceID == deviceID {
			result = append(result, *l)
		}
	}
	return result, nil
}

func (r *mockLinkRepo) GetAll() ([]domain.Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Link
	for _, l := range r.links {
		result = append(result, *l)
	}
	return result, nil
}

func (r *mockLinkRepo) Update(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[link.ID]; !ok {
		return fmt.Errorf("link not found: %s", link.ID)
	}
	link.UpdatedAt = time.Now().UTC()
	r.links[link.ID] = link
	return nil
}

func (r *mockLinkRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.links[id]; !ok {
		return fmt.Errorf("link not found: %s", id)
	}
	delete(r.links, id)
	return nil
}

func (r *mockLinkRepo) Upsert(link *domain.Link) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.links {
		if existing.SourceDeviceID == link.SourceDeviceID &&
			existing.SourceIfName == link.SourceIfName &&
			existing.TargetDeviceID == link.TargetDeviceID &&
			existing.TargetIfName == link.TargetIfName {
			link.ID = id
			link.UpdatedAt = time.Now().UTC()
			r.links[id] = link
			return nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return nil
}

// --- Mock SettingsRepo ---

type mockSettingsRepo struct {
	settings map[string]string
}

func newMockSettingsRepo() *mockSettingsRepo {
	return &mockSettingsRepo{settings: domain.DefaultSettings()}
}

func (r *mockSettingsRepo) Get(key string) (string, error) {
	v, ok := r.settings[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return v, nil
}

func (r *mockSettingsRepo) Set(key, value string) error {
	r.settings[key] = value
	return nil
}

func (r *mockSettingsRepo) GetAll() (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range r.settings {
		cp[k] = v
	}
	return cp, nil
}

// --- Mock CredentialProfileRepo ---

type mockCredentialProfileRepo struct {
	mu       sync.Mutex
	profiles map[uuid.UUID]*domain.CredentialProfile
}

func newMockCredentialProfileRepo() *mockCredentialProfileRepo {
	return &mockCredentialProfileRepo{profiles: make(map[uuid.UUID]*domain.CredentialProfile)}
}

func (r *mockCredentialProfileRepo) Create(profile *domain.CredentialProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	r.profiles[profile.ID] = profile
	return nil
}

func (r *mockCredentialProfileRepo) GetByID(id uuid.UUID) (*domain.CredentialProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.profiles[id]
	if !ok {
		return nil, fmt.Errorf("credential profile not found: %s", id)
	}
	cp := *p
	return &cp, nil
}

func (r *mockCredentialProfileRepo) GetAll() ([]domain.CredentialProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.CredentialProfile
	for _, p := range r.profiles {
		result = append(result, *p)
	}
	return result, nil
}

func (r *mockCredentialProfileRepo) Update(profile *domain.CredentialProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[profile.ID]; !ok {
		return fmt.Errorf("credential profile not found: %s", profile.ID)
	}
	profile.UpdatedAt = time.Now().UTC()
	r.profiles[profile.ID] = profile
	return nil
}

func (r *mockCredentialProfileRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[id]; !ok {
		return fmt.Errorf("credential profile not found: %s", id)
	}
	delete(r.profiles, id)
	return nil
}

func (r *mockCredentialProfileRepo) GetBackupProfileForDevice(deviceID uuid.UUID) (*domain.CredentialProfile, error) {
	return nil, nil
}

// --- Helper to build test VendorRegistry ---

func buildTestVendorRegistry() *vendor.Registry {
	defaultCfg := vendor.DBVendorRecord{
		Name: "default",
		ConfigJSON: `{
			"vendor": {"name": "default", "display_name": "Generic"},
			"detection": {},
			"backup": {"supported": false}
		}`,
	}
	reg, err := vendor.LoadRegistryFromDB([]vendor.DBVendorRecord{defaultCfg})
	if err != nil {
		panic(fmt.Sprintf("buildTestVendorRegistry: %v", err))
	}
	return reg
}

// --- Helper: newTestDeviceHandler builds a DeviceHandler with mock deps ---

func newTestDeviceHandler(t *testing.T) (*DeviceHandler, *mockDeviceRepo, *mockLinkRepo) {
	t.Helper()
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}

	svc := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)
	registry := buildTestVendorRegistry()
	handler := NewDeviceHandler(svc, credentialProfileRepo, registry)

	return handler, deviceRepo, linkRepo
}

// seedDevice inserts a device into the mock repo and returns it.
func seedDevice(t *testing.T, repo *mockDeviceRepo) *domain.Device {
	t.Helper()
	d := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.1",
		Hostname: "test-router",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(d); err != nil {
		t.Fatalf("seedDevice: %v", err)
	}
	return d
}

// --- DeviceHandler tests ---

func TestDeviceHandlerList(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)

	// Seed a device
	seedDevice(t, deviceRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp jsonAPIList
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) < 1 {
		t.Fatalf("expected at least 1 device in list, got %d", len(resp.Data))
	}
}

func TestDeviceHandlerCreate_HappyPath(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.0.0.1","hostname":"test-router","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.Type != "device" {
		t.Fatalf("expected resource type 'device', got %q", resp.Data.Type)
	}
}

func TestDeviceHandlerCreate_MalformedJSON(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(`{invalid`))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeviceHandlerCreate_MissingIP(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"hostname":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeviceHandlerGet_HappyPath(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	d := seedDevice(t, deviceRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+d.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data.ID != d.ID.String() {
		t.Fatalf("expected device ID %s, got %s", d.ID, resp.Data.ID)
	}
}

func TestDeviceHandlerGet_NotFound(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGet(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeviceHandlerDelete_HappyPath(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	d := seedDevice(t, deviceRepo)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/"+d.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}

	// Verify device is gone
	_, err := deviceRepo.GetByID(d.ID)
	if err == nil {
		t.Fatal("expected device to be deleted")
	}
}

func TestDeviceHandlerDelete_NotFound(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDelete(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeviceHandlerUpdate_AreaID(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	d := seedDevice(t, deviceRepo)

	areaID := uuid.New().String()
	body := fmt.Sprintf(`{"area_ids":["%s"]}`, areaID)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+d.ID.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	// Verify area_ids is in response
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatal("expected data object in response")
	}
	attrs, ok := data["attributes"].(map[string]interface{})
	if !ok {
		t.Fatal("expected attributes in data")
	}
	gotAreaIDs, ok := attrs["area_ids"].([]interface{})
	if !ok {
		t.Fatal("expected area_ids array in attributes")
	}
	if len(gotAreaIDs) != 1 {
		t.Fatalf("expected 1 area_id, got %d", len(gotAreaIDs))
	}
	if gotAreaIDs[0] != areaID {
		t.Errorf("area_ids[0] = %q, want %q", gotAreaIDs[0], areaID)
	}
}

// --- Virtual device tests (D-08, D-09, D-10 regression protection) ---

func TestDeviceHandlerCreate_VirtualHappyPath(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual","tags":{"display_name":"Internet","virtual_subtype":"internet"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp.Data.Attributes["device_type"]; got != "virtual" {
		t.Errorf("expected device_type 'virtual', got %q", got)
	}
}

func TestDeviceHandlerCreate_VirtualWithIP(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual","ip":"10.0.0.99","tags":{"display_name":"Cloud GW","virtual_subtype":"cloud"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp.Data.Attributes["ip"]; got != "10.0.0.99" {
		t.Errorf("expected ip '10.0.0.99', got %q", got)
	}
	if got := resp.Data.Attributes["device_type"]; got != "virtual" {
		t.Errorf("expected device_type 'virtual', got %q", got)
	}
}

func TestDeviceHandlerCreate_VirtualMissingDisplayName(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual","tags":{"virtual_subtype":"internet"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "display_name is required") {
		t.Errorf("expected error about display_name, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_VirtualInvalidSubtype(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual","tags":{"display_name":"Test","virtual_subtype":"invalid"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "virtual_subtype must be one of") {
		t.Errorf("expected error about virtual_subtype, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_VirtualMissingSubtype(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual","tags":{"display_name":"Test"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "virtual_subtype must be one of") {
		t.Errorf("expected error about virtual_subtype, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_VirtualNoTags(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"device_type":"virtual"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "display_name is required") {
		t.Errorf("expected error about display_name, got: %s", rec.Body.String())
	}
}

// TestDeviceHandlerCreate_RegularStillRequiresIP confirms D-10: no regression for regular devices.
func TestDeviceHandlerCreate_RegularStillRequiresIP(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"hostname":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ip is required") {
		t.Errorf("expected error about ip, got: %s", rec.Body.String())
	}
}

// Silence the unused import for context -- it is needed for domain import indirectly.
var _ = context.Background

// --- writeError tests ---

func TestWriteError_500_ReturnsGenericWithRef(t *testing.T) {
	rec := httptest.NewRecorder()
	realErr := errors.New("database connection failed: secret details")
	writeError(rec, http.StatusInternalServerError, "unused message", realErr)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "internal error, ref:") {
		t.Errorf("expected body to contain 'internal error, ref:', got: %s", body)
	}
	if strings.Contains(body, "database connection failed") {
		t.Errorf("expected body NOT to contain real error message, got: %s", body)
	}
	if strings.Contains(body, "secret details") {
		t.Errorf("expected body NOT to contain sensitive details, got: %s", body)
	}
}

func TestWriteError_400_ReturnsExactMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "ip is required")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["error"] != "ip is required" {
		t.Errorf("expected error 'ip is required', got %q", resp["error"])
	}
}

func TestWriteError_500_NoInternalErr(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusInternalServerError, "unused message")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "internal error, ref:") {
		t.Errorf("expected body to contain 'internal error, ref:', got: %s", body)
	}
}

// --- isValidIPOrHostname tests ---

func TestIsValidIPOrHostname(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"192.168.1.1", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"router1.example.com", true},
		{"a-b.c-d.example.com", true},
		{"localhost", true},
		{"", false},
		{"invalid hostname!", false},
		{strings.Repeat("a", 254), false},
		{"-invalid.com", false},
		{"invalid-.com", false},
		// purely numeric labels must be rejected (not valid hostnames)
		{"12345", false},
		{"999", false},
		{"0", false},
		{"123.456", false},
		// mixed alphanumeric single-label with letter must be accepted
		{"router1", true},
		{"1e100", true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := isValidIPOrHostname(tc.input)
			if got != tc.want {
				t.Errorf("isValidIPOrHostname(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

// --- sanitizeFilename tests ---

func TestSanitizeFilename(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"normal.tar.gz", "normal.tar.gz"},
		{"file\r\nname.tar.gz", "filename.tar.gz"},
		{"file\"name\";secret.tar.gz", "filenamesecret.tar.gz"},
		{"file\tname.tar.gz", "filename.tar.gz"},
		{"backup-2024-01-01.tar.gz", "backup-2024-01-01.tar.gz"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeFilename(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// =============================================================================
// D-01 / D-04: IP validation in HandleCreate
// =============================================================================

func TestHandleCreate_InvalidIP_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"not-valid!","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid IP, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "valid IP address or hostname") {
		t.Errorf("expected error about valid IP/hostname, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_HostnameTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	longHostname := strings.Repeat("a", 254)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","hostname":"%s","snmp":{"version":"2c","community":"public"}}`, longHostname)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for hostname > 253 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "hostname too long") {
		t.Errorf("expected error about hostname length, got: %s", rec.Body.String())
	}
}

// =============================================================================
// D-02: device_type and metrics_source allowlist validation
// =============================================================================

func TestHandleCreate_InvalidDeviceType_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.0.0.1","device_type":"refrigerator","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid device_type, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid device_type") {
		t.Errorf("expected error about device_type, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_InvalidMetricsSource_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.0.0.1","metrics_source":"magic","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid metrics_source, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid metrics_source") {
		t.Errorf("expected error about metrics_source, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_ValidDeviceTypes_Accept(t *testing.T) {
	validTypes := []string{"router", "switch", "access_point", "firewall", "unknown"}
	for _, dt := range validTypes {
		t.Run(dt, func(t *testing.T) {
			handler, _, _ := newTestDeviceHandler(t)
			body := fmt.Sprintf(`{"ip":"10.0.0.1","device_type":"%s","snmp":{"version":"2c","community":"public"}}`, dt)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.HandleCreate(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("expected 201 for device_type=%q, got %d; body: %s", dt, rec.Code, rec.Body.String())
			}
		})
	}
}

// =============================================================================
// D-01 / D-03: String length validation in HandleCreate
// =============================================================================

func TestHandleCreate_VendorTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	vendor := strings.Repeat("v", 256)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","vendor":"%s","snmp":{"version":"2c","community":"public"}}`, vendor)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for vendor > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "vendor too long") {
		t.Errorf("expected error about vendor length, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_PrometheusLabelNameTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	label := strings.Repeat("l", 256)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","prometheus_label_name":"%s","snmp":{"version":"2c","community":"public"}}`, label)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for prometheus_label_name > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "prometheus_label_name too long") {
		t.Errorf("expected error about prometheus_label_name length, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_PrometheusLabelValueTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	val := strings.Repeat("v", 256)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","prometheus_label_value":"%s","snmp":{"version":"2c","community":"public"}}`, val)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for prometheus_label_value > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "prometheus_label_value too long") {
		t.Errorf("expected error about prometheus_label_value length, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_TagValueTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	val := strings.Repeat("x", 256)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","snmp":{"version":"2c","community":"public"},"tags":{"env":"%s"}}`, val)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tag value > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tag value too long") {
		t.Errorf("expected error about tag value length, got: %s", rec.Body.String())
	}
}

func TestHandleCreate_TagKeyTooLong_400(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	key := strings.Repeat("k", 256)
	body := fmt.Sprintf(`{"ip":"10.0.0.1","snmp":{"version":"2c","community":"public"},"tags":{"%s":"value"}}`, key)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for tag key > 255 chars, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "tag key too long") {
		t.Errorf("expected error about tag key length, got: %s", rec.Body.String())
	}
}

// =============================================================================
// D-06: SNMP v3 allowlist validation via parseSNMPCreds
// =============================================================================

func TestParseSNMPCreds_V3_InvalidAuthProtocol(t *testing.T) {
	req := snmpCredsRequest{
		Version:       "3",
		Username:      "admin",
		AuthProtocol:  "MD4",
		SecurityLevel: "authPriv",
	}
	_, err := parseSNMPCreds(req)
	if err == nil {
		t.Fatal("expected error for invalid auth_protocol MD4, got nil")
	}
	if !strings.Contains(err.Error(), "auth_protocol") {
		t.Errorf("expected error message to mention auth_protocol, got: %s", err.Error())
	}
}

func TestParseSNMPCreds_V3_ValidAuthProtocol(t *testing.T) {
	validProtos := []string{"MD5", "SHA", "SHA-224", "SHA-256", "SHA-384", "SHA-512"}
	for _, proto := range validProtos {
		t.Run(proto, func(t *testing.T) {
			req := snmpCredsRequest{
				Version:      "3",
				Username:     "admin",
				AuthProtocol: proto,
			}
			_, err := parseSNMPCreds(req)
			if err != nil {
				t.Fatalf("expected no error for auth_protocol=%q, got: %v", proto, err)
			}
		})
	}
}

func TestParseSNMPCreds_V3_InvalidPrivProtocol(t *testing.T) {
	req := snmpCredsRequest{
		Version:      "3",
		Username:     "admin",
		PrivProtocol: "3DES",
	}
	_, err := parseSNMPCreds(req)
	if err == nil {
		t.Fatal("expected error for invalid priv_protocol 3DES, got nil")
	}
	if !strings.Contains(err.Error(), "priv_protocol") {
		t.Errorf("expected error message to mention priv_protocol, got: %s", err.Error())
	}
}

func TestParseSNMPCreds_V3_ValidPrivProtocol(t *testing.T) {
	validProtos := []string{"DES", "AES"}
	for _, proto := range validProtos {
		t.Run(proto, func(t *testing.T) {
			req := snmpCredsRequest{
				Version:      "3",
				Username:     "admin",
				PrivProtocol: proto,
			}
			_, err := parseSNMPCreds(req)
			if err != nil {
				t.Fatalf("expected no error for priv_protocol=%q, got: %v", proto, err)
			}
		})
	}
}

func TestParseSNMPCreds_V3_InvalidSecurityLevel(t *testing.T) {
	req := snmpCredsRequest{
		Version:       "3",
		Username:      "admin",
		SecurityLevel: "superAuth",
	}
	_, err := parseSNMPCreds(req)
	if err == nil {
		t.Fatal("expected error for invalid security_level superAuth, got nil")
	}
	if !strings.Contains(err.Error(), "security_level") {
		t.Errorf("expected error message to mention security_level, got: %s", err.Error())
	}
}

func TestParseSNMPCreds_V3_ValidSecurityLevel(t *testing.T) {
	validLevels := []string{"noAuthNoPriv", "authNoPriv", "authPriv"}
	for _, level := range validLevels {
		t.Run(level, func(t *testing.T) {
			req := snmpCredsRequest{
				Version:       "3",
				Username:      "admin",
				SecurityLevel: level,
			}
			_, err := parseSNMPCreds(req)
			if err != nil {
				t.Fatalf("expected no error for security_level=%q, got: %v", level, err)
			}
		})
	}
}

// =============================================================================
// D-10: HandleBatchAdd returns failures array with per-device IP and error reason
// =============================================================================

func TestHandleBatchAdd_InvalidIP_ReturnsFailures(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"devices":[{"ip":"not-valid!","snmp":{"version":"2c","community":"public"}},{"ip":"10.0.0.1","snmp":{"version":"2c","community":"public"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBatchAdd(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	failures, ok := resp["failures"]
	if !ok {
		t.Fatal("expected 'failures' key in response")
	}

	failureList, ok := failures.([]interface{})
	if !ok {
		t.Fatalf("expected failures to be an array, got %T", failures)
	}
	if len(failureList) != 1 {
		t.Fatalf("expected 1 failure (invalid IP), got %d", len(failureList))
	}

	failureItem, ok := failureList[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected failure to be an object, got %T", failureList[0])
	}
	if failureItem["ip"] != "not-valid!" {
		t.Errorf("expected failure IP to be 'not-valid!', got %v", failureItem["ip"])
	}
	if failureItem["reason"] == "" {
		t.Error("expected failure to have a non-empty reason")
	}
}

func TestHandleBatchAdd_AllValid_EmptyFailures(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"devices":[{"ip":"10.0.0.1","snmp":{"version":"2c","community":"public"}},{"ip":"10.0.0.2","snmp":{"version":"2c","community":"public"}}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBatchAdd(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	failures, ok := resp["failures"]
	if !ok {
		t.Fatal("expected 'failures' key in response")
	}

	failureList, ok := failures.([]interface{})
	if !ok {
		t.Fatalf("expected failures to be an array, got %T", failures)
	}
	if len(failureList) != 0 {
		t.Errorf("expected 0 failures for valid IPs, got %d: %v", len(failureList), failureList)
	}
}
