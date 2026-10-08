package service

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/instance"
)

// RotateOperationalSecrets replaces operational secrets during a protected
// offline operation. External database owners manage their own password.
func (m *Maintenance) RotateOperationalSecrets(ctx context.Context) error {
	return m.mutate(ctx, "rotate_operational", func(ctx context.Context, db *sql.DB, state *instance.State) error {
		connection, err := url.Parse(state.DBDSN)
		if err != nil || connection.User == nil || connection.Scheme != "postgres" || (connection.Host != "postgres:5432" && !strings.HasSuffix(connection.Host, "-postgres:5432")) || state.DBDSN != m.DBDSN {
			return fmt.Errorf("operational password rotation requires the bundled managed PostgreSQL connection")
		}
		password, err := instance.RandomSecret()
		if err != nil {
			return err
		}
		session, err := instance.RandomSecret()
		if err != nil {
			return err
		}
		metrics, err := instance.RandomSecret()
		if err != nil {
			return err
		}
		connection.User = url.UserPassword(connection.User.Username(), password)
		// Both passwords remain recoverable in the verified safety snapshot until
		// database authentication and the persistent state are verified together.
		if err := (instance.Store{Path: m.StatePath}).Update(func(s *instance.State) error {
			s.DatabasePassword = password
			s.DBDSN = connection.String()
			s.SessionSecret = session
			s.MetricsToken = metrics
			return nil
		}); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if err := setDatabaseRolePassword(ctx, tx, password); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE auth_sessions SET revoked_at=$1 WHERE revoked_at IS NULL", time.Now().UTC()); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		m.DBDSN = connection.String()
		return nil
	})
}

type passwordExecutor interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func setDatabaseRolePassword(ctx context.Context, db passwordExecutor, password string) error {
	var statement string
	if err := db.QueryRowContext(ctx, "SELECT format('ALTER ROLE %I PASSWORD %L',current_user,$1::text)", password).Scan(&statement); err != nil {
		return fmt.Errorf("prepare database password replacement failed")
	}
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("database password replacement failed")
	}
	return nil
}
