package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/security"
)

func TestBridgeServiceUpdateSettingsOnlyChangesSafeSelfServiceFields(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	other := h.addUser(t, "taken", "taken@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)

	result, err := bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(user), UpdateUserSettingsInput{
		DisplayName:        stringPtr("Alice Smith"),
		Username:           stringPtr("AliceNew"),
		Email:              stringPtr("AliceNew@example.test"),
		Timezone:           stringPtr("Europe/Rome"),
		Locale:             stringPtr("it-IT"),
		BridgePortOverride: bridgeIntPtr(1444),
	})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if result.User.DisplayName != "Alice Smith" || result.User.Username != "AliceNew" || result.User.Email != "AliceNew@example.test" {
		t.Fatalf("updated user = %+v", result.User)
	}
	stored := h.store.user(t, user.ID)
	if stored.Status != domain.UserStatusActive || stored.PasswordHash == "" || stored.MustChangePassword {
		t.Fatalf("unsafe account fields changed: %+v", stored)
	}
	if result.Preferences.Timezone != "Europe/Rome" || result.Preferences.Locale != "it-IT" || result.Preferences.BridgePort != 1444 || result.Preferences.BridgePortOverride == nil || *result.Preferences.BridgePortOverride != 1444 {
		t.Fatalf("updated preferences = %+v", result.Preferences)
	}

	_, err = bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(stored), UpdateUserSettingsInput{
		Username: stringPtr(strings.ToUpper(other.Username)),
	})
	if !errors.Is(err, ErrDuplicateUserIdentifier) {
		t.Fatalf("duplicate username error = %v, want ErrDuplicateUserIdentifier", err)
	}
	_, err = bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(stored), UpdateUserSettingsInput{
		Email: stringPtr(strings.ToUpper(other.Email)),
	})
	if !errors.Is(err, ErrDuplicateUserIdentifier) {
		t.Fatalf("duplicate email error = %v, want ErrDuplicateUserIdentifier", err)
	}
	if !bridgeSvc.auditActionExists("user.settings_updated") {
		t.Fatal("expected user.settings_updated audit event")
	}
}

func TestBridgeServiceUpdateSettingsRejectsInvalidSelfServiceFields(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)

	tests := []struct {
		name  string
		input UpdateUserSettingsInput
	}{
		{name: "blank username", input: UpdateUserSettingsInput{Username: stringPtr(" ")}},
		{name: "invalid email", input: UpdateUserSettingsInput{Email: stringPtr("not-an-email")}},
		{name: "invalid timezone", input: UpdateUserSettingsInput{Timezone: stringPtr("Mars/Base")}},
		{name: "invalid bridge port override", input: UpdateUserSettingsInput{BridgePortOverride: bridgeIntPtr(70000)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(user), tt.input)
			if !errors.Is(err, ErrInvalidUserSettings) {
				t.Fatalf("UpdateSettings error = %v, want ErrInvalidUserSettings", err)
			}
		})
	}
}

func TestBridgeServiceGetSettingsResolvesBridgePortFromGlobalDefaultAndUserOverride(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)
	if err := bridgeSvc.settingsRepo.Set(domain.SettingBridgePort, "1555"); err != nil {
		t.Fatalf("Set global bridge port: %v", err)
	}

	result, err := bridgeSvc.service.GetSettings(context.Background(), bridgeAuthUser(user))
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if result.Preferences.GlobalBridgePort != 1555 || result.Preferences.BridgePort != 1555 || result.Preferences.BridgePortOverride != nil {
		t.Fatalf("preferences without override = %+v", result.Preferences)
	}

	updated, err := bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(user), UpdateUserSettingsInput{
		BridgePortOverride: bridgeIntPtr(1666),
	})
	if err != nil {
		t.Fatalf("UpdateSettings override: %v", err)
	}
	if updated.Preferences.GlobalBridgePort != 1555 || updated.Preferences.BridgePort != 1666 || updated.Preferences.BridgePortOverride == nil || *updated.Preferences.BridgePortOverride != 1666 {
		t.Fatalf("preferences with override = %+v", updated.Preferences)
	}

	cleared, err := bridgeSvc.service.UpdateSettings(context.Background(), bridgeAuthUser(user), UpdateUserSettingsInput{
		ClearBridgePortOverride: true,
	})
	if err != nil {
		t.Fatalf("UpdateSettings clear override: %v", err)
	}
	if cleared.Preferences.GlobalBridgePort != 1555 || cleared.Preferences.BridgePort != 1555 || cleared.Preferences.BridgePortOverride != nil {
		t.Fatalf("preferences after clearing override = %+v", cleared.Preferences)
	}
}

func TestBridgeServiceSecretLifecycleHashesAndShowsRawOnlyForMutations(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)

	generated, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(user))
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if generated.Secret == "" || !generated.ShownOnce {
		t.Fatalf("generated secret result = %+v", generated)
	}
	active, err := bridgeSvc.bridgeRepo.GetActiveBridgeCredentialForUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetActiveBridgeCredentialForUser: %v", err)
	}
	if active.SecretHash == generated.Secret || strings.Contains(active.SecretHash, generated.Secret) {
		t.Fatal("stored credential exposes raw bridge secret")
	}
	if !security.VerifyBridgeSecret(generated.Secret, active.SecretHash) {
		t.Fatal("stored hash does not verify generated secret")
	}
	settings, err := bridgeSvc.service.GetSettings(context.Background(), bridgeAuthUser(user))
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if settings.Bridge.Credential == nil || settings.Bridge.Credential.ID != generated.Credential.ID {
		t.Fatalf("settings bridge metadata = %+v", settings.Bridge)
	}
	if fmt.Sprintf("%+v", settings.Bridge) == generated.Secret {
		t.Fatal("settings response exposed the raw secret")
	}

	rotated, err := bridgeSvc.service.RotateSecret(context.Background(), bridgeAuthUser(user), "lost")
	if err != nil {
		t.Fatalf("RotateSecret: %v", err)
	}
	if rotated.Secret == "" || rotated.Secret == generated.Secret {
		t.Fatalf("rotated secret result = %+v", rotated)
	}
	if _, err := bridgeSvc.service.VerifyUserBridgeSecret(context.Background(), generated.Secret); !errors.Is(err, ErrBridgeSecretInvalid) {
		t.Fatalf("old secret verify error = %v, want ErrBridgeSecretInvalid", err)
	}
	if _, err := bridgeSvc.service.VerifyUserBridgeSecret(context.Background(), rotated.Secret); err != nil {
		t.Fatalf("rotated secret should verify: %v", err)
	}

	if _, err := bridgeSvc.service.RevokeSecret(context.Background(), bridgeAuthUser(user), "disabled"); err != nil {
		t.Fatalf("RevokeSecret: %v", err)
	}
	if _, err := bridgeSvc.service.VerifyUserBridgeSecret(context.Background(), rotated.Secret); !errors.Is(err, ErrBridgeSecretInvalid) {
		t.Fatalf("revoked secret verify error = %v, want ErrBridgeSecretInvalid", err)
	}
	for _, action := range []string{"bridge.secret_generated", "bridge.secret_rotated", "bridge.secret_revoked"} {
		if !bridgeSvc.auditActionExists(action) {
			t.Fatalf("missing audit action %s", action)
		}
	}
}

func TestBridgeServiceGenerateSecretDoesNotPersistCredentialWhenAuditFails(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)
	h.store.auditErr = errors.New("audit unavailable")

	_, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(user))
	if err == nil {
		t.Fatal("GenerateSecret succeeded with failing audit log")
	}
	if _, err := bridgeSvc.bridgeRepo.GetActiveBridgeCredentialForUser(context.Background(), user.ID); !errors.Is(err, domain.ErrBridgeCredentialNotFound) {
		t.Fatalf("active credential after failed audit = %v, want ErrBridgeCredentialNotFound", err)
	}
}

func TestBridgeServiceRotateSecretKeepsOldCredentialWhenCreateFails(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)

	generated, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(user))
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	bridgeSvc.bridgeRepo.createCredentialErr = errors.New("create unavailable")

	if _, err := bridgeSvc.service.RotateSecret(context.Background(), bridgeAuthUser(user), "lost"); err == nil {
		t.Fatal("RotateSecret succeeded with failing credential creation")
	}
	if _, err := bridgeSvc.service.VerifyUserBridgeSecret(context.Background(), generated.Secret); err != nil {
		t.Fatalf("old secret should still verify after failed rotate create: %v", err)
	}
}

func TestBridgeServiceRotateSecretKeepsOldCredentialWhenAuditFails(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)

	generated, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(user))
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	h.store.auditErr = errors.New("audit unavailable")

	if _, err := bridgeSvc.service.RotateSecret(context.Background(), bridgeAuthUser(user), "lost"); err == nil {
		t.Fatal("RotateSecret succeeded with failing audit log")
	}
	if _, err := bridgeSvc.service.VerifyUserBridgeSecret(context.Background(), generated.Secret); err != nil {
		t.Fatalf("old secret should still verify after failed rotate audit: %v", err)
	}
}

func TestBridgeServiceConnectorLaunchRequiresMatchingUserAndOneTimeToken(t *testing.T) {
	h := newAuthServiceHarness(t)
	alice := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
	bob := h.addUser(t, "bob", "bob@example.test", testAuthPassword, domain.UserStatusActive)
	bridgeSvc := newBridgeServiceTestHarness(t, h.store)
	deviceID := uuid.New()
	bridgeSvc.profileRepo.assignment = &postgres.WinboxAssignmentRow{
		ProfileID:       uuid.New(),
		Username:        "admin",
		EncryptedSecret: "encrypted",
	}
	bridgeSvc.credentials.ip = "192.168.88.1"
	bridgeSvc.credentials.password = "winbox-password"

	aliceSecret, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(alice))
	if err != nil {
		t.Fatalf("GenerateSecret alice: %v", err)
	}
	bobSecret, err := bridgeSvc.service.GenerateSecret(context.Background(), bridgeAuthUser(bob))
	if err != nil {
		t.Fatalf("GenerateSecret bob: %v", err)
	}
	launch, err := bridgeSvc.service.CreateLaunchRequest(context.Background(), bridgeAuthUser(alice), deviceID)
	if err != nil {
		t.Fatalf("CreateLaunchRequest: %v", err)
	}

	if _, err := bridgeSvc.service.ResolveConnectorLaunch(context.Background(), bobSecret.Secret, launch.LaunchToken, "192.0.2.10", "agent"); !errors.Is(err, ErrBridgeLaunchUserMismatch) {
		t.Fatalf("bob connector launch error = %v, want ErrBridgeLaunchUserMismatch", err)
	}
	creds, err := bridgeSvc.service.ResolveConnectorLaunch(context.Background(), aliceSecret.Secret, launch.LaunchToken, "192.0.2.10", "agent")
	if err != nil {
		t.Fatalf("ResolveConnectorLaunch: %v", err)
	}
	if creds.IP != "192.168.88.1" || creds.Username != "admin" || creds.Password != "winbox-password" {
		t.Fatalf("resolved credentials = %+v", creds)
	}
	if _, err := bridgeSvc.service.ResolveConnectorLaunch(context.Background(), aliceSecret.Secret, launch.LaunchToken, "192.0.2.10", "agent"); !errors.Is(err, ErrBridgeLaunchTokenUsed) {
		t.Fatalf("second connector launch error = %v, want ErrBridgeLaunchTokenUsed", err)
	}
	if !bridgeSvc.auditActionExists("bridge.auth_success") {
		t.Fatal("expected bridge.auth_success audit event")
	}
	if !bridgeSvc.auditActionExists("bridge.auth_failed") {
		t.Fatal("expected bridge.auth_failed audit event")
	}
}

type bridgeServiceTestHarness struct {
	service      *BridgeService
	bridgeRepo   *fakeBridgeRepo
	settingsRepo *fakeSettingsRepo
	audit        *fakeAuthStore
	profileRepo  *fakeWinboxAssignmentRepo
	credentials  *fakeWinboxCredentialProvider
}

func newBridgeServiceTestHarness(t *testing.T, authStore *fakeAuthStore) *bridgeServiceTestHarness {
	t.Helper()
	h := &bridgeServiceTestHarness{
		bridgeRepo:   newFakeBridgeRepo(),
		settingsRepo: newFakeSettingsRepo(map[string]string{domain.SettingBridgePort: "1337"}),
		audit:        authStore,
		profileRepo:  &fakeWinboxAssignmentRepo{},
		credentials:  &fakeWinboxCredentialProvider{},
	}
	svc, err := NewBridgeService(BridgeServiceConfig{
		BridgeRepo:               h.bridgeRepo,
		SettingsRepo:             h.settingsRepo,
		Users:                    authStore,
		AuditLogs:                authStore,
		WinboxCredentialProvider: h.credentials,
		CredentialProfileRepo:    h.profileRepo,
		SessionSecret:            []byte("bridge-service-test-session-secret"),
		Now: func() time.Time {
			return time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewBridgeService: %v", err)
	}
	h.service = svc
	h.bridgeRepo.auditLogs = authStore
	return h
}

func (h *bridgeServiceTestHarness) auditActionExists(action string) bool {
	for _, log := range h.audit.auditLogs() {
		if log.Action == action {
			return true
		}
	}
	return false
}

func bridgeAuthUser(user domain.User) *AuthenticatedUser {
	return &AuthenticatedUser{
		User: domain.UserWithRolesAndPermissions{User: user},
		Session: AuthenticatedSession{
			ID:     uuid.New(),
			UserID: user.ID,
		},
	}
}

func stringPtr(value string) *string { return &value }
func bridgeIntPtr(value int) *int    { return &value }

type fakeBridgeRepo struct {
	mu          sync.Mutex
	settings    map[uuid.UUID]domain.UserSettings
	credentials map[uuid.UUID]domain.BridgeCredential
	launches    map[string]domain.BridgeLaunchRequest
	downloads   []domain.BridgeConnectorDownload
	auditLogs   domain.AuditLogRepository

	createCredentialErr error
}

func newFakeBridgeRepo() *fakeBridgeRepo {
	return &fakeBridgeRepo{
		settings:    make(map[uuid.UUID]domain.UserSettings),
		credentials: make(map[uuid.UUID]domain.BridgeCredential),
		launches:    make(map[string]domain.BridgeLaunchRequest),
	}
}

func (r *fakeBridgeRepo) GetUserSettings(_ context.Context, userID uuid.UUID) (*domain.UserSettings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	settings, ok := r.settings[userID]
	if !ok {
		settings = domain.UserSettings{UserID: userID, Timezone: "UTC", Locale: "en-US"}
		r.settings[userID] = settings
	}
	return copyUserSettings(settings), nil
}

func (r *fakeBridgeRepo) UpsertUserSettings(_ context.Context, settings *domain.UserSettings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if settings.BridgePortOverride != nil && (*settings.BridgePortOverride < 1 || *settings.BridgePortOverride > 65535) {
		return fmt.Errorf("invalid bridge port")
	}
	r.settings[settings.UserID] = *settings
	return nil
}

func (r *fakeBridgeRepo) GetActiveBridgeCredentialForUser(_ context.Context, userID uuid.UUID) (*domain.BridgeCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, credential := range r.credentials {
		if credential.UserID == userID && credential.Status == domain.BridgeCredentialStatusActive {
			return copyBridgeCredential(credential), nil
		}
	}
	return nil, domain.ErrBridgeCredentialNotFound
}

func (r *fakeBridgeRepo) GetBridgeCredentialByPrefix(_ context.Context, prefix string) (*domain.BridgeCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, credential := range r.credentials {
		if credential.SecretPrefix == prefix {
			return copyBridgeCredential(credential), nil
		}
	}
	return nil, domain.ErrBridgeCredentialNotFound
}

func (r *fakeBridgeRepo) CreateBridgeCredential(_ context.Context, credential *domain.BridgeCredential) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.createCredentialErr != nil {
		return r.createCredentialErr
	}
	for _, existing := range r.credentials {
		if existing.UserID == credential.UserID && existing.Status == domain.BridgeCredentialStatusActive && credential.Status == domain.BridgeCredentialStatusActive {
			return fmt.Errorf("active credential exists")
		}
	}
	if credential.ID == uuid.Nil {
		credential.ID = uuid.New()
	}
	r.credentials[credential.ID] = *credential
	return nil
}

func (r *fakeBridgeRepo) CreateBridgeCredentialWithAudit(ctx context.Context, credential *domain.BridgeCredential, log *domain.AuditLog) error {
	r.mu.Lock()
	snapshot := r.copyCredentialsLocked()
	r.mu.Unlock()

	if err := r.CreateBridgeCredential(ctx, credential); err != nil {
		return err
	}
	if r.auditLogs != nil {
		if err := r.auditLogs.AppendAuditLog(ctx, log); err != nil {
			r.mu.Lock()
			r.credentials = snapshot
			r.mu.Unlock()
			return err
		}
	}
	return nil
}

func (r *fakeBridgeRepo) RotateBridgeCredentialWithAudit(ctx context.Context, userID uuid.UUID, credential *domain.BridgeCredential, when time.Time, reason string, log *domain.AuditLog) error {
	r.mu.Lock()
	snapshot := r.copyCredentialsLocked()
	for id, existing := range r.credentials {
		if existing.UserID == userID && existing.Status == domain.BridgeCredentialStatusActive {
			existing.Status = domain.BridgeCredentialStatusRevoked
			existing.RevokedAt = &when
			existing.RotationReason = reason
			r.credentials[id] = existing
			break
		}
	}
	r.mu.Unlock()

	if err := r.CreateBridgeCredential(ctx, credential); err != nil {
		r.mu.Lock()
		r.credentials = snapshot
		r.mu.Unlock()
		return err
	}
	if r.auditLogs != nil {
		if err := r.auditLogs.AppendAuditLog(ctx, log); err != nil {
			r.mu.Lock()
			r.credentials = snapshot
			r.mu.Unlock()
			return err
		}
	}
	return nil
}

func (r *fakeBridgeRepo) RevokeActiveBridgeCredentialForUser(_ context.Context, userID uuid.UUID, when time.Time, reason string) (*domain.BridgeCredential, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, credential := range r.credentials {
		if credential.UserID == userID && credential.Status == domain.BridgeCredentialStatusActive {
			credential.Status = domain.BridgeCredentialStatusRevoked
			credential.RevokedAt = &when
			credential.RotationReason = reason
			r.credentials[id] = credential
			return copyBridgeCredential(credential), nil
		}
	}
	return nil, domain.ErrBridgeCredentialNotFound
}

func (r *fakeBridgeRepo) TouchBridgeCredentialLastUsed(_ context.Context, credentialID uuid.UUID, when time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	credential, ok := r.credentials[credentialID]
	if !ok {
		return domain.ErrBridgeCredentialNotFound
	}
	credential.LastUsedAt = &when
	r.credentials[credentialID] = credential
	return nil
}

func (r *fakeBridgeRepo) CreateBridgeLaunchRequest(_ context.Context, request *domain.BridgeLaunchRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.ID == uuid.Nil {
		request.ID = uuid.New()
	}
	r.launches[request.TokenHash] = *request
	return nil
}

func (r *fakeBridgeRepo) GetBridgeLaunchRequestByTokenHash(_ context.Context, tokenHash string) (*domain.BridgeLaunchRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.launches[tokenHash]
	if !ok {
		return nil, domain.ErrBridgeLaunchRequestNotFound
	}
	return copyBridgeLaunchRequest(request), nil
}

func (r *fakeBridgeRepo) ConsumeBridgeLaunchRequest(_ context.Context, tokenHash string, credentialID uuid.UUID, when time.Time) (*domain.BridgeLaunchRequest, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	request, ok := r.launches[tokenHash]
	if !ok {
		return nil, domain.ErrBridgeLaunchRequestNotFound
	}
	if request.UsedAt != nil {
		return nil, domain.ErrBridgeLaunchRequestUsed
	}
	request.UsedAt = &when
	request.ConsumedByCredentialID = &credentialID
	r.launches[tokenHash] = request
	return copyBridgeLaunchRequest(request), nil
}

func (r *fakeBridgeRepo) RecordBridgeConnectorDownload(_ context.Context, download *domain.BridgeConnectorDownload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.downloads = append(r.downloads, *download)
	return nil
}

func (r *fakeBridgeRepo) copyCredentialsLocked() map[uuid.UUID]domain.BridgeCredential {
	copied := make(map[uuid.UUID]domain.BridgeCredential, len(r.credentials))
	for id, credential := range r.credentials {
		copied[id] = credential
	}
	return copied
}

func copyUserSettings(value domain.UserSettings) *domain.UserSettings {
	copied := value
	return &copied
}

func copyBridgeCredential(value domain.BridgeCredential) *domain.BridgeCredential {
	copied := value
	return &copied
}

func copyBridgeLaunchRequest(value domain.BridgeLaunchRequest) *domain.BridgeLaunchRequest {
	copied := value
	return &copied
}

type fakeSettingsRepo struct {
	mu     sync.Mutex
	values map[string]string
}

func newFakeSettingsRepo(values map[string]string) *fakeSettingsRepo {
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return &fakeSettingsRepo{values: copied}
}

func (r *fakeSettingsRepo) Get(key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	return value, nil
}

func (r *fakeSettingsRepo) Set(key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values[key] = value
	return nil
}

func (r *fakeSettingsRepo) GetAll() (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make(map[string]string, len(r.values))
	for key, value := range r.values {
		copied[key] = value
	}
	return copied, nil
}

type fakeWinboxAssignmentRepo struct {
	assignment *postgres.WinboxAssignmentRow
	err        error
}

func (r *fakeWinboxAssignmentRepo) GetWinboxAssignment(_ uuid.UUID) (*postgres.WinboxAssignmentRow, error) {
	if r.err != nil {
		return nil, r.err
	}
	if r.assignment == nil {
		return nil, fmt.Errorf("no WinBox profile designated")
	}
	return r.assignment, nil
}

type fakeWinboxCredentialProvider struct {
	ip       string
	password string
	err      error
}

func (p *fakeWinboxCredentialProvider) GetWinboxCredentials(_ uuid.UUID, _ string, _ string) (string, string, error) {
	if p.err != nil {
		return "", "", p.err
	}
	return p.ip, p.password, nil
}
