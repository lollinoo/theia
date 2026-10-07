package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestBackupAdmissionQueriesCancelWhilePoolIsSaturated(t *testing.T) {
	setupTestDB(t)
	// A fresh pool isolates admission from the migration runner's connection.
	db, err := sql.Open("pgx", os.Getenv("THEIA_TEST_DB_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(1)
	setupCtx, setupCancel := context.WithTimeout(context.Background(), time.Second)
	defer setupCancel()
	conn, err := db.Conn(setupCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for _, operation := range []func(context.Context) error{
		func(ctx context.Context) error {
			_, err := NewDeviceRepo(db, testKeyring, nil).GetByIDContext(ctx, uuid.New())
			return err
		},
		func(ctx context.Context) error {
			_, err := NewCredentialProfileRepo(db).GetBackupProfileForDeviceContext(ctx, uuid.New())
			return err
		},
		func(ctx context.Context) error {
			return NewBackupJobRepo(db).CreateContext(ctx, &domain.BackupJob{DeviceID: uuid.New(), Status: domain.BackupStatusPending})
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		err := operation(ctx)
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("pool wait ignored deadline: %v", err)
		}
	}
	conn.Close()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM backup_jobs").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("cancelled admission created a job")
	}
}
