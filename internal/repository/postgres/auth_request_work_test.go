package postgres

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func TestAuthSessionActivityWriteIsAtomicAndThrottled(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)
	user := testAuthUser("activity-user", "activity@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	session := domain.AuthSession{ID: uuid.New(), UserID: user.ID, TokenHash: "activity-token", CSRFTokenHash: "activity-csrf", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	if err := repo.CreateSession(ctx, &session); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.raw.Exec(`CREATE TABLE test_session_activity_writes (seen timestamptz NOT NULL);
		CREATE FUNCTION test_session_activity_count() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO test_session_activity_writes VALUES(NEW.last_seen_at);RETURN NEW;END;$$;
		CREATE TRIGGER test_session_activity_trigger AFTER UPDATE OF last_seen_at ON auth_sessions FOR EACH ROW EXECUTE FUNCTION test_session_activity_count()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := repo.db.raw.Exec(`DROP TRIGGER IF EXISTS test_session_activity_trigger ON auth_sessions;DROP FUNCTION IF EXISTS test_session_activity_count();DROP TABLE IF EXISTS test_session_activity_writes`)
		if err != nil {
			t.Error(err)
		}
	})
	var group sync.WaitGroup
	failures := make(chan error, 16)
	for i := 0; i < 16; i++ {
		group.Add(1)
		go func() { defer group.Done(); failures <- repo.TouchSessionIfStale(ctx, session.ID, now, time.Minute) }()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if count := importTestCount(t, repo.db.raw, "SELECT COUNT(*) FROM test_session_activity_writes"); count != 1 {
		t.Fatalf("activity writes=%d, want 1", count)
	}
	if err := repo.TouchSessionIfStale(ctx, session.ID, now.Add(time.Minute), time.Minute); err != nil {
		t.Fatal(err)
	}
	if count := importTestCount(t, repo.db.raw, "SELECT COUNT(*) FROM test_session_activity_writes"); count != 2 {
		t.Fatalf("activity writes after interval=%d, want 2", count)
	}
	if err := repo.TouchSessionIfStale(ctx, uuid.New(), now, time.Minute); !errors.Is(err, domain.ErrAuthSessionNotFound) {
		t.Fatalf("missing session accepted: %v", err)
	}
}

func TestAuthAggregatePreservesEmptyAndDeduplicatedGrants(t *testing.T) {
	repo, ctx := newAuthRepoForTest(t)
	user := testAuthUser("aggregate-user", "aggregate@example.test")
	if err := repo.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetUserRolesAndPermissions(ctx, user.ID); err != nil || got.Roles != nil || got.Permissions != nil {
		t.Fatalf("empty aggregate=%#v error=%v", got, err)
	}
	for _, role := range []string{domain.RoleAdmin, domain.RoleViewer} {
		if err := repo.AssignRole(ctx, user.ID, role, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := repo.GetUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Roles) != 2 || !got.HasRole(domain.RoleAdmin) || !got.HasRole(domain.RoleViewer) {
		t.Fatalf("role grants=%#v", got.Roles)
	}
	keys := make(map[string]bool)
	for _, permission := range got.Permissions {
		if keys[permission.Key] {
			t.Fatalf("duplicated permission %s", permission.Key)
		}
		keys[permission.Key] = true
	}
	if !got.HasPermission(domain.PermissionTopologyRead) {
		t.Fatal("effective permission missing")
	}
}
