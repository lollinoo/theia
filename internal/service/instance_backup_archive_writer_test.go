package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

type failingArchiveDestination struct {
	bytes.Buffer
	writeErr, closeErr error
	closed             bool
}

func (d *failingArchiveDestination) Write(p []byte) (int, error) {
	// Allow the gzip header; fail when compressed data is flushed during Close.
	if d.Len() > 0 && d.writeErr != nil {
		return 0, d.writeErr
	}
	return d.Buffer.Write(p)
}
func (d *failingArchiveDestination) Close() error { d.closed = true; return d.closeErr }

func TestWriteInstanceBackupArchiveFinalization(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "database.dump")
	if err := os.WriteFile(dbPath, []byte("database"), 0o600); err != nil {
		t.Fatal(err)
	}
	req := instanceBackupArchiveWriteRequest{
		dbArtifact:   databaseBackupArtifact{tempPath: dbPath, archiveEntryName: postgresArchiveDBEntry},
		manifestJSON: []byte(`{"version":1}`),
	}
	failure := errors.New("destination failed")
	for _, tc := range []struct {
		name               string
		writeErr, closeErr error
	}{
		{name: "gzip flush", writeErr: failure}, {name: "file close", closeErr: failure}, {name: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := &failingArchiveDestination{writeErr: tc.writeErr, closeErr: tc.closeErr}
			total, err := writeInstanceBackupArchiveTo(context.Background(), req, dst)
			if !dst.closed {
				t.Fatal("destination was not closed")
			}
			if tc.writeErr != nil || tc.closeErr != nil {
				if !errors.Is(err, failure) || total != 0 {
					t.Fatalf("total=%d err=%v, want zero bytes and destination error", total, err)
				}
				return
			}
			if err != nil || total != int64(len(req.manifestJSON)+len("database")) {
				t.Fatalf("total=%d err=%v", total, err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(dst.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			if _, err := io.Copy(io.Discard, reader); err != nil {
				t.Fatalf("archive footer is incomplete: %v", err)
			}
		})
	}
}

func TestWriteInstanceBackupArchivePreservesPrimaryError(t *testing.T) {
	closeFailure := errors.New("close failed")
	dst := &failingArchiveDestination{closeErr: closeFailure}
	_, err := writeInstanceBackupArchiveTo(context.Background(), instanceBackupArchiveWriteRequest{
		dbArtifact: databaseBackupArtifact{tempPath: filepath.Join(t.TempDir(), "missing")},
	}, dst)
	if !errors.Is(err, os.ErrNotExist) || !errors.Is(err, closeFailure) || !dst.closed {
		t.Fatalf("err=%v closed=%v, want both errors and a closed destination", err, dst.closed)
	}
}
