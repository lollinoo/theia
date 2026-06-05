package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/observability"
	"github.com/lollinoo/theia/internal/service"
)

func TestInstanceBackupHandlerRestoreStatusReturnsPersistedOperation(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, ".theia-restore-status.json"), []byte(`{
		"operation_id": "restore-1",
		"phase": "failed_operator_action_required",
		"attempt_count": 2,
		"last_error": "Restore is blocked because key id \"kid-old\" is missing from THEIA_ENCRYPTION_KEYS",
		"missing_key_id": "kid-old",
		"created_at": "2026-06-05T00:00:00Z",
		"updated_at": "2026-06-05T00:01:00Z"
	}`), 0o600); err != nil {
		t.Fatalf("writing restore status: %v", err)
	}
	handler := NewInstanceBackupHandler(newInstanceBackupRestoreTestServiceWithStateDir(t, stateDir, service.DefaultRestoreArchiveLimits))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance-backups/restore-status", nil)
	handler.HandleRestoreStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := payload["data"]
	if data["phase"] != "failed_operator_action_required" {
		t.Fatalf("phase = %#v, want failed_operator_action_required", data["phase"])
	}
	if data["missing_key_id"] != "kid-old" {
		t.Fatalf("missing_key_id = %#v, want kid-old", data["missing_key_id"])
	}
}

func TestInstanceBackupHandlerRestoreStatusReturnsNullWhenAbsent(t *testing.T) {
	handler := NewInstanceBackupHandler(newInstanceBackupRestoreTestService(t, service.DefaultRestoreArchiveLimits))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/instance-backups/restore-status", nil)
	handler.HandleRestoreStatus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"data":null}` {
		t.Fatalf("body = %s, want data null", got)
	}
}

func TestInstanceBackupHandlerRestoreReportsCompressedUploadLimitRejection(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	handler := NewInstanceBackupHandler(newInstanceBackupRestoreTestService(t, service.RestoreArchiveLimits{
		MaxCompressedBytes: 3,
		MaxTotalBytes:      1 << 20,
		MaxEntryBytes:      1 << 20,
		MaxFileEntries:     10,
	}))

	req := newInstanceBackupRestoreUploadRequest(t, []byte("1234"))
	rec := httptest.NewRecorder()
	handler.HandleRestore(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body.String())
	}
	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_bulk_operation_rejections_total{operation="instance_restore",reason="compressed_size_limit",source="local"} 1`) {
		t.Fatalf("expected compressed restore rejection metric, got:\n%s", metrics)
	}
}

func TestInstanceBackupHandlerRestoreReportsArchiveQuotaLimitRejection(t *testing.T) {
	registry := observability.ResetDefaultForTest()
	handler := NewInstanceBackupHandler(newInstanceBackupRestoreTestService(t, service.RestoreArchiveLimits{
		MaxCompressedBytes: 1 << 20,
		MaxTotalBytes:      1 << 20,
		MaxEntryBytes:      3,
		MaxFileEntries:     10,
	}))

	req := newInstanceBackupRestoreUploadRequest(t, instanceBackupRestoreTestArchive(t, "manifest.json", "1234"))
	rec := httptest.NewRecorder()
	handler.HandleRestore(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", rec.Code, rec.Body.String())
	}
	metrics := string(registry.MarshalPrometheus())
	if !strings.Contains(metrics, `theia_bulk_operation_rejections_total{operation="instance_restore",reason="archive_quota_limit",source="local"} 1`) {
		t.Fatalf("expected archive restore rejection metric, got:\n%s", metrics)
	}
}

func newInstanceBackupRestoreTestService(t *testing.T, limits service.RestoreArchiveLimits) *service.InstanceBackupService {
	t.Helper()
	return newInstanceBackupRestoreTestServiceWithStateDir(t, t.TempDir(), limits)
}

func newInstanceBackupRestoreTestServiceWithStateDir(t *testing.T, stateDir string, limits service.RestoreArchiveLimits) *service.InstanceBackupService {
	t.Helper()
	tmpDir := t.TempDir()
	svc := service.NewInstanceBackupService(
		nil,
		nil,
		nil,
		tmpDir,
		t.TempDir(),
		"",
		stateDir,
		"",
		[]byte("0123456789abcdef"),
	)
	svc.SetRestoreArchiveLimitsForTest(limits)
	return svc
}

func newInstanceBackupRestoreUploadRequest(t *testing.T, payload []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "restore.tar.gz")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("write upload payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/instance-backups/restore?dry_run=true", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func instanceBackupRestoreTestArchive(t *testing.T, name, content string) []byte {
	t.Helper()
	var body bytes.Buffer
	gzipWriter := gzip.NewWriter(&body)
	tarWriter := tar.NewWriter(gzipWriter)
	data := []byte(content)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(data)),
	}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatalf("write tar entry: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return body.Bytes()
}
