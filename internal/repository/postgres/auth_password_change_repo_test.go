package postgres

import (
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"testing"
	"time"
)

func TestPasswordChangeRollsBackEveryEffect(t *testing.T) {
	for _, stage := range []string{"revocation", "audit", "success"} {
		t.Run(stage, func(t *testing.T) {
			repo, ctx := newAuthRepoForTest(t)
			user := testAuthUser("atomic", "atomic@example.test")
			if err := repo.CreateUser(ctx, &user); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			current := domain.AuthSession{ID: uuid.New(), UserID: user.ID, TokenHash: "current", CSRFTokenHash: "csrf1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
			other := current
			other.ID = uuid.New()
			other.TokenHash = "other"
			other.CSRFTokenHash = "csrf2"
			for _, session := range []*domain.AuthSession{&current, &other} {
				if err := repo.CreateSession(ctx, session); err != nil {
					t.Fatal(err)
				}
			}
			if stage != "success" {
				if _, err := repo.db.raw.Exec(`CREATE FUNCTION audit_reject_password_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`); err != nil {
					t.Fatal(err)
				}
				table, event := "auth_sessions", "UPDATE"
				if stage == "audit" {
					table, event = "audit_logs", "INSERT"
				}
				if _, err := repo.db.raw.Exec("CREATE TRIGGER audit_password_failure BEFORE " + event + " ON " + table + " FOR EACH ROW EXECUTE FUNCTION audit_reject_password_change()"); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					repo.db.raw.Exec("DROP TRIGGER IF EXISTS audit_password_failure ON " + table)
					repo.db.raw.Exec("DROP FUNCTION IF EXISTS audit_reject_password_change()")
				})
			}
			audit := &domain.AuditLog{ID: uuid.New(), ActorUserID: &user.ID, TargetUserID: &user.ID, Action: "auth.password_changed", Resource: "auth", ResourceID: user.ID.String(), MetadataJSON: "{}", CreatedAt: now}
			err := repo.ChangeUserPassword(ctx, user.ID, user.PasswordHash, "new hash", &current.ID, now, audit)
			stored, readErr := repo.GetUserByID(ctx, user.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			savedOther, readErr := repo.GetSessionByTokenHash(ctx, other.TokenHash)
			if readErr != nil {
				t.Fatal(readErr)
			}
			savedCurrent, readErr := repo.GetSessionByTokenHash(ctx, current.TokenHash)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var count int
			if readErr := repo.db.raw.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE id=$1", audit.ID).Scan(&count); readErr != nil {
				t.Fatal(readErr)
			}
			if savedCurrent.RevokedAt != nil {
				t.Fatal("current session was revoked")
			}
			if stage == "success" {
				if err != nil || stored.PasswordHash != "new hash" || savedOther.RevokedAt == nil || count != 1 {
					t.Fatalf("err=%v password=%s revoked=%v audit=%d", err, stored.PasswordHash, savedOther.RevokedAt, count)
				}
			} else {
				if err == nil || stored.PasswordHash != user.PasswordHash || savedOther.RevokedAt != nil || count != 0 {
					t.Fatalf("partial commit: err=%v password=%s revoked=%v audit=%d", err, stored.PasswordHash, savedOther.RevokedAt, count)
				}
			}
		})
	}
}
