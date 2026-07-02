package service

// This file defines backup service backup and restore service behavior, including filesystem safety and cleanup expectations.

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	gossh "golang.org/x/crypto/ssh"

	"github.com/lollinoo/theia/internal/crypto"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/ssh"
	"github.com/lollinoo/theia/internal/vendor"
)

var sanitizeRe = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

const defaultBulkBackupWorkerCount = 4
const defaultBulkBackupRunBatchSize = 10

// ErrBulkBackupRunAlreadyActive prevents more than one durable bulk run from mutating backup state.
var ErrBulkBackupRunAlreadyActive = errors.New("bulk backup run already active")

// ErrBackupJobActive prevents deletion while a backup executor can still write artifacts.
var ErrBackupJobActive = errors.New("backup job is active")

// ErrBackupJobReferencedByActiveBulkRun prevents deletion while a bulk-run item still owns the job.
var ErrBackupJobReferencedByActiveBulkRun = errors.New("backup job is referenced by an active bulk backup run")

// ErrSSHHostKeyStoreUnavailable means this runtime cannot mutate the SSH known_hosts store.
var ErrSSHHostKeyStoreUnavailable = errors.New("ssh host key store unavailable")

// SSHHostKeyStore resets remembered SSH host-key trust for one target.
type SSHHostKeyStore interface {
	RemoveHost(host string, port int) (bool, error)
}

// SSHHostKeyResetResult describes a device host-key reset.
type SSHHostKeyResetResult struct {
	Target  string
	Port    int
	Removed bool
}

// BackupService orchestrates credential profile management and config backups.
type BackupService struct {
	jobRepo               domain.BackupJobRepository
	fileRepo              domain.BackupFileRepository
	credentialProfileRepo domain.CredentialProfileRepository
	deviceRepo            domain.DeviceRepository
	settingsRepo          domain.SettingsRepository
	vendorRegistry        *vendor.Registry
	sshDialer             ssh.Dialer
	encryptionKeyring     *crypto.Keyring
	legacyEncryptionKey   []byte
	backupDir             string
	hostKeyCallback       gossh.HostKeyCallback
	hostKeyStore          SSHHostKeyStore
	deviceLocks           sync.Map // per-device mutex: map[uuid.UUID]*sync.Mutex
	bulkLimits            BulkOperationLimits
	bulkRunRepo           domain.BulkBackupRunRepository
}

// BackupServiceOption wires optional collaborators without changing the public constructor signature.
type BackupServiceOption func(*BackupService)

// WithBulkBackupRunRepo enables durable pause/resume/cancel state for multi-device backup runs.
func WithBulkBackupRunRepo(repo domain.BulkBackupRunRepository) BackupServiceOption {
	return func(s *BackupService) {
		s.bulkRunRepo = repo
	}
}

// WithSSHHostKeyStore enables explicit operator resets for remembered SSH host keys.
func WithSSHHostKeyStore(store SSHHostKeyStore) BackupServiceOption {
	return func(s *BackupService) {
		s.hostKeyStore = store
	}
}

// NewBackupService creates a backup service for device credential profiles and config archives.
// The service keeps per-device locks in memory, so callers should share one instance per process.
func NewBackupService(
	jobRepo domain.BackupJobRepository,
	fileRepo domain.BackupFileRepository,
	credentialProfileRepo domain.CredentialProfileRepository,
	deviceRepo domain.DeviceRepository,
	settingsRepo domain.SettingsRepository,
	vendorRegistry *vendor.Registry,
	sshDialer ssh.Dialer,
	encryptionKey any,
	backupDir string,
	hostKeyCallback gossh.HostKeyCallback,
	opts ...BackupServiceOption,
) *BackupService {
	keyring, legacyKey := normalizeBackupEncryptionKey(encryptionKey)
	svc := &BackupService{
		jobRepo:               jobRepo,
		fileRepo:              fileRepo,
		credentialProfileRepo: credentialProfileRepo,
		deviceRepo:            deviceRepo,
		settingsRepo:          settingsRepo,
		vendorRegistry:        vendorRegistry,
		sshDialer:             sshDialer,
		encryptionKeyring:     keyring,
		legacyEncryptionKey:   legacyKey,
		backupDir:             backupDir,
		hostKeyCallback:       hostKeyCallback,
		bulkLimits:            DefaultBulkOperationLimits,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

func normalizeBackupEncryptionKey(key any) (*crypto.Keyring, []byte) {
	switch k := key.(type) {
	case *crypto.Keyring:
		return k, nil
	case []byte:
		keyring, err := crypto.NewKeyringFromLegacyKey(k)
		if err != nil {
			return nil, k
		}
		return keyring, k
	case nil:
		return nil, nil
	default:
		return nil, nil
	}
}

// SetBulkOperationLimits overrides bulk request quotas for tests or deployments with stricter ceilings.
func (s *BackupService) SetBulkOperationLimits(limits BulkOperationLimits) {
	s.bulkLimits = normalizeBulkOperationLimits(limits)
}

// BulkOperationLimits returns the effective bulk request quotas.
func (s *BackupService) BulkOperationLimits() BulkOperationLimits {
	return normalizeBulkOperationLimits(s.bulkLimits)
}

// BulkBackupRunRepositoryConfigured reports whether durable run orchestration is available.
func (s *BackupService) BulkBackupRunRepositoryConfigured() bool {
	return s != nil && s.bulkRunRepo != nil
}

// BulkBackupRunBatchSize returns the fixed device batch size used by durable bulk backup runs.
func (s *BackupService) BulkBackupRunBatchSize() int {
	return defaultBulkBackupRunBatchSize
}

// getDeviceLock returns or creates a per-device mutex.
func (s *BackupService) getDeviceLock(deviceID uuid.UUID) *sync.Mutex {
	val, _ := s.deviceLocks.LoadOrStore(deviceID, &sync.Mutex{})
	return val.(*sync.Mutex)
}

// nowInConfiguredTZ returns the current time in the timezone configured in settings.
// Falls back to UTC if the setting is missing or invalid.
func (s *BackupService) nowInConfiguredTZ() time.Time {
	tzName, err := s.settingsRepo.Get(domain.SettingTimezone)
	if err != nil || tzName == "" {
		return time.Now().UTC()
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Now().In(loc)
}

type queuedDeviceBackup struct {
	device    domain.Device
	profile   *domain.CredentialProfile
	backupCfg vendor.BackupConfig
	jobID     uuid.UUID
}

func (s *BackupService) startBulkBackupWorkers(queuedBackups []queuedDeviceBackup, onComplete ...func()) {
	runCompleteCallbacks := func() {
		for _, callback := range onComplete {
			if callback != nil {
				callback()
			}
		}
	}
	if len(queuedBackups) == 0 {
		runCompleteCallbacks()
		return
	}

	workerCount := defaultBulkBackupWorkerCount
	if len(queuedBackups) < workerCount {
		workerCount = len(queuedBackups)
	}

	go func() {
		defer runCompleteCallbacks()
		jobs := make(chan queuedDeviceBackup)
		var wg sync.WaitGroup
		wg.Add(workerCount)
		for i := 0; i < workerCount; i++ {
			go func() {
				defer wg.Done()
				for job := range jobs {
					device := job.device
					s.runFullBackup(&device, job.profile, job.backupCfg, job.jobID)
				}
			}()
		}
		for _, job := range queuedBackups {
			jobs <- job
		}
		close(jobs)
		wg.Wait()
	}()
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func sanitizeHostname(name string) string {
	s := sanitizeRe.ReplaceAllString(name, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

func (s *BackupService) decryptSecret(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	if s.encryptionKeyring != nil {
		if crypto.IsEnvelope(encrypted) {
			return s.encryptionKeyring.DecryptString(encrypted)
		}
		return "", fmt.Errorf("credential secret is not a versioned encryption envelope")
	}

	var base64DecryptErr error
	if ciphertext, err := base64.StdEncoding.DecodeString(encrypted); err == nil {
		decrypted, err := crypto.Decrypt(ciphertext, s.legacyEncryptionKey)
		if err == nil {
			return string(decrypted), nil
		}
		base64DecryptErr = err
	}

	decrypted, err := crypto.Decrypt([]byte(encrypted), s.legacyEncryptionKey)
	if err == nil {
		return string(decrypted), nil
	}
	if base64DecryptErr != nil {
		return "", base64DecryptErr
	}
	return "", err
}

// EncryptSecret encrypts a plaintext secret for storage.
func (s *BackupService) EncryptSecret(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if s.encryptionKeyring != nil {
		return s.encryptionKeyring.EncryptString(plaintext)
	}
	return "", fmt.Errorf("encryption keyring is required")
}

// GetWinboxCredentials retrieves the decrypted WinBox password for a device.
// It fetches the device IP from the device repository and decrypts the
// credential profile's secret in the service layer (T-24-05 mitigation).
// Returns ip, decryptedPassword, and an error. username is returned separately.
func (s *BackupService) GetWinboxCredentials(deviceID uuid.UUID, encryptedSecret, username string) (ip, password string, err error) {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return "", "", fmt.Errorf("device not found: %w", err)
	}
	pwd, err := s.decryptSecret(encryptedSecret)
	if err != nil {
		return "", "", fmt.Errorf("decrypting credentials: %w", err)
	}
	if pwd == "" {
		return "", "", fmt.Errorf("WinBox profile has no password configured")
	}
	return device.IP, pwd, nil
}

// TestSSHConnection tests SSH connectivity to a device using its assigned SSH profile.
func (s *BackupService) TestSSHConnection(ctx context.Context, deviceID uuid.UUID) error {
	device, err := s.deviceRepo.GetByID(deviceID)
	if err != nil {
		return fmt.Errorf("getting device: %w", err)
	}

	profile, err := s.credentialProfileRepo.GetBackupProfileForDevice(device.ID)
	if err != nil {
		return fmt.Errorf("no credential profile assigned to device %s", deviceID)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	target := domain.BackupAddress(*device)
	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, target, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, target, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection to %s failed: %w", target, err)
	}
	defer client.Close()

	return nil
}

// TestCredentialProfile tests SSH connectivity using a credential profile against a target IP.
func (s *BackupService) TestCredentialProfile(ctx context.Context, profileID uuid.UUID, targetIP string) error {
	profile, err := s.credentialProfileRepo.GetByID(profileID)
	if err != nil {
		return fmt.Errorf("getting credential profile: %w", err)
	}

	secret, err := s.decryptSecret(profile.EncryptedSecret)
	if err != nil {
		return fmt.Errorf("decrypting credentials: %w", err)
	}

	timeout := 10 * time.Second
	var client *ssh.Client

	if profile.AuthMethod == domain.SSHAuthPassword {
		client, err = ssh.NewClient(s.sshDialer, targetIP, profile.Port, profile.Username, secret, timeout, s.hostKeyCallback)
	} else {
		client, err = ssh.NewClientWithKey(s.sshDialer, targetIP, profile.Port, profile.Username, []byte(secret), timeout, s.hostKeyCallback)
	}
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}
	defer client.Close()

	return nil
}

// CreateCredentialProfile creates a new credential profile.
// If role is empty, it defaults to "Admin".
func (s *BackupService) CreateCredentialProfile(ctx context.Context, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	encryptedSecret, err := s.EncryptSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("encrypting secret: %w", err)
	}

	if role == "" {
		role = "Admin"
	}

	profile := &domain.CredentialProfile{
		Name:            name,
		Description:     description,
		Username:        username,
		Port:            port,
		AuthMethod:      authMethod,
		EncryptedSecret: encryptedSecret,
		Role:            role,
	}
	if err := s.credentialProfileRepo.Create(profile); err != nil {
		return nil, fmt.Errorf("creating credential profile: %w", err)
	}
	return profile, nil
}

// GetCredentialProfile returns a credential profile by ID.
func (s *BackupService) GetCredentialProfile(ctx context.Context, id uuid.UUID) (*domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetByID(id)
}

// GetAllCredentialProfiles returns all credential profiles.
func (s *BackupService) GetAllCredentialProfiles(ctx context.Context) ([]domain.CredentialProfile, error) {
	return s.credentialProfileRepo.GetAll()
}

// UpdateCredentialProfile updates an existing credential profile. If secret is empty, the existing secret is kept.
// If role is empty, it defaults to "Admin".
func (s *BackupService) UpdateCredentialProfile(ctx context.Context, id uuid.UUID, name, description, username string, port int, authMethod domain.SSHAuthMethod, secret string, role string) (*domain.CredentialProfile, error) {
	profile, err := s.credentialProfileRepo.GetByID(id)
	if err != nil {
		return nil, fmt.Errorf("getting credential profile: %w", err)
	}

	profile.Name = name
	profile.Description = description
	profile.Username = username
	profile.Port = port
	profile.AuthMethod = authMethod

	if role == "" {
		role = "Admin"
	}
	profile.Role = role

	if secret != "" {
		encryptedSecret, err := s.EncryptSecret(secret)
		if err != nil {
			return nil, fmt.Errorf("encrypting secret: %w", err)
		}
		profile.EncryptedSecret = encryptedSecret
	}

	if err := s.credentialProfileRepo.Update(profile); err != nil {
		return nil, fmt.Errorf("updating credential profile: %w", err)
	}
	return profile, nil
}

// DeleteCredentialProfile removes a credential profile. Returns an error if any device references it.
func (s *BackupService) DeleteCredentialProfile(ctx context.Context, id uuid.UUID) error {
	return s.credentialProfileRepo.Delete(id)
}
