package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type unavailableBackupDestination struct{ uploads int }

func (d *unavailableBackupDestination) PutVerified(context.Context, string, string, string) error {
	d.uploads++
	return fmt.Errorf("external storage unavailable")
}
func (d *unavailableBackupDestination) Delete(context.Context, string) error { return nil }

func TestManagedInterruptedBackupRequiresCompleteVerificationReceipt(t *testing.T) {
	repo := newInstanceBackupCancelTestRepo()
	root := t.TempDir()
	backup := &domain.InstanceBackup{ID: uuid.New(), FileName: "incomplete.age", Status: domain.InstanceBackupStatusRunning}
	backup.FilePath = filepath.Join(root, backup.ID.String(), backup.FileName)
	if err := os.MkdirAll(filepath.Dir(backup.FilePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backup.FilePath, []byte("interrupted-but-nonempty-archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(backup); err != nil {
		t.Fatal(err)
	}
	svc := NewInstanceBackupService(nil, repo, nil, root, "", "", "", "", nil)
	svc.SetManagedState("managed-state")
	svc.FailStaleRunning()
	got, _ := repo.GetByID(backup.ID)
	if got.Status != domain.InstanceBackupStatusFailed {
		t.Fatal("interrupted unverified archive marked successful")
	}
}

func TestPendingExternalCopySurvivesRestartRetryAndRetention(t *testing.T) {
	repo := newInstanceBackupCancelTestRepo()
	root := t.TempDir()
	backup := &domain.InstanceBackup{ID: uuid.New(), FileName: "verified.age", Status: domain.InstanceBackupStatusRunning, CreatedAt: time.Now().Add(-30 * 24 * time.Hour)}
	backup.FilePath = filepath.Join(root, backup.ID.String(), backup.FileName)
	if err := os.MkdirAll(filepath.Dir(backup.FilePath), 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte("already-verified-archive")
	if err := os.WriteFile(backup.FilePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	backup.SizeBytes = int64(len(data))
	backup.SHA256, _ = computeFileHashContext(context.Background(), backup.FilePath)
	if err := writeBackupVerificationReceipt(backup); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(backup); err != nil {
		t.Fatal(err)
	}
	svc := NewInstanceBackupService(nil, repo, nil, root, "", "", "", "", nil)
	svc.SetManagedState("managed-state")
	destination := &unavailableBackupDestination{}
	svc.SetBackupDestination(destination)
	svc.FailStaleRunning()
	got, _ := repo.GetByID(backup.ID)
	if got.Status != domain.InstanceBackupStatusPendingUpload {
		t.Fatal("external copy was falsely marked successful")
	}
	if err := svc.RetryPendingUploads(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CleanupRetention(context.Background(), 1, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup.FilePath); err != nil {
		t.Fatal("verified local copy removed")
	}
	if err := os.WriteFile(backup.FilePath, []byte("changed-after-verification"), 0600); err != nil {
		t.Fatal(err)
	}
	before := destination.uploads
	if err := svc.RetryPendingUploads(context.Background()); err != nil {
		t.Fatal(err)
	}
	if destination.uploads != before {
		t.Fatal("changed bytes uploaded using an old verification receipt")
	}
}
