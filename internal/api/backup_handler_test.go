package api

import (
	"archive/zip"
	"bytes"
	"context"
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
			ZipPath:   "device-a/backup1.rsc",
			SizeBytes: int64(len("file1 content")),
		},
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "backup2.rsc", FilePath: file2Path},
			DeviceDir: "device-b",
			ZipPath:   "device-b/backup2.rsc",
			SizeBytes: int64(len("file2 content")),
		},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries, openBulkZipEntryForTest)
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
		"device-a/backup1.rsc": "file1 content",
		"device-b/backup2.rsc": "file2 content",
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
			ZipPath:   "device-ok/valid.rsc",
			SizeBytes: int64(len("valid content")),
		},
		{
			File:      domain.BackupFile{ID: uuid.New(), FileName: "missing.rsc", FilePath: "/nonexistent/path/missing.rsc"},
			DeviceDir: "device-fail",
			ZipPath:   "device-fail/missing.rsc",
			SizeBytes: int64(len("missing content")),
		},
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries, openBulkZipEntryForTest)

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

func TestBulkDownloadRejectsEntriesWithoutValidatedZipPath(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "backup.rsc")
	if err := os.WriteFile(filePath, []byte("backup content"), 0644); err != nil {
		t.Fatal(err)
	}

	entries := []service.BulkDownloadEntry{{
		File:      domain.BackupFile{ID: uuid.New(), FileName: "../backup.rsc", FilePath: filePath},
		DeviceDir: "device",
		SizeBytes: int64(len("backup content")),
	}}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries, openBulkZipEntryForTest)
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}

	if len(zipErrors) != 1 {
		t.Fatalf("zip errors = %d, want 1: %v", len(zipErrors), zipErrors)
	}
	if !strings.Contains(zipErrors[0], "missing validated zip entry path") {
		t.Fatalf("zip error = %q, want missing validated zip entry path", zipErrors[0])
	}
}

func TestBulkDownloadRejectsFilesChangedAfterValidation(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "backup.rsc")
	if err := os.WriteFile(filePath, []byte("larger than expected"), 0644); err != nil {
		t.Fatal(err)
	}

	entries := []service.BulkDownloadEntry{{
		File:      domain.BackupFile{ID: uuid.New(), FileName: "backup.rsc", FilePath: filePath},
		DeviceDir: "device",
		ZipPath:   "device/backup.rsc",
		SizeBytes: 4,
	}}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	zipErrors := writeBulkZipEntries(zw, entries, openBulkZipEntryForTest)
	if err := zw.Close(); err != nil {
		t.Fatalf("Close zip: %v", err)
	}

	if len(zipErrors) != 1 {
		t.Fatalf("zip errors = %d, want 1: %v", len(zipErrors), zipErrors)
	}
	if !strings.Contains(zipErrors[0], "file changed after validation") {
		t.Fatalf("zip error = %q, want file changed after validation", zipErrors[0])
	}
}

func openBulkZipEntryForTest(entry service.BulkDownloadEntry) (*os.File, error) {
	return os.Open(entry.File.FilePath)
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

type bulkBackupRunRepoForHandler struct {
	mu    sync.Mutex
	runs  map[uuid.UUID]*domain.BulkBackupRun
	items map[uuid.UUID][]domain.BulkBackupRunItem
}

func newBulkBackupRunRepoForHandler() *bulkBackupRunRepoForHandler {
	return &bulkBackupRunRepoForHandler{
		runs:  make(map[uuid.UUID]*domain.BulkBackupRun),
		items: make(map[uuid.UUID][]domain.BulkBackupRunItem),
	}
}

func (r *bulkBackupRunRepoForHandler) CreateRun(run *domain.BulkBackupRun, items []domain.BulkBackupRunItem) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	if run.StartedAt == nil {
		started := now
		run.StartedAt = &started
	}
	cp := *run
	r.runs[run.ID] = &cp
	copied := append([]domain.BulkBackupRunItem(nil), items...)
	r.items[run.ID] = copied
	return nil
}

func (r *bulkBackupRunRepoForHandler) GetRun(id uuid.UUID) (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.copyRunLocked(id), nil
}

func (r *bulkBackupRunRepoForHandler) GetLatestRun() (*domain.BulkBackupRun, error) {
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
	latest.Items = append([]domain.BulkBackupRunItem(nil), r.items[latest.ID]...)
	return latest, nil
}

func (r *bulkBackupRunRepoForHandler) GetActiveRun() (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, run := range r.runs {
		if bulkRunActiveForHandlerTest(run.Status) {
			return r.copyRunLocked(id), nil
		}
	}
	return nil, nil
}

func (r *bulkBackupRunRepoForHandler) ListResumableRuns() ([]domain.BulkBackupRun, error) {
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

func bulkRunActiveForHandlerTest(status domain.BulkBackupRunStatus) bool {
	return status == domain.BulkBackupRunStatusRunning ||
		status == domain.BulkBackupRunStatusCancelling ||
		status == domain.BulkBackupRunStatusPausing ||
		status == domain.BulkBackupRunStatusPaused
}

func (r *bulkBackupRunRepoForHandler) UpdateRun(run *domain.BulkBackupRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}

func (r *bulkBackupRunRepoForHandler) ListRunItems(runID uuid.UUID) ([]domain.BulkBackupRunItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.BulkBackupRunItem(nil), r.items[runID]...), nil
}

func (r *bulkBackupRunRepoForHandler) UpdateRunItem(item *domain.BulkBackupRunItem) error {
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
	r.items[item.RunID] = append(items, *item)
	return nil
}

func (r *bulkBackupRunRepoForHandler) RecalculateRunCounters(runID uuid.UUID) (*domain.BulkBackupRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	if run == nil {
		return nil, nil
	}
	run.TotalCount = len(r.items[runID])
	run.QueuedCount = 0
	run.SuccessCount = 0
	run.FailedCount = 0
	run.SkippedCount = 0
	run.CancelledCount = 0
	for _, item := range r.items[runID] {
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
	return r.copyRunLocked(runID), nil
}

func (r *bulkBackupRunRepoForHandler) copyRunLocked(id uuid.UUID) *domain.BulkBackupRun {
	run := r.runs[id]
	if run == nil {
		return nil
	}
	cp := *run
	cp.Items = append([]domain.BulkBackupRunItem(nil), r.items[id]...)
	return &cp
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

func decodeBackupContentData(t *testing.T, body io.Reader) map[string]interface{} {
	t.Helper()

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Data == nil {
		t.Fatal("expected data object in response")
	}
	return resp.Data
}

func TestBackupContent_SmallTextReturnsInlineContent(t *testing.T) {
	handler, _, fileRepo := setupBackupHandler(t)

	dir := t.TempDir()
	content := "/system identity set name=router\n"
	path := filepath.Join(dir, "running.rsc")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("creating backup file: %v", err)
	}

	fileID := uuid.New()
	fileRepo.Create(&domain.BackupFile{
		ID:       fileID,
		JobID:    uuid.New(),
		FileType: "running",
		FileName: "running.rsc",
		FilePath: path,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-files/"+fileID.String()+"/content", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBackupFileContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	data := decodeBackupContentData(t, rec.Body)

	if got := data["inline"]; got != true {
		t.Fatalf("expected inline true, got %#v", got)
	}
	if got := data["content"]; got != content {
		t.Fatalf("expected content %q, got %#v", content, got)
	}
	if got := data["download_url"]; got != "/api/v1/backup-files/"+fileID.String()+"/download" {
		t.Fatalf("unexpected download_url: %#v", got)
	}
	if got := data["size_bytes"]; got != float64(len(content)) {
		t.Fatalf("expected size_bytes %d, got %#v", len(content), got)
	}
	if got := data["max_inline_size_bytes"]; got != float64(1<<20) {
		t.Fatalf("expected max_inline_size_bytes %d, got %#v", 1<<20, got)
	}
}

func TestBackupContent_LargeTextReturnsDownloadMetadata(t *testing.T) {
	handler, _, fileRepo := setupBackupHandler(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "large.rsc")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating backup file: %v", err)
	}
	if err := f.Truncate(1<<20 + 1); err != nil {
		f.Close()
		t.Fatalf("sizing backup file: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("closing backup file: %v", err)
	}

	fileID := uuid.New()
	fileRepo.Create(&domain.BackupFile{
		ID:       fileID,
		JobID:    uuid.New(),
		FileType: "running",
		FileName: "large.rsc",
		FilePath: path,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-files/"+fileID.String()+"/content", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBackupFileContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	data := decodeBackupContentData(t, rec.Body)

	if got := data["inline"]; got != false {
		t.Fatalf("expected inline false, got %#v", got)
	}
	if got := data["content"]; got != "" {
		t.Fatalf("expected empty content, got %#v", got)
	}
	if got := data["download_url"]; got != "/api/v1/backup-files/"+fileID.String()+"/download" {
		t.Fatalf("unexpected download_url: %#v", got)
	}
	if got := data["reason"]; got != "too_large" {
		t.Fatalf("expected reason too_large, got %#v", got)
	}
	if got := data["size_bytes"]; got != float64(1<<20+1) {
		t.Fatalf("expected size_bytes %d, got %#v", 1<<20+1, got)
	}
	if got := data["max_inline_size_bytes"]; got != float64(1<<20) {
		t.Fatalf("expected max_inline_size_bytes %d, got %#v", 1<<20, got)
	}
}

func TestBackupContent_BinaryReturnsDownloadMetadata(t *testing.T) {
	handler, _, fileRepo := setupBackupHandler(t)

	dir := t.TempDir()
	binaryContent := []byte{0x00, 0x01, 0x02, 0xff}
	path := filepath.Join(dir, "router.backup")
	if err := os.WriteFile(path, binaryContent, 0644); err != nil {
		t.Fatalf("creating backup file: %v", err)
	}

	fileID := uuid.New()
	fileRepo.Create(&domain.BackupFile{
		ID:       fileID,
		JobID:    uuid.New(),
		FileType: "binary",
		FileName: "router.backup",
		FilePath: path,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backup-files/"+fileID.String()+"/content", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBackupFileContent(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	data := decodeBackupContentData(t, rec.Body)

	if got := data["inline"]; got != false {
		t.Fatalf("expected inline false, got %#v", got)
	}
	if got := data["content"]; got != "" {
		t.Fatalf("expected empty content, got %#v", got)
	}
	if got := data["download_url"]; got != "/api/v1/backup-files/"+fileID.String()+"/download" {
		t.Fatalf("unexpected download_url: %#v", got)
	}
	if got := data["reason"]; got != "unsupported_type" {
		t.Fatalf("expected reason unsupported_type, got %#v", got)
	}
	if got := data["size_bytes"]; got != float64(len(binaryContent)) {
		t.Fatalf("expected size_bytes %d, got %#v", len(binaryContent), got)
	}
	if got := data["max_inline_size_bytes"]; got != float64(1<<20) {
		t.Fatalf("expected max_inline_size_bytes %d, got %#v", 1<<20, got)
	}
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

func TestBackupHandlerBulkBackupRejectsInvalidRequestedDeviceID(t *testing.T) {
	handler, _, _, _, _ := setupBackupHandlerForBulkLimitTests(t, t.TempDir())

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/backups/bulk",
		strings.NewReader(`{"device_ids":["not-a-uuid"]}`),
	)
	rec := httptest.NewRecorder()
	handler.HandleBulkBackup(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestBackupHandlerStartBulkBackupRunReturnsPersistedRun(t *testing.T) {
	handler, _, _, deviceRepo, _ := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.10", SysName: "offline-core",
		Managed: true, Status: domain.DeviceStatusDown,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleStartBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID           string `json:"id"`
			Status       string `json:"status"`
			TotalCount   int    `json:"total_count"`
			SkippedCount int    `json:"skipped_count"`
			Items        []struct {
				DeviceID string `json:"device_id"`
				Status   string `json:"status"`
				Reason   string `json:"reason"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if _, err := uuid.Parse(resp.Data.ID); err != nil {
		t.Fatalf("expected run id UUID, got %q", resp.Data.ID)
	}
	if resp.Data.TotalCount != 1 || resp.Data.SkippedCount != 1 {
		t.Fatalf("expected one skipped item, got total=%d skipped=%d", resp.Data.TotalCount, resp.Data.SkippedCount)
	}
	if len(resp.Data.Items) != 1 || resp.Data.Items[0].DeviceID != deviceID.String() || resp.Data.Items[0].Status != "skipped" {
		t.Fatalf("unexpected items: %#v", resp.Data.Items)
	}
	if resp.Data.Items[0].Reason != "device offline" {
		t.Fatalf("expected offline reason, got %q", resp.Data.Items[0].Reason)
	}
}

func TestBackupHandlerStartBulkBackupRunRecordsAuthenticatedActor(t *testing.T) {
	handler, _, _, deviceRepo, _ := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.11", SysName: "actor-core",
		Managed: true, Status: domain.DeviceStatusDown,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	req := withTestOperator(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	handler.HandleStartBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			CreatedBy string `json:"created_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Data.CreatedBy != "test-operator" {
		t.Fatalf("created_by = %q, want test-operator", resp.Data.CreatedBy)
	}
}

func TestBackupHandlerStartBulkBackupRunPersistsAuditLog(t *testing.T) {
	handler, _, _, deviceRepo, _ := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	auditRepo := newBackupAuditRepoForHandler()
	WithBackupAuditLogs(auditRepo)(handler)
	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.12", SysName: "audit-core",
		Managed: true, Status: domain.DeviceStatusDown,
	}); err != nil {
		t.Fatalf("Create device: %v", err)
	}

	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	req := withTestOperator(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	handler.HandleStartBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}

	logs := auditRepo.auditLogs()
	if len(logs) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(logs))
	}
	logEntry := logs[0]
	if logEntry.Action != "backup.bulk_run_started" {
		t.Fatalf("audit action = %q, want backup.bulk_run_started", logEntry.Action)
	}
	if logEntry.Resource != "backup_bulk_run" || logEntry.ResourceID == "" {
		t.Fatalf("audit resource = %q/%q, want backup bulk run with id", logEntry.Resource, logEntry.ResourceID)
	}
	if logEntry.ActorUserID == nil || *logEntry.ActorUserID == uuid.Nil {
		t.Fatalf("ActorUserID = %#v, want authenticated actor", logEntry.ActorUserID)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	assertAuditNumber(t, metadata, "requested_device_count", 1)
	assertAuditNumber(t, metadata, "total_count", 1)
	assertAuditNumber(t, metadata, "skipped_count", 1)
}

func TestBackupHandlerStartBulkBackupRunMapsActiveRunToConflict(t *testing.T) {
	handler, _, _, _, runRepo := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	runID := uuid.New()
	if err := runRepo.CreateRun(&domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.HandleStartBulkBackupRun(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Code string `json:"code"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Code != "bulk_backup_run_active" || resp.Data.ID != runID.String() {
		t.Fatalf("unexpected conflict response: %#v", resp)
	}
}

func TestBackupHandlerGetLatestBulkBackupRunReturnsNullWhenMissing(t *testing.T) {
	handler, _, _, _, _ := setupBackupHandlerForBulkRunTests(t, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/bulk-runs/latest", nil)
	rec := httptest.NewRecorder()
	handler.HandleGetLatestBulkBackupRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if _, ok := resp["data"]; !ok || resp["data"] != nil {
		t.Fatalf("expected data null, got %#v", resp)
	}
}

func TestBackupHandlerBulkBackupRunReportsAggregateProgressAndCurrentJob(t *testing.T) {
	handler, _, _, _, runRepo := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	runID := uuid.New()
	jobID := uuid.New()
	if err := runRepo.CreateRun(&domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}, []domain.BulkBackupRunItem{
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "queued-router", Status: domain.BulkBackupRunItemStatusQueued},
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "running-router", Status: domain.BulkBackupRunItemStatusRunning, BackupJobID: &jobID},
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "active-router", Status: domain.BulkBackupRunItemStatusActive},
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "ok-router", Status: domain.BulkBackupRunItemStatusSuccess},
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "bad-router", Status: domain.BulkBackupRunItemStatusFailed},
		{ID: uuid.New(), DeviceID: uuid.New(), DeviceName: "skipped-router", Status: domain.BulkBackupRunItemStatusSkipped},
	}); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := runRepo.RecalculateRunCounters(runID); err != nil {
		t.Fatalf("RecalculateRunCounters: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups/bulk-runs/"+runID.String(), nil)
	rec := httptest.NewRecorder()
	handler.HandleGetBulkBackupRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			TotalCount        int    `json:"total_count"`
			QueuedCount       int    `json:"queued_count"`
			RunningCount      int    `json:"running_count"`
			CompletedCount    int    `json:"completed_count"`
			SuccessCount      int    `json:"success_count"`
			FailedCount       int    `json:"failed_count"`
			SkippedCount      int    `json:"skipped_count"`
			CurrentDeviceName string `json:"current_device_name"`
			CurrentJobID      string `json:"current_job_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Data.TotalCount != 6 ||
		resp.Data.QueuedCount != 3 ||
		resp.Data.RunningCount != 2 ||
		resp.Data.CompletedCount != 3 ||
		resp.Data.SuccessCount != 1 ||
		resp.Data.FailedCount != 1 ||
		resp.Data.SkippedCount != 1 {
		t.Fatalf("unexpected aggregate counts: %#v", resp.Data)
	}
	if resp.Data.CurrentDeviceName != "running-router" || resp.Data.CurrentJobID != jobID.String() {
		t.Fatalf("unexpected current job details: %#v", resp.Data)
	}
}

func TestBackupHandlerCancelBulkBackupRunMarksCancelling(t *testing.T) {
	handler, _, _, _, runRepo := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	runID := uuid.New()
	if err := runRepo.CreateRun(&domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs/"+runID.String()+"/cancel", nil)
	rec := httptest.NewRecorder()
	handler.HandleCancelBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID              string `json:"id"`
			Status          string `json:"status"`
			CancelRequested bool   `json:"cancel_requested"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Data.ID != runID.String() || resp.Data.Status != "cancelling" || !resp.Data.CancelRequested {
		t.Fatalf("unexpected cancel response: %#v", resp.Data)
	}
}

func TestBackupHandlerPauseBulkBackupRunMarksPausing(t *testing.T) {
	handler, _, _, _, runRepo := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	runID := uuid.New()
	if err := runRepo.CreateRun(&domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusRunning,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs/"+runID.String()+"/pause", nil)
	rec := httptest.NewRecorder()
	handler.HandlePauseBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Data.ID != runID.String() || resp.Data.Status != "pausing" {
		t.Fatalf("unexpected pause response: %#v", resp.Data)
	}
}

func TestBackupHandlerResumeBulkBackupRunMarksRunning(t *testing.T) {
	handler, _, _, _, runRepo := setupBackupHandlerForBulkRunTests(t, t.TempDir())
	runID := uuid.New()
	if err := runRepo.CreateRun(&domain.BulkBackupRun{
		ID:        runID,
		Status:    domain.BulkBackupRunStatusPaused,
		BatchSize: 10,
		CreatedAt: time.Now().UTC(),
	}, nil); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-runs/"+runID.String()+"/resume", nil)
	rec := httptest.NewRecorder()
	handler.HandleResumeBulkBackupRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response: %v", err)
	}
	if resp.Data.ID != runID.String() || resp.Data.Status != "running" {
		t.Fatalf("unexpected resume response: %#v", resp.Data)
	}
}

func TestBackupHandlerBulkBackupMapsLimitErrorToRequestEntityTooLarge(t *testing.T) {
	handler, _, _, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, t.TempDir())
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    1,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	firstID := uuid.New()
	secondID := uuid.New()
	for i, id := range []uuid.UUID{firstID, secondID} {
		if err := deviceRepo.Create(&domain.Device{
			ID: id, IP: fmt.Sprintf("10.0.0.%d", i+1), Vendor: "default",
			Managed: true, Status: domain.DeviceStatusUp,
		}); err != nil {
			t.Fatalf("Create device: %v", err)
		}
	}

	body := fmt.Sprintf(`{"device_ids":[%q,%q]}`, firstID.String(), secondID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBulkBackup(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("limit response must not be a zip response; headers: %#v", rec.Header())
	}
}

func TestBackupHandlerBulkBackupMapsActiveLegacyBulkLeaseToTooManyRequests(t *testing.T) {
	handler, _, _, _, backupSvc := setupBackupHandlerForBulkLimitTests(t, t.TempDir())
	leaseRepo := newBulkOperationLeaseRepoForHandler()
	lease, acquired, err := leaseRepo.TryAcquireBulkOperationLease(context.Background(), "backup.bulk_backup:legacy")
	if err != nil {
		t.Fatalf("TryAcquireBulkOperationLease: %v", err)
	}
	if !acquired {
		t.Fatal("expected test lease acquisition")
	}
	t.Cleanup(func() {
		if err := lease.Release(); err != nil {
			t.Fatalf("Release lease: %v", err)
		}
	})
	backupSvc.SetBulkOperationLeaseRepository(leaseRepo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	handler.HandleBulkBackup(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatalf("expected Retry-After header, got headers: %#v", rec.Header())
	}
}

func TestBackupHandlerBulkDownloadMapsLimitErrorToRequestEntityTooLarge(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    4,
	})

	deviceID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "large.rsc", []byte("12345"))
	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBulkDownload(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("limit response must not be a zip response; headers: %#v", rec.Header())
	}
}

func TestBackupHandlerBulkDownloadReportsSelectedFileAndByteTotals(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	firstID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "first.rsc", []byte("12345"))
	secondID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "second.rsc", []byte("abcdefg"))
	body := fmt.Sprintf(`{"device_ids":[%q,%q]}`, firstID.String(), secondID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBulkDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Bulk-Download-File-Count"); got != "2" {
		t.Fatalf("X-Bulk-Download-File-Count = %q, want 2", got)
	}
	if got := rec.Header().Get("X-Bulk-Download-Size-Bytes"); got != "12" {
		t.Fatalf("X-Bulk-Download-Size-Bytes = %q, want 12", got)
	}
	if got := rec.Header().Get("X-Bulk-Download-Device-Count"); got != "2" {
		t.Fatalf("X-Bulk-Download-Device-Count = %q, want 2", got)
	}
}

func TestBackupHandlerBulkDownloadStreamsSelectedFilesAndKeepsPrevalidatedTotalsOnFileError(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	goodContent := []byte("streamed-good")
	missingContent := []byte("selected-then-removed")
	goodID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "good.rsc", goodContent)
	missingID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "missing-after-selection.rsc", missingContent)
	unselectedID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: unselectedID, IP: "10.0.0.200", SysName: "empty",
		Tags: map[string]string{"display_name": "Empty"},
	}); err != nil {
		t.Fatalf("Create unselected device: %v", err)
	}

	missingPath := filepath.Join(backupDir, "missing-after-selection.rsc")
	handler.settingsRepo = &bulkDownloadDeletingSettingsRepo{
		mockSettingsRepo: newMockSettingsRepo(),
		path:             missingPath,
	}

	body := fmt.Sprintf(`{"device_ids":[%q,%q,%q]}`, goodID.String(), missingID.String(), unselectedID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBulkDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Bulk-Download-File-Count"); got != "2" {
		t.Fatalf("X-Bulk-Download-File-Count = %q, want 2", got)
	}
	if got := rec.Header().Get("X-Bulk-Download-Size-Bytes"); got != fmt.Sprint(len(goodContent)+len(missingContent)) {
		t.Fatalf("X-Bulk-Download-Size-Bytes = %q, want %d", got, len(goodContent)+len(missingContent))
	}

	zipEntries := readZipEntriesForBulkDownloadTest(t, rec.Body.Bytes())
	if got := zipEntries["Core/good.rsc"]; got != string(goodContent) {
		t.Fatalf("Core/good.rsc = %q, want %q", got, goodContent)
	}
	if _, ok := zipEntries["Core/missing-after-selection.rsc"]; ok {
		t.Fatal("removed file must not be included as a partial zip entry")
	}
	errorManifest, ok := zipEntries["_errors.txt"]
	if !ok {
		t.Fatal("expected _errors.txt for removed selected file")
	}
	if !strings.Contains(errorManifest, "Core/missing-after-selection.rsc") {
		t.Fatalf("_errors.txt = %q, want selected zip path", errorManifest)
	}
	if got := rec.Header().Get("X-Bulk-Download-Device-Count"); got != "2" {
		t.Fatalf("X-Bulk-Download-Device-Count = %q, want 2 selected devices", got)
	}
}

func TestBackupHandlerBulkDownloadPersistsAuditLogWithPartialStreamErrors(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	auditRepo := newBackupAuditRepoForHandler()
	WithBackupAuditLogs(auditRepo)(handler)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	goodContent := []byte("streamed-good")
	missingContent := []byte("selected-then-removed")
	goodID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "good.rsc", goodContent)
	missingID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "missing-after-selection.rsc", missingContent)
	unselectedID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: unselectedID, IP: "10.0.0.200", SysName: "empty",
		Tags: map[string]string{"display_name": "Empty"},
	}); err != nil {
		t.Fatalf("Create unselected device: %v", err)
	}

	missingPath := filepath.Join(backupDir, "missing-after-selection.rsc")
	handler.settingsRepo = &bulkDownloadDeletingSettingsRepo{
		mockSettingsRepo: newMockSettingsRepo(),
		path:             missingPath,
	}

	body := fmt.Sprintf(`{"device_ids":[%q,%q,%q]}`, goodID.String(), missingID.String(), unselectedID.String())
	req := withTestOperator(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)))
	rec := httptest.NewRecorder()
	handler.HandleBulkDownload(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	logs := auditRepo.auditLogs()
	if len(logs) != 1 {
		t.Fatalf("audit log count = %d, want 1", len(logs))
	}
	logEntry := logs[0]
	if logEntry.Action != "backup.bulk_download_completed" {
		t.Fatalf("audit action = %q, want backup.bulk_download_completed", logEntry.Action)
	}
	if logEntry.ActorUserID == nil || *logEntry.ActorUserID == uuid.Nil {
		t.Fatalf("ActorUserID = %#v, want authenticated actor", logEntry.ActorUserID)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	assertAuditNumber(t, metadata, "requested_device_count", 3)
	assertAuditNumber(t, metadata, "selected_device_count", 2)
	assertAuditNumber(t, metadata, "selected_file_count", 2)
	assertAuditNumber(t, metadata, "selected_bytes", float64(len(goodContent)+len(missingContent)))
	assertAuditNumber(t, metadata, "stream_error_count", 1)
	if partial, ok := metadata["partial"].(bool); !ok || !partial {
		t.Fatalf("metadata partial = %#v, want true", metadata["partial"])
	}
}

func TestBackupHandlerBulkDownloadRejectsConcurrentRequestForSameActor(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	deviceID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "running.rsc", []byte("streamed-content"))
	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	actorID := uuid.New()

	firstReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	firstRec := newBlockingResponseWriterForBulkDownloadTest()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.HandleBulkDownload(firstRec, firstReq)
	}()

	select {
	case <-firstRec.firstWrite:
	case <-time.After(time.Second):
		close(firstRec.release)
		<-firstDone
		t.Fatal("first bulk download did not start streaming")
	}

	secondReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	secondRec := httptest.NewRecorder()
	handler.HandleBulkDownload(secondRec, secondReq)

	close(firstRec.release)
	<-firstDone

	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second response status = %d, want 429; body: %s", secondRec.Code, secondRec.Body.String())
	}
	if strings.Contains(secondRec.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("rejected concurrent request must not start a zip response; headers: %#v", secondRec.Header())
	}
}

func TestBackupHandlerBulkDownloadRejectsConcurrentRequestAcrossHandlersForSameActor(t *testing.T) {
	backupDir := t.TempDir()
	firstHandler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})
	deviceID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "running.rsc", []byte("streamed-content"))
	leaseRepo := newBulkOperationLeaseRepoForHandler()
	auditRepo := newBackupAuditRepoForHandler()
	WithBulkDownloadLeaseRepository(leaseRepo)(firstHandler)
	WithBackupAuditLogs(auditRepo)(firstHandler)
	secondHandler := NewBackupHandler(backupSvc, firstHandler.settingsRepo, WithBulkDownloadLeaseRepository(leaseRepo), WithBackupAuditLogs(auditRepo))

	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	actorID := uuid.New()
	firstReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	firstRec := newBlockingResponseWriterForBulkDownloadTest()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		firstHandler.HandleBulkDownload(firstRec, firstReq)
	}()

	select {
	case <-firstRec.firstWrite:
	case <-time.After(time.Second):
		close(firstRec.release)
		<-firstDone
		t.Fatal("first bulk download did not start streaming")
	}

	secondReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	secondRec := httptest.NewRecorder()
	secondHandler.HandleBulkDownload(secondRec, secondReq)
	if secondRec.Code != http.StatusTooManyRequests {
		close(firstRec.release)
		<-firstDone
		t.Fatalf("second status = %d, want 429; body: %s", secondRec.Code, secondRec.Body.String())
	}
	if got := secondRec.Header().Get("Retry-After"); got != fmt.Sprint(bulkOperationRetryAfterSeconds) {
		close(firstRec.release)
		<-firstDone
		t.Fatalf("Retry-After = %q, want %d", got, bulkOperationRetryAfterSeconds)
	}
	logEntry, ok := findBackupAuditAction(auditRepo.auditLogs(), "backup.bulk_download_rejected")
	if !ok {
		close(firstRec.release)
		<-firstDone
		t.Fatalf("expected backup.bulk_download_rejected audit log, got %#v", auditRepo.auditLogs())
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry.MetadataJSON), &metadata); err != nil {
		close(firstRec.release)
		<-firstDone
		t.Fatalf("decode audit metadata: %v", err)
	}
	if reason, ok := metadata["reason"].(string); !ok || reason != "distributed_actor_concurrency_limit" {
		close(firstRec.release)
		<-firstDone
		t.Fatalf("metadata reason = %#v, want distributed_actor_concurrency_limit", metadata["reason"])
	}

	close(firstRec.release)
	<-firstDone

	thirdReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	thirdRec := httptest.NewRecorder()
	secondHandler.HandleBulkDownload(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusOK {
		t.Fatalf("third status after release = %d, want 200; body: %s", thirdRec.Code, thirdRec.Body.String())
	}
}

func TestBackupHandlerBulkDownloadRejectsConcurrentRequestAtGlobalLimit(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, _ := setupBackupHandlerForBulkDownloadConcurrencyTest(t, backupDir, service.BulkOperationLimits{
		BulkBackupMaxDevices:              10,
		BulkBackupMaxQueuedJobs:           10,
		BulkDownloadMaxDevices:            10,
		BulkDownloadMaxFiles:              10,
		BulkDownloadMaxBytes:              1024,
		BulkDownloadMaxConcurrentPerActor: 10,
		BulkDownloadMaxConcurrentGlobal:   1,
	})
	deviceID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "running.rsc", []byte("streamed-content"))
	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())

	firstReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), uuid.New())
	firstRec := newBlockingResponseWriterForBulkDownloadTest()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.HandleBulkDownload(firstRec, firstReq)
	}()
	select {
	case <-firstRec.firstWrite:
	case <-time.After(time.Second):
		close(firstRec.release)
		<-firstDone
		t.Fatal("first bulk download did not start streaming")
	}

	secondReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), uuid.New())
	secondRec := httptest.NewRecorder()
	handler.HandleBulkDownload(secondRec, secondReq)
	close(firstRec.release)
	<-firstDone

	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 at global limit; body: %s", secondRec.Code, secondRec.Body.String())
	}
	if got := secondRec.Header().Get("Retry-After"); got != fmt.Sprint(bulkOperationRetryAfterSeconds) {
		t.Fatalf("Retry-After = %q, want %d", got, bulkOperationRetryAfterSeconds)
	}
}

func TestBackupHandlerBulkDownloadAuditsConcurrentRequestRejection(t *testing.T) {
	backupDir := t.TempDir()
	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	auditRepo := newBackupAuditRepoForHandler()
	WithBackupAuditLogs(auditRepo)(handler)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	deviceID := seedBulkDownloadBackupFile(t, backupDir, jobRepo, fileRepo, deviceRepo, "running.rsc", []byte("streamed-content"))
	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	actorID := uuid.New()

	firstReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	firstRec := newBlockingResponseWriterForBulkDownloadTest()
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.HandleBulkDownload(firstRec, firstReq)
	}()

	select {
	case <-firstRec.firstWrite:
	case <-time.After(time.Second):
		close(firstRec.release)
		<-firstDone
		t.Fatal("first bulk download did not start streaming")
	}

	secondReq := withBulkDownloadTestActor(httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body)), actorID)
	secondRec := httptest.NewRecorder()
	handler.HandleBulkDownload(secondRec, secondReq)

	close(firstRec.release)
	<-firstDone

	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("second response status = %d, want 429; body: %s", secondRec.Code, secondRec.Body.String())
	}

	logEntry, ok := findBackupAuditAction(auditRepo.auditLogs(), "backup.bulk_download_rejected")
	if !ok {
		t.Fatalf("expected backup.bulk_download_rejected audit log, got %#v", auditRepo.auditLogs())
	}
	if logEntry.ActorUserID == nil || *logEntry.ActorUserID != actorID {
		t.Fatalf("ActorUserID = %#v, want %s", logEntry.ActorUserID, actorID)
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(logEntry.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode audit metadata: %v", err)
	}
	if reason, ok := metadata["reason"].(string); !ok || reason != "actor_concurrency_limit" {
		t.Fatalf("metadata reason = %#v, want actor_concurrency_limit", metadata["reason"])
	}
	assertAuditNumber(t, metadata, "requested_device_count", 1)
	assertAuditNumber(t, metadata, "per_actor_limit", 1)
}

func TestBackupHandlerBulkDownloadMapsUnsafePathToBadRequest(t *testing.T) {
	backupDir := t.TempDir()
	outsideDir := t.TempDir()
	outsidePath := filepath.Join(outsideDir, "outside.rsc")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0644); err != nil {
		t.Fatalf("WriteFile outside backup: %v", err)
	}

	handler, jobRepo, fileRepo, deviceRepo, backupSvc := setupBackupHandlerForBulkLimitTests(t, backupDir)
	backupSvc.SetBulkOperationLimits(service.BulkOperationLimits{
		BulkBackupMaxDevices:    10,
		BulkBackupMaxQueuedJobs: 10,
		BulkDownloadMaxDevices:  10,
		BulkDownloadMaxFiles:    10,
		BulkDownloadMaxBytes:    1024,
	})

	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: "10.0.0.1", SysName: "core",
		Tags: map[string]string{"display_name": "Core"},
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
	if err := fileRepo.Create(&domain.BackupFile{
		ID:       uuid.New(),
		JobID:    jobID,
		FileType: "running",
		FileName: "outside.rsc",
		FilePath: outsidePath,
	}); err != nil {
		t.Fatalf("Create file: %v", err)
	}

	body := fmt.Sprintf(`{"device_ids":[%q]}`, deviceID.String())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/bulk-download", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.HandleBulkDownload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Header().Get("Content-Type"), "application/zip") {
		t.Fatalf("path rejection must not be a zip response; headers: %#v", rec.Header())
	}
}

func setupBackupHandlerForBulkLimitTests(
	t *testing.T,
	backupDir string,
) (*BackupHandler, *backupJobRepoForHandler, *backupFileRepoForHandler, *mockDeviceRepo, *service.BackupService) {
	t.Helper()

	jobRepo := newBackupJobRepoForHandler()
	fileRepo := newBackupFileRepoForHandler()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockSettingsRepo()
	encKey := crypto.DeriveKey("test-backup-bulk-limits-key")

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
		reg, &mockSSHDialerForBackup{}, encKey, backupDir,
		gossh.InsecureIgnoreHostKey(),
	)
	handler := NewBackupHandler(backupSvc, settingsRepo)
	return handler, jobRepo, fileRepo, deviceRepo, backupSvc
}

func setupBackupHandlerForBulkDownloadConcurrencyTest(
	t *testing.T,
	backupDir string,
	limits service.BulkOperationLimits,
) (*BackupHandler, *backupJobRepoForHandler, *backupFileRepoForHandler, *mockDeviceRepo, *service.BackupService) {
	t.Helper()

	jobRepo := newBackupJobRepoForHandler()
	fileRepo := newBackupFileRepoForHandler()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockSettingsRepo()
	encKey := crypto.DeriveKey("test-backup-bulk-concurrency-key")

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
		reg, &mockSSHDialerForBackup{}, encKey, backupDir,
		gossh.InsecureIgnoreHostKey(),
	)
	backupSvc.SetBulkOperationLimits(limits)
	handler := NewBackupHandler(backupSvc, settingsRepo)
	return handler, jobRepo, fileRepo, deviceRepo, backupSvc
}

type backupAuditRepoForHandler struct {
	mu   sync.Mutex
	logs []domain.AuditLog
}

func newBackupAuditRepoForHandler() *backupAuditRepoForHandler {
	return &backupAuditRepoForHandler{}
}

func (r *backupAuditRepoForHandler) AppendAuditLog(_ context.Context, log *domain.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if log == nil {
		return nil
	}
	r.logs = append(r.logs, *log)
	return nil
}

func (r *backupAuditRepoForHandler) ListAuditLogs(_ context.Context, _ domain.AuditLogFilter) ([]domain.AuditLog, error) {
	return r.auditLogs(), nil
}

func (r *backupAuditRepoForHandler) DashboardStats(context.Context) (*domain.AdminDashboardStats, error) {
	return &domain.AdminDashboardStats{}, nil
}

func (r *backupAuditRepoForHandler) auditLogs() []domain.AuditLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.AuditLog, len(r.logs))
	copy(out, r.logs)
	return out
}

func assertAuditNumber(t *testing.T, metadata map[string]interface{}, key string, want float64) {
	t.Helper()
	got, ok := metadata[key].(float64)
	if !ok || got != want {
		t.Fatalf("metadata[%q] = %#v, want %v", key, metadata[key], want)
	}
}

func findBackupAuditAction(logs []domain.AuditLog, action string) (domain.AuditLog, bool) {
	for _, logEntry := range logs {
		if logEntry.Action == action {
			return logEntry, true
		}
	}
	return domain.AuditLog{}, false
}

type bulkOperationLeaseRepoForHandler struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func newBulkOperationLeaseRepoForHandler() *bulkOperationLeaseRepoForHandler {
	return &bulkOperationLeaseRepoForHandler{
		active: make(map[string]struct{}),
	}
}

func (r *bulkOperationLeaseRepoForHandler) TryAcquireBulkOperationLease(_ context.Context, key string) (domain.BulkOperationLease, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.active[key]; ok {
		return nil, false, nil
	}
	r.active[key] = struct{}{}
	return bulkOperationLeaseForHandler{release: func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		delete(r.active, key)
	}}, true, nil
}

type bulkOperationLeaseForHandler struct {
	release func()
}

func (l bulkOperationLeaseForHandler) Release() error {
	if l.release != nil {
		l.release()
	}
	return nil
}

type blockingResponseWriterForBulkDownloadTest struct {
	header     http.Header
	firstWrite chan struct{}
	release    chan struct{}
	once       sync.Once
	body       bytes.Buffer
	code       int
}

func newBlockingResponseWriterForBulkDownloadTest() *blockingResponseWriterForBulkDownloadTest {
	return &blockingResponseWriterForBulkDownloadTest{
		header:     http.Header{},
		firstWrite: make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (w *blockingResponseWriterForBulkDownloadTest) Header() http.Header {
	return w.header
}

func (w *blockingResponseWriterForBulkDownloadTest) WriteHeader(code int) {
	w.code = code
}

func (w *blockingResponseWriterForBulkDownloadTest) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.firstWrite)
		<-w.release
	})
	if w.code == 0 {
		w.code = http.StatusOK
	}
	return w.body.Write(p)
}

func withBulkDownloadTestActor(req *http.Request, userID uuid.UUID) *http.Request {
	return req.WithContext(withAuthenticatedUser(req.Context(), &service.AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{
			User: domain.User{
				ID:          userID,
				Username:    "bulk-operator",
				Email:       "bulk-operator@example.test",
				DisplayName: "Bulk Operator",
				Status:      domain.UserStatusActive,
			},
		},
		Session: service.AuthenticatedSession{
			ID:     uuid.New(),
			UserID: userID,
		},
	}))
}

func setupBackupHandlerForBulkRunTests(
	t *testing.T,
	backupDir string,
) (*BackupHandler, *backupJobRepoForHandler, *backupFileRepoForHandler, *mockDeviceRepo, *bulkBackupRunRepoForHandler) {
	t.Helper()

	jobRepo := newBackupJobRepoForHandler()
	fileRepo := newBackupFileRepoForHandler()
	runRepo := newBulkBackupRunRepoForHandler()
	credentialProfileRepo := newMockCredentialProfileRepo()
	deviceRepo := newMockDeviceRepo()
	settingsRepo := newMockSettingsRepo()
	encKey := crypto.DeriveKey("test-backup-bulk-run-key")

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
		reg, &mockSSHDialerForBackup{}, encKey, backupDir,
		gossh.InsecureIgnoreHostKey(),
		service.WithBulkBackupRunRepo(runRepo),
	)
	handler := NewBackupHandler(backupSvc, settingsRepo)
	return handler, jobRepo, fileRepo, deviceRepo, runRepo
}

func seedBulkDownloadBackupFile(
	t *testing.T,
	backupDir string,
	jobRepo *backupJobRepoForHandler,
	fileRepo *backupFileRepoForHandler,
	deviceRepo *mockDeviceRepo,
	fileName string,
	content []byte,
) uuid.UUID {
	t.Helper()

	deviceID := uuid.New()
	if err := deviceRepo.Create(&domain.Device{
		ID: deviceID, IP: fmt.Sprintf("10.%d.%d.%d", deviceID[0], deviceID[1], deviceID[2]), SysName: "core",
		Tags: map[string]string{"display_name": "Core"},
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
	path := filepath.Join(backupDir, fileName)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile backup: %v", err)
	}
	if err := fileRepo.Create(&domain.BackupFile{
		ID:       uuid.New(),
		JobID:    jobID,
		FileType: "running",
		FileName: fileName,
		FilePath: path,
	}); err != nil {
		t.Fatalf("Create file: %v", err)
	}
	return deviceID
}

type bulkDownloadDeletingSettingsRepo struct {
	*mockSettingsRepo
	path string
	once sync.Once
}

func (r *bulkDownloadDeletingSettingsRepo) Get(key string) (string, error) {
	r.once.Do(func() {
		_ = os.Remove(r.path)
	})
	return r.mockSettingsRepo.Get(key)
}

func readZipEntriesForBulkDownloadTest(t *testing.T, body []byte) map[string]string {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("read zip response: %v", err)
	}
	entries := make(map[string]string, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, err)
		}
		entries[f.Name] = string(content)
	}
	return entries
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
