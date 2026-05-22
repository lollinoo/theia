package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type BridgeCredentialStatus string

const (
	BridgeCredentialStatusActive  BridgeCredentialStatus = "active"
	BridgeCredentialStatusRevoked BridgeCredentialStatus = "revoked"
)

var (
	ErrBridgeCredentialNotFound    = errors.New("bridge credential not found")
	ErrBridgeLaunchRequestNotFound = errors.New("bridge launch request not found")
	ErrBridgeLaunchRequestUsed     = errors.New("bridge launch request already used")
)

type UserSettings struct {
	UserID             uuid.UUID
	Timezone           string
	Locale             string
	BridgePort         int
	BridgePortOverride *int
	UpdatedAt          time.Time
}

type BridgeCredential struct {
	ID              uuid.UUID
	UserID          uuid.UUID
	SecretHash      string
	SecretPrefix    string
	Status          BridgeCredentialStatus
	CreatedAt       time.Time
	RotatedAt       *time.Time
	RevokedAt       *time.Time
	LastUsedAt      *time.Time
	ExpiresAt       *time.Time
	CreatedByUserID *uuid.UUID
	RotationReason  string
}

type BridgeLaunchRequest struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	DeviceID               uuid.UUID
	TokenHash              string
	CreatedAt              time.Time
	ExpiresAt              time.Time
	UsedAt                 *time.Time
	ConsumedByCredentialID *uuid.UUID
}

type BridgeConnectorDownload struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	ConnectorVersion string
	Platform         string
	DownloadedAt     time.Time
	IPAddress        string
	UserAgent        string
}

type BridgeRepository interface {
	GetUserSettings(ctx context.Context, userID uuid.UUID) (*UserSettings, error)
	UpsertUserSettings(ctx context.Context, settings *UserSettings) error
	GetActiveBridgeCredentialForUser(ctx context.Context, userID uuid.UUID) (*BridgeCredential, error)
	GetBridgeCredentialByPrefix(ctx context.Context, prefix string) (*BridgeCredential, error)
	CreateBridgeCredential(ctx context.Context, credential *BridgeCredential) error
	CreateBridgeCredentialWithAudit(ctx context.Context, credential *BridgeCredential, log *AuditLog) error
	RotateBridgeCredentialWithAudit(ctx context.Context, userID uuid.UUID, credential *BridgeCredential, when time.Time, reason string, log *AuditLog) error
	RevokeActiveBridgeCredentialForUser(ctx context.Context, userID uuid.UUID, when time.Time, reason string) (*BridgeCredential, error)
	TouchBridgeCredentialLastUsed(ctx context.Context, credentialID uuid.UUID, when time.Time) error
	CreateBridgeLaunchRequest(ctx context.Context, request *BridgeLaunchRequest) error
	GetBridgeLaunchRequestByTokenHash(ctx context.Context, tokenHash string) (*BridgeLaunchRequest, error)
	ConsumeBridgeLaunchRequest(ctx context.Context, tokenHash string, credentialID uuid.UUID, when time.Time) (*BridgeLaunchRequest, error)
	RecordBridgeConnectorDownload(ctx context.Context, download *BridgeConnectorDownload) error
}
