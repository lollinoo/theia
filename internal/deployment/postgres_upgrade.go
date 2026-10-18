package deployment

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

// UpgradePostgres moves bundled PostgreSQL 17 into a separate version 18 volume.
// The original source is never deleted; all cutover phases are durable and resume
// chooses that source again if the replacement has not reopened application writes.
func (a *Admin) UpgradePostgres(ctx context.Context, targetMajor int) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	previous, err := a.loadUpgrade()
	if err != nil {
		return err
	}
	if previous != nil && previous.Phase != "completed" && previous.Phase != "rolled_back" {
		return fmt.Errorf("an upgrade is incomplete; run resume")
	}
	original, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	if !original.BundledPostgres || original.PostgresMajor != 17 || targetMajor != 18 {
		return fmt.Errorf("the managed bundled PostgreSQL transition is 17 to 18; external databases remain with their infrastructure owner")
	}
	if !a.OfflineImages {
		if err := a.run(ctx, "pull", "postgres:18-bookworm"); err != nil {
			return err
		}
	}
	target := *original
	target.PostgresMajor = 18
	target.PostgresVolume = ""
	target.PostgresBind = filepath.Join(a.Dir, "postgres-18-"+uuid.NewString())
	j := &upgradeJournal{Kind: "postgres_major", Original: *original, Target: target}
	if err := a.saveUpgrade(j, "prepared"); err != nil {
		return err
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	if err := a.job(ctx, "maintenance", "postgres-prepare", "-postgres-target", "18"); err != nil {
		return errors.Join(err, a.recoverPostgresUpgrade(ctx, j))
	}
	if err := a.saveUpgrade(j, "source_verified"); err != nil {
		return err
	}
	if err := a.removePostgresContainer(ctx); err != nil {
		return err
	}
	if err := SaveConfig(a.Dir, target); err != nil {
		return err
	}
	state, err := (instance.Store{Path: StatePath(a.Dir)}).Load()
	if err != nil {
		return err
	}
	if err := a.prepare(target, state); err != nil {
		return err
	}
	if err := a.saveUpgrade(j, "database_cutover"); err != nil {
		return err
	}
	if err := a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "postgres"); err != nil {
		return errors.Join(err, a.recoverPostgresUpgrade(ctx, j))
	}
	if err := a.job(ctx, "maintenance", "postgres-finish"); err != nil {
		return errors.Join(err, a.recoverPostgresUpgrade(ctx, j))
	}
	if err := a.saveMetadata(target); err != nil {
		return err
	}
	if err := a.saveUpgrade(j, "verified"); err != nil {
		return err
	}
	if err := a.startHTTP(ctx, target); err != nil {
		return errors.Join(err, a.recoverPostgresUpgrade(ctx, j))
	}
	return a.saveUpgrade(j, "completed")
}

func (a *Admin) removePostgresContainer(ctx context.Context) error {
	if err := a.compose(ctx, "stop", "postgres"); err != nil {
		return err
	}
	// No volume-removal flag: version 17 stays available independently of cutover.
	return a.compose(ctx, "rm", "-f", "postgres")
}

func (a *Admin) recoverPostgresUpgrade(ctx context.Context, j *upgradeJournal) error {
	op, err := (&service.Maintenance{StatePath: StatePath(a.Dir)}).Status()
	if err != nil {
		return err
	}
	if op != nil && op.Action == "postgres_major" && op.Phase == "completed" && op.WritesReopened {
		// Resume the verified target; switching back would discard newer writes.
		if err := a.startHTTP(ctx, j.Target); err != nil {
			return fmt.Errorf("PostgreSQL 18 already accepted application writes; rollback is closed: %w", err)
		}
		return a.saveUpgrade(j, "completed")
	}
	if err := a.pause(ctx, true); err != nil {
		return err
	}
	if err := a.removePostgresContainer(ctx); err != nil {
		return err
	}
	if err := SaveConfig(a.Dir, j.Original); err != nil {
		return err
	}
	if err := WriteCompose(a.Dir, j.Original); err != nil {
		return err
	}
	if err := a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "postgres"); err != nil {
		return err
	}
	if op != nil && op.Action == "postgres_major" && op.Phase != "rolled_back" {
		if err := a.job(ctx, "maintenance", "postgres-abort"); err != nil {
			return err
		}
	}
	if err := a.startHTTP(ctx, j.Original); err != nil {
		return err
	}
	return a.saveUpgrade(j, "rolled_back")
}
