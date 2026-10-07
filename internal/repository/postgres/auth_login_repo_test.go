package postgres

import (
	"context"
	"errors"
	"github.com/lollinoo/theia/internal/domain"
	"sync"
	"testing"
	"time"
)

func TestAuthLoginUpdatesPreserveAccountChanges(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)
	user := testAuthUser("login", "login@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	oldHash := user.PasswordHash
	user.PasswordHash = "changed"
	user.Status = domain.UserStatusDisabled
	if err := repo.UpdateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordSuccessfulLogin(ctx, user.ID, oldHash, time.Now()); !errors.Is(err, domain.ErrAuthUserNotFound) {
		t.Fatalf("err=%v", err)
	}
	if _, err := repo.RecordFailedLogin(ctx, user.ID, oldHash, time.Now(), 5, time.Now().Add(time.Minute)); !errors.Is(err, domain.ErrAuthUserNotFound) {
		t.Fatalf("err=%v", err)
	}
	stored, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash != "changed" || stored.Status != domain.UserStatusDisabled {
		t.Fatal("login reverted account change")
	}
}

func TestAuthFailedLoginIncrementsAreAtomic(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)
	user := testAuthUser("failed", "failed@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.RecordFailedLogin(context.Background(), user.ID, user.PasswordHash, now, 5, now.Add(time.Minute)); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	stored, err := repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.FailedLoginAttempts != 12 || stored.LockedUntil == nil {
		t.Fatalf("attempts=%d lock=%v", stored.FailedLoginAttempts, stored.LockedUntil)
	}
	if _, err := repo.RecordFailedLogin(ctx, user.ID, user.PasswordHash, now.Add(2*time.Minute), 5, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	stored, err = repo.GetUserByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.FailedLoginAttempts != 1 || stored.LockedUntil != nil {
		t.Fatal("expired lock was not reset")
	}
}
