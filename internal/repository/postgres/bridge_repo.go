package postgres

// This file defines bridge repo persistence behavior, ordering guarantees, and not-found conventions.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

// BridgeRepo represents bridge repo data used by the persistence boundary.
type BridgeRepo struct {
	db *DB
}

// NewBridgeRepo constructs bridge repo state for the persistence boundary.
func NewBridgeRepo(db *sql.DB) *BridgeRepo {
	return &BridgeRepo{db: wrapDB(db)}
}

// GetUserSettings retrieves user settings data from the persistence boundary.
func (r *BridgeRepo) GetUserSettings(ctx context.Context, userID uuid.UUID) (*domain.UserSettings, error) {
	now := time.Now().UTC()
	if _, err := r.execContext(ctx,
		`INSERT INTO user_settings (user_id, updated_at)
		 VALUES (?, ?)
		 ON CONFLICT (user_id) DO NOTHING`,
		userID.String(), now,
	); err != nil {
		return nil, fmt.Errorf("ensuring user settings: %w", err)
	}

	var settings domain.UserSettings
	var userIDStr string
	var bridgePortOverride sql.NullInt64
	if err := r.queryRowContext(ctx,
		`SELECT user_id, timezone, locale, bridge_port, bridge_port_override, updated_at
		 FROM user_settings WHERE user_id = ?`,
		userID.String(),
	).Scan(&userIDStr, &settings.Timezone, &settings.Locale, &settings.BridgePort, &bridgePortOverride, &settings.UpdatedAt); err != nil {
		return nil, fmt.Errorf("getting user settings: %w", err)
	}
	parsed, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("parsing user settings user_id: %w", err)
	}
	settings.UserID = parsed
	settings.BridgePortOverride = nullIntPtr(bridgePortOverride)
	return &settings, nil
}

func (r *BridgeRepo) UpsertUserSettings(ctx context.Context, settings *domain.UserSettings) error {
	if settings == nil {
		return fmt.Errorf("user settings is required")
	}
	if settings.BridgePortOverride != nil && (*settings.BridgePortOverride < 1 || *settings.BridgePortOverride > 65535) {
		return fmt.Errorf("bridge_port_override must be between 1 and 65535")
	}
	if settings.Timezone == "" {
		settings.Timezone = "UTC"
	}
	if settings.Locale == "" {
		settings.Locale = "en-US"
	}
	if settings.UpdatedAt.IsZero() {
		settings.UpdatedAt = time.Now().UTC()
	}
	legacyBridgePort := 1337
	if settings.BridgePortOverride != nil {
		legacyBridgePort = *settings.BridgePortOverride
	}
	_, err := r.execContext(ctx,
		`INSERT INTO user_settings (user_id, timezone, locale, bridge_port, bridge_port_override, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET
		   timezone = EXCLUDED.timezone,
		   locale = EXCLUDED.locale,
		   bridge_port = EXCLUDED.bridge_port,
		   bridge_port_override = EXCLUDED.bridge_port_override,
		   updated_at = EXCLUDED.updated_at`,
		settings.UserID.String(),
		settings.Timezone,
		settings.Locale,
		legacyBridgePort,
		nullableIntValue(settings.BridgePortOverride),
		settings.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting user settings: %w", err)
	}
	return nil
}

// GetActiveBridgeCredentialForUser retrieves active bridge credential for user data from the persistence boundary.
func (r *BridgeRepo) GetActiveBridgeCredentialForUser(ctx context.Context, userID uuid.UUID) (*domain.BridgeCredential, error) {
	credential, err := r.scanBridgeCredentialRow(r.queryRowContext(ctx,
		bridgeCredentialSelectSQL()+` WHERE user_id = ? AND status = ?`,
		userID.String(), string(domain.BridgeCredentialStatusActive),
	))
	if err != nil {
		return nil, fmt.Errorf("getting active bridge credential: %w", err)
	}
	return credential, nil
}

// GetBridgeCredentialByPrefix retrieves bridge credential by prefix data from the persistence boundary.
func (r *BridgeRepo) GetBridgeCredentialByPrefix(ctx context.Context, prefix string) (*domain.BridgeCredential, error) {
	credential, err := r.scanBridgeCredentialRow(r.queryRowContext(ctx,
		bridgeCredentialSelectSQL()+` WHERE secret_prefix = ?`,
		prefix,
	))
	if err != nil {
		return nil, fmt.Errorf("getting bridge credential by prefix: %w", err)
	}
	return credential, nil
}

// CreateBridgeCredential creates bridge credential data through the persistence boundary.
func (r *BridgeRepo) CreateBridgeCredential(ctx context.Context, credential *domain.BridgeCredential) error {
	if err := normalizeBridgeCredential(credential); err != nil {
		return err
	}
	if err := insertBridgeCredential(ctx, r.db.raw, credential); err != nil {
		return fmt.Errorf("creating bridge credential: %w", err)
	}
	return nil
}

// CreateBridgeCredentialWithAudit creates bridge credential with audit data through the persistence boundary.
func (r *BridgeRepo) CreateBridgeCredentialWithAudit(ctx context.Context, credential *domain.BridgeCredential, log *domain.AuditLog) error {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning bridge credential create: %w", err)
	}
	defer tx.Rollback()

	if err := normalizeBridgeCredential(credential); err != nil {
		return err
	}
	if err := insertBridgeCredential(ctx, tx, credential); err != nil {
		return fmt.Errorf("creating bridge credential: %w", err)
	}
	if err := insertBridgeAuditLog(ctx, tx, log); err != nil {
		return fmt.Errorf("writing bridge audit log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing bridge credential create: %w", err)
	}
	return nil
}

func (r *BridgeRepo) RotateBridgeCredentialWithAudit(ctx context.Context, userID uuid.UUID, credential *domain.BridgeCredential, when time.Time, reason string, log *domain.AuditLog) error {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning bridge credential rotate: %w", err)
	}
	defer tx.Rollback()

	active, err := r.scanBridgeCredentialRow(tx.QueryRowContext(ctx, rebindQuery(bridgeCredentialSelectSQL()+` WHERE user_id = ? AND status = ? FOR UPDATE`), userID.String(), string(domain.BridgeCredentialStatusActive)))
	if err != nil && !errors.Is(err, domain.ErrBridgeCredentialNotFound) {
		return fmt.Errorf("getting active bridge credential for rotate: %w", err)
	}
	if active != nil {
		if _, err := tx.ExecContext(ctx, rebindQuery(
			`UPDATE bridge_credentials
			 SET status = ?, revoked_at = ?, rotation_reason = ?
			 WHERE id = ?`,
		), string(domain.BridgeCredentialStatusRevoked), when, reason, active.ID.String()); err != nil {
			return fmt.Errorf("revoking bridge credential for rotate: %w", err)
		}
	}
	if err := normalizeBridgeCredential(credential); err != nil {
		return err
	}
	if err := insertBridgeCredential(ctx, tx, credential); err != nil {
		return fmt.Errorf("creating rotated bridge credential: %w", err)
	}
	if err := insertBridgeAuditLog(ctx, tx, log); err != nil {
		return fmt.Errorf("writing bridge audit log: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing bridge credential rotate: %w", err)
	}
	return nil
}

func normalizeBridgeCredential(credential *domain.BridgeCredential) error {
	if credential == nil {
		return fmt.Errorf("bridge credential is required")
	}
	if credential.ID == uuid.Nil {
		credential.ID = uuid.New()
	}
	if credential.Status == "" {
		credential.Status = domain.BridgeCredentialStatusActive
	}
	if credential.CreatedAt.IsZero() {
		credential.CreatedAt = time.Now().UTC()
	}
	return nil
}

func insertBridgeCredential(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, credential *domain.BridgeCredential) error {
	_, err := execer.ExecContext(ctx, rebindQuery(
		`INSERT INTO bridge_credentials (
			id, user_id, secret_hash, secret_prefix, status, created_at, rotated_at,
			revoked_at, last_used_at, expires_at, created_by_user_id, rotation_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	),
		credential.ID.String(),
		credential.UserID.String(),
		credential.SecretHash,
		credential.SecretPrefix,
		string(credential.Status),
		credential.CreatedAt,
		nullableTimeValue(credential.RotatedAt),
		nullableTimeValue(credential.RevokedAt),
		nullableTimeValue(credential.LastUsedAt),
		nullableTimeValue(credential.ExpiresAt),
		uuidPtrString(credential.CreatedByUserID),
		credential.RotationReason,
	)
	return err
}

func insertBridgeAuditLog(ctx context.Context, execer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, log *domain.AuditLog) error {
	if log == nil {
		return fmt.Errorf("audit log is required")
	}
	if log.ID == uuid.Nil {
		log.ID = uuid.New()
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now().UTC()
	}
	if log.MetadataJSON == "" {
		log.MetadataJSON = "{}"
	}
	_, err := execer.ExecContext(ctx, rebindQuery(
		`INSERT INTO audit_logs (
			id, actor_user_id, target_user_id, action, resource, resource_id,
			metadata_json, ip_address, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	),
		log.ID.String(),
		uuidPtrString(log.ActorUserID),
		uuidPtrString(log.TargetUserID),
		log.Action,
		log.Resource,
		log.ResourceID,
		log.MetadataJSON,
		log.IPAddress,
		log.UserAgent,
		log.CreatedAt,
	)
	return err
}

func (r *BridgeRepo) RevokeActiveBridgeCredentialForUser(ctx context.Context, userID uuid.UUID, when time.Time, reason string) (*domain.BridgeCredential, error) {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning bridge credential revoke: %w", err)
	}
	defer tx.Rollback()

	credential, err := r.scanBridgeCredentialRow(tx.QueryRowContext(ctx, rebindQuery(bridgeCredentialSelectSQL()+` WHERE user_id = ? AND status = ? FOR UPDATE`), userID.String(), string(domain.BridgeCredentialStatusActive)))
	if err != nil {
		return nil, fmt.Errorf("getting active bridge credential for revoke: %w", err)
	}
	credential.Status = domain.BridgeCredentialStatusRevoked
	credential.RevokedAt = &when
	credential.RotationReason = reason
	if _, err := tx.ExecContext(ctx, rebindQuery(
		`UPDATE bridge_credentials
		 SET status = ?, revoked_at = ?, rotation_reason = ?
		 WHERE id = ?`,
	), string(credential.Status), when, reason, credential.ID.String()); err != nil {
		return nil, fmt.Errorf("revoking bridge credential: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing bridge credential revoke: %w", err)
	}
	return credential, nil
}

func (r *BridgeRepo) TouchBridgeCredentialLastUsed(ctx context.Context, credentialID uuid.UUID, when time.Time) error {
	res, err := r.execContext(ctx,
		`UPDATE bridge_credentials SET last_used_at = ? WHERE id = ?`,
		when, credentialID.String(),
	)
	if err != nil {
		return fmt.Errorf("touching bridge credential: %w", err)
	}
	if err := requireRowsAffected(res, domain.ErrBridgeCredentialNotFound); err != nil {
		return fmt.Errorf("touching bridge credential: %w", err)
	}
	return nil
}

// CreateBridgeLaunchRequest creates bridge launch request data through the persistence boundary.
func (r *BridgeRepo) CreateBridgeLaunchRequest(ctx context.Context, request *domain.BridgeLaunchRequest) error {
	if request.ID == uuid.Nil {
		request.ID = uuid.New()
	}
	if request.CreatedAt.IsZero() {
		request.CreatedAt = time.Now().UTC()
	}
	_, err := r.execContext(ctx,
		`INSERT INTO bridge_launch_requests (
			id, user_id, device_id, token_hash, created_at, expires_at, used_at, consumed_by_credential_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		request.ID.String(),
		request.UserID.String(),
		request.DeviceID.String(),
		request.TokenHash,
		request.CreatedAt,
		request.ExpiresAt,
		nullableTimeValue(request.UsedAt),
		uuidPtrString(request.ConsumedByCredentialID),
	)
	if err != nil {
		return fmt.Errorf("creating bridge launch request: %w", err)
	}
	return nil
}

// GetBridgeLaunchRequestByTokenHash retrieves bridge launch request by token hash data from the persistence boundary.
func (r *BridgeRepo) GetBridgeLaunchRequestByTokenHash(ctx context.Context, tokenHash string) (*domain.BridgeLaunchRequest, error) {
	request, err := scanBridgeLaunchRequestRow(r.queryRowContext(ctx,
		bridgeLaunchRequestSelectSQL()+` WHERE token_hash = ?`,
		tokenHash,
	))
	if err != nil {
		return nil, fmt.Errorf("getting bridge launch request by token hash: %w", err)
	}
	return request, nil
}

func (r *BridgeRepo) ConsumeBridgeLaunchRequest(ctx context.Context, tokenHash string, credentialID uuid.UUID, when time.Time) (*domain.BridgeLaunchRequest, error) {
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("beginning bridge launch consume: %w", err)
	}
	defer tx.Rollback()

	request, err := scanBridgeLaunchRequestRow(tx.QueryRowContext(ctx, rebindQuery(
		bridgeLaunchRequestSelectSQL()+` WHERE token_hash = ? FOR UPDATE`,
	), tokenHash))
	if err != nil {
		return nil, fmt.Errorf("getting bridge launch request: %w", err)
	}
	if request.UsedAt != nil {
		return nil, domain.ErrBridgeLaunchRequestUsed
	}
	request.UsedAt = &when
	request.ConsumedByCredentialID = &credentialID
	if _, err := tx.ExecContext(ctx, rebindQuery(
		`UPDATE bridge_launch_requests
		 SET used_at = ?, consumed_by_credential_id = ?
		 WHERE id = ?`,
	), when, credentialID.String(), request.ID.String()); err != nil {
		return nil, fmt.Errorf("consuming bridge launch request: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("committing bridge launch consume: %w", err)
	}
	return request, nil
}

func (r *BridgeRepo) RecordBridgeConnectorDownload(ctx context.Context, download *domain.BridgeConnectorDownload) error {
	if download.ID == uuid.Nil {
		download.ID = uuid.New()
	}
	if download.DownloadedAt.IsZero() {
		download.DownloadedAt = time.Now().UTC()
	}
	_, err := r.execContext(ctx,
		`INSERT INTO bridge_connector_downloads (
			id, user_id, connector_version, platform, downloaded_at, ip_address, user_agent
		) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		download.ID.String(),
		download.UserID.String(),
		download.ConnectorVersion,
		download.Platform,
		download.DownloadedAt,
		download.IPAddress,
		download.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("recording bridge connector download: %w", err)
	}
	return nil
}

func (r *BridgeRepo) execContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return r.db.raw.ExecContext(ctx, rebindQuery(query), args...)
}

func (r *BridgeRepo) queryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return r.db.raw.QueryRowContext(ctx, rebindQuery(query), args...)
}

func bridgeCredentialSelectSQL() string {
	return `SELECT id, user_id, secret_hash, secret_prefix, status, created_at, rotated_at,
		revoked_at, last_used_at, expires_at, created_by_user_id, rotation_reason
		FROM bridge_credentials`
}

func bridgeLaunchRequestSelectSQL() string {
	return `SELECT id, user_id, device_id, token_hash, created_at, expires_at, used_at,
		consumed_by_credential_id
		FROM bridge_launch_requests`
}

func (r *BridgeRepo) scanBridgeCredentialRow(row *sql.Row) (*domain.BridgeCredential, error) {
	var credential domain.BridgeCredential
	var idStr, userIDStr, status string
	var rotatedAt, revokedAt, lastUsedAt, expiresAt sql.NullTime
	var createdByUserID sql.NullString
	err := row.Scan(
		&idStr,
		&userIDStr,
		&credential.SecretHash,
		&credential.SecretPrefix,
		&status,
		&credential.CreatedAt,
		&rotatedAt,
		&revokedAt,
		&lastUsedAt,
		&expiresAt,
		&createdByUserID,
		&credential.RotationReason,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrBridgeCredentialNotFound
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge credential id: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge credential user_id: %w", err)
	}
	createdBy, err := parseNullUUID(createdByUserID)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge credential created_by_user_id: %w", err)
	}
	credential.ID = id
	credential.UserID = userID
	credential.Status = domain.BridgeCredentialStatus(status)
	credential.RotatedAt = nullTimePtr(rotatedAt)
	credential.RevokedAt = nullTimePtr(revokedAt)
	credential.LastUsedAt = nullTimePtr(lastUsedAt)
	credential.ExpiresAt = nullTimePtr(expiresAt)
	credential.CreatedByUserID = createdBy
	return &credential, nil
}

func scanBridgeLaunchRequestRow(row *sql.Row) (*domain.BridgeLaunchRequest, error) {
	var request domain.BridgeLaunchRequest
	var idStr, userIDStr, deviceIDStr string
	var usedAt sql.NullTime
	var consumedByCredentialID sql.NullString
	err := row.Scan(
		&idStr,
		&userIDStr,
		&deviceIDStr,
		&request.TokenHash,
		&request.CreatedAt,
		&request.ExpiresAt,
		&usedAt,
		&consumedByCredentialID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrBridgeLaunchRequestNotFound
		}
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge launch request id: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge launch request user_id: %w", err)
	}
	deviceID, err := uuid.Parse(deviceIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge launch request device_id: %w", err)
	}
	consumedBy, err := parseNullUUID(consumedByCredentialID)
	if err != nil {
		return nil, fmt.Errorf("invalid bridge launch request consumed_by_credential_id: %w", err)
	}
	request.ID = id
	request.UserID = userID
	request.DeviceID = deviceID
	request.UsedAt = nullTimePtr(usedAt)
	request.ConsumedByCredentialID = consumedBy
	return &request, nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func nullIntPtr(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	v := int(value.Int64)
	return &v
}

func nullableIntValue(value *int) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
