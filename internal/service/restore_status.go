package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

const restoreStatusFileName = ".theia-restore-status.json"

type restoreOperationPhase string

type RestoreOperationPhase = restoreOperationPhase

const (
	restorePhaseValidationPassed             restoreOperationPhase = "validation_passed"
	restorePhaseStagedRestartPending         restoreOperationPhase = "staged_restart_pending"
	restorePhaseStartupRestoreDetected       restoreOperationPhase = "startup_restore_detected"
	restorePhaseApplyingPostgres             restoreOperationPhase = "applying_postgres"
	restorePhasePostgresApplied              restoreOperationPhase = "postgres_applied"
	restorePhaseVerifyingKeyring             restoreOperationPhase = "verifying_keyring"
	restorePhaseRunningCredentialRewrap      restoreOperationPhase = "running_credential_rewrap"
	restorePhaseCompleted                    restoreOperationPhase = "completed"
	restorePhaseFailedRetryable              restoreOperationPhase = "failed_retryable"
	restorePhaseFailedOperatorActionRequired restoreOperationPhase = "failed_operator_action_required"
)

const (
	RestorePhaseVerifyingKeyring        RestoreOperationPhase = restorePhaseVerifyingKeyring
	RestorePhaseRunningCredentialRewrap RestoreOperationPhase = restorePhaseRunningCredentialRewrap
)

type RestoreOperationStatus struct {
	OperationID  string `json:"operation_id"`
	Phase        string `json:"phase"`
	AttemptCount int    `json:"attempt_count"`
	LastError    string `json:"last_error"`
	MissingKeyID string `json:"missing_key_id,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func restoreStatusFilePath(stateDir string) string {
	return filepath.Join(stateDir, restoreStatusFileName)
}

func restoreOperationStatusFromMarker(marker restoreMarker) RestoreOperationStatus {
	return RestoreOperationStatus{
		OperationID:  marker.OperationID,
		Phase:        marker.Phase,
		AttemptCount: marker.AttemptCount,
		LastError:    marker.LastError,
		MissingKeyID: marker.MissingKeyID,
		CreatedAt:    marker.CreatedAt,
		UpdatedAt:    marker.UpdatedAt,
	}
}

func updateRestoreOperationFields(marker *restoreMarker, phase restoreOperationPhase, lastError, missingKeyID string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if marker.OperationID == "" {
		marker.OperationID = uuid.NewString()
	}
	if marker.CreatedAt == "" {
		if marker.Timestamp != "" {
			marker.CreatedAt = marker.Timestamp
		} else {
			marker.CreatedAt = now
		}
	}
	marker.Phase = string(phase)
	marker.LastError = lastError
	marker.MissingKeyID = missingKeyID
	marker.UpdatedAt = now
}

func writeRestoreOperationStatus(stateDir string, status RestoreOperationStatus) error {
	if status.OperationID == "" {
		return fmt.Errorf("restore operation status missing operation_id")
	}
	if status.Phase == "" {
		return fmt.Errorf("restore operation status missing phase")
	}
	return writeRestoreJSONFileAtomic(restoreStatusFilePath(stateDir), status)
}

func readRestoreOperationStatus(stateDir string) (*RestoreOperationStatus, bool, error) {
	data, err := os.ReadFile(restoreStatusFilePath(stateDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read restore operation status: %w", err)
	}
	var status RestoreOperationStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, true, fmt.Errorf("parse restore operation status: %w", err)
	}
	return &status, true, nil
}

func writeRestoreJSONFileAtomic(path string, value any) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling restore JSON: %w", err)
	}
	body = append(body, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating restore state dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("creating restore temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("restricting restore temp file permissions: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing restore temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("syncing restore temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing restore temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("publishing restore JSON: %w", err)
	}
	cleanup = false
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("restricting restore JSON permissions: %w", err)
	}
	if err := syncRestoreDir(dir); err != nil {
		return err
	}
	return nil
}

func syncRestoreDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open restore state dir for sync: %w", err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync restore state dir: %w", err)
	}
	return nil
}
