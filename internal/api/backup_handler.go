package api

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// maxInlineBackupContentBytes caps JSON previews so backup content is not
// loaded into memory for large files. Full content remains available through
// the download endpoint.
const maxInlineBackupContentBytes int64 = 1 << 20

const (
	maxConcurrentBulkDownloadsPerActor = 1
	bulkDownloadRetryAfterSeconds      = 30
)

// BackupHandler provides HTTP handlers for SSH credentials and config backups.
type BackupHandler struct {
	svc                 *service.BackupService
	settingsRepo        domain.SettingsRepository
	auditLogs           domain.AuditLogRepository
	bulkDownloadLimiter *bulkOperationLimiter
}

type BackupHandlerOption func(*BackupHandler)

func WithBackupAuditLogs(auditLogs domain.AuditLogRepository) BackupHandlerOption {
	return func(h *BackupHandler) {
		h.auditLogs = auditLogs
	}
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(svc *service.BackupService, settingsRepo domain.SettingsRepository, opts ...BackupHandlerOption) *BackupHandler {
	handler := &BackupHandler{
		svc:                 svc,
		settingsRepo:        settingsRepo,
		bulkDownloadLimiter: newBulkOperationLimiter(maxConcurrentBulkDownloadsPerActor),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(handler)
		}
	}
	return handler
}

// HandleTestSSH handles POST /api/v1/devices/{id}/ssh-credentials/test
func (h *BackupHandler) HandleTestSSH(w http.ResponseWriter, r *http.Request) {
	deviceID, err := extractDeviceIDForBackup(r.URL.Path, "/ssh-credentials/test")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	if err := h.svc.TestSSHConnection(r.Context(), deviceID); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// HandleListBackups handles GET /api/v1/devices/{id}/backups
func (h *BackupHandler) HandleListBackups(w http.ResponseWriter, r *http.Request) {
	deviceID, err := extractDeviceIDForBackup(r.URL.Path, "/backups")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	jobs, err := h.svc.GetBackupJobs(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	data := make([]map[string]interface{}, 0, len(jobs))
	for _, j := range jobs {
		data = append(data, jobToMap(j))
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// HandleTriggerBackup handles POST /api/v1/devices/{id}/backups
func (h *BackupHandler) HandleTriggerBackup(w http.ResponseWriter, r *http.Request) {
	deviceID, err := extractDeviceIDForBackup(r.URL.Path, "/backups")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	job, err := h.svc.TriggerBackup(r.Context(), deviceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no SSH credentials") || strings.Contains(err.Error(), "not configured") || strings.Contains(err.Error(), "require MikroTik") || strings.Contains(err.Error(), "unreachable") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": jobToMap(*job),
	})
}

// HandleGetLatestBackup handles GET /api/v1/devices/{id}/backups/latest
func (h *BackupHandler) HandleGetLatestBackup(w http.ResponseWriter, r *http.Request) {
	deviceID, err := extractDeviceIDForBackup(r.URL.Path, "/backups/latest")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	job, err := h.svc.GetLatestBackupJob(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "no successful backup found")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": jobToMap(*job),
	})
}

// HandleGetBackupJob handles GET /api/v1/backup-jobs/{id}
func (h *BackupHandler) HandleGetBackupJob(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/backup-jobs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup job ID")
		return
	}

	job, err := h.svc.GetBackupJob(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if job == nil {
		writeError(w, http.StatusNotFound, "backup job not found")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": jobToMap(*job),
	})
}

// HandleDeleteBackupJob handles DELETE /api/v1/backup-jobs/{id}
func (h *BackupHandler) HandleDeleteBackupJob(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/backup-jobs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup job ID")
		return
	}

	if err := h.svc.DeleteBackupJob(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleDownloadBackupFile handles GET /api/v1/backup-files/{id}/download
func (h *BackupHandler) HandleDownloadBackupFile(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/v1/backup-files/{id}/download
	path := strings.TrimSuffix(r.URL.Path, "/download")
	id, err := extractIDFromPath(path, "/api/v1/backup-files/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup file ID")
		return
	}

	file, err := h.svc.GetBackupFile(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if file == nil {
		writeError(w, http.StatusNotFound, "backup file not found")
		return
	}

	// Set content type based on extension
	ext := filepath.Ext(file.FileName)
	switch ext {
	case ".rsc":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sanitizeFilename(file.FileName)+"\"")

	http.ServeFile(w, r, file.FilePath)
}

// HandleGetBackupFileContent handles GET /api/v1/backup-files/{id}/content
func (h *BackupHandler) HandleGetBackupFileContent(w http.ResponseWriter, r *http.Request) {
	// Parse: /api/v1/backup-files/{id}/content
	path := strings.TrimSuffix(r.URL.Path, "/content")
	id, err := extractIDFromPath(path, "/api/v1/backup-files/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup file ID")
		return
	}

	file, err := h.svc.GetBackupFile(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if file == nil {
		writeError(w, http.StatusNotFound, "backup file not found")
		return
	}

	info, err := os.Stat(file.FilePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	sizeBytes := info.Size()
	if !isInlineBackupTextFile(file) {
		writeBackupContentMetadata(w, id, false, "", sizeBytes, "unsupported_type")
		return
	}
	if sizeBytes > maxInlineBackupContentBytes {
		writeBackupContentMetadata(w, id, false, "", sizeBytes, "too_large")
		return
	}

	rc, _, err := h.svc.GetBackupFileContent(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	defer rc.Close()

	content, err := io.ReadAll(io.LimitReader(rc, maxInlineBackupContentBytes+1))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if int64(len(content)) > maxInlineBackupContentBytes {
		writeBackupContentMetadata(w, id, false, "", sizeBytes, "too_large")
		return
	}

	writeBackupContentMetadata(w, id, true, string(content), sizeBytes, "")
}

func isInlineBackupTextFile(file *domain.BackupFile) bool {
	switch file.FileType {
	case "running", "verbose", "compact":
		return filepath.Ext(file.FileName) == ".rsc"
	default:
		return false
	}
}

func writeBackupContentMetadata(w http.ResponseWriter, id uuid.UUID, inline bool, content string, sizeBytes int64, reason string) {
	data := map[string]interface{}{
		"content":               content,
		"inline":                inline,
		"download_url":          "/api/v1/backup-files/" + id.String() + "/download",
		"size_bytes":            sizeBytes,
		"max_inline_size_bytes": maxInlineBackupContentBytes,
	}
	if reason != "" {
		data["reason"] = reason
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// HandleStartBulkBackupRun handles POST /api/v1/backups/bulk-runs.
func (h *BackupHandler) HandleStartBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string `json:"device_ids"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}

	deviceIDs := make([]uuid.UUID, 0, len(req.DeviceIDs))
	for _, idStr := range req.DeviceIDs {
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid device_id: %s", idStr))
			return
		}
		deviceIDs = append(deviceIDs, parsed)
	}

	createdBy := ""
	if subject := OperatorSubjectFromRequest(r); subject.Authenticated {
		createdBy = subject.Name
	}
	run, err := h.svc.StartBulkBackupRun(r.Context(), deviceIDs, createdBy)
	if err != nil {
		if errors.Is(err, service.ErrBulkBackupRunAlreadyActive) {
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"code":  "bulk_backup_run_active",
				"error": err.Error(),
				"data":  bulkBackupRunToMap(run),
			})
			return
		}
		if service.IsBulkLimitError(err) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	if err := h.auditBulkRunStarted(r, len(deviceIDs), run); err != nil {
		log.Printf("backup: failed to append bulk run audit log: %v", err)
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandleGetLatestBulkBackupRun handles GET /api/v1/backups/bulk-runs/latest.
func (h *BackupHandler) HandleGetLatestBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	run, err := h.svc.GetLatestBulkBackupRun(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandleGetBulkBackupRun handles GET /api/v1/backups/bulk-runs/{id}.
func (h *BackupHandler) HandleGetBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/backups/bulk-runs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bulk backup run ID")
		return
	}

	run, err := h.svc.GetBulkBackupRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "bulk backup run not found")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandleCancelBulkBackupRun handles POST /api/v1/backups/bulk-runs/{id}/cancel.
func (h *BackupHandler) HandleCancelBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/cancel")
	id, err := extractIDFromPath(path, "/api/v1/backups/bulk-runs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bulk backup run ID")
		return
	}

	run, err := h.svc.CancelBulkBackupRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "bulk backup run not found")
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandlePauseBulkBackupRun handles POST /api/v1/backups/bulk-runs/{id}/pause.
func (h *BackupHandler) HandlePauseBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/pause")
	id, err := extractIDFromPath(path, "/api/v1/backups/bulk-runs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bulk backup run ID")
		return
	}

	run, err := h.svc.PauseBulkBackupRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "bulk backup run not found")
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandleResumeBulkBackupRun handles POST /api/v1/backups/bulk-runs/{id}/resume.
func (h *BackupHandler) HandleResumeBulkBackupRun(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/resume")
	id, err := extractIDFromPath(path, "/api/v1/backups/bulk-runs/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid bulk backup run ID")
		return
	}

	run, err := h.svc.ResumeBulkBackupRun(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "bulk backup run not found")
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": bulkBackupRunToMap(run)})
}

// HandleBulkBackup handles POST /api/v1/backups/bulk
func (h *BackupHandler) HandleBulkBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string `json:"device_ids"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}

	deviceIDs := make([]uuid.UUID, 0, len(req.DeviceIDs))
	for _, idStr := range req.DeviceIDs {
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid device_id: %s", idStr))
			return
		}
		deviceIDs = append(deviceIDs, parsed)
	}

	results, err := h.svc.TriggerBulkBackup(r.Context(), deviceIDs...)
	if err != nil {
		if service.IsBulkLimitError(err) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	data := make([]map[string]interface{}, 0, len(results))
	for _, res := range results {
		entry := map[string]interface{}{
			"device_id":   res.DeviceID.String(),
			"device_name": res.DeviceName,
			"status":      res.Status,
		}
		if res.Reason != "" {
			entry["reason"] = res.Reason
		}
		if res.JobID != nil {
			entry["job_id"] = res.JobID.String()
		}
		data = append(data, entry)
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

// HandleBulkDownload handles POST /api/v1/backups/bulk-download
// Body: {"device_ids": ["uuid", ...]}
// Returns a zip file containing the latest successful backup files for each device.
func (h *BackupHandler) HandleBulkDownload(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DeviceIDs []string `json:"device_ids"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	var deviceIDs []uuid.UUID
	for _, idStr := range req.DeviceIDs {
		parsed, err := uuid.Parse(idStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid device_id: %s", idStr))
			return
		}
		deviceIDs = append(deviceIDs, parsed)
	}

	if len(deviceIDs) == 0 {
		writeError(w, http.StatusBadRequest, "device_ids is required")
		return
	}

	actorKey := bulkDownloadActorKey(r)
	if h.bulkDownloadLimiter != nil && !h.bulkDownloadLimiter.TryAcquire(actorKey) {
		if err := h.auditBulkDownloadRejected(r, len(deviceIDs), "actor_concurrency_limit"); err != nil {
			log.Printf("backup: failed to append bulk download rejection audit log: %v", err)
		}
		w.Header().Set("Retry-After", fmt.Sprint(bulkDownloadRetryAfterSeconds))
		writeError(w, http.StatusTooManyRequests, "bulk download already in progress for this user")
		return
	}
	if h.bulkDownloadLimiter != nil {
		defer h.bulkDownloadLimiter.Release(actorKey)
	}

	entries, err := h.svc.GetBulkDownloadFiles(r.Context(), deviceIDs)
	if err != nil {
		if service.IsBulkLimitError(err) {
			writeError(w, http.StatusRequestEntityTooLarge, err.Error())
			return
		}
		if service.IsBulkPathError(err) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if len(entries) == 0 {
		writeError(w, http.StatusNotFound, "no backup files found for the given devices")
		return
	}
	var totalBytes int64
	for _, entry := range entries {
		totalBytes += entry.SizeBytes
	}

	now := time.Now().UTC()
	if tzName, err := h.settingsRepo.Get(domain.SettingTimezone); err == nil && tzName != "" {
		if loc, err := time.LoadLocation(tzName); err == nil {
			now = time.Now().In(loc)
		}
	}
	zipName := now.Format("20060102_150405") + "_THEIA_BACKUPS.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))
	w.Header().Set("X-Bulk-Download-Device-Count", fmt.Sprint(bulkDownloadSelectedDeviceCount(entries)))
	w.Header().Set("X-Bulk-Download-File-Count", fmt.Sprint(len(entries)))
	w.Header().Set("X-Bulk-Download-Size-Bytes", fmt.Sprint(totalBytes))

	zw := zip.NewWriter(w)

	zipErrors := writeBulkZipEntries(zw, entries, h.svc.OpenBulkDownloadEntry)
	if len(zipErrors) > 0 {
		if w, err := zw.Create("_errors.txt"); err == nil {
			for _, e := range zipErrors {
				fmt.Fprintln(w, e)
			}
		} else {
			zipErrors = append(zipErrors, fmt.Sprintf("_errors.txt: zip entry creation failed: %v", err))
		}
	}
	if err := zw.Close(); err != nil {
		zipErrors = append(zipErrors, fmt.Sprintf("zip close failed: %v", err))
	}
	if err := h.auditBulkDownloadCompleted(r, len(deviceIDs), entries, totalBytes, len(zipErrors)); err != nil {
		log.Printf("backup: failed to append bulk download audit log: %v", err)
	}
}

// writeBulkZipEntries writes backup file entries into a zip writer using
// io.Copy streaming. Returns a list of error descriptions for failed files.
func writeBulkZipEntries(zw *zip.Writer, entries []service.BulkDownloadEntry, openEntry func(service.BulkDownloadEntry) (*os.File, error)) []string {
	var zipErrors []string
	for _, e := range entries {
		zipPath := e.ZipPath
		if zipPath == "" {
			zipErrors = append(zipErrors, fmt.Sprintf("%s: missing validated zip entry path", e.File.FileName))
			continue
		}
		if openEntry == nil {
			zipErrors = append(zipErrors, fmt.Sprintf("%s: missing validated file opener", zipPath))
			continue
		}
		f, err := openEntry(e)
		if err != nil {
			zipErrors = append(zipErrors, fmt.Sprintf("%s: %v", zipPath, err))
			continue
		}
		if e.SizeBytes < 0 {
			f.Close()
			zipErrors = append(zipErrors, fmt.Sprintf("%s: invalid validated file size", zipPath))
			continue
		}
		info, err := f.Stat()
		if err != nil {
			f.Close()
			zipErrors = append(zipErrors, fmt.Sprintf("%s: stat failed: %v", zipPath, err))
			continue
		}
		if !info.Mode().IsRegular() {
			f.Close()
			zipErrors = append(zipErrors, fmt.Sprintf("%s: not a regular file", zipPath))
			continue
		}
		if info.Size() > e.SizeBytes {
			f.Close()
			zipErrors = append(zipErrors, fmt.Sprintf("%s: file changed after validation", zipPath))
			continue
		}
		writer, err := zw.Create(zipPath)
		if err != nil {
			f.Close()
			zipErrors = append(zipErrors, fmt.Sprintf("%s: zip entry creation failed: %v", zipPath, err))
			continue
		}
		written, err := io.Copy(writer, io.LimitReader(f, e.SizeBytes))
		if err != nil {
			zipErrors = append(zipErrors, fmt.Sprintf("%s: write failed: %v", zipPath, err))
		} else if written != e.SizeBytes {
			zipErrors = append(zipErrors, fmt.Sprintf("%s: file changed while streaming", zipPath))
		}
		f.Close()
	}
	return zipErrors
}

func (h *BackupHandler) auditBulkDownloadCompleted(
	r *http.Request,
	requestedDeviceCount int,
	entries []service.BulkDownloadEntry,
	totalBytes int64,
	streamErrorCount int,
) error {
	if h.auditLogs == nil {
		return nil
	}

	metadata := map[string]interface{}{
		"requested_device_count": requestedDeviceCount,
		"selected_device_count":  bulkDownloadSelectedDeviceCount(entries),
		"selected_file_count":    len(entries),
		"selected_bytes":         totalBytes,
		"stream_error_count":     streamErrorCount,
		"partial":                streamErrorCount > 0,
	}
	return h.appendBackupAuditLog(
		r,
		"backup.bulk_download_completed",
		"backup_bulk_download",
		"bulk-download",
		metadata,
	)
}

func (h *BackupHandler) auditBulkDownloadRejected(r *http.Request, requestedDeviceCount int, reason string) error {
	if h.auditLogs == nil {
		return nil
	}

	metadata := map[string]interface{}{
		"requested_device_count": requestedDeviceCount,
		"reason":                 reason,
		"per_actor_limit":        maxConcurrentBulkDownloadsPerActor,
		"retry_after_seconds":    bulkDownloadRetryAfterSeconds,
	}
	return h.appendBackupAuditLog(
		r,
		"backup.bulk_download_rejected",
		"backup_bulk_download",
		"bulk-download",
		metadata,
	)
}

func (h *BackupHandler) auditBulkRunStarted(r *http.Request, requestedDeviceCount int, run *domain.BulkBackupRun) error {
	if h.auditLogs == nil || run == nil {
		return nil
	}

	metadata := map[string]interface{}{
		"requested_device_count": requestedDeviceCount,
		"total_count":            run.TotalCount,
		"queued_count":           run.QueuedCount,
		"skipped_count":          run.SkippedCount,
		"batch_size":             run.BatchSize,
	}
	return h.appendBackupAuditLog(
		r,
		"backup.bulk_run_started",
		"backup_bulk_run",
		run.ID.String(),
		metadata,
	)
}

func (h *BackupHandler) appendBackupAuditLog(
	r *http.Request,
	action string,
	resource string,
	resourceID string,
	metadata map[string]interface{},
) error {
	if h.auditLogs == nil {
		return nil
	}

	metadataJSON := "{}"
	if len(metadata) > 0 {
		data, err := json.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("marshalling backup audit metadata: %w", err)
		}
		metadataJSON = string(data)
	}

	var actorUserID *uuid.UUID
	if user, ok := AuthenticatedUserFromRequest(r); ok && user.User.User.ID != uuid.Nil {
		id := user.User.User.ID
		actorUserID = &id
	}

	logEntry := domain.AuditLog{
		ID:           uuid.New(),
		ActorUserID:  actorUserID,
		TargetUserID: actorUserID,
		Action:       action,
		Resource:     resource,
		ResourceID:   resourceID,
		MetadataJSON: metadataJSON,
		IPAddress:    clientIPAddress(r),
		UserAgent:    r.UserAgent(),
		CreatedAt:    time.Now().UTC(),
	}
	if err := h.auditLogs.AppendAuditLog(context.WithoutCancel(r.Context()), &logEntry); err != nil {
		return fmt.Errorf("appending backup audit log: %w", err)
	}
	return nil
}

// --- Helpers ---

func extractDeviceIDForBackup(path, suffix string) (uuid.UUID, error) {
	// Path: /api/v1/devices/{id}/suffix
	trimmed := strings.TrimSuffix(path, suffix)
	return extractIDFromPath(trimmed, "/api/v1/devices/")
}

type bulkOperationLimiter struct {
	mu     sync.Mutex
	limit  int
	active map[string]int
}

func newBulkOperationLimiter(limit int) *bulkOperationLimiter {
	if limit <= 0 {
		limit = 1
	}
	return &bulkOperationLimiter{
		limit:  limit,
		active: make(map[string]int),
	}
}

func (l *bulkOperationLimiter) TryAcquire(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "anonymous"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[key] >= l.limit {
		return false
	}
	l.active[key]++
	return true
}

func (l *bulkOperationLimiter) Release(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "anonymous"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active[key] <= 1 {
		delete(l.active, key)
		return
	}
	l.active[key]--
}

func bulkDownloadActorKey(r *http.Request) string {
	if user, ok := AuthenticatedUserFromRequest(r); ok {
		if id := user.User.User.ID; id != uuid.Nil {
			return "user:" + id.String()
		}
		if username := strings.TrimSpace(user.User.User.UsernameNormalized); username != "" {
			return "user:" + strings.ToLower(username)
		}
		if username := strings.TrimSpace(user.User.User.Username); username != "" {
			return "user:" + strings.ToLower(username)
		}
	}
	if ip := strings.TrimSpace(clientIPAddress(r)); ip != "" {
		return "ip:" + ip
	}
	return "anonymous"
}

func bulkDownloadSelectedDeviceCount(entries []service.BulkDownloadEntry) int {
	seen := make(map[uuid.UUID]struct{}, len(entries))
	for _, entry := range entries {
		if entry.DeviceID == uuid.Nil {
			continue
		}
		seen[entry.DeviceID] = struct{}{}
	}
	return len(seen)
}

func jobToMap(j domain.BackupJob) map[string]interface{} {
	files := make([]map[string]interface{}, 0, len(j.Files))
	for _, f := range j.Files {
		files = append(files, map[string]interface{}{
			"id":         f.ID.String(),
			"job_id":     f.JobID.String(),
			"file_type":  f.FileType,
			"file_name":  f.FileName,
			"file_hash":  f.FileHash,
			"size_bytes": f.SizeBytes,
			"created_at": f.CreatedAt,
		})
	}

	return map[string]interface{}{
		"id":            j.ID.String(),
		"device_id":     j.DeviceID.String(),
		"status":        string(j.Status),
		"error_message": j.ErrorMessage,
		"created_at":    j.CreatedAt,
		"files":         files,
	}
}

func bulkBackupRunToMap(run *domain.BulkBackupRun) map[string]interface{} {
	if run == nil {
		return nil
	}

	items := make([]map[string]interface{}, 0, len(run.Items))
	runningCount := 0
	completedCount := 0
	currentDeviceID := ""
	currentDeviceName := ""
	currentJobID := ""
	for _, item := range run.Items {
		entry := map[string]interface{}{
			"id":          item.ID.String(),
			"run_id":      item.RunID.String(),
			"device_id":   item.DeviceID.String(),
			"device_name": item.DeviceName,
			"status":      string(item.Status),
			"created_at":  item.CreatedAt,
			"updated_at":  item.UpdatedAt,
		}
		if item.Reason != "" {
			entry["reason"] = item.Reason
		}
		if item.BackupJobID != nil {
			entry["backup_job_id"] = item.BackupJobID.String()
		}
		if item.CompletedAt != nil {
			entry["completed_at"] = item.CompletedAt
		}
		items = append(items, entry)

		switch item.Status {
		case domain.BulkBackupRunItemStatusActive,
			domain.BulkBackupRunItemStatusRunning:
			runningCount++
			if currentDeviceID == "" {
				currentDeviceID = item.DeviceID.String()
				currentDeviceName = item.DeviceName
				if item.BackupJobID != nil {
					currentJobID = item.BackupJobID.String()
				}
			}
		case domain.BulkBackupRunItemStatusSuccess,
			domain.BulkBackupRunItemStatusFailed,
			domain.BulkBackupRunItemStatusSkipped,
			domain.BulkBackupRunItemStatusCancelled:
			completedCount++
		}
	}

	data := map[string]interface{}{
		"id":               run.ID.String(),
		"status":           string(run.Status),
		"batch_size":       run.BatchSize,
		"total_count":      run.TotalCount,
		"queued_count":     run.QueuedCount,
		"running_count":    runningCount,
		"completed_count":  completedCount,
		"success_count":    run.SuccessCount,
		"failed_count":     run.FailedCount,
		"skipped_count":    run.SkippedCount,
		"cancelled_count":  run.CancelledCount,
		"error_message":    run.ErrorMessage,
		"cancel_requested": run.CancelRequested,
		"created_by":       run.CreatedBy,
		"created_at":       run.CreatedAt,
		"items":            items,
	}
	if currentDeviceID != "" {
		data["current_device_id"] = currentDeviceID
		data["current_device_name"] = currentDeviceName
	}
	if currentJobID != "" {
		data["current_job_id"] = currentJobID
	}
	if run.StartedAt != nil {
		data["started_at"] = run.StartedAt
	}
	if run.CompletedAt != nil {
		data["completed_at"] = run.CompletedAt
	}
	return data
}
