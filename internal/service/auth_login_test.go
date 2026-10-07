package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

func (s *fakeAuthStore) RecordSuccessfulLogin(_ context.Context, id uuid.UUID, hash string, when time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok || user.PasswordHash != hash || user.Status != domain.UserStatusActive || (user.LockedUntil != nil && user.LockedUntil.After(when)) {
		return domain.ErrAuthUserNotFound
	}
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	user.LastLoginAt = &when
	user.UpdatedAt = when
	s.users[id] = user
	return nil
}

func (s *fakeAuthStore) RecordFailedLogin(_ context.Context, id uuid.UUID, hash string, when time.Time, threshold int, lockedUntil time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok || user.PasswordHash != hash || user.Status != domain.UserStatusActive {
		return 0, domain.ErrAuthUserNotFound
	}
	if user.LockedUntil != nil && !user.LockedUntil.After(when) {
		user.FailedLoginAttempts = 0
		user.LockedUntil = nil
	}
	user.FailedLoginAttempts++
	if user.FailedLoginAttempts >= threshold {
		user.LockedUntil = &lockedUntil
	}
	user.UpdatedAt = when
	s.users[id] = user
	return user.FailedLoginAttempts, nil
}

func TestLoginRejectsConcurrentAccountChanges(t *testing.T) {
	for _, change := range []string{"password", "disable", "lock"} {
		t.Run(change, func(t *testing.T) {
			h := newAuthServiceHarness(t)
			user := h.addUser(t, "alice", "alice@example.test", testAuthPassword, domain.UserStatusActive)
			h.service.verifyPassword = func(_, _ string) (bool, error) {
				current, err := h.store.GetUserByID(context.Background(), user.ID)
				if err != nil {
					return false, err
				}
				switch change {
				case "password":
					current.PasswordHash = "new hash"
				case "disable":
					current.Status = domain.UserStatusDisabled
				case "lock":
					until := h.now.Add(time.Minute)
					current.LockedUntil = &until
				}
				return true, h.store.UpdateUser(context.Background(), current)
			}
			_, err := h.service.Login(context.Background(), LoginInput{Identifier: "alice", Password: testAuthPassword})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("err=%v, want invalid credentials", err)
			}
			stored := h.store.user(t, user.ID)
			if change == "password" && stored.PasswordHash != "new hash" || change == "disable" && stored.Status != domain.UserStatusDisabled || change == "lock" && stored.LockedUntil == nil {
				t.Fatal("login reverted concurrent change")
			}
			if len(h.store.sessions) != 0 {
				t.Fatal("session issued after account changed")
			}
		})
	}
}
