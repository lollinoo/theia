package api

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

// maxInlineBackupContentBytes caps JSON previews so backup content is not
// loaded into memory for large files. Full content remains available through
// the download endpoint.
const maxInlineBackupContentBytes int64 = 1 << 20

// BackupHandler provides HTTP handlers for SSH credentials and config backups.
type BackupHandler struct {
	svc          *service.BackupService
	settingsRepo domain.SettingsRepository
}

// NewBackupHandler creates a new BackupHandler.
func NewBackupHandler(svc *service.BackupService, settingsRepo domain.SettingsRepository) *BackupHandler {
	return &BackupHandler{svc: svc, settingsRepo: settingsRepo}
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

	now := time.Now().UTC()
	if tzName, err := h.settingsRepo.Get(domain.SettingTimezone); err == nil && tzName != "" {
		if loc, err := time.LoadLocation(tzName); err == nil {
			now = time.Now().In(loc)
		}
	}
	zipName := now.Format("20060102_150405") + "_THEIA_BACKUPS.zip"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, zipName))

	zw := zip.NewWriter(w)
	defer zw.Close()

	zipErrors := writeBulkZipEntries(zw, entries, h.svc.OpenBulkDownloadEntry)
	if len(zipErrors) > 0 {
		if w, err := zw.Create("_errors.txt"); err == nil {
			for _, e := range zipErrors {
				fmt.Fprintln(w, e)
			}
		}
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

// --- Helpers ---

func extractDeviceIDForBackup(path, suffix string) (uuid.UUID, error) {
	// Path: /api/v1/devices/{id}/suffix
	trimmed := strings.TrimSuffix(path, suffix)
	return extractIDFromPath(trimmed, "/api/v1/devices/")
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
