package api

import (
	"context"
	"encoding/json"
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

// --- Mock SSHProfileRepo ---

type mockSSHProfileRepo struct {
	mu       sync.Mutex
	profiles map[uuid.UUID]*domain.SSHProfile
}

func newMockSSHProfileRepo() *mockSSHProfileRepo {
	return &mockSSHProfileRepo{profiles: make(map[uuid.UUID]*domain.SSHProfile)}
}

func (r *mockSSHProfileRepo) Create(profile *domain.SSHProfile) error {
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

func (r *mockSSHProfileRepo) GetByID(id uuid.UUID) (*domain.SSHProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.profiles[id]
	if !ok {
		return nil, fmt.Errorf("ssh profile not found: %s", id)
	}
	cp := *p
	return &cp, nil
}

func (r *mockSSHProfileRepo) GetAll() ([]domain.SSHProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.SSHProfile
	for _, p := range r.profiles {
		result = append(result, *p)
	}
	return result, nil
}

func (r *mockSSHProfileRepo) Update(profile *domain.SSHProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[profile.ID]; !ok {
		return fmt.Errorf("ssh profile not found: %s", profile.ID)
	}
	profile.UpdatedAt = time.Now().UTC()
	r.profiles[profile.ID] = profile
	return nil
}

func (r *mockSSHProfileRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[id]; !ok {
		return fmt.Errorf("ssh profile not found: %s", id)
	}
	delete(r.profiles, id)
	return nil
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
	sshProfileRepo := newMockSSHProfileRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}

	svc := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn)
	registry := buildTestVendorRegistry()
	handler := NewDeviceHandler(svc, sshProfileRepo, registry)

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

// Silence the unused import for context -- it is needed for domain import indirectly.
var _ = context.Background
