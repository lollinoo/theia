package service

// This file exercises device service behavior so refactors preserve the documented contract.

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/scheduler"
	"github.com/lollinoo/theia/internal/snmp"
	"github.com/lollinoo/theia/internal/topology"
)

// --- Mock Device Repository ---

type mockDeviceRepo struct {
	mu                       sync.Mutex
	devices                  map[uuid.UUID]*domain.Device
	updateHook               func(*domain.Device) error
	updateCalls              int
	updateCallsByDevice      map[uuid.UUID]int
	findIPConflictCalls      int
	getByIDsCalls            int
	getByIDsForTopologyCalls int
	failGetAll               bool
	failGetByIDs             bool
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
	domain.NormalizeDeviceAddresses(device)
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
	if r.failGetAll {
		return nil, fmt.Errorf("full device scan should not be used")
	}
	var result []domain.Device
	for _, d := range r.devices {
		result = append(result, *d)
	}
	return result, nil
}

func (r *mockDeviceRepo) FindPhysicalVirtualIPConflict(ip string, deviceType domain.DeviceType, excludeID uuid.UUID) (*domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.findIPConflictCalls++
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

func (r *mockDeviceRepo) GetByIDs(ids []uuid.UUID) ([]domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getByIDsCalls++
	if r.failGetByIDs {
		return nil, fmt.Errorf("full device batch path should not be used")
	}
	return r.devicesByIDsLocked(ids, false), nil
}

func (r *mockDeviceRepo) GetByIDsForTopology(ids []uuid.UUID) ([]domain.Device, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getByIDsForTopologyCalls++
	return r.devicesByIDsLocked(ids, true), nil
}

func (r *mockDeviceRepo) devicesByIDsLocked(ids []uuid.UUID, clearSensitive bool) []domain.Device {
	requested := make(map[uuid.UUID]struct{}, len(ids))
	for _, id := range ids {
		requested[id] = struct{}{}
	}
	result := make([]domain.Device, 0, len(ids))
	for _, d := range r.devices {
		if _, ok := requested[d.ID]; !ok {
			continue
		}
		cp := *d
		if clearSensitive {
			cp.SNMPCredentials = domain.SNMPCredentials{}
		}
		result = append(result, cp)
	}
	return result
}

func (r *mockDeviceRepo) Update(device *domain.Device) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.devices[device.ID]; !ok {
		return fmt.Errorf("device not found: %s", device.ID)
	}
	r.updateCalls++
	if r.updateCallsByDevice == nil {
		r.updateCallsByDevice = make(map[uuid.UUID]int)
	}
	r.updateCallsByDevice[device.ID]++
	if r.updateHook != nil {
		if err := r.updateHook(device); err != nil {
			return err
		}
	}
	domain.NormalizeDeviceAddresses(device)
	device.UpdatedAt = time.Now().UTC()
	r.devices[device.ID] = device
	return nil
}

func (r *mockDeviceRepo) UpdateCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateCalls
}

func (r *mockDeviceRepo) UpdateCountFor(deviceID uuid.UUID) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateCallsByDevice[deviceID]
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

// --- Mock Link Repository ---

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
	result, err := r.UpsertDetailed(link)
	return result.Created, err
}

func (r *mockLinkRepo) UpsertDetailed(link *domain.Link) (domain.LinkUpsertResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.links {
		if existing.SourceDeviceID == link.SourceDeviceID &&
			existing.TargetDeviceID == link.TargetDeviceID &&
			(existing.SourceIfName == link.SourceIfName || existing.SourceIfName == "" || link.SourceIfName == "") &&
			(existing.TargetIfName == link.TargetIfName || existing.TargetIfName == "" || link.TargetIfName == "") {
			updated := *existing
			if updated.SourceIfName == "" && link.SourceIfName != "" {
				updated.SourceIfName = link.SourceIfName
			}
			if updated.TargetIfName == "" && link.TargetIfName != "" {
				updated.TargetIfName = link.TargetIfName
			}
			portsChanged := updated.SourceIfName != existing.SourceIfName || updated.TargetIfName != existing.TargetIfName
			protocolChanged := updated.DiscoveryProtocol != link.DiscoveryProtocol
			if !portsChanged && !protocolChanged {
				*link = updated
				result := domain.LinkUpsertResult{
					Created: false,
					Changed: false,
					Kind:    domain.LinkUpsertKindNoop,
				}
				observability.Default().IncLinkUpsert(link.DiscoveryProtocol, result.Kind)
				return result, nil
			}
			updated.DiscoveryProtocol = link.DiscoveryProtocol
			updated.UpdatedAt = time.Now().UTC()
			r.links[id] = &updated
			*link = updated
			kind := domain.LinkUpsertKindUpdated
			if portsChanged {
				kind = domain.LinkUpsertKindEnriched
			}
			result := domain.LinkUpsertResult{
				Created: false,
				Changed: true,
				Kind:    kind,
			}
			observability.Default().IncLinkUpsert(link.DiscoveryProtocol, result.Kind)
			return result, nil
		}
	}
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}
	now := time.Now().UTC()
	link.CreatedAt = now
	link.UpdatedAt = now
	r.links[link.ID] = link
	result := domain.LinkUpsertResult{Created: true, Changed: true, Kind: domain.LinkUpsertKindCreated}
	observability.Default().IncLinkUpsert(link.DiscoveryProtocol, result.Kind)
	return result, nil
}

type recordingTopologyObservationStore struct {
	mu                  sync.Mutex
	upsertObservations  int
	pruneObservations   int
	listObservations    int
	upsertUnresolved    int
	resolveUnresolved   int
	unresolvedLookups   int
	observations        []topology.Observation
	unresolvedNeighbors []topology.UnresolvedNeighbor
}

func (s *recordingTopologyObservationStore) UpsertObservation(observation *topology.Observation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertObservations++
	if observation != nil {
		s.observations = append(s.observations, *observation)
	}
	return nil
}

func (s *recordingTopologyObservationStore) PruneLocalObservations(uuid.UUID, []domain.DiscoveryProtocol, []topology.Observation) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneObservations++
	return 0, nil
}

func (s *recordingTopologyObservationStore) ListObservationsForDevices([]uuid.UUID) ([]topology.Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listObservations++
	return append([]topology.Observation(nil), s.observations...), nil
}

func (s *recordingTopologyObservationStore) UpsertUnresolvedNeighbor(neighbor *topology.UnresolvedNeighbor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upsertUnresolved++
	if neighbor != nil {
		s.unresolvedNeighbors = append(s.unresolvedNeighbors, *neighbor)
	}
	return nil
}

func (s *recordingTopologyObservationStore) ResolveUnresolvedNeighbor(uuid.UUID, string, domain.DiscoveryProtocol, time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolveUnresolved++
	return nil
}

func (s *recordingTopologyObservationStore) GetUnresolvedNeighborsByDeviceID(uuid.UUID) ([]topology.UnresolvedNeighbor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unresolvedLookups++
	return append([]topology.UnresolvedNeighbor(nil), s.unresolvedNeighbors...), nil
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

// --- Mock Settings Repository ---

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

// --- Helper to create a service with mock SNMP ---

func newTestService(snmpResult *snmp.DiscoveryResult, snmpErr error) (*DeviceService, *mockDeviceRepo, *mockLinkRepo) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return snmpResult, snmpErr
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	return svc, deviceRepo, linkRepo
}

type fakePollRescheduler struct {
	calls          []pollRescheduleCall
	essentialCalls []pollRescheduleCall
	reconcileCalls []pollReconcileCall
}

type recordingRuntimeResetter struct {
	deviceIDs []uuid.UUID
}

type pollRescheduleCall struct {
	device    domain.Device
	changedAt time.Time
}

type pollReconcileCall struct {
	device    domain.Device
	changedAt time.Time
}

func (f *fakePollRescheduler) ReduePerformanceTask(device domain.Device, changedAt time.Time) {
	f.calls = append(f.calls, pollRescheduleCall{
		device:    device,
		changedAt: changedAt,
	})
}

func (f *fakePollRescheduler) RedueEssentialTask(device domain.Device, changedAt time.Time) {
	f.essentialCalls = append(f.essentialCalls, pollRescheduleCall{
		device:    device,
		changedAt: changedAt,
	})
}

func (f *fakePollRescheduler) ReconcileDeviceTasks(device domain.Device, changedAt time.Time) {
	f.reconcileCalls = append(f.reconcileCalls, pollReconcileCall{
		device:    device,
		changedAt: changedAt,
	})
}

func (r *recordingRuntimeResetter) ResetDeviceRuntime(deviceID uuid.UUID) {
	r.deviceIDs = append(r.deviceIDs, deviceID)
}

type recordingBootstrapScheduler struct {
	devices []domain.Device
	dueAt   []time.Time
}

func (r *recordingBootstrapScheduler) ScheduleBootstrap(device domain.Device, dueAt time.Time) bool {
	r.devices = append(r.devices, device)
	r.dueAt = append(r.dueAt, dueAt)
	return true
}

type schedulerDeviceSource struct {
	repo *mockDeviceRepo
}

func (s schedulerDeviceSource) GetDevices() ([]domain.Device, error) {
	return s.repo.GetAll()
}

// --- Tests ---

func TestAddDevice_CreatesWithStatusProbing(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{
		SysName: "router1",
	}, nil)

	device, err := svc.AddDevice(context.Background(), "192.168.1.1", "router1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}
	if device.Status != domain.DeviceStatusProbing {
		t.Errorf("expected status probing, got %s", device.Status)
	}
	if device.Managed != true {
		t.Error("expected managed=true")
	}

	// Verify device was persisted
	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("device not persisted: %v", err)
	}
	if stored.IP != "192.168.1.1" {
		t.Errorf("expected IP 192.168.1.1, got %s", stored.IP)
	}
}

func TestAddDevice_DerivesPollClassFromDeviceType(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "router1",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.PollClass != domain.PollClassCore {
		t.Fatalf("expected returned device PollClass core, got %s", device.PollClass)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc was called for a Prometheus device")
	}

	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.PollClass != domain.PollClassCore {
		t.Errorf("expected stored device PollClass core, got %s", stored.PollClass)
	}
}

func TestAddDevice_UsesGlobalPollingIntervalAsInitialOverride(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingPollingInterval, "75"); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return nil, fmt.Errorf("should not be called")
	}, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "router1",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.PollIntervalOverride == nil || *device.PollIntervalOverride != 75 {
		t.Fatalf("returned PollIntervalOverride = %#v, want 75", device.PollIntervalOverride)
	}

	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.PollIntervalOverride == nil || *stored.PollIntervalOverride != 75 {
		t.Fatalf("stored PollIntervalOverride = %#v, want 75", stored.PollIntervalOverride)
	}
}

func TestAddDevice_BootstrapOnceStartsPendingAndEffectiveModeFollowsDefault(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeBootstrapOnce)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "router1",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
		t.Fatalf("expected bootstrap state pending, got %s", device.TopologyBootstrapState)
	}
	if device.EffectiveTopologyDiscoveryMode != domain.TopologyDiscoveryModeBootstrapOnce {
		t.Fatalf("expected effective topology mode bootstrap_once, got %s", device.EffectiveTopologyDiscoveryMode)
	}
	if device.TopologyDiscoveryMode != domain.TopologyDiscoveryModeInherit {
		t.Fatalf("expected persisted topology mode inherit, got %s", device.TopologyDiscoveryMode)
	}
}

func TestAddDevice_ReturnsCanceledAfterServiceStop(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	discoverCalls := 0
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalls++
		return &snmp.DiscoveryResult{}, nil
	}, nil)

	svc.Stop()
	_, err := svc.AddDevice(context.Background(), "10.0.0.10", "edge-stopped",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AddDevice error = %v, want context.Canceled", err)
	}
	svc.WaitForProbes()
	if discoverCalls != 0 {
		t.Fatalf("expected stopped service not to start discovery, got %d calls", discoverCalls)
	}
}

func TestAddDeviceSchedulesBootstrapInsteadOfStartingDiscoveryGoroutine(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	scheduler := &recordingBootstrapScheduler{}
	discoverCalled := false
	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalled = true
		return &snmp.DiscoveryResult{SysName: "edge-bootstrap"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil, WithBootstrapScheduler(scheduler))

	device, err := svc.AddDevice(context.Background(), "10.0.0.10", "edge-bootstrap",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", domain.TopologyDiscoveryModeBootstrapOnce, nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if len(scheduler.devices) != 1 || scheduler.devices[0].ID != device.ID {
		t.Fatalf("scheduled devices = %#v, want device %s", scheduler.devices, device.ID)
	}
	if scheduler.dueAt[0].IsZero() {
		t.Fatal("expected bootstrap due time to be populated")
	}
	svc.WaitForProbes()
	if discoverCalled {
		t.Fatal("discoverFunc was called; bootstrap add should not start a direct probe goroutine")
	}
}

func TestProbeDevice_UsesResolvedTopologyDiscoveryMode(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeLLDP)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	var seenMode domain.TopologyDiscoveryMode
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		seenMode = mode
		return &snmp.DiscoveryResult{SysName: "edge-sw-1"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:                    uuid.New(),
		IP:                    "10.0.0.8",
		Hostname:              "edge-sw-1",
		Managed:               true,
		Status:                domain.DeviceStatusProbing,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeInherit,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	if seenMode != domain.TopologyDiscoveryModeLLDP {
		t.Fatalf("expected resolved topology mode lldp, got %s", seenMode)
	}
}

func TestProbeDevice_BootstrapOnceCompletesWithoutNeighbors(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			t.Fatalf("expected bootstrap_once mode, got %s", mode)
		}
		return &snmp.DiscoveryResult{
			SysName:    "edge-sw-2",
			DeviceType: domain.DeviceTypeSwitch,
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:                     uuid.New(),
		IP:                     "10.0.0.9",
		Hostname:               "edge-sw-2",
		Managed:                true,
		Status:                 domain.DeviceStatusProbing,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryAt == nil {
		t.Fatal("expected last_topology_discovery_at to be populated")
	}
	if updated.LastTopologyDiscoveryResult != "no_neighbors" {
		t.Fatalf("expected last_topology_discovery_result no_neighbors, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestProbeDevice_ReturnsCanceledAfterServiceStop(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "10.0.0.20",
		Hostname:      "edge-stopped",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	discoverCalls := 0
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalls++
		return &snmp.DiscoveryResult{}, nil
	}, nil)

	svc.Stop()
	err := svc.ProbeDevice(context.Background(), device.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ProbeDevice error = %v, want context.Canceled", err)
	}
	svc.WaitForProbes()
	if discoverCalls != 0 {
		t.Fatalf("expected stopped service not to start discovery, got %d calls", discoverCalls)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("expected stopped probe not to update status, got %s", updated.Status)
	}
}

func TestProbeDevice_RecordsDiscoveryFailedForCriticalNeighborFailureWithNoNeighbors(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			t.Fatalf("expected bootstrap_once mode, got %s", mode)
		}
		return &snmp.DiscoveryResult{
			SysName:    "edge-sw-2",
			DeviceType: domain.DeviceTypeSwitch,
			NeighborDiscoveryFailures: []snmp.NeighborDiscoveryFailure{
				{
					Protocol: domain.DiscoveryProtocolLLDP,
					OID:      snmp.OidLLDPRemChassisId,
					Critical: true,
					Error:    "lldp walk failed",
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:                     uuid.New(),
		IP:                     "10.0.0.10",
		Hostname:               "edge-sw-2",
		Managed:                true,
		Status:                 domain.DeviceStatusProbing,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	svc.probeDevice(device)

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.LastTopologyDiscoveryAt == nil {
		t.Fatal("expected last_topology_discovery_at to be populated")
	}
	if updated.LastTopologyDiscoveryResult != "discovery_failed" {
		t.Fatalf("expected last_topology_discovery_result discovery_failed, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestProbeDevice_RecordsPartialDiscoveryFailedForCriticalNeighborFailureWithNeighbors(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			t.Fatalf("expected bootstrap_once mode, got %s", mode)
		}
		return &snmp.DiscoveryResult{
			SysName:    "edge-sw-2",
			DeviceType: domain.DeviceTypeSwitch,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					LocalIfName:   "ether1",
					RemotePortID:  "ether8",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
			NeighborDiscoveryFailures: []snmp.NeighborDiscoveryFailure{
				{
					Protocol: domain.DiscoveryProtocolCDP,
					OID:      snmp.OidCDPDeviceID,
					Critical: true,
					Error:    "cdp walk failed",
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:                     uuid.New(),
		IP:                     "10.0.0.11",
		Hostname:               "edge-sw-2",
		Managed:                true,
		Status:                 domain.DeviceStatusProbing,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	svc.probeDevice(device)

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.LastTopologyDiscoveryAt == nil {
		t.Fatal("expected last_topology_discovery_at to be populated")
	}
	if updated.LastTopologyDiscoveryResult != "partial_discovery_failed" {
		t.Fatalf("expected last_topology_discovery_result partial_discovery_failed, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestRunTopologyDiscoveryNow_SetsPendingAndTriggersReprobe(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	target := &domain.Device{
		ID:                    uuid.New(),
		IP:                    "10.0.0.20",
		Hostname:              "agg-1",
		Managed:               true,
		Status:                domain.DeviceStatusUp,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeOff,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(target); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	reprobeCalls := 0
	svc.discoverFunc = func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		reprobeCalls++
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			t.Fatalf("expected manual run to use bootstrap_once, got %s", mode)
		}
		return &snmp.DiscoveryResult{SysName: "agg-1", DeviceType: domain.DeviceTypeSwitch}, nil
	}

	if err := svc.RunTopologyDiscoveryNow(context.Background(), target.ID); err != nil {
		t.Fatalf("RunTopologyDiscoveryNow failed: %v", err)
	}
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(target.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if reprobeCalls != 1 {
		t.Fatalf("expected one reprobe call, got %d", reprobeCalls)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed after manual run, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryResult != "no_neighbors" {
		t.Fatalf("expected manual discovery to record no_neighbors, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestRunTopologyDiscoveryNow_AllowsPollingDisabledDevice(t *testing.T) {
	repo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	discoverCalls := 0
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalls++
		return &snmp.DiscoveryResult{}, nil
	}
	svc := NewDeviceService(repo, linkRepo, settingsRepo, discoverFn, nil)
	disabled := false
	device := &domain.Device{
		ID:             uuid.New(),
		Hostname:       "manual-topology-router",
		IP:             "10.0.0.42",
		Managed:        true,
		PollingEnabled: &disabled,
		DeviceType:     domain.DeviceTypeRouter,
		MetricsSource:  domain.MetricsSourceSNMP,
		Status:         domain.DeviceStatusUp,
		Tags:           map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.RunTopologyDiscoveryNow(context.Background(), device.ID); err != nil {
		t.Fatalf("RunTopologyDiscoveryNow failed: %v", err)
	}
	svc.WaitForProbes()
	if discoverCalls == 0 {
		t.Fatalf("manual topology discovery did not run discovery")
	}
}

func TestDeviceDiscoveryCoordinatorTestSNMPUsesTopologyOff(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	var seenMode domain.TopologyDiscoveryMode
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		seenMode = mode
		return &snmp.DiscoveryResult{SysName: "agg-1", SysDescr: "SwitchOS"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:                    uuid.New(),
		IP:                    "10.0.0.21",
		Hostname:              "agg-1",
		Managed:               true,
		Status:                domain.DeviceStatusUp,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeLLDPCDP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	result, err := svc.discovery.TestSNMP(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("TestSNMP failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected SNMP test to succeed")
	}
	if seenMode != domain.TopologyDiscoveryModeOff {
		t.Fatalf("expected TestSNMP to force topology mode off, got %s", seenMode)
	}
}

func TestDeviceDiscoveryCoordinatorTestSNMPUsesManagementAddress(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	var seenTarget string
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		seenTarget = target
		return &snmp.DiscoveryResult{SysName: "agg-1", SysDescr: "SwitchOS"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "10.0.0.21",
		Hostname:      "agg-1",
		Managed:       true,
		Status:        domain.DeviceStatusUp,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
		Addresses: []domain.DeviceAddress{
			{Address: "10.0.0.21", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "10.0.0.22", Role: domain.DeviceAddressRoleManagement},
		},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	result, err := svc.discovery.TestSNMP(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("TestSNMP failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected SNMP test to succeed")
	}
	if seenTarget != "10.0.0.22" {
		t.Fatalf("SNMP target = %q, want management address", seenTarget)
	}
	if result.TargetIP != "10.0.0.22" {
		t.Fatalf("result target = %q, want management address", result.TargetIP)
	}
}

func TestDeviceDiscoveryCoordinatorProbeUsesManagementAddress(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	var seenTarget string
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		seenTarget = target
		return &snmp.DiscoveryResult{SysName: "agg-1", SysDescr: "SwitchOS"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "10.0.0.31",
		Hostname:      "agg-probe",
		Managed:       true,
		Status:        domain.DeviceStatusUp,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
		Addresses: []domain.DeviceAddress{
			{Address: "10.0.0.31", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "10.0.0.32", Role: domain.DeviceAddressRoleManagement},
		},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.ProbeDevice(context.Background(), device.ID); err != nil {
		t.Fatalf("ProbeDevice failed: %v", err)
	}
	svc.WaitForProbes()
	if seenTarget != "10.0.0.32" {
		t.Fatalf("probe target = %q, want management address", seenTarget)
	}
}

func TestProbeCompletes_DeviceStatusUp(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:       "core-router",
		SysDescr:      "RouterOS RB5009",
		SysObjectID:   ".1.3.6.1.4.1.14988",
		HardwareModel: "RB5009",
		DeviceType:    domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", Speed: 1000000000, AdminStatus: "up", OperStatus: "up"},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "core-router",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	// Wait for async probe to complete
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
	if updated.SysName != "core-router" {
		t.Errorf("expected sysName core-router, got %s", updated.SysName)
	}
	if len(updated.Interfaces) != 1 {
		t.Errorf("expected 1 interface, got %d", len(updated.Interfaces))
	}
}

func TestProbeFails_DeviceStatusDown(t *testing.T) {
	svc, deviceRepo, _ := newTestService(nil, fmt.Errorf("SNMP timeout"))

	device, err := svc.AddDevice(context.Background(), "10.0.0.2", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusDown {
		t.Errorf("expected status down, got %s", updated.Status)
	}
}

func TestProbeDevice_SchedulesDelayedLLDPReprobeForIncompleteNewLinks(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.60",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.50",
		Status:        domain.DeviceStatusProbing,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "RouterOS",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "RB5009",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeRouter,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					RemotePortID:  "ether8",
					LocalIfName:   "",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	var scheduledDelays []time.Duration
	reprobeCalls := make(map[uuid.UUID]int)
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		scheduledDelays = append(scheduledDelays, delay)
		fn()
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		reprobeCalls[id]++
		return nil
	}

	svc.probeDevice(probeTarget)

	if len(scheduledDelays) != 2 {
		t.Fatalf("expected delayed reprobes for local device and peer, got %d schedules", len(scheduledDelays))
	}
	for _, delay := range scheduledDelays {
		if delay != svc.reprobeDelay {
			t.Fatalf("scheduled delay = %v, want %v", delay, svc.reprobeDelay)
		}
	}
	if reprobeCalls[probeTarget.ID] != 1 {
		t.Fatalf("expected one delayed reprobe for probe target, got %d", reprobeCalls[probeTarget.ID])
	}
	if reprobeCalls[remoteDevice.ID] != 1 {
		t.Fatalf("expected one delayed reprobe for remote device, got %d", reprobeCalls[remoteDevice.ID])
	}

	updated, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("expected probe target status up, got %s", updated.Status)
	}
}

func TestProbeDevice_DoesNotScheduleDelayedLLDPReprobeForOlderDevices(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		Status:        domain.DeviceStatusProbing,
		Managed:       true,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	stored, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	stored.CreatedAt = time.Now().Add(-(incompleteLinkReprobeWindow + time.Minute))
	if err := deviceRepo.Update(stored); err != nil {
		t.Fatalf("Update older probe target failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "RouterOS",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "RB5009",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeRouter,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					RemotePortID:  "ether8",
					LocalIfName:   "",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	scheduled := false
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		scheduled = true
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		t.Fatal("delayed reprobe should not run for older devices")
		return nil
	}

	svc.probeDevice(probeTarget)

	if scheduled {
		t.Fatal("expected no delayed LLDP reprobe to be scheduled for older devices")
	}
}

func TestScheduleIncompleteLinkReprobe_DoesNotRunAfterServiceStop(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	localDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}
	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourcePrometheus,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}
	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    remoteDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	var callbacks []func()
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		callbacks = append(callbacks, fn)
	}
	reprobeCalls := 0
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		reprobeCalls++
		return nil
	}

	if !svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP) {
		t.Fatal("expected incomplete link reprobe to be scheduled")
	}
	svc.Stop()
	for _, callback := range callbacks {
		callback()
	}

	if reprobeCalls != 0 {
		t.Fatalf("expected stopped service to skip delayed reprobe, got %d calls", reprobeCalls)
	}
}

func TestScheduleIncompleteLinkReprobe_DoesNotRetryAfterServiceStop(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	localDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}
	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourcePrometheus,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}
	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    remoteDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	var callbacks []func()
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		callbacks = append(callbacks, fn)
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		t.Fatal("delayed reprobe should not run after stop")
		return nil
	}

	if !svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP) {
		t.Fatal("expected incomplete link reprobe to be scheduled")
	}
	svc.reprobeInFlight.Store(int32(svc.staticReprobeBudget()))
	svc.Stop()
	callbacks[0]()

	if got := len(callbacks); got != 1 {
		t.Fatalf("expected stopped service not to schedule a retry, got %d scheduled callbacks", got)
	}
}

func TestScheduleIncompleteLinkReprobe_StaleCallbackAfterDeleteDoesNotReprobe(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	localDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}
	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourcePrometheus,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}
	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    remoteDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	var callback func()
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		callback = fn
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		t.Fatal("delayed reprobe should not run for a deleted device")
		return nil
	}

	if !svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP) {
		t.Fatal("expected incomplete link reprobe to be scheduled")
	}
	if err := svc.DeleteDevice(context.Background(), localDevice.ID); err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}
	callback()
}

func TestScheduleIncompleteLinkReprobe_RetriesWhenStaticBudgetIsTemporarilyExhausted(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	localDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		SysName:       "edge-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourcePrometheus,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    remoteDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	var scheduledDelays []time.Duration
	reprobeCalls := 0
	svc.scheduleFunc = func(delay time.Duration, fn func()) {
		scheduledDelays = append(scheduledDelays, delay)
		if len(scheduledDelays) == 1 {
			svc.reprobeInFlight.Store(int32(svc.staticReprobeBudget()))
		} else {
			svc.reprobeInFlight.Store(0)
		}
		fn()
	}
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		reprobeCalls++
		if id != localDevice.ID {
			t.Fatalf("expected reprobe for %s, got %s", localDevice.ID, id)
		}
		return nil
	}

	svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP)

	if len(scheduledDelays) != 2 {
		t.Fatalf("expected initial schedule and retry, got %d schedules", len(scheduledDelays))
	}
	if scheduledDelays[0] != svc.reprobeDelay {
		t.Fatalf("initial delayed reprobe = %v, want %v", scheduledDelays[0], svc.reprobeDelay)
	}
	if scheduledDelays[1] != incompleteLinkReprobeRetry {
		t.Fatalf("retry delayed reprobe = %v, want %v", scheduledDelays[1], incompleteLinkReprobeRetry)
	}
	if reprobeCalls != 1 {
		t.Fatalf("expected one delayed reprobe call after retry, got %d", reprobeCalls)
	}
}

func TestScheduleIncompleteLinkReprobe_ReopensEligiblePeerBootstrapWindow(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeBootstrapOnce)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	localDevice := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "edge-01",
		IP:                     "192.0.2.51",
		SysName:                "edge-01",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}

	peerDevice := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "distribution-01",
		IP:                     "192.0.2.61",
		SysName:                "distribution-01",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeInherit,
		TopologyBootstrapState: domain.TopologyBootstrapStateCompleted,
	}
	if err := deviceRepo.Create(peerDevice); err != nil {
		t.Fatalf("Create peer device failed: %v", err)
	}

	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    peerDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	svc.scheduleFunc = func(delay time.Duration, fn func()) { fn() }

	peerModeSeen := domain.TopologyDiscoveryModeOff
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		if id != peerDevice.ID {
			return nil
		}
		device, err := deviceRepo.GetByID(id)
		if err != nil {
			return err
		}
		peerModeSeen = domain.ResolveTopologyDiscoveryMode(device, svc.defaultTopologyDiscoveryMode())
		if device.TopologyBootstrapState != domain.TopologyBootstrapStatePending {
			t.Fatalf("expected peer bootstrap state pending, got %s", device.TopologyBootstrapState)
		}
		return nil
	}

	if !svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP) {
		t.Fatal("expected incomplete link reprobe to be scheduled")
	}

	if peerModeSeen != domain.TopologyDiscoveryModeBootstrapOnce {
		t.Fatalf("expected peer delayed reprobe to run as bootstrap_once, got %s", peerModeSeen)
	}
}

func TestScheduleIncompleteLinkReprobe_DoesNotReopenExplicitlyDisabledPeer(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	if err := settingsRepo.Set(domain.SettingTopologyDiscoveryDefaultMode, string(domain.TopologyDiscoveryModeBootstrapOnce)); err != nil {
		t.Fatalf("Set setting failed: %v", err)
	}

	localDevice := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "edge-01",
		IP:                     "192.0.2.51",
		SysName:                "edge-01",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStatePending,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}

	peerDevice := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "distribution-01",
		IP:                     "192.0.2.61",
		SysName:                "distribution-01",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeOff,
		TopologyBootstrapState: domain.TopologyBootstrapStateCompleted,
	}
	if err := deviceRepo.Create(peerDevice); err != nil {
		t.Fatalf("Create peer device failed: %v", err)
	}

	if err := linkRepo.Create(&domain.Link{
		SourceDeviceID:    localDevice.ID,
		SourceIfName:      "",
		TargetDeviceID:    peerDevice.ID,
		TargetIfName:      "ether8",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}); err != nil {
		t.Fatalf("Create link failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	svc.scheduleFunc = func(delay time.Duration, fn func()) { fn() }

	peerCalls := 0
	svc.delayedReprobe = func(ctx context.Context, id uuid.UUID) error {
		if id == peerDevice.ID {
			peerCalls++
		}
		return nil
	}

	if !svc.scheduleIncompleteLinkReprobe(localDevice.ID, localDevice.IP) {
		t.Fatal("expected local incomplete link reprobe to be scheduled")
	}
	if peerCalls != 0 {
		t.Fatalf("expected explicitly disabled peer to skip delayed reprobe, got %d calls", peerCalls)
	}

	updatedPeer, err := deviceRepo.GetByID(peerDevice.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updatedPeer.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected explicit-off peer bootstrap state to stay completed, got %s", updatedPeer.TopologyBootstrapState)
	}
}

func TestApplyStaticDiscovery_CompletesBootstrapOnceDuringRegularStaticPersistence(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	localDevice := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "edge-01",
		IP:                     "192.0.2.51",
		SysName:                "edge-01",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStateFollowupScheduled,
	}
	if err := deviceRepo.Create(localDevice); err != nil {
		t.Fatalf("Create local device failed: %v", err)
	}

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	if _, err := svc.ApplyStaticDiscovery(localDevice.ID, StaticDiscoveryInput{
		SysName:    "edge-01",
		DeviceType: domain.DeviceTypeSwitch,
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName: remoteDevice.SysName,
				LocalIfName:   "ether1",
				RemotePortID:  "ether8",
				Protocol:      domain.DiscoveryProtocolLLDP,
			},
		},
	}); err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(localDevice.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryAt == nil {
		t.Fatal("expected last_topology_discovery_at to be populated")
	}
	if updated.LastTopologyDiscoveryResult != "neighbors_found" {
		t.Fatalf("expected last_topology_discovery_result neighbors_found, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestApplyStaticDiscovery_SkipsTopologyObservationStoreWhenRequested(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	topologyStore := &recordingTopologyObservationStore{}

	deviceID := uuid.New()
	device := &domain.Device{
		ID:            deviceID,
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		SysName:       "edge-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, OperStatus: "up"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil, WithTopologyObservationStore(topologyStore))
	result, err := svc.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:    "edge-01",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 10_000_000_000, OperStatus: "up"},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName: "core-01",
				LocalIfName:   "ether1",
				RemotePortID:  "ether48",
				Protocol:      domain.DiscoveryProtocolLLDP,
			},
		},
		NeighborDiscoveryProtocols:  []domain.DiscoveryProtocol{domain.DiscoveryProtocolLLDP},
		SkipTopologyMaterialization: true,
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if !result.TopologyChanged {
		t.Fatal("TopologyChanged = false, want true because interface speed changed")
	}
	if result.TopologyMaterialized {
		t.Fatal("TopologyMaterialized = true, want false when materialization is skipped")
	}

	updated, err := deviceRepo.GetByID(deviceID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got := updated.Interfaces[0].Speed; got != 10_000_000_000 {
		t.Fatalf("interface speed = %d, want updated static speed", got)
	}

	topologyStore.mu.Lock()
	defer topologyStore.mu.Unlock()
	if topologyStore.upsertObservations != 0 || topologyStore.pruneObservations != 0 ||
		topologyStore.listObservations != 0 || topologyStore.upsertUnresolved != 0 ||
		topologyStore.resolveUnresolved != 0 || topologyStore.unresolvedLookups != 0 {
		t.Fatalf("topology store calls = upsert:%d prune:%d list:%d unresolved:%d resolve:%d lookup:%d, want all zero",
			topologyStore.upsertObservations,
			topologyStore.pruneObservations,
			topologyStore.listObservations,
			topologyStore.upsertUnresolved,
			topologyStore.resolveUnresolved,
			topologyStore.unresolvedLookups,
		)
	}
}

func TestApplyStaticDiscovery_UnchangedStaticDataWithTopologyOffSkipsDeviceUpdates(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	deviceID := uuid.New()
	device := &domain.Device{
		ID:                    deviceID,
		Hostname:              "edge-01",
		IP:                    "192.0.2.51",
		SysName:               "edge-01",
		SysDescr:              "SwitchOS",
		SysObjectID:           ".1.3.6.1.4.1.14988.1",
		HardwareModel:         "CRS328-24P-4S+",
		OSVersion:             "7.16",
		Vendor:                "mikrotik",
		Status:                domain.DeviceStatusUp,
		Managed:               true,
		DeviceType:            domain.DeviceTypeSwitch,
		PollClass:             domain.PollClassCore,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeOff,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	result, err := svc.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:       "edge-01",
		SysDescr:      "SwitchOS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CRS328-24P-4S+",
		OSVersion:     "7.16",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if result.TopologyChanged {
		t.Fatal("TopologyChanged = true, want false for unchanged static data")
	}
	if got := deviceRepo.UpdateCount(); got != 0 {
		t.Fatalf("device repo updates = %d, want 0 for unchanged topology-off static discovery", got)
	}
}

func TestApplyStaticDiscovery_UnchangedStaticDataWithTopologyEnabledOnlyUpdatesDiscoveryMetadata(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	deviceID := uuid.New()
	device := &domain.Device{
		ID:                    deviceID,
		Hostname:              "edge-01",
		IP:                    "192.0.2.51",
		SysName:               "edge-01",
		SysDescr:              "SwitchOS",
		SysObjectID:           ".1.3.6.1.4.1.14988.1",
		HardwareModel:         "CRS328-24P-4S+",
		OSVersion:             "7.16",
		Vendor:                "mikrotik",
		Status:                domain.DeviceStatusUp,
		Managed:               true,
		DeviceType:            domain.DeviceTypeSwitch,
		PollClass:             domain.PollClassCore,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeLLDP,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)
	result, err := svc.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:       "edge-01",
		SysDescr:      "SwitchOS",
		SysObjectID:   ".1.3.6.1.4.1.14988.1",
		HardwareModel: "CRS328-24P-4S+",
		OSVersion:     "7.16",
		Vendor:        "mikrotik",
		DeviceType:    domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, AdminStatus: "up", OperStatus: "up"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}
	if result.TopologyChanged {
		t.Fatal("TopologyChanged = true, want false for unchanged static data")
	}
	if got := deviceRepo.UpdateCountFor(deviceID); got != 1 {
		t.Fatalf("device updates for %s = %d, want only the topology metadata update", deviceID, got)
	}
	updated, err := deviceRepo.GetByID(deviceID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.LastTopologyDiscoveryAt == nil {
		t.Fatal("expected topology discovery metadata update to set last_topology_discovery_at")
	}
	if updated.LastTopologyDiscoveryResult != "no_neighbors" {
		t.Fatalf("LastTopologyDiscoveryResult = %q, want no_neighbors", updated.LastTopologyDiscoveryResult)
	}
}

func TestApplyStaticDiscovery_PreservesLastKnownInterfacesDuringPartialStaticPoll(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	deviceID := uuid.New()
	device := &domain.Device{
		ID:            deviceID,
		Hostname:      "edge-01",
		IP:            "192.0.2.51",
		SysName:       "edge-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 1_000_000_000, OperStatus: "up"},
			{IfIndex: 2, IfName: "ether2", IfDescr: "uplink", Speed: 1_000_000_000, OperStatus: "up"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create device failed: %v", err)
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	result, err := svc.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:    "edge-01",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1", IfDescr: "ether1", Speed: 100_000_000, OperStatus: "down"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(deviceID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(updated.Interfaces) != 2 {
		t.Fatalf("interface count = %d, want preserved last-known table with 2 entries: %#v", len(updated.Interfaces), updated.Interfaces)
	}

	interfacesByName := make(map[string]domain.Interface, len(updated.Interfaces))
	for _, iface := range updated.Interfaces {
		interfacesByName[iface.IfName] = iface
	}
	if got := interfacesByName["ether1"].Speed; got != 100_000_000 {
		t.Fatalf("ether1 speed = %d, want updated observed speed", got)
	}
	if got := interfacesByName["ether1"].OperStatus; got != "down" {
		t.Fatalf("ether1 oper status = %q, want updated observed status", got)
	}
	if got := interfacesByName["ether2"].Speed; got != 1_000_000_000 {
		t.Fatalf("ether2 speed = %d, want preserved last-known speed", got)
	}
	if got := interfacesByName["ether2"].OperStatus; got != "up" {
		t.Fatalf("ether2 oper status = %q, want preserved last-known status", got)
	}
	if !result.TopologyChanged {
		t.Fatal("TopologyChanged = false, want true because observed ether1 material speed changed")
	}

	result, err = svc.ApplyStaticDiscovery(deviceID, StaticDiscoveryInput{
		SysName:    "edge-01",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: nil,
	})
	if err != nil {
		t.Fatalf("ApplyStaticDiscovery with empty interfaces failed: %v", err)
	}
	updated, err = deviceRepo.GetByID(deviceID)
	if err != nil {
		t.Fatalf("GetByID after empty interface poll failed: %v", err)
	}
	if len(updated.Interfaces) != 2 {
		t.Fatalf("interface count after empty poll = %d, want preserved table with 2 entries: %#v", len(updated.Interfaces), updated.Interfaces)
	}
	if result.TopologyChanged {
		t.Fatal("TopologyChanged = true, want false when empty degraded poll preserves the last-known table")
	}
}

func TestProbeDevice_CompletesBootstrapOnceAfterFollowupAttemptLeavesPortsUnresolved(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	remoteDevice := &domain.Device{
		ID:            uuid.New(),
		Hostname:      "distribution-01",
		IP:            "192.0.2.61",
		SysName:       "distribution-01",
		Status:        domain.DeviceStatusUp,
		Managed:       true,
		DeviceType:    domain.DeviceTypeSwitch,
		MetricsSource: domain.MetricsSourceSNMP,
	}
	if err := deviceRepo.Create(remoteDevice); err != nil {
		t.Fatalf("Create remote device failed: %v", err)
	}

	probeTarget := &domain.Device{
		ID:                     uuid.New(),
		Hostname:               "edge-01",
		IP:                     "192.0.2.51",
		Status:                 domain.DeviceStatusUp,
		Managed:                true,
		DeviceType:             domain.DeviceTypeSwitch,
		MetricsSource:          domain.MetricsSourceSNMP,
		TopologyDiscoveryMode:  domain.TopologyDiscoveryModeBootstrapOnce,
		TopologyBootstrapState: domain.TopologyBootstrapStateFollowupScheduled,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(probeTarget); err != nil {
		t.Fatalf("Create probe target failed: %v", err)
	}

	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		if mode != domain.TopologyDiscoveryModeBootstrapOnce {
			t.Fatalf("expected follow-up probe to keep bootstrap_once mode, got %s", mode)
		}
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "RouterOS",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "RB5009",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeRouter,
			Neighbors: []snmp.NeighborInfo{
				{
					RemoteSysName: remoteDevice.SysName,
					RemotePortID:  "ether8",
					LocalIfName:   "",
					Protocol:      domain.DiscoveryProtocolLLDP,
				},
			},
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)
	svc.probeDevice(probeTarget)

	updated, err := deviceRepo.GetByID(probeTarget.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed after unresolved follow-up, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryResult != "ports_pending" {
		t.Fatalf("expected last_topology_discovery_result ports_pending, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestProbeDiscoversNeighbors_SkipsUnmatchedNeighbors(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
				RemotePortID:    "ether2",
				RemoteSysName:   "switch2",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	// The current implementation skips neighbors not already in the system
	// (no placeholder creation), so we expect only the 1 managed device.
	devices, err := deviceRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device (neighbor skipped), got %d", len(devices))
	}

	// No link created since neighbor device doesn't exist
	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected 0 links (neighbor not in system), got %d", len(links))
	}
}

func TestProbeDiscoversNeighbor_ExistingIP_NoDuplicate(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router1",
		DeviceType: domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName: "router2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolLLDP,
			},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	// Pre-create a device that will appear as a neighbor by sysName
	existingDevice := &domain.Device{
		ID:       uuid.New(),
		Hostname: "router2",
		SysName:  "router2",
		IP:       "10.0.0.99",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	deviceRepo.Create(existingDevice)

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "router1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	devices, err := deviceRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	// Should be 2 (the pre-existing + the new one), NOT 3 (no duplicate for neighbor)
	if len(devices) != 2 {
		t.Errorf("expected 2 devices (no duplicate neighbor), got %d", len(devices))
	}
}

func TestProbeDiscoversNeighbors_PrefersLLDPOverCDPForSameLink(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether1"}},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether2",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName: "switch2-cdp",
				RemotePortID:  "ether2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolCDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link from matched LLDP neighbor, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolLLDP {
		t.Fatalf("expected LLDP link to win, got %s", links[0].DiscoveryProtocol)
	}
}

func TestProbeDiscoversNeighbors_UsesCDPWhenLLDPFieldsAreMissing(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether1"}},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "",
				RemotePortID:    "",
				LocalIfIndex:    1,
				LocalIfName:     "ether1",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName: "switch2",
				RemotePortID:  "ether2",
				LocalIfIndex:  1,
				LocalIfName:   "ether1",
				Protocol:      domain.DiscoveryProtocolCDP,
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 link from CDP gap fill, got %d", len(links))
	}
	if links[0].DiscoveryProtocol != domain.DiscoveryProtocolCDP {
		t.Fatalf("expected CDP link when LLDP lacks matchable data, got %s", links[0].DiscoveryProtocol)
	}
	if links[0].TargetIfName != "ether2" {
		t.Fatalf("expected target interface ether2, got %q", links[0].TargetIfName)
	}
}

func TestProbeDiscoversNeighbors_PrefersPhysicalLinkOverVirtualVariant(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "switch1",
		DeviceType: domain.DeviceTypeSwitch,
		Interfaces: []domain.Interface{{IfIndex: 1, IfName: "ether2-verso-border-botte"}},
		Neighbors: []snmp.NeighborInfo{
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "VLAN-99-MGMT-ETH6",
				LocalIfIndex:    1,
				LocalIfName:     "",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether6-link_new_apparati",
				LocalIfIndex:    1,
				LocalIfName:     "ether2-verso-border-botte",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
			{
				RemoteSysName:   "switch2",
				RemotePortID:    "ether6-link_new_apparati",
				LocalIfIndex:    1,
				LocalIfName:     "",
				Protocol:        domain.DiscoveryProtocolLLDP,
				RemoteChassisID: "aa:bb:cc:dd:ee:ff",
			},
		},
	}

	svc, deviceRepo, linkRepo := newTestService(result, nil)

	remote := &domain.Device{
		ID:       uuid.New(),
		Hostname: "switch2",
		SysName:  "switch2",
		IP:       "10.0.0.2",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(remote); err != nil {
		t.Fatalf("failed to create remote device: %v", err)
	}

	_, err := svc.AddDevice(context.Background(), "10.0.0.1", "switch1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	links, err := linkRepo.GetAll()
	if err != nil {
		t.Fatalf("GetAll links failed: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 preferred link, got %d", len(links))
	}
	if links[0].SourceIfName != "ether2-verso-border-botte" {
		t.Fatalf("expected physical source interface to win, got %q", links[0].SourceIfName)
	}
	if links[0].TargetIfName != "ether6-link_new_apparati" {
		t.Fatalf("expected physical target interface to win, got %q", links[0].TargetIfName)
	}
}

func TestNewDeviceService_WiresCapabilityCollaborators(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return nil, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	if svc.mutation == nil {
		t.Fatal("expected mutation capability to be wired")
	}
	if svc.mutation.parent != svc {
		t.Fatal("expected mutation capability to reference parent service")
	}
	if svc.mutation.deviceRepo != deviceRepo {
		t.Fatal("expected mutation capability to share device repo")
	}
	if svc.mutation.linkRepo != linkRepo {
		t.Fatal("expected mutation capability to share link repo")
	}
	if svc.mutation.settingsRepo != settingsRepo {
		t.Fatal("expected mutation capability to share settings repo")
	}
	if svc.mutation.discoverFunc == nil {
		t.Fatal("expected mutation capability to have discover func")
	}
	if svc.mutation.pollRescheduler != &svc.pollRescheduler {
		t.Fatal("expected mutation capability to share poll rescheduler reference")
	}
	if svc.mutation.runtimeResetter != &svc.runtimeResetter {
		t.Fatal("expected mutation capability to share runtime resetter reference")
	}
	if svc.mutation.now == nil {
		t.Fatal("expected mutation capability to have clock")
	}
	if svc.mutation.probeWg != &svc.probeWg {
		t.Fatal("expected mutation capability to share probe waitgroup")
	}
}

func TestUpdateDevice_ChangesFieldsWithoutReprobing(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.1",
		Hostname: "old-name",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		Tags:     map[string]string{"site": "dc1"},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	deviceRepo.Create(device)

	err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Hostname: strPtr("new-name"),
		Tags:     &map[string]string{"site": "dc2"},
	})
	if err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	updated, _ := deviceRepo.GetByID(device.ID)
	if updated.Hostname != "new-name" {
		t.Errorf("expected hostname new-name, got %s", updated.Hostname)
	}
	if updated.Tags["site"] != "dc2" {
		t.Errorf("expected tag site=dc2, got %s", updated.Tags["site"])
	}
}

func TestUpdateDevice_IPChangeTriggersSchedulerRedue(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:        uuid.New(),
		IP:        "10.0.0.1",
		Hostname:  "router-ip-change",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		IP: strPtr("10.0.0.2"),
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(rescheduler.calls); got != 1 {
		t.Fatalf("redue call count = %d, want 1", got)
	}
	if rescheduler.calls[0].device.IP != "10.0.0.2" {
		t.Fatalf("rescheduled IP = %q, want 10.0.0.2", rescheduler.calls[0].device.IP)
	}
	if rescheduler.calls[0].changedAt.IsZero() || rescheduler.calls[0].changedAt.Location() != time.UTC {
		t.Fatalf("changedAt = %v, want non-zero UTC timestamp", rescheduler.calls[0].changedAt)
	}
}

func TestUpdateDevice_IPChangeResetsRuntimeState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	resetter := &recordingRuntimeResetter{}
	svc.SetRuntimeResetter(resetter)

	device := &domain.Device{
		ID:        uuid.New(),
		IP:        "10.0.0.1",
		Hostname:  "router-runtime-reset",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		IP: strPtr("10.0.0.2"),
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(resetter.deviceIDs); got != 1 {
		t.Fatalf("runtime reset call count = %d, want 1", got)
	}
	if resetter.deviceIDs[0] != device.ID {
		t.Fatalf("runtime reset device ID = %s, want %s", resetter.deviceIDs[0], device.ID)
	}
}

func TestUpdateDevice_ProbePortsChangeResetsRuntimeState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	resetter := &recordingRuntimeResetter{}
	svc.SetRuntimeResetter(resetter)

	device := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.0.0.3",
		Hostname:   "router-probe-reset",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		PollClass:  domain.PollClassCore,
		ProbePorts: []int{22, 443},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newProbePorts := []int{22, 8291}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		ProbePorts: &newProbePorts,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(resetter.deviceIDs); got != 1 {
		t.Fatalf("runtime reset call count = %d, want 1", got)
	}
	if resetter.deviceIDs[0] != device.ID {
		t.Fatalf("runtime reset device ID = %s, want %s", resetter.deviceIDs[0], device.ID)
	}
}

func TestUpdateDevice_ProbePortsChangeTriggersSchedulerRedue(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.0.0.4",
		Hostname:   "router-probe-redue",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		PollClass:  domain.PollClassCore,
		ProbePorts: []int{22, 443},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newProbePorts := []int{22, 8291}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		ProbePorts: &newProbePorts,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(rescheduler.calls); got != 1 {
		t.Fatalf("redue call count = %d, want 1", got)
	}
	if rescheduler.calls[0].device.ID != device.ID {
		t.Fatalf("rescheduled device ID = %q, want %s", rescheduler.calls[0].device.ID, device.ID)
	}
	if rescheduler.calls[0].device.ProbePorts == nil ||
		len(rescheduler.calls[0].device.ProbePorts) != 2 ||
		rescheduler.calls[0].device.ProbePorts[1] != 8291 {
		t.Fatalf("rescheduled probe ports = %#v, want [22, 8291]", rescheduler.calls[0].device.ProbePorts)
	}
}

func TestUpdateDevice_ProbePortsChangeTriggersEssentialRedue(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.0.0.4",
		Hostname:   "router-probe-essential",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		PollClass:  domain.PollClassCore,
		ProbePorts: []int{22, 443},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newProbePorts := []int{22, 8291}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		ProbePorts: &newProbePorts,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(rescheduler.essentialCalls); got != 1 {
		t.Fatalf("essential redue call count = %d, want 1", got)
	}
	if rescheduler.essentialCalls[0].device.ID != device.ID {
		t.Fatalf("essential redue device ID = %q, want %s", rescheduler.essentialCalls[0].device.ID, device.ID)
	}
	if rescheduler.essentialCalls[0].device.ProbePorts == nil ||
		len(rescheduler.essentialCalls[0].device.ProbePorts) != 2 ||
		rescheduler.essentialCalls[0].device.ProbePorts[1] != 8291 {
		t.Fatalf("essential redue probe ports = %#v, want [22, 8291]", rescheduler.essentialCalls[0].device.ProbePorts)
	}
}

func TestUpdateDevice_ProbePortsChangeReconcilesScheduler(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.0.0.4",
		Hostname:   "router-probe-reconcile",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		PollClass:  domain.PollClassCore,
		ProbePorts: []int{22, 443},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	newProbePorts := []int{22, 8291}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		ProbePorts: &newProbePorts,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(rescheduler.reconcileCalls); got != 1 {
		t.Fatalf("reconcile call count = %d, want 1", got)
	}
	if rescheduler.reconcileCalls[0].device.ID != device.ID {
		t.Fatalf("reconciled device ID = %q, want %s", rescheduler.reconcileCalls[0].device.ID, device.ID)
	}
	if rescheduler.reconcileCalls[0].device.ProbePorts == nil ||
		len(rescheduler.reconcileCalls[0].device.ProbePorts) != 2 ||
		rescheduler.reconcileCalls[0].device.ProbePorts[1] != 8291 {
		t.Fatalf("reconciled probe ports = %#v, want [22, 8291]", rescheduler.reconcileCalls[0].device.ProbePorts)
	}
}

func TestUpdateDevice_AddressProbePortsChangeTriggersSchedulerRedue(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:        uuid.New(),
		IP:        "10.0.0.5",
		Hostname:  "router-address-ports-redue",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
		Addresses: []domain.DeviceAddress{
			{
				Address:    "10.0.0.5",
				Label:      "primary",
				Role:       domain.DeviceAddressRolePrimary,
				IsPrimary:  true,
				Priority:   0,
				ProbePorts: []int{22},
			},
			{
				Address:    "192.0.2.5",
				Label:      "backup",
				Role:       domain.DeviceAddressRoleBackup,
				IsPrimary:  false,
				Priority:   10,
				ProbePorts: []int{2222},
			},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	updated := []domain.DeviceAddress{
		{
			Address:    "10.0.0.5",
			Label:      "primary",
			Role:       domain.DeviceAddressRolePrimary,
			IsPrimary:  true,
			Priority:   0,
			ProbePorts: []int{22},
		},
		{
			Address:    "192.0.2.5",
			Label:      "backup",
			Role:       domain.DeviceAddressRoleBackup,
			IsPrimary:  false,
			Priority:   10,
			ProbePorts: []int{2223},
		},
	}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Addresses: &updated,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(rescheduler.calls); got != 1 {
		t.Fatalf("redue call count = %d, want 1", got)
	}
}

func TestAddDeviceWithAddressesNormalizesPrimaryAddress(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device, err := svc.AddDeviceWithAddresses(
		context.Background(),
		"10.20.0.1",
		"router-address-create",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{},
		map[string]string{},
		"default",
		domain.MetricsSourcePrometheus,
		"instance",
		"",
		domain.TopologyDiscoveryModeInherit,
		nil,
		nil,
		[]domain.DeviceAddress{
			{Address: "198.51.100.20", Role: domain.DeviceAddressRoleBackup, Label: "backup", Priority: 10},
		},
	)
	if err != nil {
		t.Fatalf("AddDeviceWithAddresses failed: %v", err)
	}
	svc.WaitForProbes()

	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.IP != "10.20.0.1" {
		t.Fatalf("stored IP = %q, want primary legacy IP", stored.IP)
	}
	if got := domain.BackupAddress(*stored); got != "198.51.100.20" {
		t.Fatalf("BackupAddress = %q, want backup address", got)
	}
}

func TestUpdateDeviceReplacesAddressesAndTriggersRuntimeWork(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	resetter := &recordingRuntimeResetter{}
	rescheduler := &fakePollRescheduler{}
	svc.SetRuntimeResetter(resetter)
	svc.SetPollRescheduler(rescheduler)

	device := &domain.Device{
		ID:        uuid.New(),
		IP:        "10.21.0.1",
		Hostname:  "router-address-update",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	addresses := []domain.DeviceAddress{
		{Address: "10.21.0.1", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "198.51.100.21", Role: domain.DeviceAddressRoleBackup, Label: "backup"},
	}
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{Addresses: &addresses}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if got := domain.BackupAddress(*updated); got != "198.51.100.21" {
		t.Fatalf("BackupAddress = %q, want updated backup address", got)
	}
	if got := len(resetter.deviceIDs); got != 1 {
		t.Fatalf("runtime reset calls = %d, want 1", got)
	}
	if got := len(rescheduler.calls); got != 1 {
		t.Fatalf("reschedule calls = %d, want 1", got)
	}
}

func TestUpdateDeviceRejectsDuplicateSecondaryAddress(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	existing := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.22.0.1",
		Hostname:   "router-address-owner",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		DeviceType: domain.DeviceTypeRouter,
		Addresses: []domain.DeviceAddress{
			{Address: "10.22.0.1", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
			{Address: "198.51.100.22", Role: domain.DeviceAddressRoleBackup},
		},
	}
	if err := deviceRepo.Create(existing); err != nil {
		t.Fatalf("Create existing failed: %v", err)
	}
	candidate := &domain.Device{
		ID:         uuid.New(),
		IP:         "10.22.0.2",
		Hostname:   "router-address-candidate",
		Managed:    true,
		Status:     domain.DeviceStatusUp,
		DeviceType: domain.DeviceTypeRouter,
	}
	if err := deviceRepo.Create(candidate); err != nil {
		t.Fatalf("Create candidate failed: %v", err)
	}

	addresses := []domain.DeviceAddress{
		{Address: "10.22.0.2", Role: domain.DeviceAddressRolePrimary, IsPrimary: true},
		{Address: "198.51.100.22", Role: domain.DeviceAddressRoleBackup},
	}
	err := svc.UpdateDevice(context.Background(), candidate.ID, DeviceUpdate{Addresses: &addresses})
	if err == nil {
		t.Fatal("expected duplicate secondary address error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "address conflict") {
		t.Fatalf("duplicate address error = %v, want address conflict", err)
	}
}

func TestUpdateDevice_UnchangedIPDoesNotResetRuntimeState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	resetter := &recordingRuntimeResetter{}
	svc.SetRuntimeResetter(resetter)

	device := &domain.Device{
		ID:        uuid.New(),
		IP:        "10.0.0.1",
		Hostname:  "router-runtime-keep",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Hostname: strPtr("router-runtime-renamed"),
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	if got := len(resetter.deviceIDs); got != 0 {
		t.Fatalf("runtime reset call count = %d, want 0", got)
	}
}

func TestDeviceMutationServiceUpdateDevice_TriggersReprobeOnEligibleTopologyChange(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverCalls := 0
	seenMode := domain.TopologyDiscoveryModeOff
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalls++
		seenMode = mode
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "SwitchOS edge",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "CRS326",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeSwitch,
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device := &domain.Device{
		ID:                    uuid.New(),
		IP:                    "192.0.2.40",
		Hostname:              "edge-01",
		Managed:               true,
		Status:                domain.DeviceStatusUp,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeOff,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	mode := domain.TopologyDiscoveryModeBootstrapOnce
	if err := svc.mutation.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		TopologyDiscoveryMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if discoverCalls != 1 {
		t.Fatalf("expected one bootstrap reprobe, got %d", discoverCalls)
	}
	if seenMode != domain.TopologyDiscoveryModeBootstrapOnce {
		t.Fatalf("expected reprobe mode bootstrap_once, got %s", seenMode)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed after reprobe, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryResult != "no_neighbors" {
		t.Fatalf("expected last_topology_discovery_result no_neighbors, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestUpdateDevice_TopologyDiscoveryModeChangeTriggersReprobeWhenEligible(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverCalls := 0
	seenMode := domain.TopologyDiscoveryModeOff
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		discoverCalls++
		seenMode = mode
		return &snmp.DiscoveryResult{
			SysName:       "edge-01",
			SysDescr:      "SwitchOS edge",
			SysObjectID:   ".1.3.6.1.4.1.14988.1",
			HardwareModel: "CRS326",
			Vendor:        "mikrotik",
			DeviceType:    domain.DeviceTypeSwitch,
		}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device := &domain.Device{
		ID:                    uuid.New(),
		IP:                    "192.0.2.40",
		Hostname:              "edge-01",
		Managed:               true,
		Status:                domain.DeviceStatusUp,
		DeviceType:            domain.DeviceTypeSwitch,
		MetricsSource:         domain.MetricsSourceSNMP,
		TopologyDiscoveryMode: domain.TopologyDiscoveryModeOff,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	mode := domain.TopologyDiscoveryModeBootstrapOnce
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		TopologyDiscoveryMode: &mode,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if discoverCalls != 1 {
		t.Fatalf("expected one bootstrap reprobe, got %d", discoverCalls)
	}
	if seenMode != domain.TopologyDiscoveryModeBootstrapOnce {
		t.Fatalf("expected reprobe mode bootstrap_once, got %s", seenMode)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.TopologyBootstrapState != domain.TopologyBootstrapStateCompleted {
		t.Fatalf("expected bootstrap state completed after reprobe, got %s", updated.TopologyBootstrapState)
	}
	if updated.LastTopologyDiscoveryResult != "no_neighbors" {
		t.Fatalf("expected last_topology_discovery_result no_neighbors, got %q", updated.LastTopologyDiscoveryResult)
	}
}

func TestUpdateDevice_VirtualNoIPNormalizesStatusAndMetricsSource(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "10.0.0.99",
		Hostname:             "support-node",
		DeviceType:           domain.DeviceTypeVirtual,
		Managed:              true,
		Status:               domain.DeviceStatusDown,
		MetricsSource:        domain.MetricsSourcePrometheus,
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "10.0.0.99",
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		IP: strPtr(""),
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.IP != "" {
		t.Fatalf("expected empty IP, got %q", updated.IP)
	}
	if updated.Status != domain.DeviceStatusUnknown {
		t.Fatalf("expected status unknown after removing IP from virtual device, got %s", updated.Status)
	}
	if updated.MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected metrics source none after removing IP from virtual device, got %s", updated.MetricsSource)
	}
}

func TestUpdateDevice_PollIntervalOverrideTriState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "10.0.0.1",
		Hostname:             "router1",
		Managed:              true,
		Status:               domain.DeviceStatusUp,
		PollClass:            domain.PollClassCore,
		PollIntervalOverride: intPtr(15),
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("UpdateDevice keep failed: %v", err)
	}

	unchanged, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after keep: %v", err)
	}
	if unchanged.PollIntervalOverride == nil || *unchanged.PollIntervalOverride != 15 {
		t.Fatalf("expected keep to preserve override=15, got %#v", unchanged.PollIntervalOverride)
	}

	var cleared *int
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &cleared,
	}); err != nil {
		t.Fatalf("UpdateDevice clear failed: %v", err)
	}

	clearedDevice, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after clear: %v", err)
	}
	if clearedDevice.PollIntervalOverride != nil {
		t.Fatalf("expected clear to remove override, got %d", *clearedDevice.PollIntervalOverride)
	}

	newOverride := intPtr(30)
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &newOverride,
	}); err != nil {
		t.Fatalf("UpdateDevice set failed: %v", err)
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after set: %v", err)
	}
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 30 {
		t.Fatalf("expected set to store override=30, got %#v", updated.PollIntervalOverride)
	}
}

func TestUpdateDevice_PollingEnabledTriState(t *testing.T) {
	repo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	discoverFn := func(target string, creds domain.SNMPCredentials, mode domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		return &snmp.DiscoveryResult{}, nil
	}
	svc := NewDeviceService(repo, linkRepo, settingsRepo, discoverFn, nil)
	enabled := true
	device := &domain.Device{
		ID:             uuid.New(),
		Hostname:       "poll-toggle-router",
		IP:             "10.0.0.41",
		Managed:        true,
		PollingEnabled: &enabled,
		Status:         domain.DeviceStatusUp,
		Tags:           map[string]string{},
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := repo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("keep update failed: %v", err)
	}
	kept, _ := repo.GetByID(device.ID)
	if !domain.DevicePollingEnabled(*kept) {
		t.Fatalf("keep update changed polling_enabled to false")
	}

	disabled := false
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{PollingEnabled: &disabled}); err != nil {
		t.Fatalf("disable update failed: %v", err)
	}
	disabledDevice, _ := repo.GetByID(device.ID)
	if domain.DevicePollingEnabled(*disabledDevice) {
		t.Fatalf("disable update left polling_enabled true")
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("second keep update failed: %v", err)
	}
	stillDisabled, _ := repo.GetByID(device.ID)
	if domain.DevicePollingEnabled(*stillDisabled) {
		t.Fatalf("keep update changed polling_enabled back to true")
	}
}

func TestUpdateDevice_PollingEnabledChangeReconcilesScheduler(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	rescheduler := &fakePollRescheduler{}
	svc.SetPollRescheduler(rescheduler)

	enabled := true
	device := &domain.Device{
		ID:             uuid.New(),
		IP:             "10.0.0.42",
		Hostname:       "poll-reconcile-router",
		Managed:        true,
		PollingEnabled: &enabled,
		Status:         domain.DeviceStatusUp,
		PollClass:      domain.PollClassCore,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	disabled := false
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{PollingEnabled: &disabled}); err != nil {
		t.Fatalf("disable update failed: %v", err)
	}
	if got := len(rescheduler.reconcileCalls); got != 1 {
		t.Fatalf("reconcile call count after disable = %d, want 1", got)
	}
	if domain.DevicePollingEnabled(rescheduler.reconcileCalls[0].device) {
		t.Fatalf("reconciled device polling_enabled = true, want false")
	}
	if rescheduler.reconcileCalls[0].changedAt.IsZero() || rescheduler.reconcileCalls[0].changedAt.Location() != time.UTC {
		t.Fatalf("changedAt = %v, want non-zero UTC timestamp", rescheduler.reconcileCalls[0].changedAt)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{PollingEnabled: &disabled}); err != nil {
		t.Fatalf("second disable update failed: %v", err)
	}
	if got := len(rescheduler.reconcileCalls); got != 1 {
		t.Fatalf("reconcile call count after unchanged disable = %d, want 1", got)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{PollingEnabled: &enabled}); err != nil {
		t.Fatalf("enable update failed: %v", err)
	}
	if got := len(rescheduler.reconcileCalls); got != 2 {
		t.Fatalf("reconcile call count after enable = %d, want 2", got)
	}
	if !domain.DevicePollingEnabled(rescheduler.reconcileCalls[1].device) {
		t.Fatalf("reconciled device polling_enabled = false, want true")
	}
}

func TestUpdateDevice_NotesTriState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	initialNotes := "Primary uplink near MDF"
	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.11",
		Hostname: "router-notes",
		Notes:    &initialNotes,
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
		t.Fatalf("UpdateDevice keep failed: %v", err)
	}
	unchanged, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after keep: %v", err)
	}
	if unchanged.Notes == nil || *unchanged.Notes != initialNotes {
		t.Fatalf("expected keep to preserve notes, got %#v", unchanged.Notes)
	}

	updatedNotes := strPtr("  Replace PSU during next visit  ")
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &updatedNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice set failed: %v", err)
	}
	stored, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after set: %v", err)
	}
	if stored.Notes == nil || *stored.Notes != "Replace PSU during next visit" {
		t.Fatalf("expected trimmed notes after set, got %#v", stored.Notes)
	}

	blankNotes := strPtr("   ")
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &blankNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice blank failed: %v", err)
	}
	cleared, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after blank clear: %v", err)
	}
	if cleared.Notes != nil {
		t.Fatalf("expected blank notes to clear field, got %#v", cleared.Notes)
	}

	var nilNotes *string
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		Notes: &nilNotes,
	}); err != nil {
		t.Fatalf("UpdateDevice nil clear failed: %v", err)
	}
	clearedAgain, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed after nil clear: %v", err)
	}
	if clearedAgain.Notes != nil {
		t.Fatalf("expected nil notes to remain cleared, got %#v", clearedAgain.Notes)
	}
}

func TestUpdateDevice_PollIntervalOverrideTriggersSchedulerRedueOnChange(t *testing.T) {
	t.Run("set from nil", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:        uuid.New(),
			IP:        "10.0.1.1",
			Hostname:  "router-nil",
			Managed:   true,
			Status:    domain.DeviceStatusUp,
			PollClass: domain.PollClassCore,
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(15)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride == nil || *rescheduler.calls[0].device.PollIntervalOverride != 15 {
			t.Fatalf("rescheduled override = %#v, want 15", rescheduler.calls[0].device.PollIntervalOverride)
		}
		if rescheduler.calls[0].changedAt.IsZero() || rescheduler.calls[0].changedAt.Location() != time.UTC {
			t.Fatalf("changedAt = %v, want non-zero UTC timestamp", rescheduler.calls[0].changedAt)
		}
	})

	t.Run("change existing value", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.1.2",
			Hostname:             "router-old",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(30),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(20)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride == nil || *rescheduler.calls[0].device.PollIntervalOverride != 20 {
			t.Fatalf("rescheduled override = %#v, want 20", rescheduler.calls[0].device.PollIntervalOverride)
		}
	})

	t.Run("clear existing override", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.1.3",
			Hostname:             "router-clear",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(25),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		var cleared *int
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &cleared,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 1 {
			t.Fatalf("redue call count = %d, want 1", got)
		}
		if rescheduler.calls[0].device.PollIntervalOverride != nil {
			t.Fatalf("rescheduled override = %#v, want nil", rescheduler.calls[0].device.PollIntervalOverride)
		}
	})
}

func TestUpdateDevice_PollIntervalOverrideDoesNotTriggerSchedulerRedueWhenUnchanged(t *testing.T) {
	t.Run("omit preserves existing override without redue", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.2.1",
			Hostname:             "router-keep",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(15),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})

	t.Run("same value override is a no-op", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:                   uuid.New(),
			IP:                   "10.0.2.2",
			Hostname:             "router-same",
			Managed:              true,
			Status:               domain.DeviceStatusUp,
			PollClass:            domain.PollClassCore,
			PollIntervalOverride: intPtr(30),
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		override := intPtr(30)
		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			PollIntervalOverride: &override,
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})

	t.Run("unrelated update does not redue", func(t *testing.T) {
		svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
		rescheduler := &fakePollRescheduler{}
		svc.SetPollRescheduler(rescheduler)

		device := &domain.Device{
			ID:        uuid.New(),
			IP:        "10.0.2.3",
			Hostname:  "router-name",
			Managed:   true,
			Status:    domain.DeviceStatusUp,
			PollClass: domain.PollClassCore,
		}
		if err := deviceRepo.Create(device); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
			Hostname: strPtr("router-renamed"),
		}); err != nil {
			t.Fatalf("UpdateDevice failed: %v", err)
		}

		if got := len(rescheduler.calls); got != 0 {
			t.Fatalf("redue call count = %d, want 0", got)
		}
	})
}

func TestUpdateDevice_PollIntervalOverrideReduesNextPerformanceTask(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	// Use a deterministic fixed UUID whose seeded offsets stay beyond the quiet window
	// plus a safety margin. That makes the first post-update task prove the override-
	// triggered path instead of an initial seed firing early.
	quietWindow := 150 * time.Millisecond
	seededOffsetFloor := quietWindow + 150*time.Millisecond
	deviceID := fixedSchedulerDeviceIDForQuietWindow(t, seededOffsetFloor)
	device := &domain.Device{
		ID:        deviceID,
		IP:        "10.0.3.1",
		Hostname:  "router-integration",
		Managed:   true,
		Status:    domain.DeviceStatusUp,
		PollClass: domain.PollClassCore,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	sched := scheduler.NewScheduler(schedulerDeviceSource{repo: deviceRepo}, settingsRepo)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer sched.Stop()
	svc.SetPollRescheduler(sched)

	assertNoSchedulerTaskWithin(t, sched.Tasks(), quietWindow)

	beforeSave := time.Now().UTC()
	override := intPtr(12)
	if err := svc.UpdateDevice(context.Background(), device.ID, DeviceUpdate{
		PollIntervalOverride: &override,
	}); err != nil {
		t.Fatalf("UpdateDevice failed: %v", err)
	}
	afterSave := time.Now().UTC()

	task := waitForSchedulerTask(t, sched.Tasks(), time.Second)
	if task.Key != scheduler.NewTaskKey(deviceID, domain.VolatilityClassPerformance) {
		t.Fatalf("first post-update task key = %+v, want performance task for device %s", task.Key, deviceID)
	}
	if task.VolatilityClass != domain.VolatilityClassPerformance {
		t.Fatalf("first post-update task volatility = %q, want performance", task.VolatilityClass)
	}
	if task.ExpectedInterval != 12*time.Second {
		t.Fatalf("expected interval = %v, want 12s", task.ExpectedInterval)
	}
	if task.Device.PollIntervalOverride == nil || *task.Device.PollIntervalOverride != 12 {
		t.Fatalf("task override = %#v, want 12", task.Device.PollIntervalOverride)
	}
	if task.DueAt.Before(beforeSave) || task.DueAt.After(afterSave) {
		t.Fatalf("task dueAt = %v, want between save bounds [%v, %v]", task.DueAt, beforeSave, afterSave)
	}
}

func TestDeleteDevice_RemovesDeviceAndLinks(t *testing.T) {
	svc, deviceRepo, linkRepo := newTestService(&snmp.DiscoveryResult{}, nil)

	device := &domain.Device{
		ID:      uuid.New(),
		IP:      "10.0.0.1",
		Managed: true,
		Status:  domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	deviceRepo.Create(device)

	otherDevice := &domain.Device{
		ID:      uuid.New(),
		IP:      "10.0.0.2",
		Managed: true,
		Status:  domain.DeviceStatusUp,
	}
	deviceRepo.Create(otherDevice)

	link := &domain.Link{
		ID:                uuid.New(),
		SourceDeviceID:    device.ID,
		SourceIfName:      "ether1",
		TargetDeviceID:    otherDevice.ID,
		TargetIfName:      "ether2",
		DiscoveryProtocol: domain.DiscoveryProtocolLLDP,
	}
	linkRepo.Create(link)

	err := svc.DeleteDevice(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}

	// Device should be gone
	_, err = deviceRepo.GetByID(device.ID)
	if err == nil {
		t.Error("expected device to be deleted")
	}

	// Links involving device should be gone
	links, _ := linkRepo.GetByDeviceID(device.ID)
	if len(links) != 0 {
		t.Errorf("expected 0 links for deleted device, got %d", len(links))
	}
}

func TestDeleteDevice_ResetsRuntimeStateAfterSuccessfulDelete(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)
	resetter := &recordingRuntimeResetter{}
	svc.SetRuntimeResetter(resetter)

	device := &domain.Device{
		ID:      uuid.New(),
		IP:      "10.0.0.10",
		Managed: true,
		Status:  domain.DeviceStatusUp,
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := svc.DeleteDevice(context.Background(), device.ID); err != nil {
		t.Fatalf("DeleteDevice failed: %v", err)
	}

	if got := len(resetter.deviceIDs); got != 1 {
		t.Fatalf("runtime reset call count = %d, want 1", got)
	}
	if resetter.deviceIDs[0] != device.ID {
		t.Fatalf("runtime reset device ID = %s, want %s", resetter.deviceIDs[0], device.ID)
	}
}

func TestGetAllDevices_ReturnsAllWithInterfaces(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	d1 := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.1", Managed: true, Status: domain.DeviceStatusUp,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
	}
	d2 := &domain.Device{
		ID: uuid.New(), IP: "10.0.0.2", Managed: true, Status: domain.DeviceStatusUp,
	}
	deviceRepo.Create(d1)
	deviceRepo.Create(d2)

	devices, err := svc.GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices failed: %v", err)
	}
	if len(devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(devices))
	}
}

func TestGetTopologyDevicesByIDsUsesNonSensitiveProjection(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	deviceID := uuid.New()
	device := &domain.Device{
		ID:       deviceID,
		IP:       "10.0.0.1",
		Hostname: "router-1",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "secret-community"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	deviceRepo.failGetByIDs = true

	devices, err := svc.GetTopologyDevicesByIDs(context.Background(), []uuid.UUID{deviceID})
	if err != nil {
		t.Fatalf("GetTopologyDevicesByIDs failed: %v", err)
	}
	if got := deviceRepo.getByIDsCalls; got != 0 {
		t.Fatalf("GetByIDs calls = %d, want 0", got)
	}
	if got := deviceRepo.getByIDsForTopologyCalls; got != 1 {
		t.Fatalf("GetByIDsForTopology calls = %d, want 1", got)
	}
	if len(devices) != 1 {
		t.Fatalf("device count = %d, want 1", len(devices))
	}
	if devices[0].SNMPCredentials != (domain.SNMPCredentials{}) {
		t.Fatalf("SNMP credentials = %+v, want empty for topology projection", devices[0].SNMPCredentials)
	}
}

func TestProbeDevice_ReprobeUpdatesFields(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:       "router1-updated",
		SysDescr:      "RouterOS RB5009",
		SysObjectID:   ".1.3.6.1.4.1.14988",
		HardwareModel: "RB5009",
		DeviceType:    domain.DeviceTypeRouter,
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
			{IfIndex: 2, IfName: "ether2"},
		},
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:       uuid.New(),
		IP:       "10.0.0.1",
		Hostname: "router1",
		SysName:  "router1-old",
		Managed:  true,
		Status:   domain.DeviceStatusUp,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
		Interfaces: []domain.Interface{
			{IfIndex: 1, IfName: "ether1"},
		},
	}
	deviceRepo.Create(device)

	err := svc.ProbeDevice(context.Background(), device.ID)
	if err != nil {
		t.Fatalf("ProbeDevice failed: %v", err)
	}

	svc.WaitForProbes()

	updated, _ := deviceRepo.GetByID(device.ID)
	if updated.SysName != "router1-updated" {
		t.Errorf("expected sysName router1-updated, got %s", updated.SysName)
	}
	if len(updated.Interfaces) != 2 {
		t.Errorf("expected 2 interfaces, got %d", len(updated.Interfaces))
	}
}

func TestProbeDevice_PersistsDiscoveredHostnameWhenDeviceWasAddedByIP(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:       "edge-router-01",
		SysDescr:      "RouterOS RB5009",
		SysObjectID:   ".1.3.6.1.4.1.14988",
		HardwareModel: "RB5009",
		DeviceType:    domain.DeviceTypeRouter,
	}
	svc, deviceRepo, _ := newTestService(result, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "10.0.0.1",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Hostname != "edge-router-01" {
		t.Fatalf("expected discovered hostname to persist, got %q", updated.Hostname)
	}
}

func TestProbeDevice_DoesNotOverwriteManualHostnameWithDiscoveredSysName(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "edge-router-01",
		DeviceType: domain.DeviceTypeRouter,
	}
	svc, deviceRepo, _ := newTestService(result, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "manual-router-name",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Hostname != "manual-router-name" {
		t.Fatalf("expected manual hostname to be preserved, got %q", updated.Hostname)
	}
}

// TestPrometheusDevice_SkipsSNMPProbe verifies that adding a device with
// MetricsSourcePrometheus does NOT call the gosnmp discovery function and
// immediately sets status to "up" without requiring SNMP credentials.
func TestPrometheusDevice_SkipsSNMPProbe(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
		domain.DeviceTypeUnknown,
		// No meaningful SNMP credentials — user only wants Prometheus
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc (gosnmp) was called for a Prometheus-sourced device — it should have been skipped")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up for Prometheus device, got %s", updated.Status)
	}
}

// TestPrometheusDevice_SNMPv3WithPrivProtocol verifies the specific bug scenario:
// a device with MetricsSourcePrometheus and SNMPv3 authPriv credentials (including
// priv_protocol but no priv_password) does not cause a gosnmp connection error.
func TestPrometheusDevice_SNMPv3WithPrivProtocol(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("securityParameters.PrivacyPassphrase is required when a privacy protocol is specified")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	// Simulate what happens when user picks "Prometheus without Fallback" but
	// previously had v3 credentials in the form with authPriv + empty priv_password.
	device, err := svc.AddDevice(context.Background(), "10.0.9.254", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV3,
			V3: &domain.SNMPv3Credentials{
				Username:      "monitorUser",
				AuthProtocol:  "SHA",
				AuthPassword:  "authpass123",
				PrivProtocol:  "AES",
				PrivPassword:  "", // empty — bug trigger
				SecurityLevel: "authPriv",
			},
		}, nil, "", domain.MetricsSourcePrometheus, "instance", "10.0.9.254", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc (gosnmp) was called — should have been skipped for MetricsSourcePrometheus")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
}

func TestProbeDevice_PrometheusReclassifiesPollClassWithoutOverride(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.10",
		Hostname:      "router-prometheus",
		DeviceType:    domain.DeviceTypeRouter,
		PollClass:     "",
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourcePrometheus,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	if snmpCalled {
		t.Error("discoverFunc was called for a Prometheus device")
	}

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Errorf("expected status up, got %s", updated.Status)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected poll_class core for Prometheus router, got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

// TestAddDevice_VirtualNoIP verifies that adding a virtual device with no IP
// creates a device with DeviceType=virtual, Status=unknown, and does NOT call
// the SNMP discovery function.
func TestAddDevice_VirtualNoIP(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "", "",
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{}, nil, "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if device.DeviceType != domain.DeviceTypeVirtual {
		t.Errorf("expected device_type virtual, got %s", device.DeviceType)
	}
	if device.Status != domain.DeviceStatusUnknown {
		t.Errorf("expected status unknown, got %s", device.Status)
	}
	if device.IP != "" {
		t.Errorf("expected empty IP, got %s", device.IP)
	}
	if snmpCalled {
		t.Error("discoverFunc was called for virtual device — should have been skipped")
	}
}

// TestAddDevice_VirtualWithIP verifies that a virtual device with an IP address
// is created with Status=unknown (MetricsCollector will resolve via probe_success)
// and does NOT call the SNMP discovery function.
func TestAddDevice_VirtualWithIP(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return nil, fmt.Errorf("should not be called")
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.99", "",
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{}, nil, "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	svc.WaitForProbes()

	if device.DeviceType != domain.DeviceTypeVirtual {
		t.Errorf("expected device_type virtual, got %s", device.DeviceType)
	}
	if device.Status != domain.DeviceStatusUnknown {
		t.Errorf("expected status unknown, got %s", device.Status)
	}
	if device.IP != "10.0.0.99" {
		t.Errorf("expected IP 10.0.0.99, got %s", device.IP)
	}
	if device.MetricsSource != domain.MetricsSourceNone {
		t.Errorf("expected metrics source none for virtual device with IP, got %s", device.MetricsSource)
	}
	if snmpCalled {
		t.Error("discoverFunc was called for virtual device — should have been skipped")
	}
}

func TestAddDevice_RejectsVirtualIPWhenPhysicalDeviceUsesAddress(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	physical := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "edge-physical",
		IP:         "10.0.0.99",
		DeviceType: domain.DeviceTypeRouter,
		Managed:    true,
	}
	if err := deviceRepo.Create(physical); err != nil {
		t.Fatalf("seed physical device: %v", err)
	}
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	_, err := svc.AddDevice(context.Background(), "10.0.0.99", "virtual-edge",
		domain.DeviceTypeVirtual,
		domain.SNMPCredentials{}, nil, "", "", "", "", "", nil)
	if err == nil {
		t.Fatal("expected virtual device with physical IP conflict to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "device ip conflict") {
		t.Fatalf("expected device IP conflict error, got %v", err)
	}
}

func TestAddDevice_RejectsPhysicalIPWhenVirtualDeviceUsesAddress(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	virtual := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-edge",
		IP:         "10.0.0.99",
		DeviceType: domain.DeviceTypeVirtual,
		Managed:    true,
	}
	if err := deviceRepo.Create(virtual); err != nil {
		t.Fatalf("seed virtual device: %v", err)
	}
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	_, err := svc.AddDevice(context.Background(), "10.0.0.99", "physical-edge",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{}, nil, "", "", "", "", "", nil)
	if err == nil {
		t.Fatal("expected physical device with virtual IP conflict to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "device ip conflict") {
		t.Fatalf("expected device IP conflict error, got %v", err)
	}
}

func TestAddDevice_PhysicalVirtualIPConflictCheckUsesScopedRepositoryLookup(t *testing.T) {
	deviceRepo := newMockDeviceRepo()
	deviceRepo.failGetAll = true
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()
	virtual := &domain.Device{
		ID:         uuid.New(),
		Hostname:   "virtual-edge",
		IP:         " 10.0.0.99 ",
		DeviceType: domain.DeviceTypeVirtual,
		Managed:    true,
	}
	if err := deviceRepo.Create(virtual); err != nil {
		t.Fatalf("seed virtual device: %v", err)
	}
	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, nil, nil)

	_, err := svc.AddDevice(context.Background(), "10.0.0.99", "physical-edge",
		domain.DeviceTypeRouter,
		domain.SNMPCredentials{}, nil, "", "", "", "", "", nil)

	if err == nil {
		t.Fatal("expected physical device with virtual IP conflict to fail")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "device ip conflict") {
		t.Fatalf("expected device IP conflict error, got %v", err)
	}
	if deviceRepo.findIPConflictCalls != 1 {
		t.Fatalf("scoped conflict lookups = %d, want 1", deviceRepo.findIPConflictCalls)
	}
}

func TestGetAllDevices_NormalizesLegacyVirtualWithIPMetricsSource(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	legacyVirtual := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             "support-node",
		IP:                   "10.0.0.99",
		DeviceType:           domain.DeviceTypeVirtual,
		Managed:              true,
		Status:               domain.DeviceStatusUnknown,
		MetricsSource:        domain.MetricsSourcePrometheus,
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "10.0.0.99",
	}
	if err := deviceRepo.Create(legacyVirtual); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	devices, err := svc.GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected returned metrics source none for legacy virtual node with IP, got %s", devices[0].MetricsSource)
	}

	stored, err := deviceRepo.GetByID(legacyVirtual.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.MetricsSource != domain.MetricsSourcePrometheus {
		t.Fatalf("expected repo metrics source to remain unchanged during read normalization, got %s", stored.MetricsSource)
	}
}

func TestGetAllDevices_NormalizesLegacyVirtualNoIPState(t *testing.T) {
	svc, deviceRepo, _ := newTestService(&snmp.DiscoveryResult{}, nil)

	legacyVirtual := &domain.Device{
		ID:                   uuid.New(),
		Hostname:             "support-node",
		IP:                   "",
		DeviceType:           domain.DeviceTypeVirtual,
		Managed:              true,
		Status:               domain.DeviceStatusUp,
		MetricsSource:        domain.MetricsSourcePrometheus,
		PrometheusLabelName:  "instance",
		PrometheusLabelValue: "10.0.0.99",
	}
	if err := deviceRepo.Create(legacyVirtual); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	devices, err := svc.GetAllDevices(context.Background())
	if err != nil {
		t.Fatalf("GetAllDevices failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0].Status != domain.DeviceStatusUnknown {
		t.Fatalf("expected returned status unknown for legacy no-IP virtual node, got %s", devices[0].Status)
	}
	if devices[0].MetricsSource != domain.MetricsSourceNone {
		t.Fatalf("expected returned metrics source none for legacy no-IP virtual node, got %s", devices[0].MetricsSource)
	}

	stored, err := deviceRepo.GetByID(legacyVirtual.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if stored.Status != domain.DeviceStatusUp {
		t.Fatalf("expected repo state to remain unchanged during read normalization, got %s", stored.Status)
	}
	if stored.MetricsSource != domain.MetricsSourcePrometheus {
		t.Fatalf("expected repo metrics source to remain unchanged during read normalization, got %s", stored.MetricsSource)
	}
}

// TestAddDevice_RegularStillRequiresProbe verifies that non-virtual devices
// still go through the normal SNMP probe flow.
func TestAddDevice_RegularStillRequiresProbe(t *testing.T) {
	snmpCalled := false
	deviceRepo := newMockDeviceRepo()
	linkRepo := newMockLinkRepo()
	settingsRepo := newMockSettingsRepo()

	discoverFn := func(target string, creds domain.SNMPCredentials, _ domain.TopologyDiscoveryMode) (*snmp.DiscoveryResult, error) {
		snmpCalled = true
		return &snmp.DiscoveryResult{SysName: "test-router"}, nil
	}

	svc := NewDeviceService(deviceRepo, linkRepo, settingsRepo, discoverFn, nil)

	device, err := svc.AddDevice(context.Background(), "10.0.0.1", "",
		domain.DeviceTypeUnknown,
		domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		}, nil, "", domain.MetricsSourceSNMP, "", "", "", nil)
	if err != nil {
		t.Fatalf("AddDevice failed: %v", err)
	}

	if device.Status != domain.DeviceStatusProbing {
		t.Errorf("expected status probing, got %s", device.Status)
	}

	svc.WaitForProbes()

	if !snmpCalled {
		t.Error("discoverFunc was NOT called — regular devices should trigger SNMP probe")
	}
}

type syncBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.b = append(b.b, p...)
	return len(p), nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.b)
}

func (b *syncBuffer) Contains(substr string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.Contains(string(b.b), substr)
}

// helper
func strPtr(s string) *string { return &s }

func intPtr(v int) *int { return &v }

func fixedSchedulerDeviceIDForQuietWindow(t *testing.T, minimumOffset time.Duration) uuid.UUID {
	t.Helper()

	candidates := []uuid.UUID{
		uuid.MustParse("51000000-0000-0000-0000-000000000001"),
		uuid.MustParse("52000000-0000-0000-0000-000000000002"),
		uuid.MustParse("53000000-0000-0000-0000-000000000003"),
		uuid.MustParse("54000000-0000-0000-0000-000000000004"),
	}

	for _, candidate := range candidates {
		performanceOffset := schedulerInitialOffset(candidate, domain.PollClassCore.Interval())
		operationalOffset := schedulerInitialOffset(candidate, domain.OperationalClassInterval)
		staticOffset := schedulerInitialOffset(candidate, domain.StaticClassInterval)
		if performanceOffset > minimumOffset && operationalOffset > minimumOffset && staticOffset > minimumOffset {
			return candidate
		}
	}

	t.Fatalf("no fixed UUID had seeded offsets beyond quiet window floor %v", minimumOffset)
	return uuid.Nil
}

func schedulerInitialOffset(deviceID uuid.UUID, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(deviceID[:])
	return time.Duration(hasher.Sum64() % uint64(interval))
}

func assertNoSchedulerTaskWithin(t *testing.T, tasks <-chan scheduler.PollTask, quietWindow time.Duration) {
	t.Helper()

	select {
	case task := <-tasks:
		t.Fatalf("scheduler emitted task %+v inside quiet window %v", task, quietWindow)
	case <-time.After(quietWindow):
	}
}

func waitForSchedulerTask(t *testing.T, tasks <-chan scheduler.PollTask, timeout time.Duration) scheduler.PollTask {
	t.Helper()

	select {
	case task := <-tasks:
		return task
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for scheduler task after override save within %v", timeout)
		return scheduler.PollTask{}
	}
}

// TestProbeDevice_ReclassifyOnTypeChange verifies that when SNMP probe detects a
// device_type change (unknown -> router), poll_class is auto-recomputed to core
// via domain.ClassifyPollClass. PollIntervalOverride must remain nil (untouched).
func TestProbeDevice_ReclassifyOnTypeChange(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-reclassify",
		SysDescr:   "RouterOS",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.1",
		Hostname:      "router-reclassify",
		DeviceType:    domain.DeviceTypeUnknown,
		PollClass:     domain.PollClassStandard,
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("expected device_type router, got %s", updated.DeviceType)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected poll_class core (router->core per D-04), got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

// TestProbeDevice_RespectsPollIntervalOverride verifies that when a device has a
// manual PollIntervalOverride set, the auto-reclassify hook does NOT overwrite
// poll_class even when device_type changes. DeviceType still propagates (SNMP wins).
func TestProbeDevice_RespectsPollIntervalOverride(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-override",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:                   uuid.New(),
		IP:                   "192.0.2.2",
		Hostname:             "router-override",
		DeviceType:           domain.DeviceTypeUnknown,
		PollClass:            domain.PollClassStandard,
		PollIntervalOverride: intPtr(15),
		Managed:              true,
		Status:               domain.DeviceStatusProbing,
		MetricsSource:        domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	// DeviceType MUST propagate from discovery (SNMP result wins)
	if updated.DeviceType != domain.DeviceTypeRouter {
		t.Errorf("expected device_type router (SNMP result propagates), got %s", updated.DeviceType)
	}
	// PollClass MUST NOT change when override is set (manual control wins)
	if updated.PollClass != domain.PollClassStandard {
		t.Errorf("expected poll_class standard (override set, must not stomp), got %s", updated.PollClass)
	}
	// Override value MUST be preserved
	if updated.PollIntervalOverride == nil || *updated.PollIntervalOverride != 15 {
		t.Errorf("expected PollIntervalOverride=15, got %v", updated.PollIntervalOverride)
	}
}

// TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty verifies that even when
// device_type does NOT change (router -> router), an empty PollClass on a legacy row
// is healed to the correct class on first probe (unconditional recompute path).
func TestProbeDevice_NoTypeChangeStillSyncsPollClassWhenEmpty(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-legacy",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.3",
		Hostname:      "router-legacy",
		DeviceType:    domain.DeviceTypeRouter,
		PollClass:     "", // legacy row: empty PollClass
		Managed:       true,
		Status:        domain.DeviceStatusUp,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.PollClass != domain.PollClassCore {
		t.Errorf("expected legacy empty PollClass healed to core (router->core), got %s", updated.PollClass)
	}
	if updated.PollIntervalOverride != nil {
		t.Errorf("expected PollIntervalOverride nil, got %v", updated.PollIntervalOverride)
	}
}

func TestProbeDevice_StaticDiscoveryPersistenceFailureStillMarksUp(t *testing.T) {
	result := &snmp.DiscoveryResult{
		SysName:    "router-persist-fail",
		DeviceType: domain.DeviceTypeRouter,
	}

	svc, deviceRepo, _ := newTestService(result, nil)
	deviceRepo.updateHook = func(device *domain.Device) error {
		if device.SysName == "router-persist-fail" {
			return fmt.Errorf("simulated static persistence failure")
		}
		return nil
	}

	device := &domain.Device{
		ID:            uuid.New(),
		IP:            "192.0.2.44",
		Hostname:      "router-persist-fail",
		DeviceType:    domain.DeviceTypeUnknown,
		PollClass:     domain.PollClassStandard,
		Managed:       true,
		Status:        domain.DeviceStatusProbing,
		MetricsSource: domain.MetricsSourceSNMP,
		SNMPCredentials: domain.SNMPCredentials{
			Version: domain.SNMPVersionV2c,
			V2c:     &domain.SNMPv2cCredentials{Community: "public"},
		},
	}
	if err := deviceRepo.Create(device); err != nil {
		t.Fatalf("failed to create device: %v", err)
	}

	svc.probeWg.Add(1)
	go func() {
		defer svc.probeWg.Done()
		svc.probeDevice(device)
	}()
	svc.WaitForProbes()

	updated, err := deviceRepo.GetByID(device.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.Status != domain.DeviceStatusUp {
		t.Fatalf("status = %s, want up after successful discovery", updated.Status)
	}
	if updated.SysName != "" {
		t.Fatalf("sysName = %q, want empty because static discovery persistence failed", updated.SysName)
	}
}
