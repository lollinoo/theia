package api

import (
	"net/http"
	"testing"
)

// TestRequiredPermissionsForPrefixRegressionRoutesFailClosed locks segment-exact RBAC route matching.
func TestRequiredPermissionsForPrefixRegressionRoutesFailClosed(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "canvas maps adjacent prefix", method: http.MethodGet, path: "/api/v1/canvas/maps-extra"},
		{name: "canvas map device adjacent prefix", method: http.MethodPatch, path: "/api/v1/canvas/maps/" + id + "/devices-extra/" + id},
		{name: "devices backups adjacent root", method: http.MethodGet, path: "/api/v1/devices-backups"},
		{name: "device backups adjacent child", method: http.MethodGet, path: "/api/v1/devices/" + id + "/backups-extra"},
		{name: "settings me adjacent root", method: http.MethodGet, path: "/api/v1/settings-me"},
		{name: "settings me extra child", method: http.MethodPatch, path: "/api/v1/settings/me/profile"},
		{name: "instance backups adjacent root", method: http.MethodGet, path: "/api/v1/instance-backups-restore"},
		{name: "instance backup restore child", method: http.MethodPost, path: "/api/v1/instance-backups/restore/status"},
		{name: "bridge download missing arch", method: http.MethodGet, path: "/api/v1/bridge/download/linux"},
		{name: "bridge token missing id", method: http.MethodPost, path: "/api/v1/bridge/token"},
		{name: "admin users roles adjacent root", method: http.MethodGet, path: "/api/v1/admin/users-roles"},
		{name: "admin user roles adjacent child", method: http.MethodDelete, path: "/api/v1/admin/users/" + id + "/roles-extra/viewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, known := requiredPermissionsForRoute(tt.method, tt.path); known {
				t.Fatalf("permissions = %#v known=true for prefix-regression route %s %s", got, tt.method, tt.path)
			}
		})
	}
}
