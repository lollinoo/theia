package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"time"
)

// RecordSuccessfulLogin writes only login state, rejecting a changed password,
// disabled account or newly applied lock after password verification.
func (r *AuthRepo) RecordSuccessfulLogin(ctx context.Context, id uuid.UUID, passwordHash string, when time.Time) error {
	result, err := r.execContext(ctx, `UPDATE users SET failed_login_attempts=0, locked_until=NULL, last_login_at=?, updated_at=?
  WHERE id=? AND password_hash=? AND status='active' AND (locked_until IS NULL OR locked_until<=?)`, when, when, id.String(), passwordHash, when)
	if err != nil {
		return fmt.Errorf("recording successful login: %w", err)
	}
	return requireRowsAffected(result, domain.ErrAuthUserNotFound)
}

// RecordFailedLogin increments from the latest committed counter and resets an expired lock.
func (r *AuthRepo) RecordFailedLogin(ctx context.Context, id uuid.UUID, passwordHash string, when time.Time, threshold int, lockedUntil time.Time) (int, error) {
	var attempts int
	err := r.queryRowContext(ctx, `UPDATE users SET
  failed_login_attempts=CASE WHEN locked_until<=? THEN 1 ELSE failed_login_attempts+1 END,
  locked_until=CASE WHEN (CASE WHEN locked_until<=? THEN 1 ELSE failed_login_attempts+1 END)>=? THEN ?
    WHEN locked_until<=? THEN NULL ELSE locked_until END, updated_at=?
  WHERE id=? AND password_hash=? AND status='active' RETURNING failed_login_attempts`,
		when, when, threshold, lockedUntil, when, when, id.String(), passwordHash).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, domain.ErrAuthUserNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("recording failed login: %w", err)
	}
	return attempts, nil
}
