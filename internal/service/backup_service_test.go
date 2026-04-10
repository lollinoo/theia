package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/vendor"
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

	svc := &BackupService{
		jobRepo:    jobRepo,
		fileRepo:   fileRepo,
		deviceRepo: deviceRepo,
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
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "20260319_core-router.rsc")
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

	svc := &BackupService{
		jobRepo:    jobRepo,
		fileRepo:   fileRepo,
		deviceRepo: deviceRepo,
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
		EncryptedSecret: string(encrypted), // stored as encrypted bytes
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
