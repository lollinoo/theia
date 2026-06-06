package service

// This file exercises backup executor behavior so refactors preserve the documented contract.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type shortWriterForTest struct {
	limit int
}

func (w *shortWriterForTest) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return w.limit, errors.New("short write")
	}
	return len(p), nil
}

func TestCountingWriterCountsBytesAcceptedByWrappedWriter(t *testing.T) {
	var buf bytes.Buffer
	counter := &countingWriter{w: &buf}

	n, err := counter.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != 5 || counter.n != 5 || buf.String() != "hello" {
		t.Fatalf("Write result n=%d counter=%d buf=%q, want 5/5/hello", n, counter.n, buf.String())
	}
}

func TestCountingWriterCountsPartialWrites(t *testing.T) {
	counter := &countingWriter{w: &shortWriterForTest{limit: 2}}

	n, err := counter.Write([]byte("hello"))
	if err == nil {
		t.Fatal("Write error = nil, want wrapped writer error")
	}
	if n != 2 || counter.n != 2 {
		t.Fatalf("Write result n=%d counter=%d, want 2/2", n, counter.n)
	}
}

func TestCreateBackupFileOrRemoveLocalCleansUpMetadataFailure(t *testing.T) {
	fileRepo := newMockBackupFileRepo()
	fileRepo.createErr = errors.New("metadata insert failed")
	backupDir := t.TempDir()
	filePath := filepath.Join(backupDir, "untracked.rsc")
	if err := os.WriteFile(filePath, []byte("backup"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	svc := &BackupService{fileRepo: fileRepo}

	err := svc.createBackupFileOrRemoveLocal(&domain.BackupFile{
		ID:        uuid.New(),
		JobID:     uuid.New(),
		FileType:  "running",
		FileName:  "untracked.rsc",
		FilePath:  filePath,
		FileHash:  "hash",
		SizeBytes: 6,
	})

	if err == nil {
		t.Fatal("createBackupFileOrRemoveLocal error = nil, want metadata failure")
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Fatalf("untracked file still exists, stat err = %v", statErr)
	}
}
