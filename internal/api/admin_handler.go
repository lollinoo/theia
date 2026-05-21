package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

type adminProvider interface {
	authProvider
	AdminDashboard(rctx context.Context, actor *service.AuthenticatedUser) (*service.AdminDashboardResult, error)
	ListAdminUsers(rctx context.Context, actor *service.AuthenticatedUser, filter domain.UserListFilter) ([]domain.UserWithRolesAndPermissions, error)
	CreateAdminUser(rctx context.Context, actor *service.AuthenticatedUser, input service.AdminCreateUserInput) (*domain.UserWithRolesAndPermissions, error)
	GetAdminUser(rctx context.Context, actor *service.AuthenticatedUser, id uuid.UUID) (*domain.UserWithRolesAndPermissions, error)
	UpdateAdminUser(rctx context.Context, actor *service.AuthenticatedUser, input service.AdminUpdateUserInput) (*domain.UserWithRolesAndPermissions, error)
	SetAdminUserStatus(rctx context.Context, actor *service.AuthenticatedUser, input service.AdminUserStatusInput) (*domain.UserWithRolesAndPermissions, error)
	AssignAdminUserRole(rctx context.Context, actor *service.AuthenticatedUser, input service.AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error)
	RemoveAdminUserRole(rctx context.Context, actor *service.AuthenticatedUser, input service.AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error)
	CreateAdminPasswordResetToken(rctx context.Context, actor *service.AuthenticatedUser, userID uuid.UUID) (*service.PasswordResetTokenResult, error)
	ListAdminRoles(rctx context.Context, actor *service.AuthenticatedUser) ([]service.AdminRole, error)
	ListAdminPermissions(rctx context.Context, actor *service.AuthenticatedUser) ([]domain.Permission, error)
	ListAdminAuditLogs(rctx context.Context, actor *service.AuthenticatedUser, filter domain.AuditLogFilter) ([]domain.AuditLog, error)
}

// AdminHandler manages RBAC-protected administration endpoints.
type AdminHandler struct {
	auth adminProvider
}

// NewAdminHandler creates an admin API handler.
func NewAdminHandler(auth adminProvider) *AdminHandler {
	return &AdminHandler{auth: auth}
}

type adminDashboardStatsResponse struct {
	TotalUsers                int `json:"total_users"`
	ActiveUsers               int `json:"active_users"`
	DisabledUsers             int `json:"disabled_users"`
	LockedUsers               int `json:"locked_users"`
	RecentLogins              int `json:"recent_logins"`
	RecentFailedLoginAttempts int `json:"recent_failed_login_attempts"`
}

type adminRoleResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	IsSystemRole bool     `json:"is_system_role"`
	Permissions  []string `json:"permissions"`
}

type adminPermissionResponse struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Description string `json:"description"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
}

type adminAuditLogResponse struct {
	ID           string          `json:"id"`
	ActorUserID  *string         `json:"actor_user_id,omitempty"`
	TargetUserID *string         `json:"target_user_id,omitempty"`
	Action       string          `json:"action"`
	Resource     string          `json:"resource"`
	ResourceID   string          `json:"resource_id"`
	Metadata     json.RawMessage `json:"metadata"`
	IPAddress    string          `json:"ip_address"`
	UserAgent    string          `json:"user_agent"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		writeError(w, http.StatusServiceUnavailable, "admin service not configured")
		return
	}
	if r.URL.Path == "/api/v1/admin/dashboard" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleDashboard(w, r)
		return
	}
	if r.URL.Path == "/api/v1/admin/users" {
		switch r.Method {
		case http.MethodGet:
			h.handleListUsers(w, r)
		case http.MethodPost:
			h.handleCreateUser(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/admin/users/") {
		h.handleUserRoute(w, r)
		return
	}
	if r.URL.Path == "/api/v1/admin/roles" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleListRoles(w, r)
		return
	}
	if r.URL.Path == "/api/v1/admin/permissions" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleListPermissions(w, r)
		return
	}
	if r.URL.Path == "/api/v1/admin/audit-logs" {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleListAuditLogs(w, r)
		return
	}
	writeError(w, http.StatusNotFound, "not found")
}

func (h *AdminHandler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionAdminDashboard) {
		return
	}
	result, err := h.auth.AdminDashboard(r.Context(), actor)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stats":             adminDashboardStatsFromDomain(result.Stats),
		"recent_audit_logs": adminAuditLogResponses(result.RecentAuditLogs),
	})
}

func (h *AdminHandler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersRead) {
		return
	}
	users, err := h.auth.ListAdminUsers(r.Context(), actor, parseUserListFilter(r))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	out := make([]*safeUserResponse, 0, len(users))
	for _, user := range users {
		out = append(out, safeUserFromAggregate(user))
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"users": out})
}

func (h *AdminHandler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersCreate) {
		return
	}
	var req struct {
		Username    string   `json:"username"`
		Email       string   `json:"email"`
		DisplayName string   `json:"display_name"`
		Password    string   `json:"password"`
		Roles       []string `json:"roles"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := h.auth.CreateAdminUser(r.Context(), actor, service.AdminCreateUserInput{
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Password:    req.Password,
		Roles:       req.Roles,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handleUserRoute(w http.ResponseWriter, r *http.Request) {
	userID, action, ok := parseAdminUserRoute(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	switch {
	case action == "":
		switch r.Method {
		case http.MethodGet:
			h.handleGetUser(w, r, userID)
		case http.MethodPatch:
			h.handleUpdateUser(w, r, userID)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	case action == "status":
		if r.Method != http.MethodPatch {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleSetStatus(w, r, userID)
	case action == "roles":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handleAssignRole(w, r, userID)
	case strings.HasPrefix(action, "roles/"):
		if r.Method != http.MethodDelete {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		roleID := strings.TrimPrefix(action, "roles/")
		if strings.TrimSpace(roleID) == "" || strings.Contains(roleID, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.handleRemoveRole(w, r, userID, roleID)
	case action == "password-reset":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		h.handlePasswordReset(w, r, userID)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *AdminHandler) handleGetUser(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersRead) {
		return
	}
	user, err := h.auth.GetAdminUser(r.Context(), actor, userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handleUpdateUser(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersUpdate) {
		return
	}
	var req struct {
		Username    *string            `json:"username"`
		Email       *string            `json:"email"`
		DisplayName *string            `json:"display_name"`
		Status      *domain.UserStatus `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Status != nil && *req.Status != domain.UserStatusActive {
		if !requirePermission(w, h.auth, actor, domain.PermissionUsersDisable) {
			return
		}
	}
	user, err := h.auth.UpdateAdminUser(r.Context(), actor, service.AdminUpdateUserInput{
		UserID:      userID,
		Username:    req.Username,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Status:      req.Status,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handleSetStatus(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersUpdate) {
		return
	}
	var req struct {
		Status domain.UserStatus `json:"status"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Status != domain.UserStatusActive {
		if !requirePermission(w, h.auth, actor, domain.PermissionUsersDisable) {
			return
		}
	}
	user, err := h.auth.SetAdminUserStatus(r.Context(), actor, service.AdminUserStatusInput{
		UserID: userID,
		Status: req.Status,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handleAssignRole(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionRolesAssign) {
		return
	}
	var req struct {
		RoleID string `json:"role_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := h.auth.AssignAdminUserRole(r.Context(), actor, service.AdminUserRoleInput{
		UserID: userID,
		RoleID: req.RoleID,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handleRemoveRole(w http.ResponseWriter, r *http.Request, userID uuid.UUID, roleID string) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionRolesAssign) {
		return
	}
	user, err := h.auth.RemoveAdminUserRole(r.Context(), actor, service.AdminUserRoleInput{
		UserID: userID,
		RoleID: roleID,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"user": safeUserFromAggregate(*user)})
}

func (h *AdminHandler) handlePasswordReset(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionUsersUpdate) {
		return
	}
	reset, err := h.auth.CreateAdminPasswordResetToken(r.Context(), actor, userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      reset.Token,
		"expires_at": reset.ExpiresAt,
	})
}

func (h *AdminHandler) handleListRoles(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionRolesRead) {
		return
	}
	roles, err := h.auth.ListAdminRoles(r.Context(), actor)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	out := make([]adminRoleResponse, 0, len(roles))
	for _, role := range roles {
		out = append(out, adminRoleResponse{
			ID:           role.Role.ID,
			Name:         role.Role.Name,
			Description:  role.Role.Description,
			IsSystemRole: role.Role.IsSystemRole,
			Permissions:  append([]string(nil), role.PermissionKeys...),
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"roles": out})
}

func (h *AdminHandler) handleListPermissions(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionRolesRead) {
		return
	}
	permissions, err := h.auth.ListAdminPermissions(r.Context(), actor)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	out := make([]adminPermissionResponse, 0, len(permissions))
	for _, permission := range permissions {
		out = append(out, adminPermissionResponse{
			ID:          permission.ID,
			Key:         permission.Key,
			Description: permission.Description,
			Resource:    permission.Resource,
			Action:      permission.Action,
		})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"permissions": out})
}

func (h *AdminHandler) handleListAuditLogs(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.requireActor(w, r)
	if !ok || !requirePermission(w, h.auth, actor, domain.PermissionAuditLogsRead) {
		return
	}
	logs, err := h.auth.ListAdminAuditLogs(r.Context(), actor, parseAuditLogFilter(r))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"audit_logs": adminAuditLogResponses(logs)})
}

func (h *AdminHandler) requireActor(w http.ResponseWriter, r *http.Request) (*service.AuthenticatedUser, bool) {
	actor, ok := AuthenticatedUserFromRequest(r)
	if !ok {
		writeAuthCodeError(w, http.StatusUnauthorized, "authentication_required", "authentication required")
		return nil, false
	}
	return actor, true
}

func parseAdminUserRoute(path string) (uuid.UUID, string, bool) {
	const prefix = "/api/v1/admin/users/"
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok || rest == "" || strings.Contains(rest, "//") {
		return uuid.Nil, "", false
	}
	parts := strings.Split(rest, "/")
	if parts[0] == "" {
		return uuid.Nil, "", false
	}
	userID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	return userID, strings.Join(parts[1:], "/"), true
}

func parseUserListFilter(r *http.Request) domain.UserListFilter {
	values := r.URL.Query()
	return domain.UserListFilter{
		Status: domain.UserStatus(strings.TrimSpace(values.Get("status"))),
		Query:  strings.TrimSpace(values.Get("query")),
		RoleID: strings.TrimSpace(values.Get("role")),
		Limit:  parseIntQuery(values.Get("limit")),
		Offset: parseIntQuery(values.Get("offset")),
	}
}

func parseAuditLogFilter(r *http.Request) domain.AuditLogFilter {
	values := r.URL.Query()
	return domain.AuditLogFilter{
		ActorUserID:  parseOptionalUUID(values.Get("actor_user_id")),
		TargetUserID: parseOptionalUUID(values.Get("target_user_id")),
		Action:       strings.TrimSpace(values.Get("action")),
		Resource:     strings.TrimSpace(values.Get("resource")),
		Limit:        parseIntQuery(values.Get("limit")),
		Offset:       parseIntQuery(values.Get("offset")),
	}
}

func parseIntQuery(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return parsed
}

func parseOptionalUUID(value string) *uuid.UUID {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func adminDashboardStatsFromDomain(stats *domain.AdminDashboardStats) adminDashboardStatsResponse {
	if stats == nil {
		return adminDashboardStatsResponse{}
	}
	return adminDashboardStatsResponse{
		TotalUsers:                stats.TotalUsers,
		ActiveUsers:               stats.ActiveUsers,
		DisabledUsers:             stats.DisabledUsers,
		LockedUsers:               stats.LockedUsers,
		RecentLogins:              stats.RecentLogins,
		RecentFailedLoginAttempts: stats.RecentFailedLoginAttempts,
	}
}

func adminAuditLogResponses(logs []domain.AuditLog) []adminAuditLogResponse {
	out := make([]adminAuditLogResponse, 0, len(logs))
	for _, log := range logs {
		out = append(out, adminAuditLogFromDomain(log))
	}
	return out
}

func adminAuditLogFromDomain(log domain.AuditLog) adminAuditLogResponse {
	metadata := json.RawMessage(`{}`)
	if json.Valid([]byte(log.MetadataJSON)) {
		metadata = json.RawMessage(log.MetadataJSON)
	}
	return adminAuditLogResponse{
		ID:           log.ID.String(),
		ActorUserID:  uuidStringPtr(log.ActorUserID),
		TargetUserID: uuidStringPtr(log.TargetUserID),
		Action:       log.Action,
		Resource:     log.Resource,
		ResourceID:   log.ResourceID,
		Metadata:     metadata,
		IPAddress:    log.IPAddress,
		UserAgent:    log.UserAgent,
		CreatedAt:    log.CreatedAt,
	}
}

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	out := value.String()
	return &out
}

func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrPermissionDenied):
		writeAuthCodeError(w, http.StatusForbidden, "permission_denied", "permission denied")
	case errors.Is(err, service.ErrPasswordPolicyViolation), errors.Is(err, service.ErrAdminInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid request")
	case errors.Is(err, domain.ErrAuthUserNotFound), errors.Is(err, domain.ErrAuthRoleNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, service.ErrAdminLastActiveSuperAdmin):
		writeError(w, http.StatusConflict, "cannot remove the last active super admin")
	case errors.Is(err, service.ErrUserDisabled), errors.Is(err, service.ErrUserLocked):
		writeError(w, http.StatusForbidden, "user cannot authenticate")
	default:
		writeError(w, http.StatusInternalServerError, "internal error", err)
	}
}
