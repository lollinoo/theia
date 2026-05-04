package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// InstanceBackupHandler provides HTTP handlers for instance backup operations.
type InstanceBackupHandler struct {
	svc       *service.InstanceBackupService
	mu        sync.Mutex
	restarter func()
}

const restoreMultipartEnvelopeOverheadBytes int64 = 1 << 20

// NewInstanceBackupHandler creates a new InstanceBackupHandler.
func NewInstanceBackupHandler(svc *service.InstanceBackupService) *InstanceBackupHandler {
	return NewInstanceBackupHandlerWithRestarter(svc, nil)
}

// NewInstanceBackupHandlerWithRestarter creates a restore handler with an injected restart handoff.
func NewInstanceBackupHandlerWithRestarter(svc *service.InstanceBackupService, restarter func()) *InstanceBackupHandler {
	if restarter == nil {
		restarter = func() {}
	}
	return &InstanceBackupHandler{
		svc:       svc,
		restarter: restarter,
	}
}

func (h *InstanceBackupHandler) ensureConfigured(w http.ResponseWriter) bool {
	if h.svc != nil {
		return true
	}
	writeError(w, http.StatusNotImplemented, "instance backups are not configured for this runtime")
	return false
}

// HandleCreate handles POST /api/v1/instance-backups.
// Returns 202 Accepted with backup record in "running" status.
// Returns 409 Conflict if a backup is already in progress.
func (h *InstanceBackupHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	h.mu.Lock()

	// Check for already-running backup
	backups, err := h.svc.List(r.Context())
	if err != nil {
		h.mu.Unlock()
		writeError(w, http.StatusInternalServerError, "checking backup status", err)
		return
	}
	for _, b := range backups {
		if b.Status == domain.InstanceBackupStatusRunning {
			h.mu.Unlock()
			writeError(w, http.StatusConflict, "a backup is already in progress")
			return
		}
	}

	// Track current count to detect the new record regardless of status
	prevCount := len(backups)

	// Launch async backup — use context.Background() so it outlives the HTTP request
	go func() {
		if _, err := h.svc.Create(context.Background()); err != nil {
			log.Printf("Instance backup failed: %v", err)
		}
	}()

	h.mu.Unlock()

	// Wait briefly for the new backup record to appear (may already be complete if DB is small)
	var created *domain.InstanceBackup
	for attempt := 0; attempt < 10; attempt++ {
		time.Sleep(50 * time.Millisecond)
		list, err := h.svc.List(r.Context())
		if err != nil {
			continue
		}
		if len(list) > prevCount {
			created = &list[0] // list is sorted newest first
			break
		}
	}

	w.WriteHeader(http.StatusAccepted)
	if created != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": instanceBackupToMap(*created),
		})
	} else {
		// Fallback: backup is running but we couldn't fetch the record yet
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{"status": "running"},
		})
	}
}

// HandleList handles GET /api/v1/instance-backups.
func (h *InstanceBackupHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	backups, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing backups", err)
		return
	}
	data := make([]map[string]interface{}, 0, len(backups))
	for _, b := range backups {
		data = append(data, instanceBackupToMap(b))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// HandleGet handles GET /api/v1/instance-backups/{id}.
func (h *InstanceBackupHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/instance-backups/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup ID")
		return
	}
	backup, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "getting backup", err)
		return
	}
	if backup == nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": instanceBackupToMap(*backup),
	})
}

// HandleDownload handles GET /api/v1/instance-backups/{id}/download.
// Streams the .tar.gz archive with Content-Disposition and Content-Length headers.
func (h *InstanceBackupHandler) HandleDownload(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	// Parse: /api/v1/instance-backups/{id}/download
	path := strings.TrimSuffix(r.URL.Path, "/download")
	id, err := extractIDFromPath(path, "/api/v1/instance-backups/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup ID")
		return
	}
	backup, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "getting backup", err)
		return
	}
	if backup == nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}
	if backup.Status != domain.InstanceBackupStatusSuccess {
		writeError(w, http.StatusBadRequest, "backup is not ready for download")
		return
	}
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "backup archive file not found on disk")
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeFilename(backup.FileName)))
	http.ServeFile(w, r, backup.FilePath)
}

// HandleDelete handles DELETE /api/v1/instance-backups/{id}.
func (h *InstanceBackupHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/instance-backups/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup ID")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "deleting backup", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleRestore handles POST /api/v1/instance-backups/restore.
// Accepts multipart/form-data with a "file" field containing a .tar.gz archive.
// Query param dry_run=true validates without staging.
func (h *InstanceBackupHandler) HandleRestore(w http.ResponseWriter, r *http.Request) {
	if !h.ensureConfigured(w) {
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "true"
	limits := h.svc.RestoreArchiveLimits()
	compressedLimit := limits.MaxCompressedBytes

	// Parse multipart form (32MB memory buffer; larger files spill to disk)
	if compressedLimit > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, compressedLimit+restoreMultipartEnvelopeOverheadBytes)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if isRequestBodyTooLarge(err) {
			writeError(w, http.StatusRequestEntityTooLarge, "restore upload exceeds maximum compressed size")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid multipart form data")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing 'file' field in multipart form")
		return
	}
	defer file.Close()

	// Validate filename
	if !strings.HasSuffix(header.Filename, ".tar.gz") {
		writeError(w, http.StatusBadRequest, "file must be a .tar.gz archive")
		return
	}

	// Write to temp file
	tmpFile, err := os.CreateTemp("", "theia-restore-upload-*.tar.gz")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creating temp file", err)
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Stream file to temp
	written, err := copyRestoreUpload(tmpFile, file, compressedLimit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "writing upload to temp file", err)
		return
	}
	if compressedLimit > 0 && written > compressedLimit {
		writeError(w, http.StatusRequestEntityTooLarge, "restore upload exceeds maximum compressed size")
		return
	}
	tmpFile.Close()

	// Validate and stage restore
	report, err := h.svc.ValidateAndStageRestore(tmpFile.Name(), dryRun)
	if err != nil {
		var limitErr *service.RestoreLimitError
		if errors.As(err, &limitErr) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Return report
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": report,
	})

	// If not dry run and valid, trigger the runtime restart handoff after response is written.
	if !dryRun && report.Valid {
		go h.restarter()
	}
}

func isRequestBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func copyRestoreUpload(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit <= 0 {
		return io.Copy(dst, src)
	}
	limited := &io.LimitedReader{R: src, N: limit + 1}
	return io.Copy(dst, limited)
}

// --- Helpers ---

func instanceBackupToMap(b domain.InstanceBackup) map[string]interface{} {
	return map[string]interface{}{
		"id":                b.ID.String(),
		"file_name":         b.FileName,
		"size_bytes":        b.SizeBytes,
		"sha256":            b.SHA256,
		"app_version":       b.AppVersion,
		"migration_version": b.MigrationVersion,
		"status":            string(b.Status),
		"error_message":     b.ErrorMessage,
		"trigger":           string(b.Trigger),
		"created_at":        b.CreatedAt,
	}
}
