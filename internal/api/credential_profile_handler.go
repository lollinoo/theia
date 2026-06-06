package api

// This file defines credential profile handler HTTP handler behavior and request/response boundaries.

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/service"
)

// CredentialProfileHandler provides HTTP handlers for credential profile CRUD.
type CredentialProfileHandler struct {
	svc                   *service.BackupService
	credentialProfileRepo *postgres.CredentialProfileRepo
}

// NewCredentialProfileHandler creates a new CredentialProfileHandler.
func NewCredentialProfileHandler(svc *service.BackupService, credentialProfileRepo *postgres.CredentialProfileRepo) *CredentialProfileHandler {
	return &CredentialProfileHandler{svc: svc, credentialProfileRepo: credentialProfileRepo}
}

type credentialProfileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Username    string `json:"username"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"auth_method"`
	Secret      string `json:"secret"`
	Role        string `json:"role"`
}

// credentialProfileResponse is the API response for a credential profile.
// EncryptedSecret is intentionally excluded (T-23-04 mitigation).
type credentialProfileResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Username    string `json:"username"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"auth_method"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func credentialProfileToResponse(p *domain.CredentialProfile) credentialProfileResponse {
	return credentialProfileResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Username:    p.Username,
		Port:        p.Port,
		AuthMethod:  string(p.AuthMethod),
		Role:        p.Role,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// HandleList handles GET /api/v1/credential-profiles
func (h *CredentialProfileHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.svc.GetAllCredentialProfiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	resp := make([]credentialProfileResponse, 0, len(profiles))
	for i := range profiles {
		resp = append(resp, credentialProfileToResponse(&profiles[i]))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// HandleCreate handles POST /api/v1/credential-profiles
func (h *CredentialProfileHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req credentialProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 255 {
		writeError(w, http.StatusBadRequest, "name too long (max 255 characters)")
		return
	}
	if len(req.Username) > 255 {
		writeError(w, http.StatusBadRequest, "username too long (max 255 characters)")
		return
	}
	if len(req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "description too long (max 255 characters)")
		return
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.Port < 1 || req.Port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.AuthMethod == "" {
		req.AuthMethod = "password"
	}

	authMethod := domain.SSHAuthMethod(req.AuthMethod)
	if authMethod != domain.SSHAuthPassword && authMethod != domain.SSHAuthKey {
		writeError(w, http.StatusBadRequest, "auth_method must be 'password' or 'key'")
		return
	}

	if req.Role == "" {
		req.Role = "Admin"
	}

	profile, err := h.svc.CreateCredentialProfile(r.Context(), strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Username, req.Port, authMethod, req.Secret, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": credentialProfileToResponse(profile)})
}

// HandleGet handles GET /api/v1/credential-profiles/{id}
func (h *CredentialProfileHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/credential-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	profile, err := h.svc.GetCredentialProfile(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": credentialProfileToResponse(profile)})
}

// HandleUpdate handles PUT /api/v1/credential-profiles/{id}
func (h *CredentialProfileHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/credential-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	var req credentialProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 255 {
		writeError(w, http.StatusBadRequest, "name too long (max 255 characters)")
		return
	}
	if len(req.Username) > 255 {
		writeError(w, http.StatusBadRequest, "username too long (max 255 characters)")
		return
	}
	if len(req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "description too long (max 255 characters)")
		return
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.Port < 1 || req.Port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return
	}
	if req.AuthMethod == "" {
		req.AuthMethod = "password"
	}

	authMethod := domain.SSHAuthMethod(req.AuthMethod)
	if authMethod != domain.SSHAuthPassword && authMethod != domain.SSHAuthKey {
		writeError(w, http.StatusBadRequest, "auth_method must be 'password' or 'key'")
		return
	}

	if req.Role == "" {
		req.Role = "Admin"
	}

	profile, err := h.svc.UpdateCredentialProfile(r.Context(), id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Username, req.Port, authMethod, req.Secret, req.Role)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": credentialProfileToResponse(profile)})
}

// HandleDelete handles DELETE /api/v1/credential-profiles/{id}
func (h *CredentialProfileHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/credential-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	// Check if any device references this profile
	inUse, err := h.credentialProfileRepo.IsInUse(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}
	if inUse {
		writeError(w, http.StatusConflict, "cannot delete credential profile: it is still assigned to one or more devices")
		return
	}

	if err := h.svc.DeleteCredentialProfile(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTest handles POST /api/v1/credential-profiles/{id}/test
func (h *CredentialProfileHandler) HandleTest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/test")
	id, err := extractIDFromPath(path, "/api/v1/credential-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	var body struct {
		TargetIP string `json:"target_ip"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.TargetIP == "" {
		writeError(w, http.StatusBadRequest, "target_ip is required")
		return
	}

	if err := h.svc.TestCredentialProfile(r.Context(), id, body.TargetIP); err != nil {
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
