package postgres

import (
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lollinoo/theia/internal/domain"
)

func TestSettingsRepoUpdateSerializesConcurrentMutations(t *testing.T) {
	db := setupTestDB(t)
	repo := NewSettingsRepo(db)
	if err := repo.Set(domain.SettingPollingInterval, "60"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}

	firstLocked := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, err := repo.Update(domain.SettingPollingInterval, func(current string) (string, error) {
			close(firstLocked)
			<-releaseFirst
			value, err := strconv.Atoi(current)
			if err != nil {
				return "", err
			}
			return strconv.Itoa(value + 1), nil
		})
		firstDone <- err
	}()
	<-firstLocked

	lockTx, err := db.Begin()
	if err != nil {
		close(releaseFirst)
		t.Fatalf("begin competing transaction: %v", err)
	}
	if _, err := lockTx.Exec(`SET LOCAL lock_timeout = '100ms'`); err != nil {
		lockTx.Rollback() //nolint:errcheck
		close(releaseFirst)
		t.Fatalf("set competing transaction lock timeout: %v", err)
	}
	var lockedValue string
	lockErr := lockTx.QueryRow(
		rebindQuery(`SELECT value FROM settings WHERE key = ? FOR UPDATE`),
		domain.SettingPollingInterval,
	).Scan(&lockedValue)
	if err := lockTx.Rollback(); err != nil {
		close(releaseFirst)
		t.Fatalf("roll back competing transaction: %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(lockErr, &pgErr) || pgErr.Code != "55P03" {
		close(releaseFirst)
		t.Fatalf("competing transaction error = %v, want PostgreSQL lock timeout", lockErr)
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Update returned error: %v", err)
	}
	var secondCurrent string
	_, err = repo.Update(domain.SettingPollingInterval, func(current string) (string, error) {
		secondCurrent = current
		value, err := strconv.Atoi(current)
		if err != nil {
			return "", err
		}
		return strconv.Itoa(value + 1), nil
	})
	if err != nil {
		t.Fatalf("second Update returned error: %v", err)
	}
	if secondCurrent != "61" {
		t.Fatalf("second mutation received %q, want committed value %q", secondCurrent, "61")
	}

	value, err := repo.Get(domain.SettingPollingInterval)
	if err != nil {
		t.Fatalf("get updated setting: %v", err)
	}
	if value != "62" {
		t.Fatalf("updated value = %q, want %q", value, "62")
	}
}
