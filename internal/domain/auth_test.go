package domain

// This file exercises auth behavior so refactors preserve the documented contract.

import "testing"

func TestUserWithRolesAndPermissionsHasRoleAndPermission(t *testing.T) {
	aggregate := UserWithRolesAndPermissions{
		Roles: []Role{
			{ID: RoleViewer, Name: RoleViewer},
		},
		Permissions: []Permission{
			{ID: PermissionTopologyRead, Key: PermissionTopologyRead},
		},
	}

	if !aggregate.HasRole(RoleViewer) {
		t.Fatalf("HasRole(%q) = false, want true", RoleViewer)
	}
	if aggregate.HasRole(RoleAdmin) {
		t.Fatalf("HasRole(%q) = true, want false", RoleAdmin)
	}
	if !aggregate.HasPermission(PermissionTopologyRead) {
		t.Fatalf("HasPermission(%q) = false, want true", PermissionTopologyRead)
	}
	if aggregate.HasPermission(PermissionUsersDelete) {
		t.Fatalf("HasPermission(%q) = true, want false", PermissionUsersDelete)
	}
}
