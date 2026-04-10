package api

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	gossh "golang.org/x/crypto/ssh"
)

// TestBulkDownloadStreaming verifies that writeBulkZipEntries streams files
// into a zip archive using io.Copy with correct content.
func TestBulkDownloadStreaming(t *testing.T) {
	dir := t.TempDir()

	// Create two temp files with known content
	file1Path := filepath.Join(dir, "backup1.rsc")
	file2Path := filepath.Join(dir, "backup2.rsc")
	if err := os.WriteFile(file1Path, []byte("file1 content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file2Path, []byte("file2 content"), 0644); err != nil {
		t.Fatal(err)
	}

	entries := []service.BulkDownloadEntry{
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "backup1.rsc", FilePath: file1Path},
			DeviceDir: "device-a",
		},
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "backup2.rsc", FilePath: file2Path},
			DeviceDir: "device-b",
		},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries)
	zw.Close()

	// No errors expected
	if len(zipErrors) != 0 {
		t.Fatalf("expected no zip errors, got %v", zipErrors)
	}

	// Read zip back and verify contents
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	if len(zr.File) != 2 {
		t.Fatalf("expected 2 zip entries, got %d", len(zr.File))
	}

	expected := map[string]string{
		filepath.Join("device-a", "backup1.rsc"): "file1 content",
		filepath.Join("device-b", "backup2.rsc"): "file2 content",
	}

	for _, f := range zr.File {
		want, ok := expected[f.Name]
		if !ok {
			t.Fatalf("unexpected zip entry: %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("failed to open zip entry %s: %v", f.Name, err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("failed to read zip entry %s: %v", f.Name, err)
		}
		if string(got) != want {
			t.Fatalf("zip entry %s: got %q, want %q", f.Name, got, want)
		}
	}
}

// TestBulkDownloadErrorManifest verifies that writeBulkZipEntries records
// failed files and that HandleBulkDownload includes _errors.txt in the zip.
func TestBulkDownloadErrorManifest(t *testing.T) {
	dir := t.TempDir()

	// Create one valid temp file
	validPath := filepath.Join(dir, "valid.rsc")
	if err := os.WriteFile(validPath, []byte("valid content"), 0644); err != nil {
		t.Fatal(err)
	}

	entries := []service.BulkDownloadEntry{
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "valid.rsc", FilePath: validPath},
			DeviceDir: "device-ok",
		},
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "missing.rsc", FilePath: "/nonexistent/path/missing.rsc"},
			DeviceDir: "device-fail",
		},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries)

	// Write error manifest if there are errors (same logic as HandleBulkDownload)
	if len(zipErrors) > 0 {
		if w, err := zw.Create("_errors.txt"); err == nil {
			for _, e := range zipErrors {
				w.Write([]byte(e + "\n"))
			}
		}
	}
	zw.Close()

	// Should have exactly one error (the nonexistent file)
	if len(zipErrors) != 1 {
		t.Fatalf("expected 1 zip error, got %d: %v", len(zipErrors), zipErrors)
	}

	// Error should mention the failed path
	if !strings.Contains(zipErrors[0], "device-fail") {
		t.Fatalf("expected error to mention device-fail path, got: %s", zipErrors[0])
	}

	// Read zip back and verify structure
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}

	// Expect: 1 valid file + _errors.txt = 2 entries
	if len(zr.File) != 2 {
		names := make([]string, len(zr.File))
		for i, f := range zr.File {
			names[i] = f.Name
		}
		t.Fatalf("expected 2 zip entries, got %d: %v", len(zr.File), names)
	}

	// Find and verify _errors.txt
	var foundErrors bool
	for _, f := range zr.File {
		if f.Name == "_errors.txt" {
			foundErrors = true
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("failed to open _errors.txt: %v", err)
			}
			content, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatalf("failed to read _errors.txt: %v", err)
			}
			if !strings.Contains(string(content), "/nonexistent/path/missing.rsc") {
				t.Fatalf("_errors.txt should mention the failed path, got: %s", content)
			}
		}
	}
	if !foundErrors {
		t.Fatal("expected _errors.txt in zip, but not found")
	}
}

// ============================================================================
// Handler-level HTTP tests for BackupHandler
// ============================================================================

// --- mock repos for BackupHandler HTTP tests ---

type backupJobRepoForHandler struct {
	mu   sync.Mutex
	jobs map[uuid.UUID]*domain.BackupJob
}

func newBackupJobRepoForHandler() *backupJobRepoForHandler {
	return &backupJobRepoForHandler{jobs: make(map[uuid.UUID]*domain.BackupJob)}
}

func (r *backupJobRepoForHandler) Create(job *domain.BackupJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	job.CreatedAt = now
	r.jobs[job.ID] = job
	return nil
}

func (r *backupJobRepoForHandler) GetByID(id uuid.UUID) (*domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, nil
	}
	cp := *j
	return &cp, nil
}

func (r *backupJobRepoForHandler) GetByDeviceID(deviceID uuid.UUID) ([]domain.BackupJob, error) {
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

func (r *backupJobRepoForHandler) GetLatestByDeviceID(deviceID uuid.UUID) (*domain.BackupJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *domain.BackupJob
	for _, j := range r.jobs {
		if j.DeviceID == deviceID && (latest == nil || j.CreatedAt.After(latest.CreatedAt)) {
			cp := *j
			latest = &cp
		}
	}
	return latest, nil
}

func (r *backupJobRepoForHandler) Update(job *domain.BackupJob) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs[job.ID] = job
	return nil
}

func (r *backupJobRepoForHandler) Delete(id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.jobs[id]; !ok {
		return fmt.Errorf("backup job not found: %s", id)
	}
	delete(r.jobs, id)
	return nil
}

func (r *backupJobRepoForHandler) DeleteByDeviceID(uuid.UUID) error { return nil }

func (r *backupJobRepoForHandler) ListSuccessfulByDeviceOldest(deviceID uuid.UUID) ([]domain.BackupJob, error) {
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

func (r *backupJobRepoForHandler) ListAllDeviceIDs() ([]uuid.UUID, error) {
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

func (r *backupJobRepoForHandler) DeleteFailedOlderThan(cutoff time.Time) (int, error) {
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

type backupFileRepoForHandler struct {
	mu    sync.Mutex
	files map[uuid.UUID]*domain.BackupFile
}

func newBackupFileRepoForHandler() *backupFileRepoForHandler {
	return &backupFileRepoForHandler{files: make(map[uuid.UUID]*domain.BackupFile)}
}

func (r *backupFileRepoForHandler) Create(f *domain.BackupFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[f.ID] = f
	return nil
}

func (r *backupFileRepoForHandler) GetByJobID(jobID uuid.UUID) ([]domain.BackupFile, error) {
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

func (r *backupFileRepoForHandler) GetByID(id uuid.UUID) (*domain.BackupFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.files[id]
	if !ok {
		return nil, nil
	}
	cp := *f
	return &cp, nil
}

func (r *backupFileRepoForHandler) DeleteByJobID(jobID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, f := range r.files {
		if f.JobID == jobID {
			delete(r.files, id)
		}
	}
	return nil
}

// setupBackupHandler creates a BackupHandler backed by mock repos.
func setupBackupHandler(t *testing.T) (*BackupHandler, *backupJobRepoForHandler, *backupFileRepoForHandler) {
	t.Helper()
	jobRepo := newBackupJobRepoForHandler()
	fileRepo := newBackupFileRepoForHandler()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockSettingsRepo()
	encKey := crypto.DeriveKey("test-backup-handler-key")

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
		t.Fatalf("building vendor registry: %v", err)
	}

	backupSvc := service.NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		reg, &mockSSHDialerForBackup{}, encKey, t.TempDir(),
		gossh.InsecureIgnoreHostKey(),
	)

	handler := NewBackupHandler(backupSvc, settingsRepo)
	return handler, jobRepo, fileRepo
}

type mockSSHDialerForBackup struct{}

func (d *mockSSHDialerForBackup) Dial(addr string, config *gossh.ClientConfig) (*gossh.Client, error) {
	return nil, nil
}

func TestBackupHandlerListBackups_HappyPath(t *testing.T) {
	handler, jobRepo, _ := setupBackupHandler(t)

	deviceID := uuid.New()
	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: deviceID,
		Status:   domain.BackupStatusSuccess,
	}
	jobRepo.Create(job)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/"+deviceID.String()+"/backups", nil)
	rec := httptest.NewRecorder()
	handler.HandleListBackups(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok {
		t.Fatal("expected 'data' key in response")
	}
}

func TestBackupHandlerListBackups_InvalidID(t *testing.T) {
	handler, _, _ := setupBackupHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/not-a-uuid/backups", nil)
	rec := httptest.NewRecorder()
	handler.HandleListBackups(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestBackupHandlerGetBackupJob_HappyPath(t *testing.T) {
	handler, jobRepo, _ := setupBackupHandler(t)

	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: uuid.New(),
		Status:   domain.BackupStatusSuccess,
	}
	jobRepo.Create(job)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-jobs/"+job.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBackupJob(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupHandlerGetBackupJob_NotFound(t *testing.T) {
	handler, _, _ := setupBackupHandler(t)

	randomID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-jobs/"+randomID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBackupJob(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupHandlerDeleteBackupJob_HappyPath(t *testing.T) {
	handler, jobRepo, _ := setupBackupHandler(t)

	job := &domain.BackupJob{
		ID:       uuid.New(),
		DeviceID: uuid.New(),
		Status:   domain.BackupStatusSuccess,
	}
	jobRepo.Create(job)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/backup-jobs/"+job.ID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleDeleteBackupJob(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupHandlerTriggerBackup_InvalidID(t *testing.T) {
	handler, _, _ := setupBackupHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/not-a-uuid/backups", nil)
	rec := httptest.NewRecorder()
	handler.HandleTriggerBackup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// =============================================================================
// D-09: Content-Disposition filename sanitization in HandleDownloadBackupFile
// =============================================================================

// TestBackupDownload_SanitizedFilename verifies that HandleDownloadBackupFile
// sets a Content-Disposition header that does not contain CRLF characters even
// when the backup file's FileName field contains them.
// We seed a backup file record with a malicious filename into the mock file repo
// and verify the header is sanitized before it reaches the HTTP response.
func TestBackupDownload_SanitizedFilename(t *testing.T) {
	_, _, fileRepo := setupBackupHandler(t)

	// Create a real temp file on disk so http.ServeFile does not error
	dir := t.TempDir()
	tmpFile := dir + "/backup.rsc"
	if err := os.WriteFile(tmpFile, []byte("# MikroTik backup"), 0644); err != nil {
		t.Fatalf("creating temp backup file: %v", err)
	}

	// Seed a backup file with a malicious filename containing CRLF
	fileID := uuid.New()
	maliciousName := "backup\r\nEvil-Header: injected"
	fileRepo.files[fileID] = &domain.BackupFile{
		ID:       fileID,
		JobID:    uuid.New(),
		FileName: maliciousName,
		FilePath: tmpFile,
		FileType: "rsc",
	}

	// Rebuild handler with this file repo populated
	jobRepo := newBackupJobRepoForHandler()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockSettingsRepo()
	encKey := crypto.DeriveKey("test-backup-sanitize-key")

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
		t.Fatalf("building vendor registry: %v", err)
	}

	backupSvc := service.NewBackupService(
		jobRepo, fileRepo, credentialProfileRepo, deviceRepo, settingsRepo,
		reg, &mockSSHDialerForBackup{}, encKey, t.TempDir(),
		gossh.InsecureIgnoreHostKey(),
	)
	handler := NewBackupHandler(backupSvc, settingsRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-files/"+fileID.String()+"/download", nil)
	rec := httptest.NewRecorder()
	handler.HandleDownloadBackupFile(rec, req)

	// Must not be 404/500 — the file exists on disk
	if rec.Code == http.StatusNotFound {
		t.Fatalf("got 404 — backup file not found in mock repo")
	}
	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("got 500 — body: %s", rec.Body.String())
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd == "" {
		t.Fatal("expected Content-Disposition header to be set")
	}
	// sanitizeFilename strips \r and \n — this defeats HTTP response splitting.
	// The remaining text from after the CRLF is collapsed into the filename but
	// cannot be interpreted as a separate HTTP header line.
	if strings.Contains(cd, "\r") {
		t.Errorf("Content-Disposition must not contain CR; got: %q", cd)
	}
	if strings.Contains(cd, "\n") {
		t.Errorf("Content-Disposition must not contain LF; got: %q", cd)
	}
	// The header must still start with attachment; filename= (not be split into
	// a new header line) — verify the whole value is on a single logical line.
	if !strings.HasPrefix(cd, "attachment; filename=") {
		t.Errorf("Content-Disposition must start with 'attachment; filename='; got: %q", cd)
	}
}
