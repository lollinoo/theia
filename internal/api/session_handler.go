package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/security"
)

// SessionHandler manages browser operator sessions.
type SessionHandler struct {
	operatorToken string
	sessions      *security.SessionManager
}

// NewSessionHandler creates a new SessionHandler.
func NewSessionHandler(config SecurityConfig) *SessionHandler {
	return &SessionHandler{
		operatorToken: strings.TrimSpace(config.OperatorToken),
		sessions:      config.Sessions,
	}
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	Subject       string `json:"subject,omitempty"`
}

// ServeHTTP handles /api/v1/session.
func (h *SessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *SessionHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	if h.operatorToken == "" {
		json.NewEncoder(w).Encode(sessionResponse{Authenticated: true, Subject: "anonymous"})
		return
	}
	subject, ok := security.AuthenticateRequest(r, h.operatorToken, h.sessions)
	if !ok {
		json.NewEncoder(w).Encode(sessionResponse{Authenticated: false})
		return
	}
	json.NewEncoder(w).Encode(sessionResponse{Authenticated: true, Subject: subject.Name})
}

func (h *SessionHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if h.operatorToken == "" {
		json.NewEncoder(w).Encode(sessionResponse{Authenticated: true, Subject: "anonymous"})
		return
	}

	var req struct {
		Token    string `json:"token"`
		Operator string `json:"operator"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	subject, ok := security.AuthenticateLoginToken(req.Token, h.operatorToken, req.Operator)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid operator token")
		return
	}
	cookie, _, ok := h.sessions.CreateCookie(subject.Name, security.SecureCookieForRequest(r))
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "operator sessions are not configured")
		return
	}
	http.SetCookie(w, cookie)
	json.NewEncoder(w).Encode(sessionResponse{Authenticated: true, Subject: subject.Name})
}

func (h *SessionHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, security.ClearCookie(security.SecureCookieForRequest(r)))
	w.WriteHeader(http.StatusNoContent)
}
