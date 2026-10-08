package service

import (
	"context"
	"database/sql"
	"fmt"
)

const managedInstanceIdentitySetting = "managed_instance_identity"

// VerifyInstanceIdentity prevents a ready schema on an unrelated database from
// satisfying the instance's maintenance receipt. The identifier is public.
func VerifyInstanceIdentity(ctx context.Context, db *sql.DB, instanceID string) error {
	var stored string
	if err := db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = $1", managedInstanceIdentitySetting).Scan(&stored); err != nil || stored != instanceID {
		return fmt.Errorf("database does not match the managed instance; run maintenance migrate against the intended database")
	}
	return nil
}
