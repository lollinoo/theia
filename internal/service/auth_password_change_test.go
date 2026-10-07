package service

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

func (s *fakeAuthStore) ChangeUserPassword(_ context.Context, id uuid.UUID, previousHash, newHash string, exceptSessionID *uuid.UUID, when time.Time, audit *domain.AuditLog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.users[id]
	if !ok || user.PasswordHash != previousHash {
		return domain.ErrAuthUserNotFound
	}
	if s.auditErr != nil {
		return s.auditErr
	}
	user.PasswordHash = newHash
	user.MustChangePassword = false
	user.PasswordChangedAt = &when
	user.UpdatedAt = when
	user.FailedLoginAttempts = 0
	user.LockedUntil = nil
	s.users[id] = user
	for sessionID, session := range s.sessions {
		if session.UserID == id && session.RevokedAt == nil && (exceptSessionID == nil || sessionID != *exceptSessionID) {
			session.RevokedAt = &when
			s.sessions[sessionID] = session
		}
	}
	s.audit = append(s.audit, *audit)
	return nil
}

func TestPasswordChangeAuditFailureLeavesCredentialsAndSessionsUnchanged(t *testing.T) {
	h := newAuthServiceHarness(t)
	user := h.addUser(t, "atomic", "atomic@example.test", testAuthPassword, domain.UserStatusActive)
	login, err := h.service.Login(context.Background(), LoginInput{Identifier: user.Username, Password: testAuthPassword})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("audit unavailable")
	h.store.auditErr = failure
	err = h.service.ChangePassword(context.Background(), PasswordChangeInput{UserID: user.ID, CurrentPassword: testAuthPassword, NewPassword: "AnotherPass1!"})
	if !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
	if h.store.user(t, user.ID).PasswordHash != user.PasswordHash {
		t.Fatal("password changed on failure")
	}
	if h.store.sessions[login.Session.ID].RevokedAt != nil {
		t.Fatal("session revoked on failure")
	}
}
