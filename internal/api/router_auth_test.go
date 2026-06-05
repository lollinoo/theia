package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/service"
)

const (
	testSessionToken = "test-session-token"
	testCSRFToken    = "test-csrf-token"
)

func TestNewRouterRequiresUserSessionForProtectedSurface(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "settings", method: http.MethodGet, path: "/api/v1/settings"},
		{name: "user settings", method: http.MethodGet, path: "/api/v1/settings/me"},
		{name: "bridge token", method: http.MethodPost, path: "/api/v1/bridge/token/00000000-0000-0000-0000-000000000001"},
		{name: "bridge launch request", method: http.MethodPost, path: "/api/v1/bridge/launch-requests/00000000-0000-0000-0000-000000000001"},
		{name: "health", method: http.MethodGet, path: "/api/v1/health"},
		{name: "websocket", method: http.MethodGet, path: "/api/v1/ws"},
		{name: "admin users", method: http.MethodGet, path: "/api/v1/admin/users"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestRequiredPermissionsForRegisteredProtectedRoutes(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	roleID := "viewer"
	routeTests := []struct {
		name   string
		method string
		path   string
		want   []string
	}{
		{name: "websocket", method: http.MethodGet, path: "/api/v1/ws", want: []string{domain.PermissionTopologyRead}},
		{name: "health", method: http.MethodGet, path: "/api/v1/health", want: []string{domain.PermissionSettingsRead}},
		{name: "prometheus health", method: http.MethodGet, path: "/api/v1/prometheus/health", want: []string{domain.PermissionSettingsRead}},

		{name: "settings list", method: http.MethodGet, path: "/api/v1/settings", want: []string{domain.PermissionSettingsRead}},
		{name: "settings key read", method: http.MethodGet, path: "/api/v1/settings/" + domain.SettingBridgePort, want: []string{domain.PermissionSettingsRead}},
		{name: "settings key update", method: http.MethodPut, path: "/api/v1/settings/" + domain.SettingBridgePort, want: []string{domain.PermissionSettingsUpdate}},
		{name: "account settings read", method: http.MethodGet, path: "/api/v1/settings/me", want: []string{domain.PermissionAccountManage}},
		{name: "account settings update", method: http.MethodPatch, path: "/api/v1/settings/me", want: []string{domain.PermissionAccountManage}},
		{name: "bridge settings read", method: http.MethodGet, path: "/api/v1/settings/bridge", want: []string{domain.PermissionAccountManage}},
		{name: "bridge secret create", method: http.MethodPost, path: "/api/v1/settings/bridge/secret", want: []string{domain.PermissionAccountManage}},
		{name: "bridge secret rotate", method: http.MethodPost, path: "/api/v1/settings/bridge/secret/rotate", want: []string{domain.PermissionAccountManage}},
		{name: "bridge secret revoke", method: http.MethodPost, path: "/api/v1/settings/bridge/secret/revoke", want: []string{domain.PermissionAccountManage}},
		{name: "bridge connector config", method: http.MethodGet, path: "/api/v1/settings/bridge/connector/config", want: []string{domain.PermissionAccountManage}},
		{name: "bridge connector download", method: http.MethodGet, path: "/api/v1/settings/bridge/connector/download/linux/amd64", want: []string{domain.PermissionAccountManage}},

		{name: "topology canvas", method: http.MethodGet, path: "/api/v1/topology/canvas", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas legacy", method: http.MethodGet, path: "/api/v1/canvas", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas maps list", method: http.MethodGet, path: "/api/v1/canvas/maps", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas maps create", method: http.MethodPost, path: "/api/v1/canvas/maps", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map get", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id, want: []string{domain.PermissionTopologyRead}},
		{name: "canvas map patch", method: http.MethodPatch, path: "/api/v1/canvas/maps/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map delete", method: http.MethodDelete, path: "/api/v1/canvas/maps/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map duplicate", method: http.MethodPost, path: "/api/v1/canvas/maps/" + id + "/duplicate", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map primary", method: http.MethodPost, path: "/api/v1/canvas/maps/" + id + "/primary", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map topology", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id + "/topology", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas map bootstrap", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id + "/bootstrap", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas map positions read", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id + "/positions", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas map positions update", method: http.MethodPut, path: "/api/v1/canvas/maps/" + id + "/positions", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map device areas update", method: http.MethodPut, path: "/api/v1/canvas/maps/" + id + "/device-areas", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map areas list", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id + "/areas", want: []string{domain.PermissionTopologyRead}},
		{name: "canvas map areas create", method: http.MethodPost, path: "/api/v1/canvas/maps/" + id + "/areas", want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map area update", method: http.MethodPut, path: "/api/v1/canvas/maps/" + id + "/areas/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map area delete", method: http.MethodDelete, path: "/api/v1/canvas/maps/" + id + "/areas/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map device add", method: http.MethodPost, path: "/api/v1/canvas/maps/" + id + "/devices/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map device patch", method: http.MethodPatch, path: "/api/v1/canvas/maps/" + id + "/devices/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "canvas map device remove", method: http.MethodDelete, path: "/api/v1/canvas/maps/" + id + "/devices/" + id, want: []string{domain.PermissionTopologyUpdate}},

		{name: "devices list", method: http.MethodGet, path: "/api/v1/devices", want: []string{domain.PermissionDevicesRead}},
		{name: "devices create", method: http.MethodPost, path: "/api/v1/devices", want: []string{domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate}},
		{name: "devices batch", method: http.MethodPost, path: "/api/v1/devices/batch", want: []string{domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate}},
		{name: "devices orphans", method: http.MethodGet, path: "/api/v1/devices/orphans", want: []string{domain.PermissionDevicesRead}},
		{name: "device read", method: http.MethodGet, path: "/api/v1/devices/" + id, want: []string{domain.PermissionDevicesRead}},
		{name: "device update", method: http.MethodPut, path: "/api/v1/devices/" + id, want: []string{domain.PermissionDevicesUpdate}},
		{name: "device delete", method: http.MethodDelete, path: "/api/v1/devices/" + id, want: []string{domain.PermissionDevicesDelete}},
		{name: "device interfaces", method: http.MethodGet, path: "/api/v1/devices/" + id + "/interfaces", want: []string{domain.PermissionTopologyRead}},
		{name: "device probe", method: http.MethodPost, path: "/api/v1/devices/" + id + "/probe", want: []string{domain.PermissionDevicesUpdate}},
		{name: "device snmp test", method: http.MethodPost, path: "/api/v1/devices/" + id + "/snmp-test", want: []string{domain.PermissionDevicesUpdate}},
		{name: "device topology discovery", method: http.MethodPost, path: "/api/v1/devices/" + id + "/topology-discovery", want: []string{domain.PermissionTopologyUpdate}},
		{name: "device ssh test", method: http.MethodPost, path: "/api/v1/devices/" + id + "/ssh-credentials/test", want: []string{domain.PermissionDevicesCreate, domain.PermissionDevicesUpdate}},
		{name: "device backup list", method: http.MethodGet, path: "/api/v1/devices/" + id + "/backups", want: []string{domain.PermissionBackupsRead}},
		{name: "device backup trigger", method: http.MethodPost, path: "/api/v1/devices/" + id + "/backups", want: []string{domain.PermissionBackupsUpdate}},
		{name: "device backup latest", method: http.MethodGet, path: "/api/v1/devices/" + id + "/backups/latest", want: []string{domain.PermissionBackupsRead}},
		{name: "device credential assignments", method: http.MethodGet, path: "/api/v1/devices/" + id + "/credential-profiles", want: []string{domain.PermissionCredentialsRead}},
		{name: "device credential assign", method: http.MethodPost, path: "/api/v1/devices/" + id + "/credential-profiles", want: []string{domain.PermissionCredentialsUpdate}},
		{name: "device credential unassign", method: http.MethodDelete, path: "/api/v1/devices/" + id + "/credential-profiles/" + id, want: []string{domain.PermissionCredentialsUpdate}},
		{name: "device winbox set", method: http.MethodPut, path: "/api/v1/devices/" + id + "/winbox-profile", want: []string{domain.PermissionCredentialsUpdate}},
		{name: "device winbox clear", method: http.MethodDelete, path: "/api/v1/devices/" + id + "/winbox-profile", want: []string{domain.PermissionCredentialsUpdate}},
		{name: "device winbox credentials legacy", method: http.MethodGet, path: "/api/v1/devices/" + id + "/winbox-credentials", want: []string{domain.PermissionCredentialsRead}},
		{name: "device winbox credentials reveal", method: http.MethodPost, path: "/api/v1/devices/" + id + "/winbox-credentials/reveal", want: []string{domain.PermissionCredentialsReveal}},

		{name: "links list", method: http.MethodGet, path: "/api/v1/links", want: []string{domain.PermissionTopologyRead}},
		{name: "links create", method: http.MethodPost, path: "/api/v1/links", want: []string{domain.PermissionTopologyUpdate}},
		{name: "link update", method: http.MethodPut, path: "/api/v1/links/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "link delete", method: http.MethodDelete, path: "/api/v1/links/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "positions list", method: http.MethodGet, path: "/api/v1/positions", want: []string{domain.PermissionTopologyRead}},
		{name: "positions update", method: http.MethodPut, path: "/api/v1/positions", want: []string{domain.PermissionTopologyUpdate}},

		{name: "grafana profiles list", method: http.MethodGet, path: "/api/v1/grafana/dashboard-profiles", want: []string{domain.PermissionSettingsRead}},
		{name: "grafana profiles create", method: http.MethodPost, path: "/api/v1/grafana/dashboard-profiles", want: []string{domain.PermissionSettingsUpdate}},
		{name: "grafana profile update", method: http.MethodPut, path: "/api/v1/grafana/dashboard-profiles/" + id, want: []string{domain.PermissionSettingsUpdate}},
		{name: "grafana profile delete", method: http.MethodDelete, path: "/api/v1/grafana/dashboard-profiles/" + id, want: []string{domain.PermissionSettingsUpdate}},
		{name: "grafana override update", method: http.MethodPut, path: "/api/v1/grafana/device-overrides/" + id, want: []string{domain.PermissionSettingsUpdate}},

		{name: "snmp profiles list", method: http.MethodGet, path: "/api/v1/snmp-profiles", want: []string{domain.PermissionCredentialsRead}},
		{name: "snmp profiles create", method: http.MethodPost, path: "/api/v1/snmp-profiles", want: []string{domain.PermissionCredentialsUpdate}},
		{name: "snmp profile read", method: http.MethodGet, path: "/api/v1/snmp-profiles/" + id, want: []string{domain.PermissionCredentialsRead}},
		{name: "snmp profile update", method: http.MethodPut, path: "/api/v1/snmp-profiles/" + id, want: []string{domain.PermissionCredentialsUpdate}},
		{name: "snmp profile delete", method: http.MethodDelete, path: "/api/v1/snmp-profiles/" + id, want: []string{domain.PermissionCredentialsUpdate}},
		{name: "snmp profile reveal", method: http.MethodPost, path: "/api/v1/snmp-profiles/" + id + "/reveal", want: []string{domain.PermissionCredentialsReveal}},
		{name: "credential profiles list", method: http.MethodGet, path: "/api/v1/credential-profiles", want: []string{domain.PermissionCredentialsRead}},
		{name: "credential profiles create", method: http.MethodPost, path: "/api/v1/credential-profiles", want: []string{domain.PermissionCredentialsUpdate}},
		{name: "credential profile read", method: http.MethodGet, path: "/api/v1/credential-profiles/" + id, want: []string{domain.PermissionCredentialsRead}},
		{name: "credential profile update", method: http.MethodPut, path: "/api/v1/credential-profiles/" + id, want: []string{domain.PermissionCredentialsUpdate}},
		{name: "credential profile delete", method: http.MethodDelete, path: "/api/v1/credential-profiles/" + id, want: []string{domain.PermissionCredentialsUpdate}},
		{name: "credential profile test", method: http.MethodPost, path: "/api/v1/credential-profiles/" + id + "/test", want: []string{domain.PermissionCredentialsUpdate}},

		{name: "areas list", method: http.MethodGet, path: "/api/v1/areas", want: []string{domain.PermissionTopologyRead}},
		{name: "areas create", method: http.MethodPost, path: "/api/v1/areas", want: []string{domain.PermissionTopologyUpdate}},
		{name: "area read", method: http.MethodGet, path: "/api/v1/areas/" + id, want: []string{domain.PermissionTopologyRead}},
		{name: "area update", method: http.MethodPut, path: "/api/v1/areas/" + id, want: []string{domain.PermissionTopologyUpdate}},
		{name: "area delete", method: http.MethodDelete, path: "/api/v1/areas/" + id, want: []string{domain.PermissionTopologyUpdate}},

		{name: "backup bulk status", method: http.MethodGet, path: "/api/v1/backups/bulk/status", want: []string{domain.PermissionBackupsRead}},
		{name: "backup bulk run latest", method: http.MethodGet, path: "/api/v1/backups/bulk-runs/latest", want: []string{domain.PermissionBackupsRead}},
		{name: "backup bulk run start", method: http.MethodPost, path: "/api/v1/backups/bulk-runs", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup bulk run get", method: http.MethodGet, path: "/api/v1/backups/bulk-runs/" + id, want: []string{domain.PermissionBackupsRead}},
		{name: "backup bulk run pause", method: http.MethodPost, path: "/api/v1/backups/bulk-runs/" + id + "/pause", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup bulk run resume", method: http.MethodPost, path: "/api/v1/backups/bulk-runs/" + id + "/resume", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup bulk run cancel", method: http.MethodPost, path: "/api/v1/backups/bulk-runs/" + id + "/cancel", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup bulk legacy", method: http.MethodPost, path: "/api/v1/backups/bulk", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup bulk download", method: http.MethodPost, path: "/api/v1/backups/bulk-download", want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup job get", method: http.MethodGet, path: "/api/v1/backup-jobs/" + id, want: []string{domain.PermissionBackupsRead}},
		{name: "backup job delete", method: http.MethodDelete, path: "/api/v1/backup-jobs/" + id, want: []string{domain.PermissionBackupsUpdate}},
		{name: "backup file download", method: http.MethodGet, path: "/api/v1/backup-files/" + id + "/download", want: []string{domain.PermissionBackupsRead}},
		{name: "backup file content", method: http.MethodGet, path: "/api/v1/backup-files/" + id + "/content", want: []string{domain.PermissionBackupsRead}},

		{name: "vendors list", method: http.MethodGet, path: "/api/v1/vendors", want: []string{domain.PermissionSettingsRead}},
		{name: "vendor read", method: http.MethodGet, path: "/api/v1/vendors/mikrotik", want: []string{domain.PermissionSettingsRead}},
		{name: "vendor update", method: http.MethodPut, path: "/api/v1/vendors/mikrotik", want: []string{domain.PermissionSettingsUpdate}},

		{name: "instance backups list", method: http.MethodGet, path: "/api/v1/instance-backups", want: []string{domain.PermissionBackupsRead}},
		{name: "instance backup create", method: http.MethodPost, path: "/api/v1/instance-backups", want: []string{domain.PermissionBackupsUpdate}},
		{name: "instance backup restore", method: http.MethodPost, path: "/api/v1/instance-backups/restore", want: []string{domain.PermissionBackupsUpdate}},
		{name: "instance backup get", method: http.MethodGet, path: "/api/v1/instance-backups/" + id, want: []string{domain.PermissionBackupsRead}},
		{name: "instance backup delete", method: http.MethodDelete, path: "/api/v1/instance-backups/" + id, want: []string{domain.PermissionBackupsUpdate}},
		{name: "instance backup download", method: http.MethodGet, path: "/api/v1/instance-backups/" + id + "/download", want: []string{domain.PermissionBackupsRead}},
		{name: "instance backup cancel", method: http.MethodPost, path: "/api/v1/instance-backups/" + id + "/cancel", want: []string{domain.PermissionBackupsUpdate}},

		{name: "bridge binary download", method: http.MethodGet, path: "/api/v1/bridge/download/linux/amd64", want: []string{domain.PermissionSettingsRead}},
		{name: "bridge launch request", method: http.MethodPost, path: "/api/v1/bridge/launch-requests/" + id, want: []string{domain.PermissionBridgeTokenCreate}},
		{name: "bridge legacy token", method: http.MethodPost, path: "/api/v1/bridge/token/" + id, want: []string{domain.PermissionBridgeTokenCreate}},

		{name: "admin dashboard", method: http.MethodGet, path: "/api/v1/admin/dashboard", want: []string{domain.PermissionAdminDashboard}},
		{name: "admin users list", method: http.MethodGet, path: "/api/v1/admin/users", want: []string{domain.PermissionUsersRead}},
		{name: "admin users create", method: http.MethodPost, path: "/api/v1/admin/users", want: []string{domain.PermissionUsersCreate, domain.PermissionUsersUpdate}},
		{name: "admin user read", method: http.MethodGet, path: "/api/v1/admin/users/" + id, want: []string{domain.PermissionUsersRead}},
		{name: "admin user update", method: http.MethodPatch, path: "/api/v1/admin/users/" + id, want: []string{domain.PermissionUsersUpdate}},
		{name: "admin user status", method: http.MethodPatch, path: "/api/v1/admin/users/" + id + "/status", want: []string{domain.PermissionUsersUpdate}},
		{name: "admin user role assign", method: http.MethodPost, path: "/api/v1/admin/users/" + id + "/roles", want: []string{domain.PermissionRolesAssign}},
		{name: "admin user role remove", method: http.MethodDelete, path: "/api/v1/admin/users/" + id + "/roles/" + roleID, want: []string{domain.PermissionRolesAssign}},
		{name: "admin user password reset", method: http.MethodPost, path: "/api/v1/admin/users/" + id + "/password-reset", want: []string{domain.PermissionUsersUpdate}},
		{name: "admin roles list", method: http.MethodGet, path: "/api/v1/admin/roles", want: []string{domain.PermissionRolesRead}},
		{name: "admin permissions list", method: http.MethodGet, path: "/api/v1/admin/permissions", want: []string{domain.PermissionRolesRead}},
		{name: "admin audit logs", method: http.MethodGet, path: "/api/v1/admin/audit-logs", want: []string{domain.PermissionAuditLogsRead}},
	}

	for _, tt := range routeTests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := requiredPermissionsForRoute(tt.method, tt.path)
			if !known {
				t.Fatalf("route %s %s was not known", tt.method, tt.path)
			}
			if !sameStringSlice(got, tt.want) {
				t.Fatalf("permissions = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestRequiredPermissionsForUnknownProtectedRoutesFailClosed(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "canvas unknown map action", method: http.MethodGet, path: "/api/v1/canvas/maps/" + id + "/unknown"},
		{name: "canvas unknown area child", method: http.MethodPut, path: "/api/v1/canvas/maps/" + id + "/areas/" + id + "/children"},
		{name: "device backup latest child", method: http.MethodGet, path: "/api/v1/devices/" + id + "/backups/latest/extra"},
		{name: "device credential assignment child", method: http.MethodDelete, path: "/api/v1/devices/" + id + "/credential-profiles/" + id + "/extra"},
		{name: "settings bridge unknown child", method: http.MethodPost, path: "/api/v1/settings/bridge/secret/extra"},
		{name: "grafana profile child", method: http.MethodPut, path: "/api/v1/grafana/dashboard-profiles/" + id + "/extra"},
		{name: "backup run unknown action", method: http.MethodPost, path: "/api/v1/backups/bulk-runs/" + id + "/unknown"},
		{name: "backup file unknown action", method: http.MethodGet, path: "/api/v1/backup-files/" + id + "/metadata"},
		{name: "instance backup download child", method: http.MethodGet, path: "/api/v1/instance-backups/" + id + "/download/extra"},
		{name: "bridge download child", method: http.MethodGet, path: "/api/v1/bridge/download/linux/amd64/sha256"},
		{name: "admin user role child", method: http.MethodDelete, path: "/api/v1/admin/users/" + id + "/roles/viewer/extra"},
		{name: "top level users not registered", method: http.MethodGet, path: "/api/v1/users"},
		{name: "top level users prefix", method: http.MethodGet, path: "/api/v1/usersettings"},
		{name: "top level roles not registered", method: http.MethodGet, path: "/api/v1/roles"},
		{name: "top level roles prefix", method: http.MethodGet, path: "/api/v1/roles-extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, known := requiredPermissionsForRoute(tt.method, tt.path); known {
				t.Fatalf("permissions = %#v known=true for unknown route %s %s", got, tt.method, tt.path)
			}
		})
	}
}

// TestRequiredPermissionsForUnsupportedRouteMethodsFailClosed prevents fixed path policies from granting unrelated methods.
func TestRequiredPermissionsForUnsupportedRouteMethodsFailClosed(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "topology canvas post", method: http.MethodPost, path: "/api/v1/topology/canvas"},
		{name: "settings me delete", method: http.MethodDelete, path: "/api/v1/settings/me"},
		{name: "settings bridge secret delete", method: http.MethodDelete, path: "/api/v1/settings/bridge/secret"},
		{name: "device backup latest post", method: http.MethodPost, path: "/api/v1/devices/" + id + "/backups/latest"},
		{name: "link read by id", method: http.MethodGet, path: "/api/v1/links/" + id},
		{name: "instance backup download post", method: http.MethodPost, path: "/api/v1/instance-backups/" + id + "/download"},
		{name: "bridge download post", method: http.MethodPost, path: "/api/v1/bridge/download/linux/amd64"},
		{name: "bridge launch request get", method: http.MethodGet, path: "/api/v1/bridge/launch-requests/" + id},
		{name: "bridge token get", method: http.MethodGet, path: "/api/v1/bridge/token/" + id},
		{name: "admin user roles get", method: http.MethodGet, path: "/api/v1/admin/users/" + id + "/roles"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := requiredPermissionsForRoute(tt.method, tt.path)
			if !known {
				t.Fatalf("route %s %s was not known", tt.method, tt.path)
			}
			if len(got) != 0 {
				t.Fatalf("permissions = %#v, want none for unsupported method", got)
			}
		})
	}
}

func TestRoutePermissionRegistryRejectsShadowedStaticPatterns(t *testing.T) {
	registry := newRoutePermissionRegistry([]routePermissionSpec{
		{
			pattern:     "/api/v1/devices/{deviceID}",
			permissions: fixedRoutePermissions(domain.PermissionDevicesRead),
		},
		{
			pattern:     "/api/v1/devices/batch",
			permissions: fixedRoutePermissions(domain.PermissionDevicesCreate),
		},
	})

	err := registry.validate()
	if err == nil {
		t.Fatal("registry.validate() error = nil, want shadowed pattern error")
	}
	if got, want := err.Error(), "route permission pattern /api/v1/devices/batch is shadowed by earlier pattern /api/v1/devices/{deviceID}"; got != want {
		t.Fatalf("registry.validate() error = %q, want %q", got, want)
	}
}

func TestRoutePermissionRegistryRejectsMissingPermissionPolicy(t *testing.T) {
	registry := newRoutePermissionRegistry([]routePermissionSpec{
		{pattern: "/api/v1/broken"},
	})

	err := registry.validate()
	if err == nil {
		t.Fatal("registry.validate() error = nil, want missing policy error")
	}
	if got, want := err.Error(), "route permission pattern /api/v1/broken has no permission policy"; got != want {
		t.Fatalf("registry.validate() error = %q, want %q", got, want)
	}
}

func TestProtectedRoutePermissionRegistryMetadataIsValid(t *testing.T) {
	if err := apiRouteMetadata.validate(); err != nil {
		t.Fatalf("api route metadata invalid: %v", err)
	}
	if err := protectedRoutePermissionRegistry.validate(); err != nil {
		t.Fatalf("protected route permission registry invalid: %v", err)
	}
}

func TestUserAuthRejectsUnknownProtectedRouteBeforeHandler(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("alice", false, allSystemPermissionKeys()...))
	served := false
	handler := UserAuth(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if served {
		t.Fatal("handler was called for an unknown protected route")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "permission_denied" {
		t.Fatalf("code = %q, want permission_denied", body["code"])
	}
}

func TestUserSettingsRequiresAccountManagePermission(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("alice", false, domain.PermissionSettingsRead))
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/me", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestNewRouterRejectsAuthorizationHeaderWithoutSession(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef0123456789abcdef")
	req.Header.Set("X-Theia-Operator", "alice")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestNewRouterRejectsAuthenticatedUserMissingPermission(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("alice", false))
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminUsersListDeniedWithoutUsersRead(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("alice", false, domain.PermissionAdminDashboard))
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if auth.listAdminUsersCalled {
		t.Fatal("ListAdminUsers was called despite missing users:read")
	}
}

func TestAdminUsersListReturnsSafePayload(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, domain.PermissionUsersRead))
	auth.adminUsers = []domain.UserWithRolesAndPermissions{{
		User: domain.User{
			ID:           uuid.New(),
			Username:     "operator",
			Email:        "operator@example.test",
			DisplayName:  "Operator",
			Status:       domain.UserStatusActive,
			PasswordHash: "secret-password-hash",
		},
		Roles: []domain.Role{{ID: domain.RoleViewer, Name: domain.RoleViewer}},
	}}
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"password_hash", "secret-password-hash", "token_hash", "reset_token_hash"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("admin users response leaked %q in %s", forbidden, body)
		}
	}
	var parsed struct {
		Users []safeUserResponse `json:"users"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(parsed.Users) != 1 || parsed.Users[0].Username != "operator" {
		t.Fatalf("users response = %+v, want operator", parsed.Users)
	}
	if !auth.listAdminUsersCalled {
		t.Fatal("ListAdminUsers was not called")
	}
}

func TestAdminUsersCreateReturnsSafePayload(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, domain.PermissionUsersCreate))
	auth.createdAdminUser = &domain.UserWithRolesAndPermissions{
		User: domain.User{
			ID:                 uuid.New(),
			Username:           "created",
			Email:              "created@example.test",
			DisplayName:        "Created User",
			Status:             domain.UserStatusActive,
			MustChangePassword: true,
			PasswordHash:       "secret-password-hash",
		},
		Roles: []domain.Role{{ID: domain.RoleUser, Name: domain.RoleUser}},
	}
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users",
		strings.NewReader(`{"username":"created","email":"created@example.test","display_name":"Created User","password":"Correct Horse Battery Staple 2026!","roles":["user"]}`),
	)
	addSessionCookie(req, testSessionToken)
	addCSRFCookieAndHeader(req, testCSRFToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"password_hash", "secret-password-hash", "token_hash", "reset_token_hash"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("admin user create response leaked %q in %s", forbidden, body)
		}
	}
	var parsed struct {
		User safeUserResponse `json:"user"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if parsed.User.Username != "created" || !parsed.User.MustChangePassword {
		t.Fatalf("created user response = %+v, want safe created user", parsed.User)
	}
	if !auth.createAdminUserCalled {
		t.Fatal("CreateAdminUser was not called")
	}
}

func TestAdminUsersCreateReturnsClearPasswordPolicyError(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(testSessionToken, testCSRFToken, testAPIUser("admin", false, domain.PermissionUsersCreate))
	auth.createAdminUserErr = service.ErrPasswordPolicyViolation
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/admin/users",
		strings.NewReader(`{"username":"created","email":"created@example.test","password":"short"}`),
	)
	addSessionCookie(req, testSessionToken)
	addCSRFCookieAndHeader(req, testCSRFToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "invalid request") {
		t.Fatalf("body = %s, should not use generic invalid request", body)
	}
	if !strings.Contains(body, "Password must be 10 to 24 characters") {
		t.Fatalf("body = %s, want password policy detail", body)
	}
}

func TestNewRouterRejectsNormalRoutesUntilPasswordChanged(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("bootstrap", true, domain.PermissionSettingsRead),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "password_change_required" {
		t.Fatalf("code = %q, want password_change_required", body["code"])
	}
}

func TestAuthPasswordChangeAllowedWhilePasswordChangeRequired(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	user := testAPIUser("bootstrap", true, domain.PermissionSettingsRead)
	auth.setSession(testSessionToken, testCSRFToken, user)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password/change",
		strings.NewReader(`{"current_password":"theia","new_password":"Correct Horse Battery Staple 2026!"}`),
	)
	addSessionCookie(req, testSessionToken)
	addCSRFCookieAndHeader(req, testCSRFToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			MustChangePassword bool `json:"must_change_password"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Authenticated || body.User.MustChangePassword {
		t.Fatalf("response = %+v, want authenticated user with must_change_password=false", body)
	}
	if !auth.changePasswordCalled {
		t.Fatal("ChangePassword was not called")
	}
}

func TestAuthPasswordResetPublicRouteCompletesWithoutSessionOrCSRF(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password/reset",
		strings.NewReader(`{"token":"raw-reset-token","new_password":"Correct Horse Battery Staple Reset 2026!"}`),
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", rec.Code, rec.Body.String())
	}
	if !auth.completePasswordResetCalled {
		t.Fatal("CompletePasswordReset was not called")
	}
	if auth.completedPasswordReset.Token != "raw-reset-token" {
		t.Fatalf("reset token = %q, want raw-reset-token", auth.completedPasswordReset.Token)
	}
	if auth.completedPasswordReset.NewPassword != "Correct Horse Battery Staple Reset 2026!" {
		t.Fatalf("new password = %q", auth.completedPasswordReset.NewPassword)
	}
}

func TestAuthPasswordResetMapsServiceErrorsWithoutTokenLeak(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "invalid token",
			err:        service.ErrInvalidCredentials,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_credentials",
		},
		{
			name:       "expired token",
			err:        service.ErrPasswordResetExpired,
			wantStatus: http.StatusGone,
			wantCode:   "password_reset_expired",
		},
		{
			name:       "password policy",
			err:        service.ErrPasswordPolicyViolation,
			wantStatus: http.StatusBadRequest,
			wantCode:   "password_policy_violation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := newFakeAPIAuthProvider()
			auth.completePasswordResetErr = tt.err
			router := newAuthTestRouter(auth)

			req := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/auth/password/reset",
				strings.NewReader(`{"token":"sensitive-reset-token","new_password":"short"}`),
			)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			body := rec.Body.String()
			if strings.Contains(body, "sensitive-reset-token") || strings.Contains(body, "token_hash") {
				t.Fatalf("password reset error response leaked token material: %s", body)
			}
			var parsed map[string]string
			if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if parsed["code"] != tt.wantCode {
				t.Fatalf("code = %q, want %q", parsed["code"], tt.wantCode)
			}
			if tt.wantCode == "password_policy_violation" &&
				!strings.Contains(parsed["error"], "Password must be 10 to 24 characters") {
				t.Fatalf("error = %q, want password policy detail", parsed["error"])
			}
		})
	}
}

func TestAuthPasswordResetPublicRouteUsesOriginGuard(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	router := NewRouter(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		WithSecurity(SecurityConfig{AllowedOrigins: []string{"https://ops.example"}}),
		withAuthProvider(auth),
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/password/reset",
		strings.NewReader(`{"token":"raw-reset-token","new_password":"Correct Horse Battery Staple Reset 2026!"}`),
	)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	if auth.completePasswordResetCalled {
		t.Fatal("CompletePasswordReset was called despite rejected origin")
	}
}

func TestAuthLoginSetsSessionAndCSRFCookies(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.login = &service.LoginResult{
		User:         testAPIUser("alice", false, domain.PermissionSettingsRead).User,
		SessionToken: testSessionToken,
		CSRFToken:    testCSRFToken,
		ExpiresAt:    time.Now().UTC().Add(time.Hour),
	}
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"identifier":"alice","password":"password"}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	sessionCookie := findCookie(t, rec.Result().Cookies(), authSessionCookieName)
	if !sessionCookie.HttpOnly || sessionCookie.Value != testSessionToken {
		t.Fatalf("session cookie = %+v, want HttpOnly session token", sessionCookie)
	}
	csrfCookie := findCookie(t, rec.Result().Cookies(), authCSRFCookieName)
	if csrfCookie.HttpOnly || csrfCookie.Value != testCSRFToken {
		t.Fatalf("csrf cookie = %+v, want readable csrf token", csrfCookie)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{"password_hash", "token_hash", testSessionToken, testCSRFToken} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("login response leaked %q in %s", forbidden, body)
		}
	}
}

func TestAuthMeReturnsSafeCurrentUserPayload(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("alice", false, domain.PermissionSettingsRead, domain.PermissionTopologyRead),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "password_hash") || strings.Contains(body, "token_hash") {
		t.Fatalf("me response exposed secret-bearing fields: %s", body)
	}
	var parsed struct {
		Authenticated bool `json:"authenticated"`
		User          struct {
			Username           string   `json:"username"`
			Status             string   `json:"status"`
			MustChangePassword bool     `json:"must_change_password"`
			Permissions        []string `json:"permissions"`
		} `json:"user"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !parsed.Authenticated || parsed.User.Username != "alice" || parsed.User.Status != string(domain.UserStatusActive) {
		t.Fatalf("response = %+v, want authenticated alice", parsed)
	}
	if len(parsed.User.Permissions) != 2 {
		t.Fatalf("permissions = %#v, want 2 entries", parsed.User.Permissions)
	}
}

func TestAuthMeReturnsUnauthenticatedWithoutSession(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("authenticated = true, want false")
	}
}

func TestLegacyAuthMeAliasReturnsUnauthenticatedWithoutSession(t *testing.T) {
	router := newAuthTestRouter(newFakeAPIAuthProvider())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Authenticated {
		t.Fatalf("authenticated = true, want false")
	}
}

func TestCSRFRequiredForMutatingProtectedRequests(t *testing.T) {
	auth := newFakeAPIAuthProvider()
	auth.setSession(
		testSessionToken,
		testCSRFToken,
		testAPIUser("alice", false, domain.PermissionSettingsUpdate),
	)
	router := newAuthTestRouter(auth)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/bridge.secret", strings.NewReader(`{"value":"redacted"}`))
	addSessionCookie(req, testSessionToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["code"] != "csrf_required" {
		t.Fatalf("code = %q, want csrf_required", body["code"])
	}
}

func newAuthTestRouter(auth *fakeAPIAuthProvider) http.Handler {
	return NewRouter(
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		"",
		nil,
		nil,
		withAuthProvider(auth),
	)
}

type fakeAPIAuthProvider struct {
	login                       *service.LoginResult
	loginErr                    error
	changeErr                   error
	completePasswordResetErr    error
	usersByToken                map[string]*service.AuthenticatedUser
	csrfByToken                 map[string]string
	logoutTokens                []string
	changePasswordCalled        bool
	completePasswordResetCalled bool
	completedPasswordReset      service.PasswordResetCompleteInput
	adminUsers                  []domain.UserWithRolesAndPermissions
	createdAdminUser            *domain.UserWithRolesAndPermissions
	createAdminUserErr          error
	listAdminUsersCalled        bool
	createAdminUserCalled       bool
}

func newFakeAPIAuthProvider() *fakeAPIAuthProvider {
	return &fakeAPIAuthProvider{
		usersByToken: make(map[string]*service.AuthenticatedUser),
		csrfByToken:  make(map[string]string),
	}
}

func (f *fakeAPIAuthProvider) setSession(token, csrf string, user *service.AuthenticatedUser) {
	f.usersByToken[token] = user
	f.csrfByToken[token] = csrf
}

func (f *fakeAPIAuthProvider) Login(context.Context, service.LoginInput) (*service.LoginResult, error) {
	if f.loginErr != nil {
		return nil, f.loginErr
	}
	if f.login == nil {
		return nil, service.ErrInvalidCredentials
	}
	user := &service.AuthenticatedUser{
		User:    f.login.User,
		Session: f.login.Session,
	}
	f.setSession(f.login.SessionToken, f.login.CSRFToken, user)
	return f.login, nil
}

func (f *fakeAPIAuthProvider) CurrentUser(_ context.Context, rawSessionToken string) (*service.AuthenticatedUser, error) {
	user, ok := f.usersByToken[strings.TrimSpace(rawSessionToken)]
	if !ok {
		return nil, service.ErrInvalidSession
	}
	return user, nil
}

func (f *fakeAPIAuthProvider) Logout(_ context.Context, rawSessionToken string) error {
	rawSessionToken = strings.TrimSpace(rawSessionToken)
	if _, ok := f.usersByToken[rawSessionToken]; !ok {
		return service.ErrInvalidSession
	}
	f.logoutTokens = append(f.logoutTokens, rawSessionToken)
	delete(f.usersByToken, rawSessionToken)
	return nil
}

func (f *fakeAPIAuthProvider) ChangePassword(_ context.Context, input service.PasswordChangeInput) error {
	if f.changeErr != nil {
		return f.changeErr
	}
	f.changePasswordCalled = true
	for _, user := range f.usersByToken {
		if user.User.User.ID == input.UserID {
			user.User.User.MustChangePassword = false
			return nil
		}
	}
	return service.ErrInvalidSession
}

func (f *fakeAPIAuthProvider) CompletePasswordReset(_ context.Context, input service.PasswordResetCompleteInput) error {
	f.completePasswordResetCalled = true
	f.completedPasswordReset = input
	return f.completePasswordResetErr
}

func (f *fakeAPIAuthProvider) ValidateCSRF(_ context.Context, rawSessionToken, csrfToken string) error {
	want, ok := f.csrfByToken[strings.TrimSpace(rawSessionToken)]
	if !ok {
		return service.ErrInvalidSession
	}
	if strings.TrimSpace(csrfToken) == "" {
		return errAPICSRFRequired
	}
	if csrfToken != want {
		return service.ErrInvalidSession
	}
	return nil
}

func (f *fakeAPIAuthProvider) RequirePermission(user *service.AuthenticatedUser, permissionKey string) error {
	if user != nil && user.HasPermission(permissionKey) {
		return nil
	}
	return service.ErrPermissionDenied
}

func (f *fakeAPIAuthProvider) RequireRole(user *service.AuthenticatedUser, roleID string) error {
	if user != nil && user.HasRole(roleID) {
		return nil
	}
	return service.ErrPermissionDenied
}

func (f *fakeAPIAuthProvider) AdminDashboard(context.Context, *service.AuthenticatedUser) (*service.AdminDashboardResult, error) {
	return &service.AdminDashboardResult{}, nil
}

func (f *fakeAPIAuthProvider) ListAdminUsers(context.Context, *service.AuthenticatedUser, domain.UserListFilter) ([]domain.UserWithRolesAndPermissions, error) {
	f.listAdminUsersCalled = true
	return f.adminUsers, nil
}

func (f *fakeAPIAuthProvider) CreateAdminUser(context.Context, *service.AuthenticatedUser, service.AdminCreateUserInput) (*domain.UserWithRolesAndPermissions, error) {
	f.createAdminUserCalled = true
	if f.createAdminUserErr != nil {
		return nil, f.createAdminUserErr
	}
	if f.createdAdminUser == nil {
		return nil, domain.ErrAuthUserNotFound
	}
	return f.createdAdminUser, nil
}

func (f *fakeAPIAuthProvider) GetAdminUser(_ context.Context, _ *service.AuthenticatedUser, id uuid.UUID) (*domain.UserWithRolesAndPermissions, error) {
	for _, user := range f.adminUsers {
		if user.User.ID == id {
			return &user, nil
		}
	}
	return nil, domain.ErrAuthUserNotFound
}

func (f *fakeAPIAuthProvider) UpdateAdminUser(context.Context, *service.AuthenticatedUser, service.AdminUpdateUserInput) (*domain.UserWithRolesAndPermissions, error) {
	return f.createdAdminUser, nil
}

func (f *fakeAPIAuthProvider) SetAdminUserStatus(context.Context, *service.AuthenticatedUser, service.AdminUserStatusInput) (*domain.UserWithRolesAndPermissions, error) {
	return f.createdAdminUser, nil
}

func (f *fakeAPIAuthProvider) AssignAdminUserRole(context.Context, *service.AuthenticatedUser, service.AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error) {
	return f.createdAdminUser, nil
}

func (f *fakeAPIAuthProvider) RemoveAdminUserRole(context.Context, *service.AuthenticatedUser, service.AdminUserRoleInput) (*domain.UserWithRolesAndPermissions, error) {
	return f.createdAdminUser, nil
}

func (f *fakeAPIAuthProvider) CreateAdminPasswordResetToken(context.Context, *service.AuthenticatedUser, uuid.UUID) (*service.PasswordResetTokenResult, error) {
	return &service.PasswordResetTokenResult{Token: "raw-reset-token", ExpiresAt: time.Now().UTC().Add(time.Minute)}, nil
}

func (f *fakeAPIAuthProvider) ListAdminRoles(context.Context, *service.AuthenticatedUser) ([]service.AdminRole, error) {
	return nil, nil
}

func (f *fakeAPIAuthProvider) ListAdminPermissions(context.Context, *service.AuthenticatedUser) ([]domain.Permission, error) {
	return nil, nil
}

func (f *fakeAPIAuthProvider) ListAdminAuditLogs(context.Context, *service.AuthenticatedUser, domain.AuditLogFilter) ([]domain.AuditLog, error) {
	return nil, nil
}

func testAPIUser(username string, mustChange bool, permissions ...string) *service.AuthenticatedUser {
	userID := uuid.New()
	grants := make([]domain.Permission, 0, len(permissions))
	for _, permission := range permissions {
		grants = append(grants, domain.Permission{ID: permission, Key: permission})
	}
	return &service.AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{
			User: domain.User{
				ID:                 userID,
				Username:           username,
				Email:              username + "@example.test",
				DisplayName:        username,
				Status:             domain.UserStatusActive,
				MustChangePassword: mustChange,
			},
			Roles:       []domain.Role{{ID: domain.RoleUser, Name: domain.RoleUser}},
			Permissions: grants,
		},
		Session: service.AuthenticatedSession{
			ID:     uuid.New(),
			UserID: userID,
		},
	}
}

func addSessionCookie(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: authSessionCookieName, Value: token})
}

func addCSRFCookieAndHeader(req *http.Request, token string) {
	req.AddCookie(&http.Cookie{Name: authCSRFCookieName, Value: token})
	req.Header.Set(csrfHeaderName, token)
}

func allSystemPermissionKeys() []string {
	permissions := domain.SystemPermissions()
	keys := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		keys = append(keys, permission.Key)
	}
	return keys
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s not found in %#v", name, cookies)
	return nil
}

var errAPICSRFRequired = errors.New("csrf token required")
