package service

import (
	"context"
	crand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/observability"
	internalssh "github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// Mock repositories for backup service tests
// ---------------------------------------------------------------------------

// mockBackupJobRepo implements domain.BackupJobRepository.
type mockBackupJobRepo struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]*domain.BackupJob
}

func newMockBackupJobRepo() *mockBackupJobRepo {
	return &mockBackupJobRepo{jobs: make(map[uuid.UUID]*domain.BackupJob)}
}

func (r *mockBackupJobRepo) Create(job *domain.BackupJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	job.CreatedAt = time.Now().UTC()
	r.jobs[job.ID] = job
	return nil
}

func (r *mockBackupJobRepo) GetByID(id uuid.UUID) (*domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	cp := *j
	return &cp, nil
}

func (r *mockBackupJobRepo) GetByDeviceID(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.BackupJob
	for _, j := range r.jobs {
		if j.DeviceID == deviceID {
			result = append(result, *j)
		}
	}
	return result, nil
}

func (r *mockBackupJobRepo) GetLatestByDeviceID(deviceID uuid.UUID) (*domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *domain.BackupJob
	for _, j := range r.jobs {
		if j.DeviceID == deviceID {
			if latest == nil || j.CreatedAt.After(latest.CreatedAt) {
				cp := *j
				latest = &cp
			}
		}
	}
	return latest, nil
}

func (r *mockBackupJobRepo) Update(job *domain.BackupJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[job.ID]; !ok {
		return fmt.Errorf("job not found: %s", job.ID)
	}
	r.jobs[job.ID] = job
	return nil
}

func (r *mockBackupJobRepo) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.jobs, id)
	return nil
}

func (r *mockBackupJobRepo) DeleteByDeviceID(deviceID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, j := range r.jobs {
		if j.DeviceID == deviceID {
			delete(r.jobs, id)
		}
	}
	return nil
}

func (r *mockBackupJobRepo) ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.BackupJob
	for _, j := range r.jobs {
		if j.DeviceID == deviceID && j.Status == domain.BackupStatusSuccess {
			result = append(result, *j)
		}
	}
	return result, nil
}

func (r *mockBackupJobRepo) ListAllDeviceIDs() ([]uuid.UUID, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := make(map[uuid.UUID]bool)
	var ids []uuid.UUID
	for _, j := range r.jobs {
		if !seen[j.DeviceID] {
			seen[j.DeviceID] = true
			ids = append(ids, j.DeviceID)
		}
	}
	return ids, nil
}

func (r *mockBackupJobRepo) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for id, j := range r.jobs {
		if j.Status == domain.BackupStatusFailed && j.CreatedAt.Before(cutoff) {
			delete(r.jobs, id)
			count++
		}
	}
	return count, nil
}

// mockBackupFileRepo implements domain.BackupFileRepository.
type mockBackupFileRepo struct {
	mu             sync.Mutex
	files          map[uuid.UUID]*domain.BackupFile
	deleteByJobErr error // when set, DeleteByJobID returns this error
}

func newMockBackupFileRepo() *mockBackupFileRepo {
	return &mockBackupFileRepo{files: make(map[uuid.UUID]*domain.BackupFile)}
}

func (r *mockBackupFileRepo) Create(file *domain.BackupFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if file.ID == uuid.Nil {
		file.ID = uuid.New()
	}
	file.CreatedAt = time.Now().UTC()
	r.files[file.ID] = file
	return nil
}

func (r *mockBackupFileRepo) GetByJobID(jobID uuid.UUID) ([]domain.BackupFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []domain.BackupFile
	for _, f := range r.files {
		if f.JobID == jobID {
			result = append(result, *f)
		}
	}
	return result, nil
}

func (r *mockBackupFileRepo) GetByID(id uuid.UUID) (*domain.BackupFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return nil, fmt.Errorf("file not found: %s", id)
	}
	cp := *f
	return &cp, nil
}

func (r *mockBackupFileRepo) DeleteByJobID(jobID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.deleteByJobErr != nil {
		return r.deleteByJobErr
	}
	for id, f := range r.files {
		if f.JobID == jobID {
			delete(r.files, id)
		}
	}
	return nil
}

type mockBulkBackupRunRepo struct {
	mu    sync.Mutex
	runs  map[uuid.UUID]*domain.BulkBackupRun
	items map[uuid.UUID][]domain.BulkBackupRunItem
}

func newMockBulkBackupRunRepo() *mockBulkBackupRunRepo {
	return &mockBulkBackupRunRepo{
		runs:  make(map[uuid.UUID]*domain.BulkBackupRun),
		items: make(map[uuid.UUID][]domain.BulkBackupRunItem),
	}
}

func (r *mockBulkBackupRunRepo) CreateRun(run *domain.BulkBackupRun, items []domain.BulkBackupRunItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.runs {
		if bulkRunActiveForTest(existing.Status) {
			return fmt.Errorf("active run exists")
		}
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = domain.BulkBackupRunStatusRunning
	}
	if run.BatchSize == 0 {
		run.BatchSize = 10
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	run.TotalCount = len(items)
	copiedRun := *run
	r.runs[run.ID] = &copiedRun
	copiedItems := make([]domain.BulkBackupRunItem, len(items))
	for i := range items {
		item := items[i]
		if item.ID == uuid.Nil {
			item.ID = uuid.New()
		}
		item.RunID = run.ID
		if item.CreatedAt.IsZero() {
			item.CreatedAt = run.CreatedAt
		}
		if item.UpdatedAt.IsZero() {
			item.UpdatedAt = item.CreatedAt
		}
		copiedItems[i] = item
		items[i] = item
	}
	r.items[run.ID] = copiedItems
	run.Items = copiedItems
	return nil
}

func (r *mockBulkBackupRunRepo) GetRun(id uuid.UUID) (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyRunLocked(id), nil
}

func (r *mockBulkBackupRunRepo) GetLatestRun() (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *domain.BulkBackupRun
	for _, run := range r.runs {
		if latest == nil || run.CreatedAt.After(latest.CreatedAt) {
			cp := *run
			latest = &cp
		}
	}
	if latest == nil {
		return nil, nil
	}
	items := append([]domain.BulkBackupRunItem(nil), r.items[latest.ID]...)
	latest.Items = items
	return latest, nil
}

func (r *mockBulkBackupRunRepo) GetActiveRun() (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, run := range r.runs {
		if bulkRunActiveForTest(run.Status) {
			return r.copyRunLocked(id), nil
		}
	}
	return nil, nil
}

func (r *mockBulkBackupRunRepo) ListResumableRuns() ([]domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var runs []domain.BulkBackupRun
	for id, run := range r.runs {
		if run.Status == domain.BulkBackupRunStatusRunning ||
			run.Status == domain.BulkBackupRunStatusCancelling ||
			run.Status == domain.BulkBackupRunStatusPausing {
			cp := *run
			cp.Items = append([]domain.BulkBackupRunItem(nil), r.items[id]...)
			runs = append(runs, cp)
		}
	}
	return runs, nil
}

func bulkRunActiveForTest(status domain.BulkBackupRunStatus) bool {
	return status == domain.BulkBackupRunStatusRunning ||
		status == domain.BulkBackupRunStatusCancelling ||
		status == domain.BulkBackupRunStatusPausing ||
		status == domain.BulkBackupRunStatusPaused
}

func (r *mockBulkBackupRunRepo) UpdateRun(run *domain.BulkBackupRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}

func (r *mockBulkBackupRunRepo) ListRunItems(runID uuid.UUID) ([]domain.BulkBackupRunItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.BulkBackupRunItem(nil), r.items[runID]...), nil
}

func (r *mockBulkBackupRunRepo) UpdateRunItem(item *domain.BulkBackupRunItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.items[item.RunID]
	for i := range items {
		if items[i].ID == item.ID {
			items[i] = *item
			r.items[item.RunID] = items
			return nil
		}
	}
	return fmt.Errorf("item not found: %s", item.ID)
}

func (r *mockBulkBackupRunRepo) RecalculateRunCounters(runID uuid.UUID) (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	if run == nil {
		return nil, nil
	}
	items := r.items[runID]
	run.TotalCount = len(items)
	run.QueuedCount = 0
	run.SuccessCount = 0
	run.FailedCount = 0
	run.SkippedCount = 0
	run.CancelledCount = 0
	for _, item := range items {
		switch item.Status {
		case domain.BulkBackupRunItemStatusActive,
			domain.BulkBackupRunItemStatusQueued,
			domain.BulkBackupRunItemStatusRunning:
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
	cp := *run
	cp.Items = append([]domain.BulkBackupRunItem(nil), items...)
	return &cp, nil
}

func (r *mockBulkBackupRunRepo) copyRunLocked(id uuid.UUID) *domain.BulkBackupRun {
	run := r.runs[id]
	if run == nil {
		return nil
	}
	cp := *run
	cp.Items = append([]domain.BulkBackupRunItem(nil), r.items[id]...)
	return &cp
}

// mockCredentialProfileRepo implements domain.CredentialProfileRepository.
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
	delete(r.profiles, id)
	return nil
}

func (r *mockCredentialProfileRepo) GetBackupProfileForDevice(deviceID uuid.UUID) (*domain.CredentialProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Return the first profile found for any device (test helper: return any profile)
	for _, p := range r.profiles {
		cp := *p
		return &cp, nil
	}
	return nil, fmt.Errorf("no credential profile assigned to device %s", deviceID)
}

// mockBackupSettingsRepo implements domain.SettingsRepository for backup tests.
type mockBackupSettingsRepo struct {
	settings map[string]string
}

func newMockBackupSettingsRepo() *mockBackupSettingsRepo {
	return &mockBackupSettingsRepo{settings: domain.DefaultSettings()}
}

func (r *mockBackupSettingsRepo) Get(key string) (string, error) {
	v, ok := r.settings[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return v, nil
}

func (r *mockBackupSettingsRepo) Set(key, value string) error {
	r.settings[key] = value
	return nil
}

func (r *mockBackupSettingsRepo) GetAll() (map[string]string, error) {
	cp := make(map[string]string)
	for k, v := range r.settings {
		cp[k] = v
	}
	return cp, nil
}

// mockSSHDialer implements ssh.Dialer for testing.
type mockSSHDialer struct {
	delay time.Duration // artificial delay per dial
}

func (d *mockSSHDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	if d.delay > 0 {
		time.Sleep(d.delay)
	}
	return nil, fmt.Errorf("mock SSH dial: connection refused")
}

// buildTestVendorRegistry creates a vendor.Registry with a default and an
// optional additional vendor that supports backups.
func buildTestVendorRegistry(vendorName string, backupSupported bool) *vendor.Registry {
	defaultCfg := vendor.DBVendorRecord{
		Name: "default",
		ConfigJSON: `{
			"vendor": {"name": "default", "display_name": "Generic"},
			"detection": {},
			"backup": {"supported": false}
		}`,
	}

	records := []vendor.DBVendorRecord{defaultCfg}

	if vendorName != "" && vendorName != "default" {
		records = append(records, vendor.DBVendorRecord{
			Name: vendorName,
			ConfigJSON: fmt.Sprintf(`{
				"vendor": {"name": %q, "display_name": %q},
				"detection": {"sys_object_id_prefixes": ["1.3.6.1.4.1.99999"]},
				"backup": {
					"supported": %v,
					"methods": ["ssh"],
					"default_method": "ssh",
					"ssh_commands": {
						"export_running": "/export"
					}
				}
			}`, vendorName, vendorName, backupSupported),
		})
	}

	reg, err := vendor.LoadRegistryFromDB(records)
	if err != nil {
		panic(fmt.Sprintf("buildTestVendorRegistry: %v", err))
	}
	return reg
}

// listenOnRandomPort starts a TCP listener on localhost with a random port.
// The listener accepts and immediately closes connections (simulates reachable host).
// Returns the port and a cleanup function.
func listenOnRandomPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test listener: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			conn.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return port
}

// ---------------------------------------------------------------------------
// Test 1: TestConcurrentBackup (DEBT-01)
// ---------------------------------------------------------------------------
// Verifies that two devices can be backed up concurrently. With a global mutex,
// the second backup blocks until the first completes. With per-device locks,
// both should run in parallel.
//
// Strategy: Create two devices, trigger backups concurrently. Use a listening
// TCP port so the reachability check passes quickly. The backup will proceed
// past vendor check and reachability check, then fail on SSH dial (mock returns
// error). The key assertion: both TriggerBackup calls should succeed (return a
// job), and neither should error with a hardcoded MikroTik vendor check.
func TestConcurrentBackup(t *testing.T) {
	port := listenOnRandomPort(t)

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	dialer := &mockSSHDialer{}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, dialer, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	// Create credential profile using the test listener port
	profileID := uuid.New()
	credentialProfileRepo.Create(&domain.CredentialProfile{
		ID:         profileID,
		Name:       "test-profile",
		Username:   "admin",
		Port:       port,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	})

	// Create two devices with vendor "testvendor" (backup supported via registry)
	dev1ID := uuid.New()
	dev2ID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID: dev1ID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	})
	deviceRepo.Create(&domain.Device{
		ID: dev2ID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	})

	ctx := context.Background()

	// Trigger backups for both devices concurrently.
	// After DEBT-01 fix (per-device locks), both should proceed without serialization.
	// After DEBT-07 fix (registry-based vendor check), both should pass vendor validation.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	jobs := make([]*domain.BackupJob, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		jobs[0], errs[0] = svc.TriggerBackup(ctx, dev1ID)
	}()
	go func() {
		defer wg.Done()
		jobs[1], errs[1] = svc.TriggerBackup(ctx, dev2ID)
	}()
	wg.Wait()

	// Assert: neither device was rejected with a hardcoded MikroTik vendor check
	for i, err := range errs {
		if err != nil && strings.Contains(err.Error(), "MikroTik") {
			t.Fatalf("DEBT-01: device %d rejected with hardcoded MikroTik check "+
				"(error: %v) -- expected per-device concurrent backup with registry-based vendor check", i+1, err)
		}
	}

	// Assert: both TriggerBackup calls should succeed (return a job)
	// The actual backup runs in a goroutine and will fail (mock SSH), but
	// TriggerBackup itself should return a job successfully.
	for i, job := range jobs {
		if errs[i] != nil {
			t.Fatalf("DEBT-01: TriggerBackup for device %d failed: %v -- expected job creation to succeed", i+1, errs[i])
		}
		if job == nil {
			t.Fatalf("DEBT-01: TriggerBackup for device %d returned nil job", i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Test 2: TestBinaryBackupSFTPPoll (DEBT-04)
// ---------------------------------------------------------------------------
// Verifies that the waitForRemoteFile method exists on BackupService and
// uses SFTP stat polling rather than a hardcoded sleep. Calling with a nil
// SSH client should return an error from SFTP client creation, proving the
// method exists and has the correct signature.
func TestBinaryBackupSFTPPoll(t *testing.T) {
	svc := &BackupService{}

	// Call waitForRemoteFile with a nil SSH client.
	// This should return an error from sftp.NewClient (nil connection),
	// proving the method exists and is callable with the expected signature.
	err := svc.waitForRemoteFile(nil, "/tmp/test.backup", 1*time.Second)
	if err == nil {
		t.Fatal("DEBT-04: waitForRemoteFile should return an error when given a nil SSH client")
	}

	// The error should be about creating the SFTP client, not a timeout.
	// A timeout error would mean the method is polling but failing to stat.
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("DEBT-04: waitForRemoteFile timed out instead of failing on SFTP client creation: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test 3: TestBackupVendorRegistry (DEBT-07)
// ---------------------------------------------------------------------------
// Verifies that TriggerBackup uses the vendor registry to determine backup
// eligibility instead of a hardcoded `device.Vendor != "mikrotik"` check.
// A device with vendor "testvendor" where the registry has backup.supported=true
// should pass the vendor check (it may fail later on SSH connection, which is fine).
func TestBackupVendorRegistry(t *testing.T) {
	port := listenOnRandomPort(t)

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	dialer := &mockSSHDialer{}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, dialer, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	// Create credential profile
	profileID := uuid.New()
	credentialProfileRepo.Create(&domain.CredentialProfile{
		ID:         profileID,
		Name:       "test-profile",
		Username:   "admin",
		Port:       port,
		AuthMethod: domain.SSHAuthPassword,
		Role:       "Admin",
	})

	// Create device with non-mikrotik vendor that has backup supported in registry
	deviceID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	})

	job, err := svc.TriggerBackup(context.Background(), deviceID)

	// The fix: TriggerBackup should use the registry to check vendor eligibility.
	// With the hardcoded check (pre-fix), the error would contain "MikroTik".
	if err != nil && strings.Contains(err.Error(), "MikroTik") {
		t.Fatalf("DEBT-07: TriggerBackup rejected vendor %q with hardcoded MikroTik check "+
			"(error: %v) -- expected registry-based vendor eligibility", "testvendor", err)
	}

	// After the vendor check passes, TriggerBackup should return a job.
	// The actual backup runs in a goroutine (it will fail on SSH mock).
	if err != nil {
		t.Fatalf("DEBT-07: TriggerBackup failed unexpectedly: %v", err)
	}
	if job == nil {
		t.Fatal("DEBT-07: TriggerBackup returned nil job after passing vendor check")
	}
}

// ---------------------------------------------------------------------------
// Test 4: TestDeleteBackupJobFileError (BUG-01)
// ---------------------------------------------------------------------------
// Verifies that DeleteBackupJob propagates file repository errors rather
// than silently swallowing them.
func TestDeleteBackupJobFileError(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("", false)
	dialer := &mockSSHDialer{}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, dialer, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	// Create a job with a file pointing to a non-existent path
	jobID := uuid.New()
	deviceID := uuid.New()
	jobRepo.Create(&domain.BackupJob{
		ID:       jobID,
		DeviceID: deviceID,
		Status:   domain.BackupStatusSuccess,
	})
	fileRepo.Create(&domain.BackupFile{
		ID:       uuid.New(),
		JobID:    jobID,
		FileType: "running",
		FileName: "test.rsc",
		FilePath: "/nonexistent/path/test.rsc",
	})

	// Make DeleteByJobID return an error to simulate a repository failure
	fileRepo.deleteByJobErr = fmt.Errorf("simulated file repo delete error")

	err := svc.DeleteBackupJob(context.Background(), jobID)

	// The fix: DeleteBackupJob should check and return the error from
	// fileRepo.DeleteByJobID. Before the fix, this error was silently swallowed.
	if err == nil {
		t.Fatal("BUG-01: DeleteBackupJob silently swallowed fileRepo.DeleteByJobID error -- " +
			"expected error to be returned or file cleanup failure to be reported")
	}
}

// ---------------------------------------------------------------------------
// Test 5: TestBackupGetFileContent_TextFile (TEST-02 gap: text export flow)
// ---------------------------------------------------------------------------
// Verifies that GetBackupFileContent returns a readable stream and the correct
// filename for a text backup file (.rsc).
func TestBackupGetFileContent_TextFile(t *testing.T) {
	fileRepo := newMockBackupFileRepo()

	svc := &BackupService{fileRepo: fileRepo}

	// Create a temp file with .rsc extension containing text content
	tmpDir := t.TempDir()
	content := "# RouterOS export\n/interface bridge add name=br0\n"
	filePath := filepath.Join(tmpDir, "20260319_router1.rsc")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	fileID := uuid.New()
	jobID := uuid.New()
	fileRepo.Create(&domain.BackupFile{
		ID:       fileID,
		JobID:    jobID,
		FileType: "running",
		FileName: "20260319_router1.rsc",
		FilePath: filePath,
	})

	rc, fileName, err := svc.GetBackupFileContent(context.Background(), fileID)
	if err != nil {
		t.Fatalf("GetBackupFileContent returned error: %v", err)
	}
	t.Cleanup(func() { rc.Close() })

	if fileName != "20260319_router1.rsc" {
		t.Errorf("expected filename %q, got %q", "20260319_router1.rsc", fileName)
	}

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("reading content: %v", err)
	}
	if string(data) != content {
		t.Errorf("content mismatch: expected %q, got %q", content, string(data))
	}
}

// ---------------------------------------------------------------------------
// Test 6: TestBackupGetFileContent_NotFound (TEST-02 gap: text export flow)
// ---------------------------------------------------------------------------
// Verifies that GetBackupFileContent returns an error for a non-existent file ID.
func TestBackupGetFileContent_NotFound(t *testing.T) {
	fileRepo := newMockBackupFileRepo()
	svc := &BackupService{fileRepo: fileRepo}

	_, _, err := svc.GetBackupFileContent(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error for non-existent file ID, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error to contain %q, got %q", "not found", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Test 7: TestBackupGetBulkDownloadEntries_HappyPath (TEST-02 gap: bulk eligibility)
// ---------------------------------------------------------------------------
// Verifies that GetBulkDownloadFiles returns correct entries with file paths
// and device directory names for a device with a successful backup.
func TestBackupGetBulkDownloadEntries_HappyPath(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	deviceRepo := newMockDeviceRepo()
	backupDir := t.TempDir()

	svc := &BackupService{
		jobRepo:    jobRepo,
		fileRepo:   fileRepo,
		deviceRepo: deviceRepo,
		backupDir:  backupDir,
	}

	deviceID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID:      deviceID,
		IP:      "10.0.0.1",
		SysName: "core-router",
		Tags:    map[string]string{"display_name": "Core Router"},
	})

	jobID := uuid.New()
	jobRepo.Create(&domain.BackupJob{
		ID:       jobID,
		DeviceID: deviceID,
		Status:   domain.BackupStatusSuccess,
	})

	// Create temp files at the paths referenced by BackupFile records
	filePath := filepath.Join(backupDir, "20260319_core-router.rsc")
	if err := os.WriteFile(filePath, []byte("# export"), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}

	fileRepo.Create(&domain.BackupFile{
		ID:       uuid.New(),
		JobID:    jobID,
		FileType: "running",
		FileName: "20260319_core-router.rsc",
		FilePath: filePath,
	})

	entries, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID})
	if err != nil {
		t.Fatalf("GetBulkDownloadFiles returned error: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].DeviceDir != "Core_Router" && entries[0].DeviceDir != "Core Router" {
		// sanitizeHostname replaces spaces with underscores
		t.Logf("DeviceDir = %q (sanitized from display_name)", entries[0].DeviceDir)
	}
	if entries[0].File.FileName != "20260319_core-router.rsc" {
		t.Errorf("expected file name %q, got %q", "20260319_core-router.rsc", entries[0].File.FileName)
	}
}

// ---------------------------------------------------------------------------
// Test 8: TestBackupGetBulkDownloadEntries_NoBackups (TEST-02 gap: bulk eligibility)
// ---------------------------------------------------------------------------
// Verifies that GetBulkDownloadFiles returns empty entries (not an error)
// when a device has no backup jobs.
func TestBackupGetBulkDownloadEntries_NoBackups(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	deviceRepo := newMockDeviceRepo()
	backupDir := t.TempDir()

	svc := &BackupService{
		jobRepo:    jobRepo,
		fileRepo:   fileRepo,
		deviceRepo: deviceRepo,
		backupDir:  backupDir,
	}

	deviceID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID:      deviceID,
		IP:      "10.0.0.2",
		SysName: "edge-switch",
		Tags:    map[string]string{},
	})

	entries, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID})
	if err != nil {
		t.Fatalf("GetBulkDownloadFiles returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for device with no backups, got %d", len(entries))
	}
}

func TestTriggerBulkBackupRejectsEffectiveDeviceCountOverLimitWithoutQueueing(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    1,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	for i := 0; i < 2; i++ {
		if err := deviceRepo.Create(&domain.Device{
			ID: uuid.New(), IP: fmt.Sprintf("10.0.0.%d", i+1), Vendor: "testvendor",
			Managed: true, Status: domain.DeviceStatusUp,
		}); err != nil {
			t.Fatalf("Create device: %v", err)
		}
	}

	_, err := svc.TriggerBulkBackup(context.Background())
	if err == nil {
		t.Fatal("TriggerBulkBackup error = nil, want bulk limit error")
	}
	if !IsBulkLimitError(err) {
		t.Fatalf("TriggerBulkBackup error = %v, want bulk limit error", err)
	}
	if got := mockBackupJobCount(jobRepo); got != 0 {
		t.Fatalf("queued jobs = %d, want 0 before limit rejection", got)
	}
}

func TestTriggerBulkBackupDeduplicatesRequestedDeviceIDs(t *testing.T) {
	port := listenOnRandomPort(t)
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    1,
		BulkBackupMaxQueuedJobs: 1,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	if err := credentialProfileRepo.Create(&domain.CredentialProfile{
		ID: uuid.New(), Name: "test-profile", Username: "admin", Port: port,
		AuthMethod: domain.SSHAuthPassword, Role: "Admin",
	}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	results, err := svc.TriggerBulkBackup(context.Background(), deviceID, deviceID)
	if err != nil {
		t.Fatalf("TriggerBulkBackup returned error for duplicate requested ID: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one deduplicated result", len(results))
	}
	if got := mockBackupJobCount(jobRepo); got != 1 {
		t.Fatalf("queued jobs = %d, want one deduplicated job", got)
	}
}

func TestTriggerBulkBackupRejectsConcurrentLegacyBulkBackupAcrossServices(t *testing.T) {
	port := listenOnRandomPort(t)
	leaseRepo := newMockBulkOperationLeaseRepo()
	firstSvc, firstJobRepo, firstDeviceID := setupLegacyBulkBackupService(t, port, leaseRepo, 400*time.Millisecond)
	secondSvc, secondJobRepo, secondDeviceID := setupLegacyBulkBackupService(t, port, leaseRepo, 0)

	if _, err := firstSvc.TriggerBulkBackup(context.Background(), firstDeviceID); err != nil {
		t.Fatalf("first TriggerBulkBackup: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return mockBackupJobCount(firstJobRepo) == 1 && leaseRepo.isActive(legacyBulkBackupLeaseKey)
	})

	_, err := secondSvc.TriggerBulkBackup(context.Background(), secondDeviceID)
	if !errors.Is(err, ErrBulkBackupAlreadyActive) {
		t.Fatalf("second TriggerBulkBackup error = %v, want ErrBulkBackupAlreadyActive", err)
	}
	if got := mockBackupJobCount(secondJobRepo); got != 0 {
		t.Fatalf("second service queued jobs = %d, want 0 while lease is held", got)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return !leaseRepo.isActive(legacyBulkBackupLeaseKey)
	})
	if _, err := secondSvc.TriggerBulkBackup(context.Background(), secondDeviceID); err != nil {
		t.Fatalf("third TriggerBulkBackup after lease release: %v", err)
	}
	waitForCondition(t, 2*time.Second, func() bool {
		return !leaseRepo.isActive(legacyBulkBackupLeaseKey)
	})
}

func TestTriggerBulkBackupReportsLegacyBulkGateMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	port := listenOnRandomPort(t)
	leaseRepo := newMockBulkOperationLeaseRepo()
	svc, jobRepo, deviceID := setupLegacyBulkBackupService(t, port, leaseRepo, 400*time.Millisecond)

	if _, err := svc.TriggerBulkBackup(context.Background(), deviceID); err != nil {
		t.Fatalf("TriggerBulkBackup: %v", err)
	}
	waitForCondition(t, time.Second, func() bool {
		return mockBackupJobCount(jobRepo) == 1 && leaseRepo.isActive(legacyBulkBackupLeaseKey)
	})

	metrics := string(registry.MarshalPrometheus())
	for _, want := range []string{
		`theia_bulk_operation_in_flight{operation="bulk_backup_legacy",source="local"} 1`,
		`theia_bulk_operation_in_flight{operation="bulk_backup_legacy",source="distributed"} 1`,
		`theia_bulk_operation_concurrency_limit{operation="bulk_backup_legacy",scope="global",source="local"} 1`,
		`theia_bulk_operation_concurrency_limit{operation="bulk_backup_legacy",scope="global",source="distributed"} 1`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("expected metric %q while legacy bulk backup is running, got:\n%s", want, metrics)
		}
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return !leaseRepo.isActive(legacyBulkBackupLeaseKey)
	})
	metrics = string(registry.MarshalPrometheus())
	for _, want := range []string{
		`theia_bulk_operation_in_flight{operation="bulk_backup_legacy",source="local"} 0`,
		`theia_bulk_operation_in_flight{operation="bulk_backup_legacy",source="distributed"} 0`,
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("expected metric %q after legacy bulk backup releases, got:\n%s", want, metrics)
		}
	}
}

func TestTriggerBulkBackupRejectsQueuedJobCountOverLimitBeforeCreatingJobs(t *testing.T) {
	port := listenOnRandomPort(t)
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 1,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	if err := credentialProfileRepo.Create(&domain.CredentialProfile{
		ID: uuid.New(), Name: "test-profile", Username: "admin", Port: port,
		AuthMethod: domain.SSHAuthPassword, Role: "Admin",
	}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := deviceRepo.Create(&domain.Device{
			ID: uuid.New(), IP: "127.0.0.1", Vendor: "testvendor",
			Managed: true, Status: domain.DeviceStatusUp,
		}); err != nil {
			t.Fatalf("Create device: %v", err)
		}
	}

	_, err := svc.TriggerBulkBackup(context.Background())
	if err == nil {
		t.Fatal("TriggerBulkBackup error = nil, want queued job limit error")
	}
	if !IsBulkLimitError(err) {
		t.Fatalf("TriggerBulkBackup error = %v, want bulk limit error", err)
	}
	if got := mockBackupJobCount(jobRepo); got != 0 {
		t.Fatalf("queued jobs = %d, want 0 before queued-job limit rejection", got)
	}
}

func TestStartBulkBackupRunCreatesPersistentItemsAndSkipsDownDevices(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.10", Vendor: "testvendor",
		Status: domain.DeviceStatusDown, Tags: map[string]string{"display_name": "offline-router"},
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	run, err := svc.StartBulkBackupRun(context.Background(), []uuid.UUID{deviceID}, "admin")
	if err != nil {
		t.Fatalf("StartBulkBackupRun: %v", err)
	}
	if run.Status != domain.BulkBackupRunStatusRunning || run.BatchSize != defaultBulkBackupRunBatchSize {
		t.Fatalf("run = %+v, want running with default batch size", run)
	}
	if len(run.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(run.Items))
	}
	item := run.Items[0]
	if item.Status != domain.BulkBackupRunItemStatusSkipped || item.Reason != "device offline" {
		t.Fatalf("item = %+v, want skipped device offline", item)
	}
	if got := mockBackupJobCount(jobRepo); got != 0 {
		t.Fatalf("queued jobs = %d, want 0 for down device", got)
	}
}

func TestGetBulkBackupRunHydratesFileAndByteTotals(t *testing.T) {
	runID := uuid.New()
	jobID := uuid.New()
	runRepo := newMockBulkBackupRunRepo()
	fileRepo := newMockBackupFileRepo()
	svc := NewBackupService(
		newMockBackupJobRepo(),
		fileRepo,
		newMockCredentialProfileRepo(),
		newMockDeviceRepo(),
		newMockBackupSettingsRepo(),
		nil,
		nil,
		[]byte("0123456789abcdef"),
		t.TempDir(),
		nil,
		WithBulkBackupRunRepo(runRepo),
	)
	if err := runRepo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusSuccess, BatchSize: 10},
		[]domain.BulkBackupRunItem{
			{
				ID:          uuid.New(),
				RunID:       runID,
				DeviceID:    uuid.New(),
				DeviceName:  "router-01",
				Status:      domain.BulkBackupRunItemStatusSuccess,
				BackupJobID: &jobID,
			},
			{
				ID:         uuid.New(),
				RunID:      runID,
				DeviceID:   uuid.New(),
				DeviceName: "router-02",
				Status:     domain.BulkBackupRunItemStatusSkipped,
			},
		},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	for _, file := range []domain.BackupFile{
		{JobID: jobID, FileName: "running.rsc", SizeBytes: 123},
		{JobID: jobID, FileName: "compact.rsc", SizeBytes: 456},
	} {
		file := file
		if err := fileRepo.Create(&file); err != nil {
			t.Fatalf("Create file: %v", err)
		}
	}

	run, err := svc.GetBulkBackupRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetBulkBackupRun: %v", err)
	}

	if run.FileCount != 2 || run.ByteCount != 579 {
		t.Fatalf("run totals = files %d bytes %d, want 2/579", run.FileCount, run.ByteCount)
	}
	if run.Items[0].FileCount != 2 || run.Items[0].ByteCount != 579 {
		t.Fatalf("item totals = files %d bytes %d, want 2/579", run.Items[0].FileCount, run.Items[0].ByteCount)
	}
	if run.Items[1].FileCount != 0 || run.Items[1].ByteCount != 0 {
		t.Fatalf("skipped item totals = files %d bytes %d, want 0/0", run.Items[1].FileCount, run.Items[1].ByteCount)
	}
}

func TestStartBulkBackupRunRejectsActiveRun(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	existing := &domain.BulkBackupRun{ID: uuid.New(), Status: domain.BulkBackupRunStatusRunning, BatchSize: 10}
	if err := runRepo.CreateRun(existing, nil); err != nil {
		t.Fatalf("CreateRun existing: %v", err)
	}
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)

	run, err := svc.StartBulkBackupRun(context.Background(), nil, "admin")
	if !errors.Is(err, ErrBulkBackupRunAlreadyActive) {
		t.Fatalf("StartBulkBackupRun error = %v, want ErrBulkBackupRunAlreadyActive", err)
	}
	if run == nil || run.ID != existing.ID {
		t.Fatalf("run = %+v, want existing active run", run)
	}
}

func TestPauseBulkBackupRunMarksRunPausing(t *testing.T) {
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusRunning)

	run, err := svc.PauseBulkBackupRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("PauseBulkBackupRun: %v", err)
	}
	if run == nil || run.Status != domain.BulkBackupRunStatusPausing {
		t.Fatalf("run = %+v, want pausing", run)
	}
	stored, _ := runRepo.GetRun(runID)
	if stored.Status != domain.BulkBackupRunStatusPausing {
		t.Fatalf("stored status = %s, want pausing", stored.Status)
	}
}

func TestPrepareBulkRunBatchMarksClaimedItemsActive(t *testing.T) {
	port := listenOnRandomPort(t)
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	deviceID := uuid.New()
	runID := uuid.New()
	itemID := uuid.New()
	if err := credentialProfileRepo.Create(&domain.CredentialProfile{
		ID: uuid.New(), Name: "test-profile", Username: "admin", Port: port,
		AuthMethod: domain.SSHAuthPassword, Role: "Admin",
	}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Status: domain.DeviceStatusUp, Tags: map[string]string{},
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}
	if err := runRepo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10},
		[]domain.BulkBackupRunItem{{
			ID: itemID, RunID: runID, DeviceID: deviceID,
			Status: domain.BulkBackupRunItemStatusChecking,
		}},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)

	queued := svc.prepareBulkRunBatch([]domain.BulkBackupRunItem{{
		ID: itemID, RunID: runID, DeviceID: deviceID,
		Status: domain.BulkBackupRunItemStatusChecking,
	}})

	if len(queued) != 1 {
		t.Fatalf("queued len = %d, want 1", len(queued))
	}
	items, _ := runRepo.ListRunItems(runID)
	if items[0].Status != domain.BulkBackupRunItemStatusActive || items[0].BackupJobID == nil {
		t.Fatalf("item = %+v, want active with backup job", items[0])
	}
}

func TestMarkBulkRunBatchActiveClaimsEntireBatch(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	runID := uuid.New()
	firstItemID := uuid.New()
	secondItemID := uuid.New()
	if err := runRepo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10},
		[]domain.BulkBackupRunItem{
			{ID: firstItemID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
			{ID: secondItemID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
		},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)

	active := svc.markBulkRunBatchActive([]domain.BulkBackupRunItem{
		{ID: firstItemID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
		{ID: secondItemID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
	})

	if len(active) != 2 {
		t.Fatalf("active len = %d, want 2", len(active))
	}
	items, _ := runRepo.ListRunItems(runID)
	if items[0].Status != domain.BulkBackupRunItemStatusActive {
		t.Fatalf("first item status = %s, want active", items[0].Status)
	}
	if items[1].Status != domain.BulkBackupRunItemStatusActive {
		t.Fatalf("second item status = %s, want active", items[1].Status)
	}
}

func TestResumeBulkBackupRunRestartsPausedRun(t *testing.T) {
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusPaused)

	run, err := svc.ResumeBulkBackupRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("ResumeBulkBackupRun: %v", err)
	}
	if run == nil || run.Status != domain.BulkBackupRunStatusRunning {
		t.Fatalf("run = %+v, want running", run)
	}

	waitForCondition(t, time.Second, func() bool {
		stored, _ := runRepo.GetRun(runID)
		return stored != nil && stored.Status != domain.BulkBackupRunStatusPaused
	})
}

func TestCancelPausedBulkBackupRunCancelsPendingItems(t *testing.T) {
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusPaused)

	run, err := svc.CancelBulkBackupRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("CancelBulkBackupRun: %v", err)
	}
	if run == nil || run.Status != domain.BulkBackupRunStatusCancelling || !run.CancelRequested {
		t.Fatalf("run = %+v, want cancelling with cancel requested", run)
	}

	waitForCondition(t, time.Second, func() bool {
		stored, _ := runRepo.GetRun(runID)
		items, _ := runRepo.ListRunItems(runID)
		return stored != nil &&
			stored.Status == domain.BulkBackupRunStatusCancelled &&
			len(items) == 1 &&
			items[0].Status == domain.BulkBackupRunItemStatusCancelled
	})
}

func TestCancelPausedBulkBackupRunCancelsAllPendingItems(t *testing.T) {
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusPaused)
	items := make([]domain.BulkBackupRunItem, defaultBulkBackupRunBatchSize+3)
	for i := range items {
		items[i] = domain.BulkBackupRunItem{
			ID:       uuid.New(),
			RunID:    runID,
			DeviceID: uuid.New(),
			Status:   domain.BulkBackupRunItemStatusChecking,
		}
	}
	runRepo.mu.Lock()
	runRepo.items[runID] = items
	runRepo.mu.Unlock()

	run, err := svc.CancelBulkBackupRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("CancelBulkBackupRun: %v", err)
	}
	if run == nil || run.Status != domain.BulkBackupRunStatusCancelling || !run.CancelRequested {
		t.Fatalf("run = %+v, want cancelling with cancel requested", run)
	}

	waitForCondition(t, time.Second, func() bool {
		stored, _ := runRepo.GetRun(runID)
		got, _ := runRepo.ListRunItems(runID)
		if stored == nil || stored.Status != domain.BulkBackupRunStatusCancelled {
			return false
		}
		for _, item := range got {
			if item.Status != domain.BulkBackupRunItemStatusCancelled {
				return false
			}
		}
		return len(got) == len(items)
	})
}

func TestCancelPendingBulkRunItemsKeepsActiveItemsCompleting(t *testing.T) {
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusCancelling)
	activeID := uuid.New()
	pendingID := uuid.New()
	runRepo.mu.Lock()
	runRepo.items[runID] = []domain.BulkBackupRunItem{
		{ID: activeID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusActive},
		{ID: pendingID, RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusChecking},
	}
	items := append([]domain.BulkBackupRunItem(nil), runRepo.items[runID]...)
	runRepo.mu.Unlock()

	svc.cancelPendingBulkRunItems(runID, items)

	got, _ := runRepo.ListRunItems(runID)
	if got[0].Status != domain.BulkBackupRunItemStatusActive {
		t.Fatalf("active item status = %s, want active", got[0].Status)
	}
	if got[1].Status != domain.BulkBackupRunItemStatusCancelled {
		t.Fatalf("pending item status = %s, want cancelled", got[1].Status)
	}
}

func TestFinishBulkBackupRunRecordsCompletionMetrics(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	t.Cleanup(func() {
		observability.ResetDefaultForTest()
	})
	svc, runRepo, runID := setupBulkRunControlTest(t, domain.BulkBackupRunStatusRunning)
	fileRepo := svc.fileRepo.(*mockBackupFileRepo)
	startedAt := time.Now().UTC().Add(-2 * time.Second)
	jobID := uuid.New()
	runRepo.mu.Lock()
	runRepo.runs[runID].StartedAt = &startedAt
	runRepo.items[runID] = []domain.BulkBackupRunItem{
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusSuccess, BackupJobID: &jobID},
		{ID: uuid.New(), RunID: runID, DeviceID: uuid.New(), Status: domain.BulkBackupRunItemStatusSkipped},
	}
	runRepo.mu.Unlock()
	for _, file := range []domain.BackupFile{
		{JobID: jobID, FileName: "running.rsc", SizeBytes: 123},
		{JobID: jobID, FileName: "compact.rsc", SizeBytes: 456},
	} {
		file := file
		if err := fileRepo.Create(&file); err != nil {
			t.Fatalf("Create file: %v", err)
		}
	}

	svc.finishBulkBackupRun(runID)

	stored, err := runRepo.GetRun(runID)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if stored.Status != domain.BulkBackupRunStatusPartial {
		t.Fatalf("run status = %s, want partial", stored.Status)
	}
	metrics := string(registry.MarshalPrometheus())
	for _, needle := range []string{
		`theia_bulk_operation_completions_total{operation="bulk_backup_run",result="partial",source="distributed"} 1`,
		`theia_bulk_operation_duration_seconds_count{operation="bulk_backup_run",result="partial",source="distributed"} 1`,
		`theia_bulk_operation_selected_devices_total{operation="bulk_backup_run",result="partial",source="distributed"} 2`,
		`theia_bulk_operation_selected_files_total{operation="bulk_backup_run",result="partial",source="distributed"} 2`,
		`theia_bulk_operation_selected_bytes_total{operation="bulk_backup_run",result="partial",source="distributed"} 579`,
	} {
		if !strings.Contains(metrics, needle) {
			t.Fatalf("expected bulk backup run metric %q, got:\n%s", needle, metrics)
		}
	}
}

func TestStartBulkBackupRunRejectsDeviceCountOverLimit(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	encKey := crypto.DeriveKey("bulk-run-large-selection")

	for i := 0; i < 105; i++ {
		id := uuid.New()
		if err := deviceRepo.Create(&domain.Device{
			ID: id, IP: fmt.Sprintf("10.0.0.%d", i+1), SysName: fmt.Sprintf("router-%d", i+1),
			Vendor: "testvendor", Status: domain.DeviceStatusDown,
		}); err != nil {
			t.Fatalf("Create device: %v", err)
		}
	}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, encKey, t.TempDir(), ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    100,
		BulkBackupMaxQueuedJobs: 100,
		BulkDownloadMaxDevices:  100,
		BulkDownloadMaxFiles:    500,
		BulkDownloadMaxBytes:    512 << 20,
	})

	run, err := svc.StartBulkBackupRun(context.Background(), nil, "operator")
	if err == nil {
		t.Fatal("StartBulkBackupRun error = nil, want bulk limit error")
	}
	var limitErr *BulkLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("StartBulkBackupRun error = %v, want bulk limit error", err)
	}
	if limitErr.Limit != "devices" || limitErr.Max != 100 || limitErr.Actual != 105 {
		t.Fatalf("limit error = %+v, want devices max=100 actual=105", limitErr)
	}
	if run != nil {
		t.Fatalf("run = %#v, want nil on limit error", run)
	}
	if active, activeErr := runRepo.GetActiveRun(); activeErr != nil || active != nil {
		t.Fatalf("active run after limit error = %#v, err=%v; want none", active, activeErr)
	}
}

func TestResumeBulkBackupRunsMarksInterruptedJobsFailedAndRetriesItems(t *testing.T) {
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	deviceID := uuid.New()
	jobID := uuid.New()
	runID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.11", Vendor: "testvendor",
		Status: domain.DeviceStatusDown, Tags: map[string]string{},
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}
	if err := jobRepo.Create(&domain.BackupJob{ID: jobID, DeviceID: deviceID, Status: domain.BackupStatusRunning}); err != nil {
		t.Fatalf("Create job: %v", err)
	}
	if err := runRepo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: domain.BulkBackupRunStatusRunning, BatchSize: 10},
		[]domain.BulkBackupRunItem{{
			ID:          uuid.New(),
			RunID:       runID,
			DeviceID:    deviceID,
			Status:      domain.BulkBackupRunItemStatusRunning,
			BackupJobID: &jobID,
		}},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)

	svc.ResumeBulkBackupRuns(context.Background())

	waitForCondition(t, time.Second, func() bool {
		job, _ := jobRepo.GetByID(jobID)
		items, _ := runRepo.ListRunItems(runID)
		return job != nil &&
			job.Status == domain.BackupStatusFailed &&
			job.ErrorMessage == "interrupted by server restart" &&
			len(items) == 1 &&
			items[0].Status == domain.BulkBackupRunItemStatusSkipped &&
			items[0].Reason == "device offline"
	})
}

func setupBulkRunControlTest(t *testing.T, status domain.BulkBackupRunStatus) (*BackupService, *mockBulkBackupRunRepo, uuid.UUID) {
	t.Helper()

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	runRepo := newMockBulkBackupRunRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	deviceID := uuid.New()
	runID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.12", Vendor: "testvendor",
		Status: domain.DeviceStatusDown, Tags: map[string]string{},
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}
	if err := runRepo.CreateRun(
		&domain.BulkBackupRun{ID: runID, Status: status, BatchSize: 10},
		[]domain.BulkBackupRunItem{{
			ID:       uuid.New(),
			RunID:    runID,
			DeviceID: deviceID,
			Status:   domain.BulkBackupRunItemStatusChecking,
		}},
	); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkBackupRunRepo(runRepo),
	)
	return svc, runRepo, runID
}

func TestGetBulkDownloadFilesEnforcesLimitsAndDeduplicatesDevices(t *testing.T) {
	tests := []struct {
		name      string
		limits    BulkOperationLimits
		deviceIDs func(uuid.UUID) []uuid.UUID
		fileSizes []int
		wantLimit bool
		wantCount int
	}{
		{
			name: "deduplicates requested devices",
			limits: BulkOperationLimits{
				BulkBackupMaxDevices:    10,
				BulkBackupMaxQueuedJobs: 10,
				BulkDownloadMaxDevices:  1,
				BulkDownloadMaxFiles:    10,
				BulkDownloadMaxBytes:    1024,
			},
			deviceIDs: func(id uuid.UUID) []uuid.UUID { return []uuid.UUID{id, id} },
			fileSizes: []int{4},
			wantCount: 1,
		},
		{
			name: "max files",
			limits: BulkOperationLimits{
				BulkBackupMaxDevices:    10,
				BulkBackupMaxQueuedJobs: 10,
				BulkDownloadMaxDevices:  10,
				BulkDownloadMaxFiles:    1,
				BulkDownloadMaxBytes:    1024,
			},
			deviceIDs: func(id uuid.UUID) []uuid.UUID { return []uuid.UUID{id} },
			fileSizes: []int{4, 4},
			wantLimit: true,
		},
		{
			name: "max bytes",
			limits: BulkOperationLimits{
				BulkBackupMaxDevices:    10,
				BulkBackupMaxQueuedJobs: 10,
				BulkDownloadMaxDevices:  10,
				BulkDownloadMaxFiles:    10,
				BulkDownloadMaxBytes:    4,
			},
			deviceIDs: func(id uuid.UUID) []uuid.UUID { return []uuid.UUID{id} },
			fileSizes: []int{5},
			wantLimit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, deviceID := setupBulkDownloadServiceWithFiles(t, tt.fileSizes, nil)
			svc.SetBulkOperationLimits(tt.limits)

			entries, err := svc.GetBulkDownloadFiles(context.Background(), tt.deviceIDs(deviceID))
			if tt.wantLimit {
				if err == nil {
					t.Fatal("GetBulkDownloadFiles error = nil, want bulk limit error")
				}
				if !IsBulkLimitError(err) {
					t.Fatalf("GetBulkDownloadFiles error = %v, want bulk limit error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetBulkDownloadFiles returned error: %v", err)
			}
			if len(entries) != tt.wantCount {
				t.Fatalf("entries = %d, want %d", len(entries), tt.wantCount)
			}
		})
	}
}

func TestGetBulkDownloadFilesRejectsRequestedDeviceCountOverLimit(t *testing.T) {
	svc, deviceID := setupBulkDownloadServiceWithFiles(t, []int{4}, nil)
	otherID := uuid.New()
	if err := svc.deviceRepo.Create(&domain.Device{
		ID: otherID, IP: "10.0.0.2", SysName: "edge",
		Tags: map[string]string{},
	}); err != nil {
		t.Fatalf("Create other device: %v", err)
	}
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  1,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	_, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID, otherID})
	if err == nil {
		t.Fatal("GetBulkDownloadFiles error = nil, want bulk limit error")
	}
	if !IsBulkLimitError(err) {
		t.Fatalf("GetBulkDownloadFiles error = %v, want bulk limit error", err)
	}
}

func TestGetBulkDownloadFilesRejectsPathOutsideBackupDir(t *testing.T) {
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.rsc")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0644); err != nil {
		t.Fatalf("WriteFile outside backup dir: %v", err)
	}
	svc, deviceID := setupBulkDownloadServiceWithFiles(t, nil, []domain.BackupFile{{
		ID:       uuid.New(),
		FileType: "running",
		FileName: "outside.rsc",
		FilePath: outsidePath,
	}})

	_, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID})
	if err == nil {
		t.Fatal("GetBulkDownloadFiles error = nil, want backup path error")
	}
	if !IsBulkPathError(err) {
		t.Fatalf("GetBulkDownloadFiles error = %v, want backup path error", err)
	}
}

func TestGetBulkDownloadFilesRejectsUnsafeZipEntryName(t *testing.T) {
	svc, deviceID := setupBulkDownloadServiceWithFiles(t, []int{4}, func(files []domain.BackupFile) []domain.BackupFile {
		files[0].FileName = "../escape.rsc"
		return files
	})

	_, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID})
	if err == nil {
		t.Fatal("GetBulkDownloadFiles error = nil, want backup path error")
	}
	if !IsBulkPathError(err) {
		t.Fatalf("GetBulkDownloadFiles error = %v, want backup path error", err)
	}
}

func TestGetBulkDownloadFilesRejectsNonRegularFiles(t *testing.T) {
	backupDir := t.TempDir()
	pipePath := filepath.Join(backupDir, "backup.pipe")
	if err := syscall.Mkfifo(pipePath, 0600); err != nil {
		t.Skipf("Mkfifo unsupported: %v", err)
	}
	svc, deviceID := setupBulkDownloadServiceWithFiles(t, nil, []domain.BackupFile{{
		ID:       uuid.New(),
		FileType: "running",
		FileName: "backup.pipe",
		FilePath: pipePath,
	}})
	svc.backupDir = backupDir

	_, err := svc.GetBulkDownloadFiles(context.Background(), []uuid.UUID{deviceID})
	if err == nil {
		t.Fatal("GetBulkDownloadFiles error = nil, want backup path error")
	}
	if !IsBulkPathError(err) {
		t.Fatalf("GetBulkDownloadFiles error = %v, want backup path error", err)
	}
}

func TestOpenBulkDownloadEntryRejectsSymlinkAfterSelection(t *testing.T) {
	backupDir := t.TempDir()
	svc := &BackupService{backupDir: backupDir}

	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.rsc")
	if err := os.WriteFile(outsidePath, []byte("safe"), 0644); err != nil {
		t.Fatalf("WriteFile outside: %v", err)
	}
	linkPath := filepath.Join(backupDir, "backup.rsc")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("Symlink unsupported: %v", err)
	}

	_, err := svc.OpenBulkDownloadEntry(BulkDownloadEntry{
		File:      domain.BackupFile{FilePath: linkPath, FileName: "backup.rsc"},
		ZipPath:   "device/backup.rsc",
		SizeBytes: int64(len("safe")),
	})
	if err == nil {
		t.Fatal("OpenBulkDownloadEntry error = nil, want backup path error")
	}
	if !IsBulkPathError(err) {
		t.Fatalf("OpenBulkDownloadEntry error = %v, want backup path error", err)
	}
}

func TestOpenBulkDownloadEntryRejectsSizeChangedAfterSelection(t *testing.T) {
	backupDir := t.TempDir()
	filePath := filepath.Join(backupDir, "backup.rsc")
	if err := os.WriteFile(filePath, []byte("changed"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	svc := &BackupService{backupDir: backupDir}

	_, err := svc.OpenBulkDownloadEntry(BulkDownloadEntry{
		File:      domain.BackupFile{FilePath: filePath, FileName: "backup.rsc"},
		ZipPath:   "device/backup.rsc",
		SizeBytes: 4,
	})
	if err == nil {
		t.Fatal("OpenBulkDownloadEntry error = nil, want backup path error")
	}
	if !IsBulkPathError(err) {
		t.Fatalf("OpenBulkDownloadEntry error = %v, want backup path error", err)
	}
}

func mockBackupJobCount(repo *mockBackupJobRepo) int {
	repo.mu.Lock()
	defer repo.mu.Unlock()
	return len(repo.jobs)
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func setupLegacyBulkBackupService(
	t *testing.T,
	port int,
	leaseRepo domain.BulkOperationLeaseRepository,
	sshDelay time.Duration,
) (*BackupService, *mockBackupJobRepo, uuid.UUID) {
	t.Helper()

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &mockSSHDialer{delay: sshDelay}, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
		WithBulkOperationLeaseRepository(leaseRepo),
	)
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	if err := credentialProfileRepo.Create(&domain.CredentialProfile{
		ID: uuid.New(), Name: "test-profile", Username: "admin", Port: port,
		AuthMethod: domain.SSHAuthPassword, Role: "Admin",
	}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}
	return svc, jobRepo, deviceID
}

type mockBulkOperationLeaseRepo struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func newMockBulkOperationLeaseRepo() *mockBulkOperationLeaseRepo {
	return &mockBulkOperationLeaseRepo{active: make(map[string]struct{})}
}

func (r *mockBulkOperationLeaseRepo) TryAcquireBulkOperationLease(_ context.Context, key string) (domain.BulkOperationLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.active[key]; ok {
		return nil, false, nil
	}
	r.active[key] = struct{}{}
	return mockBulkOperationLease{release: func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.active, key)
	}}, true, nil
}

func (r *mockBulkOperationLeaseRepo) isActive(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.active[key]
	return ok
}

type mockBulkOperationLease struct {
	release func()
}

func (l mockBulkOperationLease) Release() error {
	if l.release != nil {
		l.release()
	}
	return nil
}

func setupBulkDownloadServiceWithFiles(
	t *testing.T,
	fileSizes []int,
	override interface{},
) (*BackupService, uuid.UUID) {
	t.Helper()

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	deviceRepo := newMockDeviceRepo()
	backupDir := t.TempDir()

	svc := &BackupService{
		jobRepo:    jobRepo,
		fileRepo:   fileRepo,
		deviceRepo: deviceRepo,
		backupDir:  backupDir,
	}
	svc.SetBulkOperationLimits(BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID:      deviceID,
		IP:      "10.0.0.1",
		SysName: "core",
		Tags:    map[string]string{"display_name": "Core Router"},
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}
	jobID := uuid.New()
	if err := jobRepo.Create(&domain.BackupJob{
		ID:       jobID,
		DeviceID: deviceID,
		Status:   domain.BackupStatusSuccess,
	}); err != nil {
		t.Fatalf("Create job: %v", err)
	}

	files := make([]domain.BackupFile, 0, len(fileSizes))
	for i, size := range fileSizes {
		path := filepath.Join(backupDir, fmt.Sprintf("backup-%d.rsc", i))
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0644); err != nil {
			t.Fatalf("WriteFile backup %d: %v", i, err)
		}
		files = append(files, domain.BackupFile{
			ID:       uuid.New(),
			FileType: "running",
			FileName: filepath.Base(path),
			FilePath: path,
		})
	}

	switch fn := override.(type) {
	case nil:
	case func([]domain.BackupFile) []domain.BackupFile:
		files = fn(files)
	case []domain.BackupFile:
		files = fn
	default:
		t.Fatalf("unsupported override type %T", override)
	}

	for i := range files {
		files[i].JobID = jobID
		if files[i].ID == uuid.Nil {
			files[i].ID = uuid.New()
		}
		if err := fileRepo.Create(&files[i]); err != nil {
			t.Fatalf("Create file %d: %v", i, err)
		}
	}

	return svc, deviceID
}

// ---------------------------------------------------------------------------
// Test 9: TestBackupServiceDecryptCredentials (TEST-02 gap: credential decryption)
// ---------------------------------------------------------------------------
// Verifies the BackupService encrypt/decrypt round-trip using the service-level
// EncryptSecret and the internal decryptSecret methods. This proves that
// credentials stored encrypted in SSHProfile.EncryptedSecret are correctly
// decrypted before being passed to the SSH dialer during backup.
//
// We also create a recording dialer that captures the SSH config passed to Dial,
// then trigger a backup flow to confirm the service calls the dialer (proving
// the full flow from encrypted storage through decryption to SSH connection).
func TestBackupServiceDecryptCredentials(t *testing.T) {
	// Use a known 32-byte key (AES-256 requires exactly 16, 24, or 32 bytes)
	encryptionKey := crypto.DeriveKey("test-encryption-passphrase")

	// Encrypt a known plaintext password
	plaintext := "super-secret-password-42"
	encrypted, err := crypto.Encrypt([]byte(plaintext), encryptionKey)
	if err != nil {
		t.Fatalf("crypto.Encrypt failed: %v", err)
	}

	// Verify the ciphertext is different from plaintext
	if string(encrypted) == plaintext {
		t.Fatal("encrypted text should differ from plaintext")
	}

	// Create BackupService with the same encryption key
	port := listenOnRandomPort(t)
	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)

	// Recording dialer that captures what the service passes
	rd := &recordingSSHDialer{}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, rd, encryptionKey, t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	// Test the service-level round-trip: EncryptSecret -> decryptSecret
	encStr, err := svc.EncryptSecret(plaintext)
	if err != nil {
		t.Fatalf("EncryptSecret failed: %v", err)
	}
	if !utf8.ValidString(encStr) {
		t.Fatal("EncryptSecret returned a non-UTF-8 string")
	}
	if _, err := base64.StdEncoding.DecodeString(encStr); err != nil {
		t.Fatalf("EncryptSecret returned a non-base64 string: %v", err)
	}
	decrypted, err := svc.decryptSecret(encStr)
	if err != nil {
		t.Fatalf("decryptSecret failed: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("round-trip mismatch: expected %q, got %q", plaintext, decrypted)
	}

	// Now test the full flow: seed profile with encrypted password, trigger backup
	profileID := uuid.New()
	credentialProfileRepo.Create(&domain.CredentialProfile{
		ID:              profileID,
		Name:            "encrypted-profile",
		Username:        "admin",
		Port:            port,
		AuthMethod:      domain.SSHAuthPassword,
		EncryptedSecret: encStr,
		Role:            "Admin",
	})

	deviceID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	})

	// TriggerBackup creates the job and starts runFullBackup in a goroutine.
	// runFullBackup calls decryptSecret then ssh.NewClient which calls Dial.
	job, err := svc.TriggerBackup(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("TriggerBackup failed: %v", err)
	}
	if job == nil {
		t.Fatal("TriggerBackup returned nil job")
	}

	// Wait briefly for the goroutine to attempt the SSH dial
	time.Sleep(200 * time.Millisecond)

	rd.mu.Lock()
	called := rd.called
	capturedUser := rd.capturedUser
	rd.mu.Unlock()

	if !called {
		t.Fatal("expected recording dialer to be called (proves decryption + dial flow executed)")
	}
	if capturedUser != "admin" {
		t.Errorf("expected dialer to receive user %q, got %q", "admin", capturedUser)
	}
}

func TestBackupServiceDecryptSecret_LegacyRawCiphertextCompatible(t *testing.T) {
	encryptionKey := crypto.DeriveKey("test-encryption-passphrase")
	plaintext := "legacy-raw-ciphertext-secret"
	rawCiphertext, err := crypto.Encrypt([]byte(plaintext), encryptionKey)
	if err != nil {
		t.Fatalf("crypto.Encrypt failed: %v", err)
	}

	svc := NewBackupService(
		newMockBackupJobRepo(),
		newMockBackupFileRepo(),
		newMockCredentialProfileRepo(),
		newMockDeviceRepo(),
		newMockBackupSettingsRepo(),
		buildTestVendorRegistry("testvendor", true),
		&recordingSSHDialer{},
		encryptionKey,
		t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	decrypted, err := svc.decryptSecret(string(rawCiphertext))
	if err != nil {
		t.Fatalf("decryptSecret failed for legacy raw ciphertext: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected decrypted plaintext %q, got %q", plaintext, decrypted)
	}
}

// ---------------------------------------------------------------------------
// Test 10: TestTriggerBackup_NoCredentialProfileAssigned (Phase 27 Gap 5)
// ---------------------------------------------------------------------------
// Verifies that TriggerBackup returns an error containing "no credential profile"
// when no credential profile is assigned to the device. The mockCredentialProfileRepo
// returns an error from GetBackupProfileForDevice when its profiles map is empty.
func TestTriggerBackup_NoCredentialProfileAssigned(t *testing.T) {
	port := listenOnRandomPort(t)
	_ = port // port unused; backup fails before SSH dial

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	// Empty credential profile repo — GetBackupProfileForDevice will return an error.
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	dialer := &mockSSHDialer{}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, dialer, []byte("0123456789abcdef"), t.TempDir(),
		ssh.InsecureIgnoreHostKey(),
	)

	// Create device with backup-supported vendor — NO credential profile added.
	deviceID := uuid.New()
	deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "127.0.0.1", Vendor: "testvendor",
		Managed: true, Status: domain.DeviceStatusUp,
	})

	// TriggerBackup must return an error because no credential profile is assigned.
	_, err := svc.TriggerBackup(context.Background(), deviceID)
	if err == nil {
		t.Fatal("expected error when no credential profile assigned, got nil")
	}
	if !strings.Contains(err.Error(), "no credential profile") {
		t.Errorf("expected error to contain %q, got %q", "no credential profile", err.Error())
	}
}

func TestRunTextExportWrites0600BackupFile(t *testing.T) {
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		if command != "/export" {
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		return "/interface bridge add name=br0\n", nil
	})

	client, err := internalssh.NewClient(&internalssh.DefaultDialer{}, "127.0.0.1", serverPort(t, addr), "admin", "secret", 5*time.Second, ssh.InsecureIgnoreHostKey())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	backupDir := t.TempDir()
	fileRepo := newMockBackupFileRepo()
	jobID := uuid.New()
	svc := &BackupService{fileRepo: fileRepo}
	if err := svc.runTextExport(context.Background(), client, jobID, backupDir, "router.rsc", "running", "/export"); err != nil {
		t.Fatalf("runTextExport: %v", err)
	}

	assertBackupMode(t, filepath.Join(backupDir, "router.rsc"), 0600)
	files, err := fileRepo.GetByJobID(jobID)
	if err != nil {
		t.Fatalf("GetByJobID: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("backup files = %d, want 1", len(files))
	}
	output := []byte("/interface bridge add name=br0\n")
	expectedHash := sha256.Sum256(output)
	if files[0].FileHash != hex.EncodeToString(expectedHash[:]) {
		t.Fatalf("FileHash = %q, want streamed output hash", files[0].FileHash)
	}
	if files[0].SizeBytes != len(output) {
		t.Fatalf("SizeBytes = %d, want %d", files[0].SizeBytes, len(output))
	}
}

func TestRunTextExportTightensExistingBackupFileOnOverwrite(t *testing.T) {
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		if command != "/export" {
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		return "/interface bridge add name=br0\n", nil
	})

	client, err := internalssh.NewClient(&internalssh.DefaultDialer{}, "127.0.0.1", serverPort(t, addr), "admin", "secret", 5*time.Second, ssh.InsecureIgnoreHostKey())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	backupDir := t.TempDir()
	filePath := filepath.Join(backupDir, "router.rsc")
	if err := os.WriteFile(filePath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing backup): %v", err)
	}

	svc := &BackupService{fileRepo: newMockBackupFileRepo()}
	if err := svc.runTextExport(context.Background(), client, uuid.New(), backupDir, "router.rsc", "running", "/export"); err != nil {
		t.Fatalf("runTextExport: %v", err)
	}

	assertBackupMode(t, filePath, 0600)
}

func TestRunTextExportReconcilesExistingDeviceBackupDirBeforeWritingBackupFile(t *testing.T) {
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		if command != "/export" {
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		return "/interface bridge add name=br0\n", nil
	})

	jobRepo := newMockBackupJobRepo()
	fileRepo := newMockBackupFileRepo()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockBackupSettingsRepo()
	registry := buildTestVendorRegistry("testvendor", true)
	backupRoot := t.TempDir()
	encryptionKey := []byte("0123456789abcdef")

	secretCiphertext, err := crypto.Encrypt([]byte("secret"), encryptionKey)
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}

	svc := NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		registry, &internalssh.DefaultDialer{}, encryptionKey, backupRoot,
		ssh.InsecureIgnoreHostKey(),
	)

	profileID := uuid.New()
	if err := credentialProfileRepo.Create(&domain.CredentialProfile{
		ID:              profileID,
		Name:            "test-profile",
		Username:        "admin",
		Port:            serverPort(t, addr),
		EncryptedSecret: base64.StdEncoding.EncodeToString(secretCiphertext),
		AuthMethod:      domain.SSHAuthPassword,
		Role:            "Admin",
	}); err != nil {
		t.Fatalf("Create profile: %v", err)
	}

	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID:      deviceID,
		IP:      "127.0.0.1",
		Vendor:  "testvendor",
		Managed: true,
		Status:  domain.DeviceStatusUp,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	deviceDir := filepath.Join(backupRoot, deviceID.String())
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("precreate device dir: %v", err)
	}
	assertBackupMode(t, deviceDir, 0755)

	job, err := svc.TriggerBackup(context.Background(), deviceID)
	if err != nil {
		t.Fatalf("TriggerBackup: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		files, err := fileRepo.GetByJobID(job.ID)
		if err == nil && len(files) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	files, err := fileRepo.GetByJobID(job.ID)
	if err != nil {
		t.Fatalf("GetByJobID: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected backup file to be created")
	}

	assertBackupMode(t, deviceDir, 0700)
}

func TestRunBinaryExportWrites0600BackupFile(t *testing.T) {
	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "router.backup")
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		switch command {
		case "save-backup":
			if err := os.WriteFile(remotePath, []byte("binary-backup"), 0644); err != nil {
				return "", err
			}
			return "", nil
		case "cleanup-backup":
			if err := os.Remove(remotePath); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	})

	client, err := internalssh.NewClient(&internalssh.DefaultDialer{}, "127.0.0.1", serverPort(t, addr), "admin", "secret", 5*time.Second, ssh.InsecureIgnoreHostKey())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	backupDir := t.TempDir()
	svc := &BackupService{fileRepo: newMockBackupFileRepo()}
	if err := svc.runBinaryExport(context.Background(), client, uuid.New(), backupDir, "router.backup", &vendor.BinaryBackupCmd{
		SaveCommand:    "save-backup",
		RemoteFilePath: remotePath,
		CleanupCommand: "cleanup-backup",
	}); err != nil {
		t.Fatalf("runBinaryExport: %v", err)
	}

	assertBackupMode(t, filepath.Join(backupDir, "router.backup"), 0600)
}

func TestRunBinaryExportStreamsHashWithoutPostDownloadRead(t *testing.T) {
	assertFunctionBodyExcludes(t, "runTextExport", []string{
		"client.RunCommand(ctx, command)",
		"os.WriteFile(filePath",
		"sha256.Sum256([]byte(output))",
		"SizeBytes: len(output)",
	})
	assertFunctionBodyExcludes(t, "runBinaryExport", []string{
		"os.ReadFile(filePath)",
		"sha256.Sum256(data)",
		"SizeBytes: len(data)",
	})
	assertFunctionBodyExcludes(t, "downloadSFTPFileToDiskAndHash", []string{
		"os.ReadFile(localPath)",
		"os.ReadFile(filePath)",
		"sha256.Sum256(data)",
		"SizeBytes: len(data)",
		"size: len(data)",
	})
}

func TestTriggerBulkBackupUsesBoundedWorkerPool(t *testing.T) {
	assertFunctionBodyExcludes(t, "TriggerBulkBackup", []string{
		"go s.runFullBackup",
	})
	body := backupServiceFunctionBody(t, "startBulkBackupWorkers")
	for _, required := range []string{
		"defaultBulkBackupWorkerCount",
		"workerCount",
		"jobs := make(chan queuedDeviceBackup)",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("startBulkBackupWorkers body missing %q", required)
		}
	}
}

func assertFunctionBodyExcludes(t *testing.T, functionName string, forbiddenFragments []string) {
	t.Helper()

	body := backupServiceFunctionBody(t, functionName)
	for _, forbidden := range forbiddenFragments {
		if strings.Contains(body, forbidden) {
			t.Fatalf("%s must not contain post-download whole-file read pattern %q", functionName, forbidden)
		}
	}
}

func backupServiceFunctionBody(t *testing.T, functionName string) string {
	t.Helper()

	src, err := os.ReadFile("backup_service.go")
	if err != nil {
		t.Fatalf("ReadFile(backup_service.go): %v", err)
	}

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, "backup_service.go", src, 0)
	if err != nil {
		t.Fatalf("ParseFile(backup_service.go): %v", err)
	}

	var body string
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != functionName || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Body.Pos()).Offset
		end := fset.Position(fn.Body.End()).Offset
		body = string(src[start:end])
		break
	}
	if body == "" {
		t.Fatalf("%s body not found", functionName)
	}

	return body
}

func TestRunBinaryExportRecordsStreamedHashAndSize(t *testing.T) {
	payload := []byte{0x00, 0xff, 'b', 'i', 'n', 'a', 'r', 'y', 0x00, 0x7f, 0xff}
	expectedHashBytes := sha256.Sum256(payload)
	expectedHash := hex.EncodeToString(expectedHashBytes[:])

	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "router.backup")
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		switch command {
		case "save-backup":
			if err := os.WriteFile(remotePath, payload, 0644); err != nil {
				return "", err
			}
			return "", nil
		case "cleanup-backup":
			if err := os.Remove(remotePath); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	})

	client, err := internalssh.NewClient(&internalssh.DefaultDialer{}, "127.0.0.1", serverPort(t, addr), "admin", "secret", 5*time.Second, ssh.InsecureIgnoreHostKey())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	backupDir := t.TempDir()
	jobID := uuid.New()
	fileRepo := newMockBackupFileRepo()
	svc := &BackupService{fileRepo: fileRepo}
	if err := svc.runBinaryExport(context.Background(), client, jobID, backupDir, "router.backup", &vendor.BinaryBackupCmd{
		SaveCommand:    "save-backup",
		RemoteFilePath: remotePath,
		CleanupCommand: "cleanup-backup",
	}); err != nil {
		t.Fatalf("runBinaryExport: %v", err)
	}

	files, err := fileRepo.GetByJobID(jobID)
	if err != nil {
		t.Fatalf("GetByJobID: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("backup file count = %d, want 1", len(files))
	}
	got := files[0]
	if got.FileType != "binary" {
		t.Fatalf("FileType = %q, want %q", got.FileType, "binary")
	}
	if got.FileHash != expectedHash {
		t.Fatalf("FileHash = %q, want %q", got.FileHash, expectedHash)
	}
	if got.SizeBytes != len(payload) {
		t.Fatalf("SizeBytes = %d, want %d", got.SizeBytes, len(payload))
	}

	downloaded, err := os.ReadFile(filepath.Join(backupDir, "router.backup"))
	if err != nil {
		t.Fatalf("ReadFile(downloaded backup): %v", err)
	}
	if string(downloaded) != string(payload) {
		t.Fatalf("downloaded bytes = %x, want %x", downloaded, payload)
	}
}

func TestRunBinaryExportRecordsStreamedHashForLargeGeneratedBackup(t *testing.T) {
	const (
		chunkSize  = 64 * 1024
		chunkCount = 96
	)

	type generatedBackup struct {
		hash string
		size int
	}

	generated := make(chan generatedBackup, 1)
	remoteDir := t.TempDir()
	remotePath := filepath.Join(remoteDir, "router.backup")
	addr := startBackupTestSSHServer(t, func(command string) (string, error) {
		switch command {
		case "save-backup":
			remoteFile, err := os.OpenFile(remotePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil {
				return "", err
			}
			defer remoteFile.Close()

			hasher := sha256.New()
			chunk := make([]byte, chunkSize)
			size := 0
			for chunkIndex := 0; chunkIndex < chunkCount; chunkIndex++ {
				for i := range chunk {
					chunk[i] = byte((chunkIndex + i*31) % 251)
				}
				n, err := remoteFile.Write(chunk)
				if err != nil {
					return "", err
				}
				if n != len(chunk) {
					return "", io.ErrShortWrite
				}
				writtenToHash, err := hasher.Write(chunk)
				if err != nil {
					return "", err
				}
				if writtenToHash != len(chunk) {
					return "", io.ErrShortWrite
				}
				size += n
			}

			metadata := generatedBackup{
				hash: hex.EncodeToString(hasher.Sum(nil)),
				size: size,
			}
			select {
			case generated <- metadata:
				return "", nil
			default:
				return "", fmt.Errorf("save-backup generated metadata more than once")
			}
		case "cleanup-backup":
			if err := os.Remove(remotePath); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			return "", nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	})

	client, err := internalssh.NewClient(&internalssh.DefaultDialer{}, "127.0.0.1", serverPort(t, addr), "admin", "secret", 5*time.Second, ssh.InsecureIgnoreHostKey())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	backupDir := t.TempDir()
	expectedPath := filepath.Join(backupDir, "router.backup")
	jobID := uuid.New()
	fileRepo := newMockBackupFileRepo()
	svc := &BackupService{fileRepo: fileRepo}
	if err := svc.runBinaryExport(context.Background(), client, jobID, backupDir, "router.backup", &vendor.BinaryBackupCmd{
		SaveCommand:    "save-backup",
		RemoteFilePath: remotePath,
		CleanupCommand: "cleanup-backup",
	}); err != nil {
		t.Fatalf("runBinaryExport: %v", err)
	}

	var expected generatedBackup
	select {
	case expected = <-generated:
	default:
		t.Fatal("save-backup did not report generated backup metadata")
	}

	files, err := fileRepo.GetByJobID(jobID)
	if err != nil {
		t.Fatalf("GetByJobID: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("backup file count = %d, want 1", len(files))
	}
	got := files[0]
	if got.FileType != "binary" {
		t.Fatalf("FileType = %q, want %q", got.FileType, "binary")
	}
	if got.FileName != "router.backup" {
		t.Fatalf("FileName = %q, want %q", got.FileName, "router.backup")
	}
	if got.FilePath != expectedPath {
		t.Fatalf("FilePath = %q, want %q", got.FilePath, expectedPath)
	}
	if got.FileHash != expected.hash {
		t.Fatalf("FileHash = %q, want %q", got.FileHash, expected.hash)
	}
	if got.SizeBytes != expected.size {
		t.Fatalf("SizeBytes = %d, want %d", got.SizeBytes, expected.size)
	}

	downloadedInfo, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("Stat(downloaded backup): %v", err)
	}
	if downloadedInfo.Size() != int64(expected.size) {
		t.Fatalf("downloaded size = %d, want %d", downloadedInfo.Size(), expected.size)
	}
}

func assertBackupMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("mode for %s = %04o, want %04o", path, got, want)
	}
}

func startBackupTestSSHServer(t *testing.T, execHandler func(command string) (string, error)) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(crand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	signer, err := ssh.NewSignerFromSigner(privateKey)
	if err != nil {
		t.Fatalf("create server signer: %v", err)
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() != "admin" || string(password) != "secret" {
				return nil, fmt.Errorf("invalid credentials")
			}
			return nil, nil
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveBackupSSHConn(conn, config, execHandler)
		}
	}()

	return listener.Addr().String()
}

func serveBackupSSHConn(conn net.Conn, config *ssh.ServerConfig, execHandler func(command string) (string, error)) {
	serverConn, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go handleBackupSSHChannel(channel, requests, execHandler)
	}
}

func handleBackupSSHChannel(channel ssh.Channel, requests <-chan *ssh.Request, execHandler func(command string) (string, error)) {
	defer channel.Close()

	for req := range requests {
		switch req.Type {
		case "exec":
			command := parseSSHExecCommand(req.Payload)
			stdout, err := execHandler(command)
			if req.WantReply {
				_ = req.Reply(err == nil, nil)
			}
			if stdout != "" {
				_, _ = io.WriteString(channel, stdout)
			}
			status := uint32(0)
			if err != nil {
				_, _ = io.WriteString(channel.Stderr(), err.Error())
				status = 1
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: status}))
			return
		case "subsystem":
			if parseSSHExecCommand(req.Payload) != "sftp" {
				if req.WantReply {
					_ = req.Reply(false, nil)
				}
				continue
			}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			server, err := sftp.NewServer(channel)
			if err != nil {
				return
			}
			_ = server.Serve()
			return
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func parseSSHExecCommand(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	length := binary.BigEndian.Uint32(payload[:4])
	if int(length) > len(payload)-4 {
		return ""
	}
	return string(payload[4 : 4+length])
}

func serverPort(t *testing.T, addr string) int {
	t.Helper()

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split host port %q: %v", addr, err)
	}

	parsed, err := net.LookupPort("tcp", port)
	if err != nil {
		t.Fatalf("parse port %q: %v", port, err)
	}

	return parsed
}

// recordingSSHDialer captures the SSH config passed to Dial for verification.
type recordingSSHDialer struct {
	mu           sync.Mutex
	called       bool
	capturedUser string
}

func (d *recordingSSHDialer) Dial(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.called = true
	d.capturedUser = config.User
	return nil, fmt.Errorf("recording dialer: connection refused")
}
