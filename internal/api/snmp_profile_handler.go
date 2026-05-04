package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
)

// SNMPProfileHandler provides HTTP handlers for SNMP credential profile CRUD.
type SNMPProfileHandler struct {
	repo domain.SNMPProfileRepository
}

// NewSNMPProfileHandler creates a new SNMPProfileHandler.
func NewSNMPProfileHandler(repo domain.SNMPProfileRepository) *SNMPProfileHandler {
	return &SNMPProfileHandler{repo: repo}
}

// --- request/response types ---

type snmpProfileRequest struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	SNMP        snmpCredsRequest `json:"snmp"`
}

type snmpProfileResponse struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	SNMP        snmpCredsResponse `json:"snmp"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

type snmpCredsResponse struct {
	Version         string `json:"version"`
	Community       string `json:"community,omitempty"`
	CommunitySet    bool   `json:"community_set,omitempty"`
	Username        string `json:"username,omitempty"`
	AuthProtocol    string `json:"auth_protocol,omitempty"`
	AuthPassword    string `json:"auth_password,omitempty"`
	AuthPasswordSet bool   `json:"auth_password_set,omitempty"`
	PrivProtocol    string `json:"priv_protocol,omitempty"`
	PrivPassword    string `json:"priv_password,omitempty"`
	PrivPasswordSet bool   `json:"priv_password_set,omitempty"`
	SecurityLevel   string `json:"security_level,omitempty"`
}

// HandleList handles GET /api/v1/snmp-profiles
func (h *SNMPProfileHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.repo.GetAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	resp := make([]snmpProfileResponse, 0, len(profiles))
	for i := range profiles {
		resp = append(resp, profileToResponse(&profiles[i]))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"data": resp})
}

// HandleCreate handles POST /api/v1/snmp-profiles
func (h *SNMPProfileHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req snmpProfileRequest
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
	if len(req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "description too long (max 255 characters)")
		return
	}

	creds, err := parseSNMPCreds(req.SNMP)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	profile := &domain.SNMPProfile{
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		Credentials: creds,
	}
	if err := h.repo.Create(profile); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": profileToResponse(profile)})
}

// HandleGet handles GET /api/v1/snmp-profiles/{id}
func (h *SNMPProfileHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/snmp-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	profile, err := h.repo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": profileToResponse(profile)})
}

// HandleUpdate handles PUT /api/v1/snmp-profiles/{id}
func (h *SNMPProfileHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/snmp-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	var req snmpProfileRequest
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
	if len(req.Description) > 255 {
		writeError(w, http.StatusBadRequest, "description too long (max 255 characters)")
		return
	}

	profile, err := h.repo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	creds, err := parseSNMPCreds(req.SNMP)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	profile.Name = strings.TrimSpace(req.Name)
	profile.Description = strings.TrimSpace(req.Description)
	profile.Credentials = creds

	if err := h.repo.Update(profile); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, http.StatusConflict, "a profile with that name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"data": profileToResponse(profile)})
}

// HandleReveal handles POST /api/v1/snmp-profiles/{id}/reveal.
func (h *SNMPProfileHandler) HandleReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	trimmedPath := strings.TrimSuffix(r.URL.Path, "/reveal")
	id, err := extractIDFromPath(trimmedPath, "/api/v1/snmp-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "reason is required")
		return
	}
	if len(reason) > 255 {
		writeError(w, http.StatusBadRequest, "reason too long (max 255 characters)")
		return
	}

	profile, err := h.repo.GetByID(id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Printf("snmp profile reveal profile_id=%s reason=%q remote_addr=%q user_agent=%q outcome=not_found", id, reason, r.RemoteAddr, r.UserAgent())
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		log.Printf("snmp profile reveal profile_id=%s reason=%q remote_addr=%q user_agent=%q outcome=error", id, reason, r.RemoteAddr, r.UserAgent())
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	log.Printf("snmp profile reveal profile_id=%s reason=%q remote_addr=%q user_agent=%q outcome=success", id, reason, r.RemoteAddr, r.UserAgent())
	json.NewEncoder(w).Encode(map[string]interface{}{"data": profileToRevealResponse(profile)})
}

// HandleDelete handles DELETE /api/v1/snmp-profiles/{id}
func (h *SNMPProfileHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := extractIDFromPath(r.URL.Path, "/api/v1/snmp-profiles/")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile ID")
		return
	}

	if err := h.repo.Delete(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func profileToResponse(p *domain.SNMPProfile) snmpProfileResponse {
	resp := snmpProfileResponse{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		CreatedAt:   p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	resp.SNMP.Version = string(p.Credentials.Version)
	if p.Credentials.V2c != nil {
		resp.SNMP.CommunitySet = p.Credentials.V2c.Community != ""
	}
	if p.Credentials.V3 != nil {
		v3 := p.Credentials.V3
		resp.SNMP.Username = v3.Username
		resp.SNMP.AuthProtocol = v3.AuthProtocol
		resp.SNMP.AuthPasswordSet = v3.AuthPassword != ""
		resp.SNMP.PrivProtocol = v3.PrivProtocol
		resp.SNMP.PrivPasswordSet = v3.PrivPassword != ""
		resp.SNMP.SecurityLevel = v3.SecurityLevel
	}
	return resp
}

func profileToRevealResponse(p *domain.SNMPProfile) snmpProfileResponse {
	resp := profileToResponse(p)
	if p.Credentials.V2c != nil {
		resp.SNMP.Community = p.Credentials.V2c.Community
	}
	if p.Credentials.V3 != nil {
		resp.SNMP.AuthPassword = p.Credentials.V3.AuthPassword
		resp.SNMP.PrivPassword = p.Credentials.V3.PrivPassword
	}
	return resp
}
