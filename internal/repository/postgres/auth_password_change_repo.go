package postgres

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"time"
)

// ChangeUserPassword commits all password-change effects together and rejects a
// password changed concurrently after the caller verified the previous hash.
func (r *AuthRepo) ChangeUserPassword(ctx context.Context, id uuid.UUID, previousHash, newHash string, exceptSessionID *uuid.UUID, when time.Time, audit *domain.AuditLog) error {
	if audit == nil {
		return fmt.Errorf("password-change audit entry is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning password change: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=?, must_change_password=FALSE,
  password_changed_at=?, updated_at=?, failed_login_attempts=0, locked_until=NULL
  WHERE id=? AND password_hash=?`, newHash, when, when, id.String(), previousHash)
	if err != nil {
		return fmt.Errorf("changing password: %w", err)
	}
	if err := requireRowsAffected(result, domain.ErrAuthUserNotFound); err != nil {
		return err
	}
	query := `UPDATE auth_sessions SET revoked_at=? WHERE user_id=? AND revoked_at IS NULL`
	args := []interface{}{when, id.String()}
	if exceptSessionID != nil {
		query += ` AND id<>?`
		args = append(args, exceptSessionID.String())
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("revoking sessions after password change: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs
  (id,actor_user_id,target_user_id,action,resource,resource_id,metadata_json,ip_address,user_agent,created_at)
  VALUES (?,?,?,?,?,?,?,?,?,?)`, audit.ID.String(), uuidPtrString(audit.ActorUserID), uuidPtrString(audit.TargetUserID),
		audit.Action, audit.Resource, audit.ResourceID, audit.MetadataJSON, audit.IPAddress, audit.UserAgent, audit.CreatedAt); err != nil {
		return fmt.Errorf("auditing password change: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing password change: %w", err)
	}
	return nil
}
