package postgres

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/lollinoo/theia/internal/domain"
)

func TestPostgresMigrationsAreEmbedded(t *testing.T) {
	files, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected embedded PostgreSQL migrations")
	}
	legacyName := "sqli" + "te"
	for _, file := range files {
		if strings.Contains(strings.ToLower(file), legacyName) {
			t.Fatalf("migration path still references legacy dialect: %s", file)
		}
	}
}

func TestPostgresMigrationsDeclareScaleCriticalIndexes(t *testing.T) {
	files, err := fs.Glob(migrationsFS, "migrations/*.up.sql")
	if err != nil {
		t.Fatalf("globbing migrations: %v", err)
	}

	var builder strings.Builder
	for _, file := range files {
		content, err := migrationsFS.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		builder.Write(content)
		builder.WriteByte('\n')
	}
	allMigrations := builder.String()

	for _, expected := range []string{
		"idx_devices_sys_name_lookup",
		"idx_links_target_device_created_at",
		"idx_topology_observations_remote_identity_protocol",
		"idx_unresolved_neighbors_local_device_id",
	} {
		if !strings.Contains(allMigrations, expected) {
			t.Fatalf("PostgreSQL migrations missing critical index %s", expected)
		}
	}
	if strings.Contains(strings.ToLower(allMigrations), "sqli"+"te") {
		t.Fatal("PostgreSQL migrations still reference legacy dialect")
	}
}

func TestPostgresMigrationsDeclareAuthRBACSchemaAndIndexes(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000016_auth_rbac.up.sql")
	if err != nil {
		t.Fatalf("reading auth RBAC migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS users",
		"CREATE TABLE IF NOT EXISTS roles",
		"CREATE TABLE IF NOT EXISTS permissions",
		"CREATE TABLE IF NOT EXISTS user_roles",
		"CREATE TABLE IF NOT EXISTS role_permissions",
		"CREATE TABLE IF NOT EXISTS auth_sessions",
		"CREATE TABLE IF NOT EXISTS password_reset_tokens",
		"CREATE TABLE IF NOT EXISTS audit_logs",
		"idx_users_username_normalized",
		"idx_users_email_normalized",
		"idx_user_roles_user_id",
		"idx_user_roles_role_id",
		"idx_auth_sessions_user_id",
		"idx_auth_sessions_token_hash",
		"idx_auth_sessions_expires_at",
		"idx_password_reset_tokens_token_hash",
		"idx_password_reset_tokens_expires_at",
		"idx_audit_logs_actor_user_id",
		"idx_audit_logs_target_user_id",
		"idx_audit_logs_created_at",
		"ON DELETE CASCADE",
		"ON DELETE SET NULL",
		"status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('active', 'disabled', 'pending', 'locked'))",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("auth RBAC migration missing %q", expected)
		}
	}

	if strings.Contains(strings.ToLower(migration), "sqli"+"te") {
		t.Fatal("auth RBAC migration still references legacy dialect")
	}
}

func TestRunMigrationsOnConfiguredPostgresTestDB(t *testing.T) {
	db := setupTestDB(t)
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("running PostgreSQL migrations twice should be idempotent: %v", err)
	}
	if err := RunMigrations(db, testKey); err != nil {
		t.Fatalf("re-running PostgreSQL migrations with auth seed should be idempotent: %v", err)
	}

	var roleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM roles WHERE is_system_role = TRUE`).Scan(&roleCount); err != nil {
		t.Fatalf("counting system roles: %v", err)
	}
	if roleCount != len(domain.SystemRoleNames()) {
		t.Fatalf("system role count = %d, want %d", roleCount, len(domain.SystemRoleNames()))
	}

	var permissionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM permissions`).Scan(&permissionCount); err != nil {
		t.Fatalf("counting permissions: %v", err)
	}
	if permissionCount != len(domain.SystemPermissions()) {
		t.Fatalf("permission count = %d, want %d", permissionCount, len(domain.SystemPermissions()))
	}

	for _, roleName := range domain.SystemRoleNames() {
		var assignedCount int
		if err := db.QueryRow(
			`SELECT COUNT(*)
			FROM role_permissions rp
			JOIN roles r ON r.id = rp.role_id
			WHERE r.name = $1`,
			roleName,
		).Scan(&assignedCount); err != nil {
			t.Fatalf("counting permissions for role %q: %v", roleName, err)
		}
		if assignedCount != len(domain.SystemRolePermissionKeys(roleName)) {
			t.Fatalf("permission count for role %q = %d, want %d", roleName, assignedCount, len(domain.SystemRolePermissionKeys(roleName)))
		}
	}
}
