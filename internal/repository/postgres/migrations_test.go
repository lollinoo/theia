package postgres

// This file exercises migrations behavior so refactors preserve the documented contract.

import (
	"encoding/base64"
	"io/fs"
	"strings"
	"testing"

	"github.com/google/uuid"
	theiacrypto "github.com/lollinoo/theia/internal/crypto"
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
		"metadata_json TEXT NOT NULL DEFAULT '{}'",
		"status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('active', 'disabled', 'pending', 'locked'))",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("auth RBAC migration missing %q", expected)
		}
	}

	if strings.Contains(migration, "metadata_json JSONB") {
		t.Fatal("auth RBAC migration stores audit metadata as JSONB instead of TEXT")
	}
	if strings.Contains(strings.ToLower(migration), "sqli"+"te") {
		t.Fatal("auth RBAC migration still references legacy dialect")
	}
}

func TestPostgresMigrationsDeclarePerUserBridgeSchemaAndIndexes(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000017_per_user_bridge_credentials.up.sql")
	if err != nil {
		t.Fatalf("reading per-user bridge migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS user_settings",
		"CREATE TABLE IF NOT EXISTS bridge_credentials",
		"CREATE TABLE IF NOT EXISTS bridge_launch_requests",
		"CREATE TABLE IF NOT EXISTS bridge_connector_downloads",
		"idx_bridge_credentials_user_id",
		"idx_bridge_credentials_secret_prefix",
		"idx_bridge_credentials_status",
		"idx_bridge_credentials_last_used_at",
		"idx_bridge_credentials_one_active_per_user",
		"idx_bridge_launch_requests_user_id",
		"idx_bridge_launch_requests_device_id",
		"idx_bridge_launch_requests_expires_at",
		"idx_bridge_connector_downloads_user_id",
		"idx_bridge_connector_downloads_downloaded_at",
		"DELETE FROM settings WHERE key = 'bridge_secret'",
		"CHECK (bridge_port BETWEEN 1 AND 65535)",
		"CHECK (status IN ('active', 'revoked'))",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("per-user bridge migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareUserBridgePortOverride(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000018_user_bridge_port_override.up.sql")
	if err != nil {
		t.Fatalf("reading user bridge port override migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"ADD COLUMN bridge_port_override INTEGER NULL",
		"bridge_port_override IS NULL OR bridge_port_override BETWEEN 1 AND 65535",
		"SET bridge_port_override = bridge_port",
		"WHERE bridge_port <> 1337",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("user bridge port override migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareBulkBackupRunSchema(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000019_bulk_backup_runs.up.sql")
	if err != nil {
		t.Fatalf("reading bulk backup runs migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS backup_bulk_runs",
		"CREATE TABLE IF NOT EXISTS backup_bulk_run_items",
		"backup_bulk_runs_one_active",
		"ON backup_bulk_runs ((TRUE))",
		"backup_bulk_runs_created_at_idx",
		"status IN ('running', 'success', 'partial', 'failed', 'cancelled', 'cancelling')",
		"status IN ('checking', 'skipped', 'queued', 'running', 'success', 'failed', 'cancelled')",
		"backup_job_id TEXT REFERENCES backup_jobs(id) ON DELETE SET NULL",
		"cancel_requested BOOLEAN NOT NULL DEFAULT FALSE",
		"ON DELETE CASCADE",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("bulk backup runs migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareBulkBackupRunPauseSchema(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000020_bulk_backup_run_pause.up.sql")
	if err != nil {
		t.Fatalf("reading bulk backup run pause migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"backup_bulk_runs_status_check",
		"status IN ('running', 'pausing', 'paused', 'success', 'partial', 'failed', 'cancelled', 'cancelling')",
		"WHERE status IN ('running', 'pausing', 'paused', 'cancelling')",
		"DROP INDEX IF EXISTS backup_bulk_runs_one_active",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("bulk backup run pause migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareBulkBackupRunItemActiveSchema(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000021_bulk_backup_run_item_active.up.sql")
	if err != nil {
		t.Fatalf("reading bulk backup run item active migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"backup_bulk_run_items_status_check",
		"status IN ('checking', 'skipped', 'active', 'queued', 'running', 'success', 'failed', 'cancelled')",
		"ALTER TABLE backup_bulk_run_items DROP CONSTRAINT",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("bulk backup run item active migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareBulkBackupRunProcessorLeaseSchema(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000022_bulk_backup_run_processor_lease.up.sql")
	if err != nil {
		t.Fatalf("reading bulk backup run processor lease migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"processing_owner TEXT NOT NULL DEFAULT ''",
		"processing_lease_expires_at TIMESTAMPTZ",
		"backup_bulk_runs_processing_lease_idx",
		"WHERE processing_owner <> ''",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("bulk backup run processor lease migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareDeviceAddressSchema(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000024_device_addresses.up.sql")
	if err != nil {
		t.Fatalf("reading device addresses migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS device_addresses",
		"device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE",
		"normalized_address TEXT NOT NULL",
		"role TEXT NOT NULL DEFAULT 'other'",
		"CHECK (role IN ('primary', 'management', 'backup', 'monitoring', 'other'))",
		"UNIQUE (device_id, normalized_address)",
		"idx_device_addresses_device_id",
		"idx_device_addresses_normalized_address",
		"idx_device_addresses_device_role_priority",
		"idx_device_addresses_one_primary",
		"INSERT INTO device_addresses",
		"lower(btrim(ip))",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("device addresses migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareProbePorts(t *testing.T) {
	content, err := migrationsFS.ReadFile("migrations/000025_probe_ports.up.sql")
	if err != nil {
		t.Fatalf("reading probe ports migration: %v", err)
	}
	migration := string(content)

	for _, expected := range []string{
		"ALTER TABLE devices",
		"probe_ports",
		"ALTER TABLE device_addresses",
	} {
		if !strings.Contains(migration, expected) {
			t.Fatalf("probe ports migration missing %q", expected)
		}
	}
}

func TestPostgresMigrationsDeclareLeastPrivilegeSystemRolePermissions(t *testing.T) {
	managerPermissions := domain.SystemRolePermissionKeys(domain.RoleManager)
	for _, disallowed := range []string{
		domain.PermissionUsersRead,
		domain.PermissionRolesRead,
		domain.PermissionCredentialsRead,
		domain.PermissionCredentialsUpdate,
	} {
		if containsPermissionKey(managerPermissions, disallowed) {
			t.Fatalf("manager role unexpectedly includes %q", disallowed)
		}
	}
	for _, expected := range []string{
		domain.PermissionAdminDashboard,
		domain.PermissionAccountManage,
		domain.PermissionSettingsRead,
		domain.PermissionTopologyRead,
		domain.PermissionTopologyUpdate,
		domain.PermissionDevicesRead,
		domain.PermissionDevicesCreate,
		domain.PermissionDevicesUpdate,
		domain.PermissionBackupsRead,
		domain.PermissionBackupsUpdate,
		domain.PermissionBridgeTokenCreate,
	} {
		if !containsPermissionKey(managerPermissions, expected) {
			t.Fatalf("manager role missing %q", expected)
		}
	}

	userPermissions := domain.SystemRolePermissionKeys(domain.RoleUser)
	if containsPermissionKey(userPermissions, domain.PermissionBridgeTokenCreate) {
		t.Fatal("user role unexpectedly includes bridge token creation")
	}
	for _, expected := range []string{
		domain.PermissionAccountManage,
		domain.PermissionSettingsRead,
		domain.PermissionTopologyRead,
		domain.PermissionTopologyUpdate,
		domain.PermissionDevicesRead,
		domain.PermissionDevicesUpdate,
		domain.PermissionBackupsRead,
	} {
		if !containsPermissionKey(userPermissions, expected) {
			t.Fatalf("user role missing %q", expected)
		}
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

func TestNormalizeSensitiveMigrationField(t *testing.T) {
	keyring := mustTestKeyring(t, "kid-new", map[string]string{
		"kid-old": "old migration secret",
		"kid-new": "new migration secret",
	})

	t.Run("plaintext becomes active envelope", func(t *testing.T) {
		normalized, changed, err := normalizeSensitiveMigrationField("plain-secret", keyring)
		if err != nil {
			t.Fatalf("normalizeSensitiveMigrationField failed: %v", err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}
		assertEnvelopeWithKeyID(t, normalized, "kid-new")
	})

	t.Run("legacy ciphertext becomes active envelope", func(t *testing.T) {
		legacyRaw, err := theiacrypto.Encrypt([]byte("legacy-secret"), theiacrypto.DeriveKey("kid legacy passphrase"))
		if err != nil {
			t.Fatalf("legacy Encrypt failed: %v", err)
		}
		keyring := mustTestKeyring(t, "kid-new", map[string]string{
			"kid-new": "new migration secret",
			"legacy":  "kid legacy passphrase",
		})

		normalized, changed, err := normalizeSensitiveMigrationField(base64.StdEncoding.EncodeToString(legacyRaw), keyring)
		if err != nil {
			t.Fatalf("normalizeSensitiveMigrationField failed: %v", err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}
		assertEnvelopeWithKeyID(t, normalized, "kid-new")
	})

	t.Run("old key envelope becomes active envelope", func(t *testing.T) {
		oldKeyring := mustTestKeyring(t, "kid-old", map[string]string{
			"kid-old": "old migration secret",
			"kid-new": "new migration secret",
		})
		oldEnvelope, err := oldKeyring.EncryptString("old-envelope-secret")
		if err != nil {
			t.Fatalf("EncryptString failed: %v", err)
		}

		normalized, changed, err := normalizeSensitiveMigrationField(oldEnvelope, keyring)
		if err != nil {
			t.Fatalf("normalizeSensitiveMigrationField failed: %v", err)
		}
		if !changed {
			t.Fatal("changed = false, want true")
		}
		assertEnvelopeWithKeyID(t, normalized, "kid-new")
	})

	t.Run("active envelope is unchanged", func(t *testing.T) {
		activeEnvelope, err := keyring.EncryptString("active-envelope-secret")
		if err != nil {
			t.Fatalf("EncryptString failed: %v", err)
		}

		normalized, changed, err := normalizeSensitiveMigrationField(activeEnvelope, keyring)
		if err != nil {
			t.Fatalf("normalizeSensitiveMigrationField failed: %v", err)
		}
		if changed {
			t.Fatal("changed = true, want false")
		}
		if normalized != activeEnvelope {
			t.Fatal("active envelope should remain byte-for-byte unchanged")
		}
	})

	t.Run("undecryptable encrypted-looking field errors", func(t *testing.T) {
		bogus := base64.StdEncoding.EncodeToString([]byte("1234567890121234567890123456"))
		_, _, err := normalizeSensitiveMigrationField(bogus, keyring)
		if err == nil {
			t.Fatal("normalizeSensitiveMigrationField error = nil, want error")
		}
	})
}

func TestRunMigrationsNormalizesCredentialProfileSecrets(t *testing.T) {
	db := setupTestDB(t)
	keyring := mustTestKeyring(t, "kid-active", map[string]string{
		"kid-active": "active credential migration secret",
	})
	profileID := uuid.New()
	if _, err := db.Exec(
		`INSERT INTO credential_profiles (id, name, description, username, port, auth_method, encrypted_secret, role, created_at, updated_at)
		 VALUES ($1, 'ssh-profile', '', 'admin', 22, 'password', 'plain-ssh-secret', 'Admin', NOW(), NOW())`,
		profileID.String(),
	); err != nil {
		t.Fatalf("inserting credential profile: %v", err)
	}

	if err := RunMigrations(db, keyring); err != nil {
		t.Fatalf("RunMigrations failed: %v", err)
	}

	var normalized string
	if err := db.QueryRow(`SELECT encrypted_secret FROM credential_profiles WHERE id = $1`, profileID.String()).Scan(&normalized); err != nil {
		t.Fatalf("querying normalized secret: %v", err)
	}
	assertEnvelopeWithKeyID(t, normalized, "kid-active")
	if strings.Contains(normalized, "plain-ssh-secret") {
		t.Fatal("normalized secret contains plaintext")
	}

	if err := RunMigrations(db, keyring); err != nil {
		t.Fatalf("second RunMigrations failed: %v", err)
	}
	var afterSecondRun string
	if err := db.QueryRow(`SELECT encrypted_secret FROM credential_profiles WHERE id = $1`, profileID.String()).Scan(&afterSecondRun); err != nil {
		t.Fatalf("querying secret after second run: %v", err)
	}
	if afterSecondRun != normalized {
		t.Fatal("credential profile secret migration is not idempotent")
	}
}

func assertEnvelopeWithKeyID(t *testing.T, value, wantKeyID string) {
	t.Helper()
	keyID, err := theiacrypto.EnvelopeKeyID(value)
	if err != nil {
		t.Fatalf("EnvelopeKeyID failed: %v", err)
	}
	if keyID != wantKeyID {
		t.Fatalf("envelope key id = %q, want %q", keyID, wantKeyID)
	}
}

func containsPermissionKey(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
