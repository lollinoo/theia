package service

import (
	"context"
	"fmt"

	"github.com/lollinoo/theia/internal/instance"
)

// MarkWritesReopened durably closes the rollback window before HTTP serves any
// application request. The caller must hold the runtime lease for its lifetime.
func (m *Maintenance) MarkWritesReopened(instanceID, activeKeyID string) error {
	if err := m.RequireVerifiedState(instanceID, activeKeyID); err != nil {
		return err
	}
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op.WritesReopened {
		return nil
	}
	op.WritesReopened = true
	return m.persist(op, op.Phase)
}

// RollbackBeforeReopen recovers a completed migration when deployment validation
// fails before HTTP starts. A reopened or ambiguous write window cannot roll back.
func (m *Maintenance) RollbackBeforeReopen(ctx context.Context) error {
	if err := m.validatePaths(); err != nil {
		return err
	}
	release, err := instance.AcquireLease(m.StatePath)
	if err != nil {
		return err
	}
	defer release()
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op == nil || op.Phase != "completed" || op.WritesReopened || op.SnapshotSHA256 == "" {
		return fmt.Errorf("rollback requires a verified operation whose application writes have not reopened")
	}
	op.Attempt++
	return m.rollback(ctx, op)
}
