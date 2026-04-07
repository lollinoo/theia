package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
)

// DeviceCredentialProfileHandler provides HTTP handlers for per-device
// credential profile assignment and WinBox credential endpoints.
type DeviceCredentialProfileHandler struct {
	svc                   *service.BackupService
	credentialProfileRepo *sqlite.CredentialProfileRepo
}

// NewDeviceCredentialProfileHandler creates a new DeviceCredentialProfileHandler.
func NewDeviceCredentialProfileHandler(svc *service.BackupService, credentialProfileRepo *sqlite.CredentialProfileRepo) *DeviceCredentialProfileHandler {
	return &DeviceCredentialProfileHandler{svc: svc, credentialProfileRepo: credentialProfileRepo}
}

// --- Response types ---

// assignedProfileResponse is the response shape for a device-assigned credential profile.
// EncryptedSecret is intentionally excluded (T-24-06 mitigation).
type assignedProfileResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Username   string `json:"username"`
	Port       int    `json:"port"`
	AuthMethod string `json:"auth_method"`
	Role       string `json:"role"`
	IsWinbox   bool   `json:"is_winbox"`
	AssignedAt string `json:"assigned_at"`
}

// --- HandleListAssignments ---

// HandleListAssignments handles GET /api/v1/devices/{id}/credential-profiles
func (h *DeviceCredentialProfileHandler) HandleListAssignments(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/devices/{id}/credential-profiles
	trimmed := strings.TrimSuffix(r.URL.Path, "/credential-profiles")
	deviceID, err := extractIDFromPath(trimmed, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	rows, err := h.credentialProfileRepo.ListAssignedProfiles(deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	resp := make([]assignedProfileResponse, 0, len(rows))
	for _, row := range rows {
		resp = append(resp, assignedProfileResponse{
			ID:         row.ProfileID.String(),
			Name:       row.Name,
			Username:   row.Username,
			Port:       row.Port,
			AuthMethod: string(row.AuthMethod),
			Role:       row.Role,
			IsWinbox:   row.IsWinbox,
			AssignedAt: row.CreatedAt.Format(time.RFC3339),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// --- HandleAssign ---

// HandleAssign handles POST /api/v1/devices/{id}/credential-profiles
func (h *DeviceCredentialProfileHandler) HandleAssign(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimSuffix(r.URL.Path, "/credential-profiles")
	deviceID, err := extractIDFromPath(trimmed, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ProfileID == "" {
		writeError(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	profileID, err := uuid.Parse(body.ProfileID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile_id: must be a valid UUID")
		return
	}

	if err := h.credentialProfileRepo.AssignProfile(deviceID, profileID); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "UNIQUE") {
			writeError(w, http.StatusConflict, "profile already assigned to this device")
			return
		}
		if strings.Contains(errStr, "FOREIGN KEY") {
			writeError(w, http.StatusNotFound, "device or profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]string{
			"device_id":  deviceID.String(),
			"profile_id": profileID.String(),
		},
	})
}

// --- HandleUnassign ---

// HandleUnassign handles DELETE /api/v1/devices/{id}/credential-profiles/{profileId}
func (h *DeviceCredentialProfileHandler) HandleUnassign(w http.ResponseWriter, r *http.Request) {
	// Path: /api/v1/devices/{deviceId}/credential-profiles/{profileId}
	// T-24-08 mitigation: both IDs parsed via uuid.Parse from URL path segments
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/devices/")
	parts := strings.Split(suffix, "/")
	// parts[0]=deviceID, parts[1]="credential-profiles", parts[2]=profileID
	if len(parts) < 3 || parts[1] != "credential-profiles" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return
	}
	deviceID, err := uuid.Parse(parts[0])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}
	profileID, err := uuid.Parse(parts[2])
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	if err := h.credentialProfileRepo.UnassignProfile(deviceID, profileID); err != nil {
		if strings.Contains(err.Error(), "not assigned") {
			writeError(w, http.StatusNotFound, "profile not assigned to this device")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- HandleSetWinbox ---

// HandleSetWinbox handles PUT /api/v1/devices/{id}/winbox-profile
func (h *DeviceCredentialProfileHandler) HandleSetWinbox(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimSuffix(r.URL.Path, "/winbox-profile")
	deviceID, err := extractIDFromPath(trimmed, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	var body struct {
		ProfileID string `json:"profile_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.ProfileID == "" {
		writeError(w, http.StatusBadRequest, "profile_id is required")
		return
	}
	profileID, err := uuid.Parse(body.ProfileID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile_id: must be a valid UUID")
		return
	}

	if err := h.credentialProfileRepo.SetWinboxProfile(deviceID, profileID); err != nil {
		if strings.Contains(err.Error(), "not assigned") {
			writeError(w, http.StatusNotFound, "profile is not assigned to this device")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": map[string]interface{}{
			"device_id":  deviceID.String(),
			"profile_id": profileID.String(),
			"is_winbox":  true,
		},
	})
}

// --- HandleClearWinbox ---

// HandleClearWinbox handles DELETE /api/v1/devices/{id}/winbox-profile
func (h *DeviceCredentialProfileHandler) HandleClearWinbox(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimSuffix(r.URL.Path, "/winbox-profile")
	deviceID, err := extractIDFromPath(trimmed, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	// Idempotent — always succeeds (D-09)
	if err := h.credentialProfileRepo.ClearWinboxProfile(deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- HandleGetWinboxCredentials ---

// HandleGetWinboxCredentials handles GET /api/v1/devices/{id}/winbox-credentials
// Returns decrypted IP + username + password for the designated WinBox profile.
// NOTE: Flat JSON response (no {"data":...} envelope) per D-10 spec.
// T-24-05 mitigation: decryption happens in BackupService, never in the handler.
func (h *DeviceCredentialProfileHandler) HandleGetWinboxCredentials(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimSuffix(r.URL.Path, "/winbox-credentials")
	deviceID, err := extractIDFromPath(trimmed, "/api/v1/devices/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid device ID")
		return
	}

	assignment, err := h.credentialProfileRepo.GetWinboxAssignment(deviceID)
	if err != nil {
		// "no WinBox profile designated" is the canonical message from the repo
		writeError(w, http.StatusNotFound, "no WinBox profile designated")
		return
	}

	ip, password, err := h.svc.GetWinboxCredentials(deviceID, assignment.EncryptedSecret, assignment.Username)
	if err != nil {
		if strings.Contains(err.Error(), "no password") {
			writeError(w, http.StatusUnprocessableEntity, "WinBox profile has no password configured")
			return
		}
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	// Flat response per D-10 spec (no {"data":...} envelope)
	json.NewEncoder(w).Encode(map[string]string{
		"ip":       ip,
		"username": assignment.Username,
		"password": password,
	})
}
