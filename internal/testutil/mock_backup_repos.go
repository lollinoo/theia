package testutil

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// MockCredentialProfileRepo implements domain.CredentialProfileRepository.
type MockCredentialProfileRepo struct {
	Mu          sync.Mutex
	Profiles    map[uuid.UUID]*domain.CredentialProfile
	GetDeviceFn func(deviceID uuid.UUID) (*domain.CredentialProfile, error)
}

func NewMockCredentialProfileRepo() *MockCredentialProfileRepo {
	return &MockCredentialProfileRepo{
		Profiles: make(map[uuid.UUID]*domain.CredentialProfile),
	}
}

func (r *MockCredentialProfileRepo) Create(profile *domain.CredentialProfile) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	now := time.Now().UTC()
	profile.CreatedAt = now
	profile.UpdatedAt = now
	r.Profiles[profile.ID] = profile
	return nil
}

func (r *MockCredentialProfileRepo) GetByID(id uuid.UUID) (*domain.CredentialProfile, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	p, ok := r.Profiles[id]
	if !ok {
		return nil, fmt.Errorf("credential profile not found: %s", id)
	}
	cp := *p
	return &cp, nil
}

func (r *MockCredentialProfileRepo) GetAll() ([]domain.CredentialProfile, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	var result []domain.CredentialProfile
	for _, p := range r.Profiles {
		result = append(result, *p)
	}
	return result, nil
}

func (r *MockCredentialProfileRepo) Update(profile *domain.CredentialProfile) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if _, ok := r.Profiles[profile.ID]; !ok {
		return fmt.Errorf("credential profile not found: %s", profile.ID)
	}
	profile.UpdatedAt = time.Now().UTC()
	r.Profiles[profile.ID] = profile
	return nil
}

func (r *MockCredentialProfileRepo) Delete(id uuid.UUID) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if _, ok := r.Profiles[id]; !ok {
		return fmt.Errorf("credential profile not found: %s", id)
	}
	delete(r.Profiles, id)
	return nil
}

func (r *MockCredentialProfileRepo) GetBackupProfileForDevice(deviceID uuid.UUID) (*domain.CredentialProfile, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	if r.GetDeviceFn != nil {
		return r.GetDeviceFn(deviceID)
	}
	for _, p := range r.Profiles {
		cp := *p
		return &cp, nil
	}
	return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
}

// MockBulkBackupRunRepo implements domain.BulkBackupRunRepository for testing.
type MockBulkBackupRunRepo struct {
	Mu    sync.Mutex
	Runs  map[uuid.UUID]*domain.BulkBackupRun
	Items map[uuid.UUID][]domain.BulkBackupRunItem
}

func NewMockBulkBackupRunRepo() *MockBulkBackupRunRepo {
	return &MockBulkBackupRunRepo{
		Runs:  make(map[uuid.UUID]*domain.BulkBackupRun),
		Items: make(map[uuid.UUID][]domain.BulkBackupRunItem),
	}
}

func (r *MockBulkBackupRunRepo) CreateRun(run *domain.BulkBackupRun, items []domain.BulkBackupRunItem) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	if run.StartedAt == nil {
		started := now
		run.StartedAt = &started
	}
	cp := *run
	r.Runs[run.ID] = &cp
	copied := append([]domain.BulkBackupRunItem(nil), items...)
	r.Items[run.ID] = copied
	return nil
}

func (r *MockBulkBackupRunRepo) GetRun(id uuid.UUID) (*domain.BulkBackupRun, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	return r.CopyRunLocked(id), nil
}

func (r *MockBulkBackupRunRepo) GetLatestRun() (*domain.BulkBackupRun, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	var latest *domain.BulkBackupRun
	for _, run := range r.Runs {
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) {
			cp := *run
			latest = &cp
		}
	}
	if latest == nil {
		return nil, nil
	}
	latest.Items = append([]domain.BulkBackupRunItem(nil), r.Items[latest.ID]...)
	return latest, nil
}

func (r *MockBulkBackupRunRepo) GetActiveRun() (*domain.BulkBackupRun, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	for id, run := range r.Runs {
		if bulkRunActive(run.Status) {
			return r.CopyRunLocked(id), nil
		}
	}
	return nil, nil
}

func (r *MockBulkBackupRunRepo) ListResumableRuns() ([]domain.BulkBackupRun, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	var runs []domain.BulkBackupRun
	for id, run := range r.Runs {
		if run.Status == domain.BulkBackupRunStatusRunning ||
			run.Status == domain.BulkBackupRunStatusCancelling ||
			run.Status == domain.BulkBackupRunStatusPausing {
			cp := *run
			cp.Items = append([]domain.BulkBackupRunItem(nil), r.Items[id]...)
			runs = append(runs, cp)
		}
	}
	return runs, nil
}

func (r *MockBulkBackupRunRepo) UpdateRun(run *domain.BulkBackupRun) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	cp := *run
	r.Runs[run.ID] = &cp
	return nil
}

func (r *MockBulkBackupRunRepo) ListRunItems(runID uuid.UUID) ([]domain.BulkBackupRunItem, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	return append([]domain.BulkBackupRunItem(nil), r.Items[runID]...), nil
}

func (r *MockBulkBackupRunRepo) UpdateRunItem(item *domain.BulkBackupRunItem) error {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	items := r.Items[item.RunID]
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = *item
			r.Items[item.RunID] = items
			return nil
		}
	}
	r.Items[item.RunID] = append(items, *item)
	return nil
}

func (r *MockBulkBackupRunRepo) RecalculateRunCounters(runID uuid.UUID) (*domain.BulkBackupRun, error) {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	run := r.Runs[runID]
	if run == nil {
		return nil, nil
	}
	run.TotalCount = len(r.Items[runID])
	run.QueuedCount = 0
	run.SuccessCount = 0
	run.FailedCount = 0
	run.SkippedCount = 0
	run.CancelledCount = 0
	for _, item := range r.Items[runID] {
		switch item.Status {
		case domain.BulkBackupRunItemStatusActive,
			domain.BulkBackupRunItemStatusQueued,
			domain.BulkBackupRunItemStatusRunning,
			domain.BulkBackupRunItemStatusChecking:
			run.QueuedCount++
		case domain.BulkBackupRunItemStatusSuccess:
			run.SuccessCount++
		case domain.BulkBackupRunItemStatusFailed:
			run.FailedCount++
		case domain.BulkBackupRunItemStatusSkipped:
			run.SkippedCount++
		case domain.BulkBackupRunItemStatusCancelled:
			run.CancelledCount++
		}
	}
	return r.CopyRunLocked(runID), nil
}

func (r *MockBulkBackupRunRepo) CopyRunLocked(id uuid.UUID) *domain.BulkBackupRun {
	run := r.Runs[id]
	if run == nil {
		return nil
	}
	cp := *run
	cp.Items = append([]domain.BulkBackupRunItem(nil), r.Items[id]...)
	return &cp
}

func bulkRunActive(status domain.BulkBackupRunStatus) bool {
	return status == domain.BulkBackupRunStatusRunning ||
		status == domain.BulkBackupRunStatusCancelling ||
		status == domain.BulkBackupRunStatusPausing ||
		status == domain.BulkBackupRunStatusPaused
}
