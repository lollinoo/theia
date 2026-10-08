package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

// Maintenance owns offline mutations. StatePath and its operations directory must
// survive database replacement, separately from DataDir and restore staging.
type Maintenance struct {
	StatePath       string
	DataDir         string
	BackupDir       string
	DeviceBackupDir string
	KnownHostsPath  string
	DBDSN           string
	RestoreLimits   RestoreArchiveLimits
	ReleaseTag      string
	// AfterApply is a failure-injection seam for integration tests.
	AfterApply func() error
}

// MaintenanceOperation is safe to display. It contains no keys, passwords, DSNs,
// or operator recovery identities.
type MaintenanceOperation struct {
	ID                  string    `json:"id"`
	Action              string    `json:"action"`
	Phase               string    `json:"phase"`
	Attempt             int       `json:"attempt"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	SnapshotSHA256      string    `json:"snapshot_sha256,omitempty"`
	PreventiveBackupID  string    `json:"preventive_backup_id,omitempty"`
	SourceWasEmpty      bool      `json:"source_was_empty"`
	VerifiedInstanceID  string    `json:"verified_instance_id,omitempty"`
	VerifiedActiveKeyID string    `json:"verified_active_key_id,omitempty"`
	VerifiedReleaseTag  string    `json:"verified_release_tag,omitempty"`
	OriginalReleaseTag  string    `json:"original_release_tag,omitempty"`
	WritesReopened      bool      `json:"writes_reopened"`
	Error               string    `json:"error,omitempty"`
}

func (m *Maintenance) operationsDir() string { return m.StatePath + ".operations" }

func (m *Maintenance) restoreLimits() RestoreArchiveLimits {
	if m.RestoreLimits.MaxTotalBytes > 0 {
		return normalizeRestoreArchiveLimits(m.RestoreLimits)
	}
	return RestoreArchiveLimits{MaxCompressedBytes: 2 << 30, MaxTotalBytes: 2 << 30, MaxEntryBytes: 1 << 30, MaxFileEntries: 50000}
}
func (m *Maintenance) operationDir(op *MaintenanceOperation) string {
	return filepath.Join(m.operationsDir(), op.ID)
}

// Status reads the durable operation journal without connecting to PostgreSQL.
func (m *Maintenance) Status() (*MaintenanceOperation, error) {
	data, err := os.ReadFile(filepath.Join(m.operationsDir(), "current.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var op MaintenanceOperation
	if err := json.Unmarshal(data, &op); err != nil {
		return nil, fmt.Errorf("invalid maintenance journal")
	}
	if _, err := uuid.Parse(op.ID); err != nil {
		return nil, fmt.Errorf("invalid maintenance operation identifier")
	}
	return &op, nil
}

func terminalMaintenancePhase(phase string) bool {
	return phase == "completed" || phase == "rolled_back"
}

// RequireReady excludes unresolved mutations before a managed HTTP process starts.
func (m *Maintenance) RequireReady() error {
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op != nil && !terminalMaintenancePhase(op.Phase) {
		return fmt.Errorf("instance remains in maintenance; resume operation %s before HTTP startup", op.ID)
	}
	return nil
}

// RequireVerifiedState binds HTTP startup to the last verified keyring. Checking
// this receipt avoids repeating expensive credential decryption on every restart.
func (m *Maintenance) RequireVerifiedState(instanceID, activeKeyID string) error {
	if err := m.RequireReady(); err != nil {
		return err
	}
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op == nil || op.VerifiedInstanceID != instanceID || op.VerifiedActiveKeyID != activeKeyID || op.VerifiedReleaseTag != m.ReleaseTag {
		return fmt.Errorf("instance credentials require maintenance migrate verification before HTTP startup")
	}
	return nil
}

func (m *Maintenance) persist(op *MaintenanceOperation, phase string) error {
	op.Phase, op.UpdatedAt = phase, time.Now().UTC()
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	if err := instance.WritePrivateFile(filepath.Join(m.operationDir(op), "journal.json"), data); err != nil {
		return err
	}
	if err := instance.WritePrivateFile(filepath.Join(m.operationsDir(), "current.json"), data); err != nil {
		return err
	}
	public, err := json.Marshal(struct {
		Maintenance bool      `json:"maintenance"`
		Action      string    `json:"action"`
		Phase       string    `json:"phase"`
		UpdatedAt   time.Time `json:"updated_at"`
	}{!terminalMaintenancePhase(phase), op.Action, phase, op.UpdatedAt})
	if err != nil {
		return err
	}
	// Deployments may serve this folder while the backend is stopped. It contains
	// no recovery keys, snapshot hashes, connection strings, or operation paths.
	publicDir := filepath.Join(filepath.Dir(m.StatePath), "public")
	if err := instance.WritePrivateFile(filepath.Join(publicDir, "status.json"), public); err != nil {
		return err
	}
	marker := filepath.Join(publicDir, "maintenance")
	if !terminalMaintenancePhase(phase) {
		return instance.WritePrivateFile(marker, []byte("maintenance\n"))
	}
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Maintenance) openDB() (*sql.DB, error) {
	db, err := postgres.OpenPrimaryDB(m.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("invalid maintenance database connection")
	}
	return db, nil
}

func (m *Maintenance) backupService(db *sql.DB, state *instance.State) (*InstanceBackupService, error) {
	keys, err := state.Keyring()
	if err != nil {
		return nil, err
	}
	s := NewInstanceBackupService(db, postgres.NewInstanceBackupRepo(db), postgres.NewSettingsRepo(db), m.BackupDir, m.DeviceBackupDir, m.KnownHostsPath, m.DataDir, m.DBDSN, keys)
	s.SetManagedState(m.StatePath)
	if state.BackupDestination != nil {
		destination, err := instance.NewS3Destination(*state.BackupDestination)
		if err != nil {
			return nil, err
		}
		s.SetBackupDestination(destination)
	}
	return s, nil
}

// Backup uses the same verified archive engine as the UI. The administration
// adapter stops the server first when running this offline command.
func (m *Maintenance) Backup(ctx context.Context) (*domain.InstanceBackup, error) {
	if err := m.validatePaths(); err != nil {
		return nil, err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := m.RequireReady(); err != nil {
		return nil, err
	}
	state, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		return nil, err
	}
	db, err := m.openDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	s, err := m.backupService(db, state)
	if err != nil {
		return nil, err
	}
	s.FailStaleRunning()
	if err := s.RetryPendingUploads(ctx); err != nil {
		return nil, err
	}
	return s.Create(ctx)
}

// Migrate runs SQL and credential migrations with automatic rollback while writes
// are stopped. Due credential keys rotate here, never at HTTP startup.
func (m *Maintenance) Migrate(ctx context.Context, forceRotation bool) error {
	if err := m.validatePaths(); err != nil {
		return err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	state, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		release()
		return err
	}
	if !forceRotation && time.Since(state.CredentialKeys[state.ActiveKeyID].CreatedAt) < instance.CredentialRotationInterval && m.RequireVerifiedState(state.InstanceID, state.ActiveKeyID) == nil {
		db, err := m.openDB()
		if err == nil {
			err = postgres.RequireCurrentSchema(ctx, db)
			if err == nil {
				err = VerifyInstanceIdentity(ctx, db, state.InstanceID)
			}
			db.Close()
			if err == nil {
				op, statusErr := m.Status()
				if statusErr == nil {
					statusErr = m.persist(op, op.Phase)
				}
				release()
				return statusErr
			}
		}
	}
	release()
	return m.mutate(ctx, "migrate", func(ctx context.Context, db *sql.DB, state *instance.State) error {
		if err := (instance.Store{Path: m.StatePath}).Update(func(s *instance.State) error { _, err := s.RotateCredentials(time.Now(), forceRotation); return err }); err != nil {
			return err
		}
		rotated, err := (instance.Store{Path: m.StatePath}).Load()
		if err != nil {
			return err
		}
		keys, err := rotated.Keyring()
		if err != nil {
			return err
		}
		return postgres.RunMigrations(db, keys)
	})
}

func (m *Maintenance) mutate(ctx context.Context, action string, apply func(context.Context, *sql.DB, *instance.State) error) error {
	if err := m.validatePaths(); err != nil {
		return err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	if err := m.RequireReady(); err != nil {
		return err
	}
	state, err := (instance.Store{Path: m.StatePath}).Load()
	if err != nil {
		return err
	}
	db, err := m.openDB()
	if err != nil {
		return err
	}
	defer db.Close()
	op := &MaintenanceOperation{ID: uuid.NewString(), Action: action, Attempt: 1, CreatedAt: time.Now().UTC()}
	previous, err := m.Status()
	if err != nil {
		return err
	}
	if previous != nil {
		op.OriginalReleaseTag = previous.VerifiedReleaseTag
	}
	if err := m.persist(op, "preparing"); err != nil {
		return err
	}
	if err := m.prepareSafetySnapshot(ctx, db, state, op, previous); err != nil {
		// Preparation has not changed live data. No rollback is needed.
		op.Error = "maintenance preparation failed; live data was not changed"
		if previous != nil {
			op.VerifiedInstanceID, op.VerifiedActiveKeyID, op.VerifiedReleaseTag = previous.VerifiedInstanceID, previous.VerifiedActiveKeyID, previous.VerifiedReleaseTag
		}
		_ = m.persist(op, "rolled_back")
		return err
	}
	if err := m.persist(op, "applying"); err != nil {
		return err
	}
	applyErr := apply(ctx, db, state)
	if applyErr == nil && m.AfterApply != nil {
		applyErr = m.AfterApply()
	}
	if applyErr == nil {
		applyErr = m.persist(op, "verifying")
	}
	if applyErr == nil {
		verificationDB, err := m.openDB()
		if err != nil {
			applyErr = err
		} else {
			defer verificationDB.Close()
			current, err := (instance.Store{Path: m.StatePath}).Load()
			if err != nil {
				applyErr = err
			} else {
				keys, err := current.Keyring()
				if err == nil {
					err = postgres.RequireCurrentSchema(ctx, verificationDB)
				}
				if err == nil {
					err = VerifyStoredCredentials(ctx, verificationDB, keys, true)
				}
				if err == nil {
					err = postgres.NewSettingsRepo(verificationDB).Set(managedInstanceIdentitySetting, current.InstanceID)
				}
				applyErr = err
				if err == nil {
					op.VerifiedInstanceID, op.VerifiedActiveKeyID = current.InstanceID, current.ActiveKeyID
					op.VerifiedReleaseTag = m.ReleaseTag
				}
			}
		}
	}
	if applyErr != nil {
		op.Error = "maintenance failed; recovering the verified pre-operation state"
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := m.rollback(rollbackCtx, op); err != nil {
			return errors.Join(applyErr, fmt.Errorf("automatic rollback incomplete; instance remains in maintenance: %w", err))
		}
		return fmt.Errorf("maintenance failed and original state was restored: %w", applyErr)
	}
	if op.SourceWasEmpty && action == "migrate" {
		settings := postgres.NewSettingsRepo(db)
		if err := settings.Set(domain.SettingInstanceBackupIntervalHours, "24"); err != nil {
			return err
		}
		if err := settings.Set(domain.SettingInstanceBackupRetentionCount, "7"); err != nil {
			return err
		}
	}
	return m.persist(op, "completed")
}
