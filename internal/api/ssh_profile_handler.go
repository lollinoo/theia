package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
)

// SSHProfileHandler provides HTTP handlers for SSH profile CRUD.
type SSHProfileHandler struct {
	svc             *service.BackupService
	sshProfileRepo  *sqlite.SSHProfileRepo
}

// NewSSHProfileHandler creates a new SSHProfileHandler.
func NewSSHProfileHandler(svc *service.BackupService, sshProfileRepo *sqlite.SSHProfileRepo) *SSHProfileHandler {
	return &SSHProfileHandler{svc: svc, sshProfileRepo: sshProfileRepo}
}

type sshProfileRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Username    string `json:"username"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"auth_method"`
	Secret      string `json:"secret"`
}

type sshProfileResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Username    string `json:"username"`
	Port        int    `json:"port"`
	AuthMethod  string `json:"auth_method"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func sshProfileToResponse(p *domain.SSHProfile) sshProfileResponse {
	return sshProfileResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		Username:    p.Username,
		Port:        p.Port,
		AuthMethod:  string(p.AuthMethod),
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// HandleList handles GET /api/v1/ssh-profiles
func (h *SSHProfileHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.svc.GetAllSSHProfiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]sshProfileResponse, 0, len(profiles))
	for i := range profiles {
		resp = append(resp, sshProfileToResponse(&profiles[i]))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// HandleCreate handles POST /api/v1/ssh-profiles
func (h *SSHProfileHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req sshProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.AuthMethod == "" {
		req.AuthMethod = "password"
	}

	authMethod := domain.SSHAuthMethod(req.AuthMethod)
	if authMethod != domain.SSHAuthPassword && authMethod != domain.SSHAuthKey {
		writeError(w, http.StatusBadRequest, "auth_method must be 'password' or 'key'")
		return
	}

	profile, err := h.svc.CreateSSHProfile(r.Context(), strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Username, req.Port, authMethod, req.Secret)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": sshProfileToResponse(profile)})
}

// HandleGet handles GET /api/v1/ssh-profiles/{id}
func (h *SSHProfileHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/ssh-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	profile, err := h.svc.GetSSHProfile(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": sshProfileToResponse(profile)})
}

// HandleUpdate handles PUT /api/v1/ssh-profiles/{id}
func (h *SSHProfileHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/ssh-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	var req sshProfileRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Username == "" {
		req.Username = "admin"
	}
	if req.Port == 0 {
		req.Port = 22
	}
	if req.AuthMethod == "" {
		req.AuthMethod = "password"
	}

	authMethod := domain.SSHAuthMethod(req.AuthMethod)
	if authMethod != domain.SSHAuthPassword && authMethod != domain.SSHAuthKey {
		writeError(w, http.StatusBadRequest, "auth_method must be 'password' or 'key'")
		return
	}

	profile, err := h.svc.UpdateSSHProfile(r.Context(), id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), req.Username, req.Port, authMethod, req.Secret)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": sshProfileToResponse(profile)})
}

// HandleDelete handles DELETE /api/v1/ssh-profiles/{id}
func (h *SSHProfileHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/ssh-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	// Check if any device references this profile
	inUse, err := h.sshProfileRepo.IsInUse(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if inUse {
		writeError(w, http.StatusConflict, "cannot delete SSH profile: it is still assigned to one or more devices")
		return
	}

	if err := h.svc.DeleteSSHProfile(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleTest handles POST /api/v1/ssh-profiles/{id}/test
func (h *SSHProfileHandler) HandleTest(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/test")
	id, err := extractIDFromPath(path, "/api/v1/ssh-profiles/")
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

	if err := h.svc.TestSSHProfile(r.Context(), id, body.TargetIP); err != nil {
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
