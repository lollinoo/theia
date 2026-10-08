package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

type upgradeJournal struct {
	Original  Config    `json:"original"`
	Target    Config    `json:"target"`
	Phase     string    `json:"phase"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (a *Admin) saveUpgrade(j *upgradeJournal, phase string) error {
	j.Phase, j.UpdatedAt = phase, time.Now().UTC()
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(filepath.Join(a.Dir, "control", "upgrade.json"), data)
}

func (a *Admin) loadUpgrade() (*upgradeJournal, error) {
	data, err := os.ReadFile(filepath.Join(a.Dir, "control", "upgrade.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j upgradeJournal
	if json.Unmarshal(data, &j) != nil || j.Original.Validate() != nil || j.Target.Validate() != nil {
		return nil, fmt.Errorf("invalid deployment upgrade journal")
	}
	return &j, nil
}

// Upgrade pins the requested release, downloads images before stopping writes,
// and restores the original release when database maintenance rolls back.
func (a *Admin) Upgrade(ctx context.Context, release, backendImage, frontendImage string) error {
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
	target := *original
	target.Release = release
	target.BackendImage = "ghcr.io/lollinoo/theia-backend:" + release
	target.FrontendImage = "ghcr.io/lollinoo/theia-frontend:" + release
	if backendImage != "" {
		target.BackendImage = backendImage
	}
	if frontendImage != "" {
		target.FrontendImage = frontendImage
	}
	if err := target.Validate(); err != nil {
		return err
	}
	if !a.OfflineImages {
		for _, image := range []string{target.BackendImage, target.FrontendImage} {
			if err := a.run(ctx, "pull", image); err != nil {
				return err
			}
		}
	}
	j := &upgradeJournal{Original: *original, Target: target}
	if err := a.saveUpgrade(j, "prepared"); err != nil {
		return err
	}
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	if err := SaveConfig(a.Dir, target); err != nil {
		return err
	}
	if err := WriteCompose(a.Dir, target); err != nil {
		return err
	}
	if err := a.saveUpgrade(j, "applying"); err != nil {
		return err
	}
	if err := a.job(ctx, "maintenance", "migrate"); err != nil {
		return errors.Join(err, a.recoverUpgrade(ctx, j))
	}
	if err := a.saveUpgrade(j, "verified"); err != nil {
		return err
	}
	if err := a.saveMetadata(target); err != nil {
		return err
	}
	if err := a.startHTTP(ctx, target); err != nil {
		return errors.Join(err, a.recoverUpgrade(ctx, j))
	}
	return a.saveUpgrade(j, "completed")
}

func (a *Admin) recoverUpgrade(ctx context.Context, j *upgradeJournal) error {
	if err := a.pause(ctx, false); err != nil {
		return err
	}
	m := &service.Maintenance{StatePath: StatePath(a.Dir)}
	op, err := m.Status()
	if err != nil {
		return err
	}
	if op != nil && op.Phase == "completed" && op.WritesReopened && op.VerifiedReleaseTag == j.Target.Release {
		// Once writes reopen, keep the current release and let resume retry startup.
		return fmt.Errorf("application writes already reopened on the requested release; automatic rollback is closed, run resume after fixing startup")
	}
	if op != nil && !op.WritesReopened && op.Phase == "completed" && op.VerifiedReleaseTag == j.Target.Release {
		if err := a.job(ctx, "maintenance", "rollback"); err != nil {
			return err
		}
	} else if op != nil && op.Phase != "completed" && op.Phase != "rolled_back" {
		if err := a.job(ctx, "maintenance", "resume"); err != nil {
			return err
		}
	}
	op, err = m.Status()
	if err != nil {
		return err
	}
	if op == nil || op.VerifiedReleaseTag != j.Original.Release {
		return fmt.Errorf("original release is not verified; instance remains in maintenance")
	}
	if err := SaveConfig(a.Dir, j.Original); err != nil {
		return err
	}
	if err := WriteCompose(a.Dir, j.Original); err != nil {
		return err
	}
	if err := a.startHTTP(ctx, j.Original); err != nil {
		return err
	}
	return a.saveUpgrade(j, "rolled_back")
}

// Resume resolves an interrupted database operation using its own immutable
// snapshot, then restores the release coordinates recorded before the upgrade.
func (a *Admin) Resume(ctx context.Context) error {
	unlock, err := a.lock()
	if err != nil {
		return err
	}
	defer unlock()
	j, err := a.loadUpgrade()
	if err != nil {
		return err
	}
	if j != nil && j.Phase != "completed" && j.Phase != "rolled_back" {
		op, err := (&service.Maintenance{StatePath: StatePath(a.Dir)}).Status()
		if err != nil {
			return err
		}
		if op != nil && op.Phase == "completed" && op.WritesReopened && op.VerifiedReleaseTag == j.Target.Release {
			if err := a.startHTTP(ctx, j.Target); err != nil {
				return err
			}
			return a.saveUpgrade(j, "completed")
		}
		return a.recoverUpgrade(ctx, j)
	}
	c, err := LoadConfig(a.Dir)
	if err != nil {
		return err
	}
	if err := a.pause(ctx, true); err != nil {
		return err
	}
	if err := a.job(ctx, "maintenance", "resume"); err != nil {
		return err
	}
	if err := a.job(ctx, "maintenance", "migrate"); err != nil {
		return err
	}
	return a.startHTTP(ctx, *c)
}
