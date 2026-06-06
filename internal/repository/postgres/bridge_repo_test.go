package postgres

// This file exercises bridge repo behavior so refactors preserve the documented contract.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBridgeRepoUserSettingsDefaultsAndUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBridgeRepo(db)
	userID := insertBridgeRepoUser(t, db, "settings-user")

	settings, err := repo.GetUserSettings(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserSettings: %v", err)
	}
	if settings.UserID != userID || settings.Timezone != "UTC" || settings.Locale != "en-US" || settings.BridgePortOverride != nil {
		t.Fatalf("default settings = %+v, want UTC/en-US/no bridge port override for user", settings)
	}

	settings.Timezone = "Europe/Rome"
	settings.Locale = "it-IT"
	settings.BridgePortOverride = bridgeRepoIntPtr(1444)
	if err := repo.UpsertUserSettings(context.Background(), settings); err != nil {
		t.Fatalf("UpsertUserSettings: %v", err)
	}
	updated, err := repo.GetUserSettings(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserSettings updated: %v", err)
	}
	if updated.Timezone != "Europe/Rome" || updated.Locale != "it-IT" || updated.BridgePortOverride == nil || *updated.BridgePortOverride != 1444 {
		t.Fatalf("updated settings = %+v", updated)
	}

	updated.BridgePortOverride = bridgeRepoIntPtr(70000)
	if err := repo.UpsertUserSettings(context.Background(), updated); err == nil {
		t.Fatal("UpsertUserSettings accepted invalid bridge port override")
	}
}

func TestBridgeRepoCredentialLifecycle(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBridgeRepo(db)
	userID := insertBridgeRepoUser(t, db, "bridge-user")
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)

	credential := &domain.BridgeCredential{
		ID:           uuid.New(),
		UserID:       userID,
		SecretHash:   "sha256:first",
		SecretPrefix: "theia_bridge_first",
		Status:       domain.BridgeCredentialStatusActive,
		CreatedAt:    now,
	}
	if err := repo.CreateBridgeCredential(context.Background(), credential); err != nil {
		t.Fatalf("CreateBridgeCredential: %v", err)
	}
	if err := repo.CreateBridgeCredential(context.Background(), &domain.BridgeCredential{
		ID:           uuid.New(),
		UserID:       userID,
		SecretHash:   "sha256:second",
		SecretPrefix: "theia_bridge_second",
		Status:       domain.BridgeCredentialStatusActive,
		CreatedAt:    now,
	}); err == nil {
		t.Fatal("CreateBridgeCredential allowed a second active credential for one user")
	}

	byPrefix, err := repo.GetBridgeCredentialByPrefix(context.Background(), "theia_bridge_first")
	if err != nil {
		t.Fatalf("GetBridgeCredentialByPrefix: %v", err)
	}
	if byPrefix.UserID != userID || byPrefix.SecretHash != "sha256:first" {
		t.Fatalf("credential by prefix = %+v", byPrefix)
	}

	revoked, err := repo.RevokeActiveBridgeCredentialForUser(context.Background(), userID, now.Add(time.Minute), "rotate")
	if err != nil {
		t.Fatalf("RevokeActiveBridgeCredentialForUser: %v", err)
	}
	if revoked.Status != domain.BridgeCredentialStatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("revoked credential = %+v", revoked)
	}
	if _, err := repo.GetActiveBridgeCredentialForUser(context.Background(), userID); !errors.Is(err, domain.ErrBridgeCredentialNotFound) {
		t.Fatalf("GetActiveBridgeCredentialForUser after revoke error = %v, want ErrBridgeCredentialNotFound", err)
	}
}

func bridgeRepoIntPtr(value int) *int { return &value }

func TestBridgeRepoLaunchRequestCanBeConsumedOnlyOnce(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBridgeRepo(db)
	userID := insertBridgeRepoUser(t, db, "launch-user")
	credentialID := uuid.New()
	now := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	if err := repo.CreateBridgeCredential(context.Background(), &domain.BridgeCredential{
		ID:           credentialID,
		UserID:       userID,
		SecretHash:   "sha256:launch",
		SecretPrefix: "theia_bridge_launch",
		Status:       domain.BridgeCredentialStatusActive,
		CreatedAt:    now,
	}); err != nil {
		t.Fatalf("CreateBridgeCredential: %v", err)
	}

	request := &domain.BridgeLaunchRequest{
		ID:        uuid.New(),
		UserID:    userID,
		DeviceID:  uuid.New(),
		TokenHash: "hmac-sha256:launch-token",
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
	}
	if err := repo.CreateBridgeLaunchRequest(context.Background(), request); err != nil {
		t.Fatalf("CreateBridgeLaunchRequest: %v", err)
	}
	consumed, err := repo.ConsumeBridgeLaunchRequest(context.Background(), request.TokenHash, credentialID, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ConsumeBridgeLaunchRequest: %v", err)
	}
	if consumed.UsedAt == nil || consumed.ConsumedByCredentialID == nil || *consumed.ConsumedByCredentialID != credentialID {
		t.Fatalf("consumed request = %+v", consumed)
	}
	if _, err := repo.ConsumeBridgeLaunchRequest(context.Background(), request.TokenHash, credentialID, now.Add(2*time.Minute)); !errors.Is(err, domain.ErrBridgeLaunchRequestUsed) {
		t.Fatalf("second ConsumeBridgeLaunchRequest error = %v, want ErrBridgeLaunchRequestUsed", err)
	}
}

func TestBridgeRepoRecordsConnectorDownload(t *testing.T) {
	db := setupTestDB(t)
	repo := NewBridgeRepo(db)
	userID := insertBridgeRepoUser(t, db, "download-user")

	if err := repo.RecordBridgeConnectorDownload(context.Background(), &domain.BridgeConnectorDownload{
		ID:               uuid.New(),
		UserID:           userID,
		ConnectorVersion: "1.5.12",
		Platform:         "linux/amd64",
		DownloadedAt:     time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC),
		IPAddress:        "192.0.2.10",
		UserAgent:        "test-agent",
	}); err != nil {
		t.Fatalf("RecordBridgeConnectorDownload: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM bridge_connector_downloads WHERE user_id = $1`, userID.String()).Scan(&count); err != nil {
		t.Fatalf("counting downloads: %v", err)
	}
	if count != 1 {
		t.Fatalf("download count = %d, want 1", count)
	}
}

func insertBridgeRepoUser(t *testing.T, db *sql.DB, username string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.Exec(
		`INSERT INTO users (id, username, username_normalized, email, email_normalized, password_hash, display_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'active')`,
		id.String(),
		username,
		strings.ToLower(username),
		username+"@example.test",
		strings.ToLower(username+"@example.test"),
		"password-hash",
		username,
	)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	return id
}
