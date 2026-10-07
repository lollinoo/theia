package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type cleanupFileRepo struct {
	*mockBackupFileRepo
	pending     []domain.BackupFile
	references  map[string]bool
	deferred    []uuid.UUID
	completeErr error
}

func (r *cleanupFileRepo) ListPendingFileDeletions(ctx context.Context, limit int) ([]domain.BackupFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return slices.Clone(r.pending[:min(limit, len(r.pending))]), nil
}

func (r *cleanupFileRepo) BackupFilePathsReferenced(ctx context.Context, paths []string) (bool, error) {
	for _, path := range paths {
		if r.references[path] {
			return true, nil
		}
	}
	return false, ctx.Err()
}

func (r *cleanupFileRepo) CompleteFileDeletion(ctx context.Context, id uuid.UUID) error {
	if r.completeErr != nil {
		return r.completeErr
	}
	r.pending = slices.DeleteFunc(r.pending, func(file domain.BackupFile) bool { return file.ID == id })
	return ctx.Err()
}

func (r *cleanupFileRepo) DeferFileDeletion(ctx context.Context, id uuid.UUID) error {
	r.deferred = append(r.deferred, id)
	return ctx.Err()
}

func TestCleanupDeletedBackupFiles(t *testing.T) {
	for _, scenario := range []string{"regular", "missing", "restored", "shared", "outside root", "symlink", "directory", "ack failure"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, uuid.NewString())
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "partial.rsc")
			file := domain.BackupFile{ID: uuid.New(), FileName: "partial.rsc", FilePath: path}
			if err := os.WriteFile(path, []byte("configuration"), 0600); err != nil {
				t.Fatal(err)
			}
			repo := &cleanupFileRepo{mockBackupFileRepo: newMockBackupFileRepo(), references: map[string]bool{}}
			switch scenario {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "restored":
				file.FilePath = filepath.Join(t.TempDir(), "old-backups", filepath.Base(dir), file.FileName)
			case "shared":
				repo.references[path] = true
			case "outside root":
				path = filepath.Join(t.TempDir(), file.FileName)
				if err := os.WriteFile(path, []byte("unrelated"), 0600); err != nil {
					t.Fatal(err)
				}
				file.FilePath = path
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(t.TempDir(), "target.rsc")
				if err := os.WriteFile(target, []byte("unrelated"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "ack failure":
				repo.completeErr = errors.New("database unavailable")
			}
			repo.pending = []domain.BackupFile{file}
			svc := NewBackupService(nil, repo, nil, nil, nil, nil, nil, nil, root, nil)
			count, err := svc.CleanupDeletedBackupFiles(context.Background())
			unsafe := scenario == "outside root" || scenario == "symlink" || scenario == "directory"
			if unsafe {
				var pathErr *BulkPathError
				if !errors.As(err, &pathErr) || count != 0 || len(repo.pending) != 1 || len(repo.deferred) != 1 {
					t.Fatalf("unsafe path lost: count=%d pending=%v error=%v", count, repo.pending, err)
				}
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf("unsafe path was removed: %v", err)
				}
				return
			}
			if scenario == "shared" {
				if err != nil || count != 0 || len(repo.pending) != 1 || len(repo.deferred) != 1 {
					t.Fatalf("shared path lost: count=%d pending=%v error=%v", count, repo.pending, err)
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("shared file was removed: %v", err)
				}
				return
			}
			if scenario == "ack failure" {
				if !errors.Is(err, repo.completeErr) || count != 0 || len(repo.pending) != 1 {
					t.Fatalf("ack failure lost retry state: count=%d pending=%v error=%v", count, repo.pending, err)
				}
				repo.completeErr = nil
				count, err = svc.CleanupDeletedBackupFiles(context.Background())
			}
			if err != nil || count != 1 || len(repo.pending) != 0 {
				t.Fatalf("count=%d pending=%v error=%v, want completed cleanup", count, repo.pending, err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("disk file still present: %v", err)
			}
		})
	}
}

func TestCleanupDeletedBackupFilesBoundsWorkAndHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	repo := &cleanupFileRepo{mockBackupFileRepo: newMockBackupFileRepo()}
	for range 101 {
		repo.pending = append(repo.pending, domain.BackupFile{ID: uuid.New(), FilePath: filepath.Join(root, uuid.NewString()+".rsc")})
	}
	svc := NewBackupService(nil, repo, nil, nil, nil, nil, nil, nil, root, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if count, err := svc.CleanupDeletedBackupFiles(ctx); count != 0 || !errors.Is(err, context.Canceled) || len(repo.pending) != 101 {
		t.Fatalf("cancelled cleanup: count=%d pending=%d error=%v", count, len(repo.pending), err)
	}
	if count, err := svc.CleanupDeletedBackupFiles(context.Background()); err != nil || count != 100 || len(repo.pending) != 1 {
		t.Fatalf("unbounded cleanup: count=%d pending=%d error=%v", count, len(repo.pending), err)
	}
}

func TestCleanupDeletedBackupFilesRetriesDiskFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires Unix directory permission enforcement")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "device")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "partial.rsc")
	if err := os.WriteFile(path, []byte("configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := &cleanupFileRepo{mockBackupFileRepo: newMockBackupFileRepo(), pending: []domain.BackupFile{{ID: uuid.New(), FilePath: path}}}
	svc := NewBackupService(nil, repo, nil, nil, nil, nil, nil, nil, root, nil)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if count, err := svc.CleanupDeletedBackupFiles(context.Background()); count != 0 || !errors.Is(err, os.ErrPermission) || len(repo.pending) != 1 || len(repo.deferred) != 1 {
		t.Fatalf("disk failure lost cleanup path: count=%d pending=%v error=%v", count, repo.pending, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if count, err := svc.CleanupDeletedBackupFiles(context.Background()); err != nil || count != 1 || len(repo.pending) != 0 {
		t.Fatalf("disk cleanup retry failed: count=%d pending=%v error=%v", count, repo.pending, err)
	}
}

func TestCleanupDeletedBackupFilesWaitsForDeviceExecutor(t *testing.T) {
	root := t.TempDir()
	deviceID := uuid.New()
	dir := filepath.Join(root, deviceID.String())
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "partial.rsc")
	if err := os.WriteFile(path, []byte("configuration"), 0600); err != nil {
		t.Fatal(err)
	}
	repo := &cleanupFileRepo{mockBackupFileRepo: newMockBackupFileRepo(), pending: []domain.BackupFile{{ID: uuid.New(), FilePath: path}}}
	svc := NewBackupService(nil, repo, nil, nil, nil, nil, nil, nil, root, nil)
	release, err := svc.lockBackupDevice(context.Background(), deviceID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	count, err := svc.CleanupDeletedBackupFiles(ctx)
	release()
	if count != 0 || !errors.Is(err, context.DeadlineExceeded) || len(repo.pending) != 1 {
		t.Fatalf("cleanup bypassed the executor: count=%d pending=%v error=%v", count, repo.pending, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active executor's file was removed: %v", err)
	}
	if count, err := svc.CleanupDeletedBackupFiles(context.Background()); err != nil || count != 1 || len(svc.deviceLocks) != 0 {
		t.Fatalf("cleanup did not recover after executor release: count=%d error=%v", count, err)
	}
}
