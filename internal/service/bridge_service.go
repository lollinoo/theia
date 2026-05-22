package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/security"
)

const bridgeLaunchTTL = 5 * time.Minute

var (
	ErrBridgeCredentialNotConfigured = errors.New("bridge credential not configured")
	ErrBridgeCredentialRevoked       = errors.New("bridge credential revoked")
	ErrBridgeCredentialExpired       = errors.New("bridge credential expired")
	ErrBridgeSecretInvalid           = errors.New("bridge secret invalid")
	ErrBridgeSecretAlreadyConfigured = errors.New("bridge secret already configured")
	ErrBridgeLaunchTokenInvalid      = errors.New("bridge launch token invalid")
	ErrBridgeLaunchTokenExpired      = errors.New("bridge launch token expired")
	ErrBridgeLaunchTokenUsed         = errors.New("bridge launch token already used")
	ErrBridgeLaunchUserMismatch      = errors.New("bridge launch user mismatch")
	ErrDuplicateUserIdentifier       = errors.New("user identifier already exists")
	ErrInvalidUserSettings           = errors.New("invalid user settings")
)

type WinboxCredentialProvider interface {
	GetWinboxCredentials(deviceID uuid.UUID, encryptedSecret, username string) (string, string, error)
}

type WinboxAssignmentProvider interface {
	GetWinboxAssignment(uuid.UUID) (*postgres.WinboxAssignmentRow, error)
}

type BridgeServiceConfig struct {
	BridgeRepo               domain.BridgeRepository
	SettingsRepo             domain.SettingsRepository
	Users                    domain.UserRepository
	AuditLogs                domain.AuditLogRepository
	BackupService            *BackupService
	WinboxCredentialProvider WinboxCredentialProvider
	CredentialProfileRepo    WinboxAssignmentProvider
	SessionSecret            []byte
	Now                      func() time.Time
}

type BridgeService struct {
	bridgeRepo            domain.BridgeRepository
	settingsRepo          domain.SettingsRepository
	users                 domain.UserRepository
	auditLogs             domain.AuditLogRepository
	winboxCredentials     WinboxCredentialProvider
	credentialProfileRepo WinboxAssignmentProvider
	sessionSecret         []byte
	now                   func() time.Time
}

type UserSettingsResult struct {
	User        UserSettingsUser        `json:"user"`
	Preferences UserSettingsPreferences `json:"preferences"`
	Bridge      UserSettingsBridge      `json:"bridge"`
}

type UserSettingsUser struct {
	ID                uuid.UUID  `json:"id"`
	Username          string     `json:"username"`
	Email             string     `json:"email"`
	DisplayName       string     `json:"display_name"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	PasswordChangedAt *time.Time `json:"password_changed_at,omitempty"`
}

type UserSettingsPreferences struct {
	Timezone           string `json:"timezone"`
	Locale             string `json:"locale"`
	BridgePort         int    `json:"bridge_port"`
	GlobalBridgePort   int    `json:"global_bridge_port"`
	BridgePortOverride *int   `json:"bridge_port_override"`
}

type UserSettingsBridge struct {
	Configured bool                      `json:"configured"`
	Credential *BridgeCredentialMetadata `json:"credential,omitempty"`
}

type BridgeCredentialMetadata struct {
	ID           uuid.UUID  `json:"id"`
	SecretPrefix string     `json:"secret_prefix"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	RotatedAt    *time.Time `json:"rotated_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

type UpdateUserSettingsInput struct {
	DisplayName             *string
	Username                *string
	Email                   *string
	Timezone                *string
	Locale                  *string
	BridgePortOverride      *int
	ClearBridgePortOverride bool
}

type BridgeSecretResult struct {
	Credential BridgeCredentialMetadata `json:"credential"`
	Secret     string                   `json:"secret"`
	ShownOnce  bool                     `json:"shown_once"`
}

type BridgeLaunchRequestResult struct {
	LaunchToken string    `json:"launch_token"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type BridgeLaunchCredentials struct {
	IP        string    `json:"ip"`
	Username  string    `json:"username"`
	Password  string    `json:"password"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewBridgeService(config BridgeServiceConfig) (*BridgeService, error) {
	if config.BridgeRepo == nil {
		return nil, errors.New("bridge service repository is required")
	}
	if config.SettingsRepo == nil {
		return nil, errors.New("bridge service settings repository is required")
	}
	if config.Users == nil {
		return nil, errors.New("bridge service users repository is required")
	}
	if config.AuditLogs == nil {
		return nil, errors.New("bridge service audit repository is required")
	}
	winboxCredentials := config.WinboxCredentialProvider
	if winboxCredentials == nil {
		winboxCredentials = config.BackupService
	}
	if winboxCredentials == nil {
		return nil, errors.New("bridge service WinBox credential provider is required")
	}
	if config.CredentialProfileRepo == nil {
		return nil, errors.New("bridge service credential profile repository is required")
	}
	if len(config.SessionSecret) == 0 {
		return nil, errors.New("bridge service session secret is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &BridgeService{
		bridgeRepo:            config.BridgeRepo,
		settingsRepo:          config.SettingsRepo,
		users:                 config.Users,
		auditLogs:             config.AuditLogs,
		winboxCredentials:     winboxCredentials,
		credentialProfileRepo: config.CredentialProfileRepo,
		sessionSecret:         append([]byte(nil), config.SessionSecret...),
		now:                   func() time.Time { return now().UTC() },
	}, nil
}

func (s *BridgeService) GetSettings(ctx context.Context, user *AuthenticatedUser) (*UserSettingsResult, error) {
	current, err := s.users.GetUserByID(ctx, user.User.User.ID)
	if err != nil {
		return nil, fmt.Errorf("loading current user settings account: %w", err)
	}
	preferences, err := s.bridgeRepo.GetUserSettings(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	credential, err := s.bridgeRepo.GetActiveBridgeCredentialForUser(ctx, current.ID)
	if err != nil && !errors.Is(err, domain.ErrBridgeCredentialNotFound) {
		return nil, err
	}
	return s.buildUserSettingsResult(current, preferences, credential), nil
}

func (s *BridgeService) UpdateSettings(ctx context.Context, user *AuthenticatedUser, input UpdateUserSettingsInput) (*UserSettingsResult, error) {
	current, err := s.users.GetUserByID(ctx, user.User.User.ID)
	if err != nil {
		return nil, fmt.Errorf("loading current user for settings update: %w", err)
	}
	if input.Timezone != nil {
		timezone := strings.TrimSpace(*input.Timezone)
		if timezone != "" {
			if _, err := time.LoadLocation(timezone); err != nil {
				return nil, fmt.Errorf("%w: invalid timezone", ErrInvalidUserSettings)
			}
		}
	}
	if input.Locale != nil {
		locale := strings.TrimSpace(*input.Locale)
		if len(locale) > 32 {
			return nil, fmt.Errorf("%w: locale is too long", ErrInvalidUserSettings)
		}
	}
	if input.BridgePortOverride != nil && (*input.BridgePortOverride < 1 || *input.BridgePortOverride > 65535) {
		return nil, fmt.Errorf("%w: bridge port must be between 1 and 65535", ErrInvalidUserSettings)
	}
	if input.DisplayName != nil {
		current.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Username != nil {
		username := strings.TrimSpace(*input.Username)
		if username == "" {
			return nil, fmt.Errorf("%w: username is required", ErrInvalidUserSettings)
		}
		normalized := normalizeLoginIdentifier(username)
		if err := s.ensureIdentifierAvailable(ctx, normalized, current.ID); err != nil {
			return nil, err
		}
		current.Username = username
		current.UsernameNormalized = normalized
	}
	if input.Email != nil {
		email := strings.TrimSpace(*input.Email)
		normalized := normalizeLoginIdentifier(email)
		if email != "" {
			parsed, err := mail.ParseAddress(email)
			if err != nil || parsed.Address != email {
				return nil, fmt.Errorf("%w: invalid email", ErrInvalidUserSettings)
			}
			if err := s.ensureIdentifierAvailable(ctx, normalized, current.ID); err != nil {
				return nil, err
			}
		}
		current.Email = email
		current.EmailNormalized = normalized
	}
	now := s.now()
	current.UpdatedAt = now
	if err := s.users.UpdateUser(ctx, current); err != nil {
		return nil, fmt.Errorf("updating current user settings account: %w", err)
	}

	preferences, err := s.bridgeRepo.GetUserSettings(ctx, current.ID)
	if err != nil {
		return nil, err
	}
	if input.Timezone != nil {
		preferences.Timezone = strings.TrimSpace(*input.Timezone)
	}
	if input.Locale != nil {
		preferences.Locale = strings.TrimSpace(*input.Locale)
	}
	if input.ClearBridgePortOverride {
		preferences.BridgePortOverride = nil
	} else if input.BridgePortOverride != nil {
		preferences.BridgePortOverride = input.BridgePortOverride
	}
	preferences.UpdatedAt = now
	if err := s.bridgeRepo.UpsertUserSettings(ctx, preferences); err != nil {
		return nil, err
	}
	if err := s.appendAuditLog(ctx, &current.ID, &current.ID, "user.settings_updated", "account", current.ID.String(), `{}`); err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, &AuthenticatedUser{User: domain.UserWithRolesAndPermissions{User: *current}, Session: user.Session})
}

func (s *BridgeService) GenerateSecret(ctx context.Context, user *AuthenticatedUser) (*BridgeSecretResult, error) {
	if _, err := s.bridgeRepo.GetActiveBridgeCredentialForUser(ctx, user.User.User.ID); err == nil {
		return nil, ErrBridgeSecretAlreadyConfigured
	} else if !errors.Is(err, domain.ErrBridgeCredentialNotFound) {
		return nil, err
	}
	rawSecret, credential, err := s.newBridgeSecretCredential(user.User.User.ID, nil, "")
	if err != nil {
		return nil, err
	}
	log := s.newBridgeAuditLog(&user.User.User.ID, &user.User.User.ID, "bridge.secret_generated", "bridge_credential", credential.ID.String(), `{}`)
	if err := s.bridgeRepo.CreateBridgeCredentialWithAudit(ctx, credential, log); err != nil {
		return nil, err
	}
	return &BridgeSecretResult{
		Credential: bridgeCredentialMetadata(credential),
		Secret:     rawSecret,
		ShownOnce:  true,
	}, nil
}

func (s *BridgeService) RotateSecret(ctx context.Context, user *AuthenticatedUser, reason string) (*BridgeSecretResult, error) {
	now := s.now()
	reason = strings.TrimSpace(reason)
	rawSecret, credential, err := s.newBridgeSecretCredential(user.User.User.ID, &now, reason)
	if err != nil {
		return nil, err
	}
	log := s.newBridgeAuditLog(&user.User.User.ID, &user.User.User.ID, "bridge.secret_rotated", "bridge_credential", credential.ID.String(), `{}`)
	if err := s.bridgeRepo.RotateBridgeCredentialWithAudit(ctx, user.User.User.ID, credential, now, reason, log); err != nil {
		return nil, err
	}
	return &BridgeSecretResult{
		Credential: bridgeCredentialMetadata(credential),
		Secret:     rawSecret,
		ShownOnce:  true,
	}, nil
}

func (s *BridgeService) RevokeSecret(ctx context.Context, user *AuthenticatedUser, reason string) (*BridgeCredentialMetadata, error) {
	credential, err := s.bridgeRepo.RevokeActiveBridgeCredentialForUser(ctx, user.User.User.ID, s.now(), strings.TrimSpace(reason))
	if err != nil {
		if errors.Is(err, domain.ErrBridgeCredentialNotFound) {
			return nil, ErrBridgeCredentialNotConfigured
		}
		return nil, err
	}
	metadata := bridgeCredentialMetadata(credential)
	if err := s.appendAuditLog(ctx, &user.User.User.ID, &user.User.User.ID, "bridge.secret_revoked", "bridge_credential", credential.ID.String(), `{}`); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (s *BridgeService) VerifyUserBridgeSecret(ctx context.Context, rawSecret string) (*domain.BridgeCredential, error) {
	prefix, err := security.BridgeSecretPrefix(rawSecret)
	if err != nil {
		return nil, ErrBridgeSecretInvalid
	}
	credential, err := s.bridgeRepo.GetBridgeCredentialByPrefix(ctx, prefix)
	if err != nil {
		return nil, ErrBridgeSecretInvalid
	}
	if credential.Status != domain.BridgeCredentialStatusActive {
		if credential.Status == domain.BridgeCredentialStatusRevoked {
			return nil, ErrBridgeSecretInvalid
		}
		return nil, ErrBridgeCredentialRevoked
	}
	if credential.ExpiresAt != nil && !s.now().Before(*credential.ExpiresAt) {
		return nil, ErrBridgeCredentialExpired
	}
	if !security.VerifyBridgeSecret(rawSecret, credential.SecretHash) {
		return nil, ErrBridgeSecretInvalid
	}
	return credential, nil
}

func (s *BridgeService) CreateLaunchRequest(ctx context.Context, user *AuthenticatedUser, deviceID uuid.UUID) (*BridgeLaunchRequestResult, error) {
	token, err := security.GenerateToken()
	if err != nil {
		return nil, err
	}
	now := s.now()
	expiresAt := now.Add(bridgeLaunchTTL)
	request := &domain.BridgeLaunchRequest{
		ID:        uuid.New(),
		UserID:    user.User.User.ID,
		DeviceID:  deviceID,
		TokenHash: security.HashToken(token, s.sessionSecret),
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}
	if err := s.bridgeRepo.CreateBridgeLaunchRequest(ctx, request); err != nil {
		return nil, err
	}
	return &BridgeLaunchRequestResult{LaunchToken: token, ExpiresAt: expiresAt}, nil
}

func (s *BridgeService) ResolveConnectorLaunch(ctx context.Context, rawSecret string, launchToken string, ipAddress string, userAgent string) (*BridgeLaunchCredentials, error) {
	credential, err := s.VerifyUserBridgeSecret(ctx, rawSecret)
	if err != nil {
		_ = s.appendAuditLog(ctx, nil, nil, "bridge.auth_failed", "bridge", "", `{"reason":"invalid_secret"}`)
		return nil, err
	}
	tokenHash := security.HashToken(strings.TrimSpace(launchToken), s.sessionSecret)
	request, err := s.bridgeRepo.GetBridgeLaunchRequestByTokenHash(ctx, tokenHash)
	if err != nil {
		_ = s.appendAuditLog(ctx, &credential.UserID, &credential.UserID, "bridge.auth_failed", "bridge", credential.ID.String(), `{"reason":"invalid_launch_token"}`)
		return nil, ErrBridgeLaunchTokenInvalid
	}
	if request.UsedAt != nil {
		_ = s.appendAuditLog(ctx, &credential.UserID, &request.UserID, "bridge.auth_failed", "bridge", credential.ID.String(), `{"reason":"used_launch_token"}`)
		return nil, ErrBridgeLaunchTokenUsed
	}
	if !s.now().Before(request.ExpiresAt) {
		_ = s.appendAuditLog(ctx, &credential.UserID, &request.UserID, "bridge.auth_failed", "bridge", credential.ID.String(), `{"reason":"expired_launch_token"}`)
		return nil, ErrBridgeLaunchTokenExpired
	}
	if request.UserID != credential.UserID {
		_ = s.appendAuditLog(ctx, &credential.UserID, &request.UserID, "bridge.auth_failed", "bridge", credential.ID.String(), `{"reason":"user_mismatch"}`)
		return nil, ErrBridgeLaunchUserMismatch
	}
	if _, err := s.bridgeRepo.ConsumeBridgeLaunchRequest(ctx, tokenHash, credential.ID, s.now()); err != nil {
		if errors.Is(err, domain.ErrBridgeLaunchRequestUsed) {
			return nil, ErrBridgeLaunchTokenUsed
		}
		return nil, err
	}

	assignment, err := s.credentialProfileRepo.GetWinboxAssignment(request.DeviceID)
	if err != nil {
		return nil, err
	}
	ip, password, err := s.winboxCredentials.GetWinboxCredentials(request.DeviceID, assignment.EncryptedSecret, assignment.Username)
	if err != nil {
		return nil, err
	}
	if err := s.bridgeRepo.TouchBridgeCredentialLastUsed(ctx, credential.ID, s.now()); err != nil {
		return nil, err
	}
	if err := s.appendAuditLog(ctx, &credential.UserID, &credential.UserID, "bridge.auth_success", "bridge", credential.ID.String(), "{}"); err != nil {
		return nil, err
	}
	return &BridgeLaunchCredentials{
		IP:        ip,
		Username:  assignment.Username,
		Password:  password,
		ExpiresAt: request.ExpiresAt,
	}, nil
}

func (s *BridgeService) RecordConnectorDownload(ctx context.Context, user *AuthenticatedUser, platform string, ipAddress string, userAgent string) error {
	download := &domain.BridgeConnectorDownload{
		ID:               uuid.New(),
		UserID:           user.User.User.ID,
		ConnectorVersion: "",
		Platform:         platform,
		DownloadedAt:     s.now(),
		IPAddress:        ipAddress,
		UserAgent:        userAgent,
	}
	if err := s.bridgeRepo.RecordBridgeConnectorDownload(ctx, download); err != nil {
		return err
	}
	return s.appendAuditLog(ctx, &user.User.User.ID, &user.User.User.ID, "bridge.connector_downloaded", "bridge_connector", platform, `{}`)
}

func (s *BridgeService) newBridgeSecretCredential(userID uuid.UUID, rotatedAt *time.Time, reason string) (string, *domain.BridgeCredential, error) {
	rawSecret, prefix, err := security.GenerateBridgeSecret()
	if err != nil {
		return "", nil, err
	}
	hash, err := security.HashBridgeSecret(rawSecret)
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	credential := &domain.BridgeCredential{
		ID:              uuid.New(),
		UserID:          userID,
		SecretHash:      hash,
		SecretPrefix:    prefix,
		Status:          domain.BridgeCredentialStatusActive,
		CreatedAt:       now,
		RotatedAt:       rotatedAt,
		CreatedByUserID: &userID,
		RotationReason:  reason,
	}
	return rawSecret, credential, nil
}

func (s *BridgeService) ensureIdentifierAvailable(ctx context.Context, normalized string, currentUserID uuid.UUID) error {
	if normalized == "" {
		return nil
	}
	existing, err := s.users.GetUserByLoginIdentifier(ctx, normalized)
	if err != nil {
		if errors.Is(err, domain.ErrAuthUserNotFound) {
			return nil
		}
		return err
	}
	if existing.ID != currentUserID {
		return ErrDuplicateUserIdentifier
	}
	return nil
}

func (s *BridgeService) appendAuditLog(ctx context.Context, actorUserID, targetUserID *uuid.UUID, action, resource, resourceID, metadataJSON string) error {
	log := s.newBridgeAuditLog(actorUserID, targetUserID, action, resource, resourceID, metadataJSON)
	if err := s.auditLogs.AppendAuditLog(ctx, log); err != nil {
		return fmt.Errorf("writing bridge audit log: %w", err)
	}
	return nil
}

func (s *BridgeService) newBridgeAuditLog(actorUserID, targetUserID *uuid.UUID, action, resource, resourceID, metadataJSON string) *domain.AuditLog {
	return &domain.AuditLog{
		ID:           uuid.New(),
		ActorUserID:  actorUserID,
		TargetUserID: targetUserID,
		Action:       action,
		Resource:     resource,
		ResourceID:   resourceID,
		MetadataJSON: metadataJSON,
		CreatedAt:    s.now(),
	}
}

func (s *BridgeService) buildUserSettingsResult(user *domain.User, preferences *domain.UserSettings, credential *domain.BridgeCredential) *UserSettingsResult {
	globalBridgePort := s.globalBridgePort()
	effectiveBridgePort := globalBridgePort
	if preferences.BridgePortOverride != nil {
		effectiveBridgePort = *preferences.BridgePortOverride
	}
	result := &UserSettingsResult{
		User: UserSettingsUser{
			ID:                user.ID,
			Username:          user.Username,
			Email:             user.Email,
			DisplayName:       user.DisplayName,
			LastLoginAt:       user.LastLoginAt,
			PasswordChangedAt: user.PasswordChangedAt,
		},
		Preferences: UserSettingsPreferences{
			Timezone:           preferences.Timezone,
			Locale:             preferences.Locale,
			BridgePort:         effectiveBridgePort,
			GlobalBridgePort:   globalBridgePort,
			BridgePortOverride: cloneIntPtr(preferences.BridgePortOverride),
		},
	}
	if credential != nil {
		metadata := bridgeCredentialMetadata(credential)
		result.Bridge.Configured = true
		result.Bridge.Credential = &metadata
	}
	return result
}

func (s *BridgeService) globalBridgePort() int {
	const defaultBridgePort = 1337
	value, err := s.settingsRepo.Get(domain.SettingBridgePort)
	if err != nil {
		return defaultBridgePort
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 || parsed > 65535 {
		return defaultBridgePort
	}
	return parsed
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func bridgeCredentialMetadata(credential *domain.BridgeCredential) BridgeCredentialMetadata {
	if credential == nil {
		return BridgeCredentialMetadata{}
	}
	return BridgeCredentialMetadata{
		ID:           credential.ID,
		SecretPrefix: credential.SecretPrefix,
		Status:       string(credential.Status),
		CreatedAt:    credential.CreatedAt,
		RotatedAt:    credential.RotatedAt,
		RevokedAt:    credential.RevokedAt,
		LastUsedAt:   credential.LastUsedAt,
		ExpiresAt:    credential.ExpiresAt,
	}
}
