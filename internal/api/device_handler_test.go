package api

// This file exercises device handler behavior so refactors preserve the documented contract.

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
	mu              sync.Mutex
	devices         map[uuid.UUID]*domain.Device
	orphanDeviceIDs []uuid.UUID
}

func newMockDeviceRepo() *mockDeviceRepo {
	return &mockDeviceRepo{devices: make(map[uuid.UUID]*domain.Device)}
}

func (r *mockDeviceRepo) Create(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	domain.NormalizeDeviceAddresses(device)
	for _, existing := range r.devices {
		if device.IP != "" && existing.IP == device.IP {
			return fmt.Errorf("inserting device: UNIQUE constraint failed: devices.ip")
		}
	}
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
		if strings.EqualFold(strings.TrimSpace(d.IP), strings.TrimSpace(ip)) {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) GetByAddress(address string) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized := domain.NormalizeDeviceAddressValue(address)
	if normalized == "" {
		return nil, nil
	}
	for _, d := range r.devices {
		for _, value := range domain.DeviceAddressValues(*d) {
			if domain.NormalizeDeviceAddressValue(value) == normalized {
				cp := *d
				return &cp, nil
			}
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) GetDeviceAddresses(deviceID uuid.UUID) ([]domain.DeviceAddress, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[deviceID]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", deviceID)
	}
	return append([]domain.DeviceAddress(nil), d.Addresses...), nil
}

func (r *mockDeviceRepo) ReplaceDeviceAddresses(deviceID uuid.UUID, addresses []domain.DeviceAddress) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found: %s", deviceID)
	}
	cp := *d
	cp.Addresses = append([]domain.DeviceAddress(nil), addresses...)
	domain.NormalizeDeviceAddresses(&cp)
	cp.UpdatedAt = time.Now().UTC()
	r.devices[deviceID] = &cp
	return nil
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

func (r *mockDeviceRepo) FindPhysicalVirtualIPConflict(ip string, deviceType domain.DeviceType, excludeID uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	address := strings.TrimSpace(ip)
	if address == "" {
		return nil, nil
	}
	candidateVirtual := deviceType == domain.DeviceTypeVirtual
	for _, d := range r.devices {
		if excludeID != uuid.Nil && d.ID == excludeID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(d.IP), address) {
			continue
		}
		if (d.DeviceType == domain.DeviceTypeVirtual) != candidateVirtual {
			cp := *d
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) FindAddressConflict(address string, deviceType domain.DeviceType, excludeID uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	normalized := domain.NormalizeDeviceAddressValue(address)
	if normalized == "" || deviceType == domain.DeviceTypeVirtual {
		return nil, nil
	}
	for _, d := range r.devices {
		if excludeID != uuid.Nil && d.ID == excludeID {
			continue
		}
		if d.DeviceType == domain.DeviceTypeVirtual {
			continue
		}
		for _, value := range domain.DeviceAddressValues(*d) {
			if domain.NormalizeDeviceAddressValue(value) == normalized {
				cp := *d
				return &cp, nil
			}
		}
	}
	return nil, nil
}

func (r *mockDeviceRepo) GetOrphans() ([]domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.Device
	for _, id := range r.orphanDeviceIDs {
		if device, ok := r.devices[id]; ok {
			result = append(result, *device)
		}
	}
	return result, nil
}

func (r *mockDeviceRepo) Update(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[device.ID]; !ok {
		return fmt.Errorf("device not found: %s", device.ID)
	}
	for existingID, existing := range r.devices {
		if existingID != device.ID && device.IP != "" && existing.IP == device.IP {
			return fmt.Errorf("updating device: UNIQUE constraint failed: devices.ip")
		}
	}
	domain.NormalizeDeviceAddresses(device)
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

func (r *mockLinkRepo) CreateManualIdempotent(link *domain.Link, browserLocalStorageMigration bool) (*domain.Link, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.links {
		if mockEquivalentManualLink(existing, link, browserLocalStorageMigration) {
			cp := *existing
			return &cp, false, nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	stored := *link
	r.links[link.ID] = &stored
	return &stored, true, nil
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

func (r *mockLinkRepo) Upsert(link *domain.Link) (bool, error) {
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
			return false, nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	return true, nil
}

func mockEquivalentManualLink(existing, candidate *domain.Link, browserLocalStorageMigration bool) bool {
	if browserLocalStorageMigration {
		return (existing.SourceDeviceID == candidate.SourceDeviceID && existing.TargetDeviceID == candidate.TargetDeviceID) ||
			(existing.SourceDeviceID == candidate.TargetDeviceID && existing.TargetDeviceID == candidate.SourceDeviceID)
	}
	return mockSameLinkEndpoints(existing, candidate) || mockReverseLinkEndpoints(existing, candidate)
}

func mockSameLinkEndpoints(a, b *domain.Link) bool {
	return a.SourceDeviceID == b.SourceDeviceID &&
		a.SourceIfName == b.SourceIfName &&
		a.TargetDeviceID == b.TargetDeviceID &&
		a.TargetIfName == b.TargetIfName
}

func mockReverseLinkEndpoints(a, b *domain.Link) bool {
	return a.SourceDeviceID == b.TargetDeviceID &&
		a.SourceIfName == b.TargetIfName &&
		a.TargetDeviceID == b.SourceDeviceID &&
		a.TargetIfName == b.SourceIfName
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

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}

	svc := service.NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
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

func intPtr(v int) *int { return &v }

func responseAddresses(t *testing.T, attrs map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := attrs["addresses"].([]interface{})
	if !ok {
		t.Fatalf("expected addresses array in attributes, got %#v", attrs["addresses"])
	}
	addresses := make([]map[string]interface{}, 0, len(raw))
	for i, item := range raw {
		record, ok := item.(map[string]interface{})
		if !ok {
			t.Fatalf("addresses[%d] = %#v, want object", i, item)
		}
		addresses = append(addresses, record)
	}
	return addresses
}

func responseIntArray(t *testing.T, value interface{}) []int {
	t.Helper()
	raw, ok := value.([]interface{})
	if !ok {
		t.Fatalf("expected JSON array, got %#v", value)
	}
	result := make([]int, 0, len(raw))
	for i, item := range raw {
		number, ok := item.(float64)
		if !ok {
			t.Fatalf("expected JSON number at index %d, got %#v", i, item)
		}
		if number != float64(int(number)) {
			t.Fatalf("expected integer JSON number at index %d, got %#v", i, number)
		}
		result = append(result, int(number))
	}
	return result
}

func assertIntSliceEqual(t *testing.T, got []int, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
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

func TestDeviceHandlerList_IncludesPollClassificationFields(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "10.0.0.1",
		Hostname:             "core-router",
		Managed:              true,
		Status:               domain.DeviceStatusUp,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(45),
		Tags:                 map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

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
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 device in list, got %d", len(resp.Data))
	}

	attrs := resp.Data[0].Attributes
	if got := attrs["poll_class"]; got != "core" {
		t.Fatalf("expected poll_class core, got %#v", got)
	}
	if got := attrs["poll_interval_override"]; got != float64(45) {
		t.Fatalf("expected poll_interval_override 45, got %#v", got)
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

func TestDeviceHandlerCreate_LegacyIPReturnsAddresses(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.30.0.1","hostname":"router-addresses","snmp":{"version":"2c","community":"public"}}`
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
	addresses := responseAddresses(t, resp.Data.Attributes)
	if len(addresses) != 1 {
		t.Fatalf("addresses len = %d, want 1: %#v", len(addresses), addresses)
	}
	if addresses[0]["address"] != "10.30.0.1" || addresses[0]["role"] != "primary" || addresses[0]["is_primary"] != true {
		t.Fatalf("primary address response = %#v", addresses[0])
	}
}

func TestDeviceHandlerCreate_AddressesWithoutIPDerivesPrimary(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"hostname":"router-derived","snmp":{"version":"2c","community":"public"},"addresses":[{"address":"10.31.0.1","role":"management","is_primary":true},{"address":"198.51.100.31","role":"backup","label":"backup link"}]}`
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
	if got := resp.Data.Attributes["ip"]; got != "10.31.0.1" {
		t.Fatalf("ip = %#v, want derived primary", got)
	}
	addresses := responseAddresses(t, resp.Data.Attributes)
	if len(addresses) != 2 {
		t.Fatalf("addresses len = %d, want 2: %#v", len(addresses), addresses)
	}
	if addresses[1]["address"] != "198.51.100.31" || addresses[1]["role"] != "backup" {
		t.Fatalf("backup address response = %#v", addresses[1])
	}
}

func TestDeviceHandlerCreate_ProbePortsRoundTrip(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.34.0.1","hostname":"router-probe-ports","probe_ports":[22,8291],"snmp":{"version":"2c","community":"public"},"addresses":[{"address":"10.34.0.1","role":"primary","is_primary":true},{"address":"198.51.100.34","role":"backup","label":"backup link","probe_ports":[2222]}]}`
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
	assertIntSliceEqual(t, responseIntArray(t, resp.Data.Attributes["probe_ports"]), []int{22, 8291})
	addresses := responseAddresses(t, resp.Data.Attributes)
	if len(addresses) != 2 {
		t.Fatalf("addresses len = %d, want 2: %#v", len(addresses), addresses)
	}
	assertIntSliceEqual(t, responseIntArray(t, addresses[1]["probe_ports"]), []int{2222})
}

func TestDeviceHandlerCreate_InvalidProbePortsRejected(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.34.0.2","hostname":"router-bad-probe-ports","probe_ports":[0],"snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "probe_ports") {
		t.Fatalf("expected probe_ports error, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_AddressesDuplicateValidation(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.32.0.1","snmp":{"version":"2c","community":"public"},"addresses":[{"address":"10.32.0.1","role":"primary","is_primary":true},{"address":" 10.32.0.1 ","role":"backup"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "duplicate address") {
		t.Fatalf("expected duplicate address error, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_AddressInvalidValidation(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.33.0.1","snmp":{"version":"2c","community":"public"},"addresses":[{"address":"not valid!!","role":"backup"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "address must be a valid IP address or hostname") {
		t.Fatalf("expected invalid address error, got: %s", rec.Body.String())
	}
}

func TestDeviceHandlerCreate_NotesRoundTrip(t *testing.T) {
	handler, _, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.0.0.9","hostname":"notes-router","notes":"Installed in row B","snmp":{"version":"2c","community":"public"}}`
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
	if got := resp.Data.Attributes["notes"]; got != "Installed in row B" {
		t.Fatalf("expected notes to round-trip, got %#v", got)
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

func TestDeviceHandlerUpdate_ReplacesAddresses(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, repo)

	body := `{"addresses":[{"address":"10.0.0.1","role":"primary","is_primary":true},{"address":"198.51.100.40","role":"backup","label":"backup"}]}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	addresses := responseAddresses(t, resp.Data.Attributes)
	if len(addresses) != 2 {
		t.Fatalf("addresses len = %d, want 2: %#v", len(addresses), addresses)
	}
	if addresses[1]["address"] != "198.51.100.40" || addresses[1]["role"] != "backup" {
		t.Fatalf("backup address response = %#v", addresses[1])
	}
}

func TestDeviceHandlerUpdate_OmittedProbePortsPreservesOverride(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, repo)
	device.ProbePorts = []int{22, 8291}
	if err := repo.Update(device); err != nil {
		t.Fatalf("Update seed failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(`{"hostname":"renamed-router"}`))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	assertIntSliceEqual(t, updated.ProbePorts, []int{22, 8291})
}

func TestDeviceHandlerUpdate_NullProbePortsClearsOverride(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, repo)
	device.ProbePorts = []int{22, 8291}
	if err := repo.Update(device); err != nil {
		t.Fatalf("Update seed failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(`{"probe_ports":null}`))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp.Data.Attributes["probe_ports"]; got != nil {
		t.Fatalf("probe_ports = %#v, want nil", got)
	}
	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(updated.ProbePorts) != 0 {
		t.Fatalf("ProbePorts = %#v, want empty", updated.ProbePorts)
	}
}

func TestDeviceHandlerUpdate_EmptyProbePortsClearsOverride(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, repo)
	device.ProbePorts = []int{22, 8291}
	if err := repo.Update(device); err != nil {
		t.Fatalf("Update seed failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(`{"probe_ports":[]}`))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	updated, err := repo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(updated.ProbePorts) != 0 {
		t.Fatalf("ProbePorts = %#v, want empty", updated.ProbePorts)
	}
}

func TestDeviceHandlerUpdate_PollIntervalOverrideSet(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, deviceRepo)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"poll_interval_override":30}`),
	)
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 30 {
		t.Fatalf("expected override=30, got %#v", updated.PollIntervalOverride)
	}
}

func TestDeviceHandlerList_IncludesPollingEnabled(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	enabled := false
	device := &domain.Device{
		ID:             uuid.New(),
		IP:             "10.0.0.31",
		Hostname:       "suspended-router",
		Managed:        true,
		PollingEnabled: &enabled,
		Status:         domain.DeviceStatusUp,
		Tags:           map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	rec := httptest.NewRecorder()
	handler.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp jsonAPIList
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp.Data[0].Attributes["polling_enabled"]; got != false {
		t.Fatalf("polling_enabled = %#v, want false", got)
	}
}

func TestDeviceHandlerListOrphans_ReturnsJSONAPIDevices(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	mapped := seedDevice(t, deviceRepo)
	orphan := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.32",
		Hostname: "orphan-router",
		Managed:  true,
		Status:   domain.DeviceStatusUnknown,
		Tags:     map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(orphan); err != nil {
		t.Fatalf("Create orphan failed: %v", err)
	}
	deviceRepo.orphanDeviceIDs = []uuid.UUID{orphan.ID}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/orphans", nil)
	rec := httptest.NewRecorder()
	handler.HandleListOrphans(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	var resp jsonAPIList
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 orphan device, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != orphan.ID.String() {
		t.Fatalf("expected orphan device %s, got %s", orphan.ID, resp.Data[0].ID)
	}
	if resp.Data[0].ID == mapped.ID.String() {
		t.Fatalf("mapped device %s should not be returned as orphan", mapped.ID)
	}
	if resp.Data[0].Type != "device" {
		t.Fatalf("expected JSON:API type device, got %q", resp.Data[0].Type)
	}
	if got := resp.Data[0].Attributes["hostname"]; got != orphan.Hostname {
		t.Fatalf("hostname = %#v, want %q", got, orphan.Hostname)
	}
}

func TestDeviceHandlerUpdate_PollingEnabled(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, deviceRepo)

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"polling_enabled":false}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if domain.DevicePollingEnabled(*updated) {
		t.Fatalf("PollingEnabled = true, want false")
	}

	var resp jsonAPISingle
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got := resp.Data.Attributes["polling_enabled"]; got != false {
		t.Fatalf("response polling_enabled = %#v, want false", got)
	}
}

func TestDeviceHandlerUpdate_PollIntervalOverrideClear(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, deviceRepo)
	device.PollIntervalOverride = intPtr(45)
	if err := deviceRepo.Update(device); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"poll_interval_override":null}`),
	)
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rec.Code, rec.Body.String())
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.PollIntervalOverride != nil {
		t.Fatalf("expected override cleared, got %d", *updated.PollIntervalOverride)
	}
}

func TestDeviceHandlerUpdate_PollIntervalOverrideRejectsOutOfRange(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, deviceRepo)
	device.PollIntervalOverride = intPtr(45)
	if err := deviceRepo.Update(device); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"poll_interval_override":4}`),
	)
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "poll_interval_override must be between 5 and 3600 seconds") {
		t.Fatalf("expected range validation error, got %s", rec.Body.String())
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 45 {
		t.Fatalf("expected override to remain 45 after rejection, got %#v", updated.PollIntervalOverride)
	}
}

func TestDeviceHandlerUpdate_NotesTriState(t *testing.T) {
	handler, deviceRepo, _ := newTestDeviceHandler(t)
	device := seedDevice(t, deviceRepo)
	initialNotes := "Before change"
	device.Notes = &initialNotes
	if err := deviceRepo.Update(device); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	setReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"notes":"  Replaced SFP on 2026-04-16  "}`),
	)
	setRec := httptest.NewRecorder()
	handler.HandleUpdate(setRec, setReq)
	if setRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on set, got %d; body=%s", setRec.Code, setRec.Body.String())
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after set failed: %v", err)
	}
	if updated.Notes == nil || *updated.Notes != "Replaced SFP on 2026-04-16" {
		t.Fatalf("expected trimmed notes after set, got %#v", updated.Notes)
	}

	keepReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"hostname":"renamed-router"}`),
	)
	keepRec := httptest.NewRecorder()
	handler.HandleUpdate(keepRec, keepReq)
	if keepRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on keep, got %d; body=%s", keepRec.Code, keepRec.Body.String())
	}

	kept, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after keep failed: %v", err)
	}
	if kept.Notes == nil || *kept.Notes != "Replaced SFP on 2026-04-16" {
		t.Fatalf("expected notes to stay unchanged, got %#v", kept.Notes)
	}

	clearReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"notes":"   "}`),
	)
	clearRec := httptest.NewRecorder()
	handler.HandleUpdate(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on blank clear, got %d; body=%s", clearRec.Code, clearRec.Body.String())
	}

	cleared, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after blank clear failed: %v", err)
	}
	if cleared.Notes != nil {
		t.Fatalf("expected blank notes to clear field, got %#v", cleared.Notes)
	}

	nullReq := httptest.NewRequest(
		http.MethodPut,
		"/api/v1/devices/"+device.ID.String(),
		strings.NewReader(`{"notes":null}`),
	)
	nullRec := httptest.NewRecorder()
	handler.HandleUpdate(nullRec, nullReq)
	if nullRec.Code != http.StatusOK {
		t.Fatalf("expected 200 on null clear, got %d; body=%s", nullRec.Code, nullRec.Body.String())
	}

	clearedAgain, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID after null clear failed: %v", err)
	}
	if clearedAgain.Notes != nil {
		t.Fatalf("expected null notes to remain cleared, got %#v", clearedAgain.Notes)
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

func TestHandleCreate_TopologyDiscoveryMode_PersistsToDevice(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)

	body := `{"ip":"10.0.0.50","hostname":"sw-bootstrap","snmp":{"version":"2c","community":"public"},"topology_discovery_mode":"bootstrap_once"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	devices, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].TopologyDiscoveryMode != domain.TopologyDiscoveryModeBootstrapOnce {
		t.Fatalf("expected topology_discovery_mode bootstrap_once, got %s", devices[0].TopologyDiscoveryMode)
	}
	if devices[0].TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("expected topology_bootstrap_state pending, got %s", devices[0].TopologyBootstrapState)
	}
}

func TestHandleUpdate_TopologyDiscoveryMode_Invalid_400(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	device := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch-a",
		IP:       "10.0.0.5",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(`{"topology_discovery_mode":"bogus"}`))
	rec := httptest.NewRecorder()

	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdate_AllowsFrontendMetricsSources(t *testing.T) {
	tests := []string{"prometheus_snmp_fallback", "none"}

	for _, metricsSource := range tests {
		t.Run(metricsSource, func(t *testing.T) {
			handler, repo, _ := newTestDeviceHandler(t)

			device := &domain.Device{
				IP:            "10.0.0.1",
				DeviceType:    domain.DeviceTypeVirtual,
				MetricsSource: domain.MetricsSourcePrometheus,
			}
			if err := repo.Create(device); err != nil {
				t.Fatalf("failed to seed device: %v", err)
			}

			body := fmt.Sprintf(`{"metrics_source":"%s"}`, metricsSource)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+device.ID.String(), strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.HandleUpdate(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 for metrics_source=%q, got %d; body: %s", metricsSource, rec.Code, rec.Body.String())
			}
		})
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

func TestHandleCreate_DuplicateIP_Returns409(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)

	if err := repo.Create(&domain.Device{
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
	}); err != nil {
		t.Fatalf("failed to seed existing device: %v", err)
	}

	body := `{"ip":"10.0.0.1","hostname":"router-dup","snmp":{"version":"2c","community":"public"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate IP, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `a device with IP/host \"10.0.0.1\" already exists`) {
		t.Errorf("expected duplicate IP message, got: %s", rec.Body.String())
	}
}

func TestHandleUpdate_DuplicateIP_Returns409(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)

	first := &domain.Device{
		IP:         "10.0.0.1",
		DeviceType: domain.DeviceTypeRouter,
	}
	second := &domain.Device{
		IP:         "10.0.0.2",
		DeviceType: domain.DeviceTypeRouter,
	}

	if err := repo.Create(first); err != nil {
		t.Fatalf("failed to seed first device: %v", err)
	}
	if err := repo.Create(second); err != nil {
		t.Fatalf("failed to seed second device: %v", err)
	}

	body := `{"ip":"10.0.0.1"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/devices/"+second.ID.String(), strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleUpdate(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate IP update, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `a device with IP/host \"10.0.0.1\" already exists`) {
		t.Errorf("expected duplicate IP update message, got: %s", rec.Body.String())
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

func TestHandleBatchAdd_ValidationParityWithSingleCreate(t *testing.T) {
	tests := []struct {
		name       string
		deviceJSON string
		wantReason string
	}{
		{
			name:       "regular device requires ip",
			deviceJSON: `{"hostname":"missing-ip","snmp":{"version":"2c","community":"public"}}`,
			wantReason: "ip is required",
		},
		{
			name:       "invalid device type",
			deviceJSON: `{"ip":"10.0.0.10","device_type":"refrigerator","snmp":{"version":"2c","community":"public"}}`,
			wantReason: "invalid device_type",
		},
		{
			name:       "invalid metrics source",
			deviceJSON: `{"ip":"10.0.0.11","metrics_source":"magic","snmp":{"version":"2c","community":"public"}}`,
			wantReason: "invalid metrics_source",
		},
		{
			name:       "invalid topology discovery mode",
			deviceJSON: `{"ip":"10.0.0.12","topology_discovery_mode":"bogus","snmp":{"version":"2c","community":"public"}}`,
			wantReason: "invalid topology_discovery_mode",
		},
		{
			name:       "tag key too long",
			deviceJSON: fmt.Sprintf(`{"ip":"10.0.0.13","tags":{"%s":"value"},"snmp":{"version":"2c","community":"public"}}`, strings.Repeat("k", 256)),
			wantReason: "tag key too long",
		},
		{
			name:       "tag value too long",
			deviceJSON: fmt.Sprintf(`{"ip":"10.0.0.14","tags":{"role":"%s"},"snmp":{"version":"2c","community":"public"}}`, strings.Repeat("v", 256)),
			wantReason: "tag value too long",
		},
		{
			name:       "prometheus label name too long",
			deviceJSON: fmt.Sprintf(`{"ip":"10.0.0.15","prometheus_label_name":"%s","snmp":{"version":"2c","community":"public"}}`, strings.Repeat("n", 256)),
			wantReason: "prometheus_label_name too long",
		},
		{
			name:       "prometheus label value too long",
			deviceJSON: fmt.Sprintf(`{"ip":"10.0.0.16","prometheus_label_value":"%s","snmp":{"version":"2c","community":"public"}}`, strings.Repeat("v", 256)),
			wantReason: "prometheus_label_value too long",
		},
		{
			name:       "virtual missing display name",
			deviceJSON: `{"device_type":"virtual","tags":{"virtual_subtype":"internet"}}`,
			wantReason: "tags.display_name is required for virtual devices",
		},
		{
			name:       "virtual invalid subtype",
			deviceJSON: `{"device_type":"virtual","tags":{"display_name":"Cloud","virtual_subtype":"gateway"}}`,
			wantReason: "tags.virtual_subtype must be one of",
		},
		{
			name:       "invalid area id",
			deviceJSON: `{"ip":"10.0.0.17","area_ids":["not-a-uuid"],"snmp":{"version":"2c","community":"public"}}`,
			wantReason: "invalid area_id: not-a-uuid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			createHandler, _, _ := newTestDeviceHandler(t)
			createReq := httptest.NewRequest(http.MethodPost, "/api/v1/devices", strings.NewReader(tc.deviceJSON))
			createRec := httptest.NewRecorder()

			createHandler.HandleCreate(createRec, createReq)

			if createRec.Code != http.StatusBadRequest {
				t.Fatalf("expected single create 400, got %d; body: %s", createRec.Code, createRec.Body.String())
			}
			if !strings.Contains(createRec.Body.String(), tc.wantReason) {
				t.Fatalf("expected single create reason to contain %q, got: %s", tc.wantReason, createRec.Body.String())
			}

			batchHandler, repo, _ := newTestDeviceHandler(t)
			body := fmt.Sprintf(`{"devices":[%s]}`, tc.deviceJSON)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
			rec := httptest.NewRecorder()

			batchHandler.HandleBatchAdd(rec, req)

			if rec.Code != http.StatusAccepted {
				t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
			}
			resp := decodeBatchAddTestResponse(t, rec.Body.String())
			if resp.Count != 1 {
				t.Fatalf("expected batch count 1, got %d; body: %s", resp.Count, rec.Body.String())
			}
			if len(resp.Failures) != 1 {
				t.Fatalf("expected 1 batch failure, got %d: %s", len(resp.Failures), rec.Body.String())
			}
			if !strings.Contains(resp.Failures[0].Reason, tc.wantReason) {
				t.Fatalf("expected batch failure reason to contain %q, got %q", tc.wantReason, resp.Failures[0].Reason)
			}
			devices, err := repo.GetAll()
			if err != nil {
				t.Fatalf("GetAll failed: %v", err)
			}
			if len(devices) != 0 {
				t.Fatalf("expected invalid batch row not to create a device, got %d", len(devices))
			}
		})
	}
}

func TestHandleBatchAdd_ValidRowsPersistBatchOnlyFields(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)
	regularAreaID := uuid.New()
	regularAreaID2 := uuid.New()
	virtualAreaID := uuid.New()

	body := fmt.Sprintf(`{"devices":[
		{
			"ip":"10.20.0.1",
			"hostname":"batch-regular",
			"tags":{"role":"aggregation","site":"lab"},
			"metrics_source":"prometheus_snmp_fallback",
			"prometheus_label_name":"instance",
			"prometheus_label_value":"batch-regular:9100",
			"topology_discovery_mode":"lldp_cdp",
			"area_ids":["%s","%s"],
			"snmp":{"version":"2c","community":"public"}
		},
		{
			"ip":"10.20.0.2",
			"device_type":"virtual",
			"metrics_source":"prometheus",
			"tags":{"display_name":"Internet Edge","virtual_subtype":"internet","role":"wan"},
			"area_ids":["%s"]
		}
	]}`, regularAreaID.String(), regularAreaID2.String(), virtualAreaID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBatchAdd(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeBatchAddTestResponse(t, rec.Body.String())
	if resp.Count != 2 {
		t.Fatalf("expected batch count 2, got %d; body: %s", resp.Count, rec.Body.String())
	}
	if len(resp.Failures) != 0 {
		t.Fatalf("expected 0 failures for valid rows, got %d: %+v", len(resp.Failures), resp.Failures)
	}

	devices, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 stored devices, got %d", len(devices))
	}

	regular, err := repo.GetByIP("10.20.0.1")
	if err != nil {
		t.Fatalf("GetByIP regular failed: %v", err)
	}
	if regular == nil {
		t.Fatal("expected regular batch row to be stored")
	}
	if got := regular.Tags["role"]; got != "aggregation" {
		t.Fatalf("expected regular role tag aggregation, got %q", got)
	}
	if got := regular.Tags["site"]; got != "lab" {
		t.Fatalf("expected regular site tag lab, got %q", got)
	}
	if regular.MetricsSource != domain.MetricsSourcePrometheusSNMPFallback {
		t.Fatalf("expected regular metrics_source %q, got %q", domain.MetricsSourcePrometheusSNMPFallback, regular.MetricsSource)
	}
	if regular.PrometheusLabelName != "instance" {
		t.Fatalf("expected regular prometheus_label_name instance, got %q", regular.PrometheusLabelName)
	}
	if regular.PrometheusLabelValue != "batch-regular:9100" {
		t.Fatalf("expected regular prometheus_label_value batch-regular:9100, got %q", regular.PrometheusLabelValue)
	}
	if regular.TopologyDiscoveryMode != domain.TopologyDiscoveryModeLLDPCDP {
		t.Fatalf("expected regular topology_discovery_mode %q, got %q", domain.TopologyDiscoveryModeLLDPCDP, regular.TopologyDiscoveryMode)
	}
	assertDeviceAreaIDs(t, regular, []uuid.UUID{regularAreaID, regularAreaID2})

	virtual, err := repo.GetByIP("10.20.0.2")
	if err != nil {
		t.Fatalf("GetByIP virtual failed: %v", err)
	}
	if virtual == nil {
		t.Fatal("expected virtual batch row to be stored")
	}
	if virtual.DeviceType != domain.DeviceTypeVirtual {
		t.Fatalf("expected virtual device_type %q, got %q", domain.DeviceTypeVirtual, virtual.DeviceType)
	}
	if got := virtual.Tags["display_name"]; got != "Internet Edge" {
		t.Fatalf("expected virtual display_name tag Internet Edge, got %q", got)
	}
	if got := virtual.Tags["virtual_subtype"]; got != "internet" {
		t.Fatalf("expected virtual virtual_subtype tag internet, got %q", got)
	}
	if virtual.MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected virtual metrics_source to normalize to %q, got %q", domain.MetricsSourceNone, virtual.MetricsSource)
	}
	assertDeviceAreaIDs(t, virtual, []uuid.UUID{virtualAreaID})
}

func TestHandleBatchAdd_ProbePortsPersist(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)

	body := `{"devices":[
		{
			"ip":"10.20.1.1",
			"hostname":"batch-probe-ports",
			"probe_ports":[22,8291],
			"addresses":[
				{"address":"10.20.1.1","role":"primary","is_primary":true},
				{"address":"198.51.100.201","role":"backup","label":"backup","probe_ports":[2222]}
			],
			"snmp":{"version":"2c","community":"public"}
		}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBatchAdd(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	resp := decodeBatchAddTestResponse(t, rec.Body.String())
	if len(resp.Failures) != 0 {
		t.Fatalf("expected 0 failures, got %d: %+v", len(resp.Failures), resp.Failures)
	}
	device, err := repo.GetByIP("10.20.1.1")
	if err != nil {
		t.Fatalf("GetByIP failed: %v", err)
	}
	if device == nil {
		t.Fatal("expected batch device to be stored")
	}
	assertIntSliceEqual(t, device.ProbePorts, []int{22, 8291})
	addresses, err := repo.GetDeviceAddresses(device.ID)
	if err != nil {
		t.Fatalf("GetDeviceAddresses failed: %v", err)
	}
	if len(addresses) != 2 {
		t.Fatalf("addresses len = %d, want 2: %#v", len(addresses), addresses)
	}
	assertIntSliceEqual(t, addresses[1].ProbePorts, []int{2222})
}

func TestHandleBatchAdd_MixedRowsPreserveDiagnosticsAndCreateOnlyValidRows(t *testing.T) {
	handler, repo, _ := newTestDeviceHandler(t)

	body := fmt.Sprintf(`{"devices":[
		{"ip":"10.30.0.1","hostname":"valid-row","snmp":{"version":"2c","community":"public"}},
		{"ip":"10.30.0.2","metrics_source":"magic","snmp":{"version":"2c","community":"public"}},
		{"ip":"10.30.0.3","topology_discovery_mode":"bogus","snmp":{"version":"2c","community":"public"}},
		{"ip":"10.30.0.4","tags":{"role":"%s"},"snmp":{"version":"2c","community":"public"}}
	]}`, strings.Repeat("v", 256))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBatchAdd(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}

	resp := decodeBatchAddTestResponse(t, rec.Body.String())
	if resp.Count != 4 {
		t.Fatalf("expected batch count 4, got %d; body: %s", resp.Count, rec.Body.String())
	}
	wantFailures := []struct {
		ip     string
		reason string
	}{
		{ip: "10.30.0.2", reason: "invalid metrics_source"},
		{ip: "10.30.0.3", reason: "invalid topology_discovery_mode"},
		{ip: "10.30.0.4", reason: "tag value too long"},
	}
	if len(resp.Failures) != len(wantFailures) {
		t.Fatalf("expected %d failures, got %d: %+v", len(wantFailures), len(resp.Failures), resp.Failures)
	}
	for i, want := range wantFailures {
		got := resp.Failures[i]
		if got.IP != want.ip {
			t.Fatalf("failure[%d].IP = %q, want %q", i, got.IP, want.ip)
		}
		if !strings.Contains(got.Reason, want.reason) {
			t.Fatalf("failure[%d].Reason = %q, want reason containing %q", i, got.Reason, want.reason)
		}
	}

	devices, err := repo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected only the valid row to be stored, got %d devices", len(devices))
	}
	if devices[0].IP != "10.30.0.1" {
		t.Fatalf("expected stored device IP 10.30.0.1, got %q", devices[0].IP)
	}
	for _, ip := range []string{"10.30.0.2", "10.30.0.3", "10.30.0.4"} {
		device, err := repo.GetByIP(ip)
		if err != nil {
			t.Fatalf("GetByIP(%s) failed: %v", ip, err)
		}
		if device != nil {
			t.Fatalf("expected invalid row %s not to be stored", ip)
		}
	}
}

type batchAddTestFailure struct {
	IP     string `json:"ip"`
	Reason string `json:"reason"`
}

type batchAddTestResponse struct {
	BatchID  string                `json:"batch_id"`
	Status   string                `json:"status"`
	Count    int                   `json:"count"`
	Failures []batchAddTestFailure `json:"failures"`
}

func decodeBatchAddTestResponse(t *testing.T, body string) batchAddTestResponse {
	t.Helper()
	var resp batchAddTestResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Failures == nil {
		resp.Failures = []batchAddTestFailure{}
	}
	return resp
}

func assertDeviceAreaIDs(t *testing.T, device *domain.Device, want []uuid.UUID) {
	t.Helper()
	if len(device.AreaIDs) != len(want) {
		t.Fatalf("expected %s area_ids length %d, got %d: %+v", device.IP, len(want), len(device.AreaIDs), device.AreaIDs)
	}
	for i := range want {
		if device.AreaIDs[i] != want[i] {
			t.Fatalf("expected %s area_ids[%d] %s, got %s", device.IP, i, want[i], device.AreaIDs[i])
		}
	}
}
