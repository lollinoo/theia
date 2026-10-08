package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

type countedSessionStore struct {
	domain.SessionRepository
	reads, writes int
	touchErr      error
}

func (s *countedSessionStore) GetSessionByTokenHash(ctx context.Context, hash string) (*domain.AuthSession, error) {
	s.reads++
	return s.SessionRepository.GetSessionByTokenHash(ctx, hash)
}

func (s *countedSessionStore) TouchSession(ctx context.Context, id uuid.UUID, when time.Time) error {
	s.writes++
	if s.touchErr != nil {
		return s.touchErr
	}
	return s.SessionRepository.TouchSession(ctx, id, when)
}

func TestAuthRequestReusesSessionAndThrottlesActivityWrites(t *testing.T) {
	h := newAuthServiceHarness(t)
	u := h.addUser(t, "request-user", "request@example.test", testAuthPassword, domain.UserStatusActive)
	h.assignRole(t, u.ID, domain.RoleViewer)
	login, err := h.service.Login(context.Background(), LoginInput{Identifier: u.Username, Password: testAuthPassword})
	if err != nil {
		t.Fatal(err)
	}
	store := &countedSessionStore{SessionRepository: h.store}
	h.service.sessions = store
	for i := 0; i < 3; i++ {
		current, err := h.service.CurrentUser(context.Background(), login.SessionToken)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.service.ValidateAuthenticatedCSRF(context.Background(), current, login.SessionToken, login.CSRFToken); err != nil {
			t.Fatal(err)
		}
		for _, tokens := range [][2]string{{"wrong", login.CSRFToken}, {login.SessionToken, "wrong"}, {login.SessionToken, ""}} {
			if err := h.service.ValidateAuthenticatedCSRF(context.Background(), current, tokens[0], tokens[1]); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("invalid tokens accepted: %v", err)
			}
		}
		encoded, err := json.Marshal(current)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), current.csrfTokenHash) || strings.Contains(string(encoded), current.tokenHash) {
			t.Fatal("session hashes leaked")
		}
		h.now = h.now.Add(20 * time.Second)
	}
	if store.reads != 3 || store.writes != 1 {
		t.Fatalf("session work reads=%d writes=%d, want 3/1", store.reads, store.writes)
	}
	current, err := h.service.CurrentUser(context.Background(), login.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	if store.writes != 2 {
		t.Fatalf("writes after one minute=%d, want 2", store.writes)
	}
	h.now = current.Session.ExpiresAt
	if err := h.service.ValidateAuthenticatedCSRF(context.Background(), current, login.SessionToken, login.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired request accepted: %v", err)
	}
	if err := h.service.ValidateAuthenticatedCSRF(context.Background(), &AuthenticatedUser{Session: current.Session}, login.SessionToken, login.CSRFToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("fabricated session accepted")
	}
}

func TestAuthSessionTouchFailureStillFailsRequest(t *testing.T) {
	h := newAuthServiceHarness(t)
	u := h.addUser(t, "touch-user", "touch@example.test", testAuthPassword, domain.UserStatusActive)
	login, err := h.service.Login(context.Background(), LoginInput{Identifier: u.Username, Password: testAuthPassword})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("activity write failed")
	h.service.sessions = &countedSessionStore{SessionRepository: h.store, touchErr: injected}
	if _, err := h.service.CurrentUser(context.Background(), login.SessionToken); !errors.Is(err, injected) {
		t.Fatalf("write failure=%v", err)
	}
}
