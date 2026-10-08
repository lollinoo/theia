package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lollinoo/theia/internal/instance"
	"github.com/lollinoo/theia/internal/service"
)

type kubernetesUpgrade struct {
	Original, Target KubernetesConfig
	Phase            string
}

func (k *Kubernetes) saveUpgrade(j kubernetesUpgrade) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(filepath.Join(k.Dir, "control", "kubernetes-upgrade.json"), data)
}

func (k *Kubernetes) operation(ctx context.Context, c KubernetesConfig, pod string) (*service.MaintenanceOperation, error) {
	data, err := k.capture(ctx, c, "exec", pod, "--", "theia", "maintenance", "status")
	if err != nil {
		return nil, err
	}
	var op *service.MaintenanceOperation
	if err := json.Unmarshal(data, &op); err != nil {
		return nil, fmt.Errorf("invalid operation status")
	}
	return op, nil
}

// Upgrade uses a verified native migration Job before replacing either HTTP
// deployment. The old release is selected again only after verified rollback.
func (k *Kubernetes) Upgrade(ctx context.Context, release, backend, frontend string) error {
	original, err := k.Load()
	if err != nil {
		return err
	}
	if data, err := os.ReadFile(filepath.Join(k.Dir, "control", "kubernetes-upgrade.json")); err == nil {
		var previous kubernetesUpgrade
		if json.Unmarshal(data, &previous) != nil || previous.Phase != "completed" {
			return fmt.Errorf("a Kubernetes upgrade is incomplete; run resume")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	target := *original
	target.Release = release
	target.BackendImage = "ghcr.io/lollinoo/theia-backend:" + release
	target.FrontendImage = "ghcr.io/lollinoo/theia-frontend:" + release
	if backend != "" {
		target.BackendImage = backend
	}
	if frontend != "" {
		target.FrontendImage = frontend
	}
	if err := target.Validate(); err != nil {
		return err
	}
	journal := kubernetesUpgrade{Original: *original, Target: target, Phase: "prepared"}
	if err := k.saveUpgrade(journal); err != nil {
		return err
	}
	if err := k.stop(ctx, *original); err != nil {
		return err
	}
	pod, cleanup, err := k.job(ctx, target, "")
	if err != nil {
		return err
	}
	defer cleanup()
	migrationErr := k.exec(ctx, target, pod, "theia", "maintenance", "migrate")
	op, err := k.operation(ctx, target, pod)
	if err != nil {
		return err
	}
	if migrationErr != nil {
		if op != nil && op.Phase == "rolled_back" && op.VerifiedReleaseTag == original.Release {
			cleanup()
			if err := k.helm(ctx, *original); err != nil {
				return err
			}
			journal.Phase = "completed"
			_ = k.saveUpgrade(journal)
		}
		return migrationErr
	}
	if err := k.saveMetadata(ctx, target, pod); err != nil {
		return err
	}
	cleanup()
	if err := k.save(target); err != nil {
		return err
	}
	if err := k.helm(ctx, target); err != nil {
		return err
	}
	journal.Phase = "completed"
	return k.saveUpgrade(journal)
}

func (k *Kubernetes) Resume(ctx context.Context) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	if err := k.stop(ctx, *c); err != nil {
		return err
	}
	var journal *kubernetesUpgrade
	if data, err := os.ReadFile(filepath.Join(k.Dir, "control", "kubernetes-upgrade.json")); err == nil {
		var candidate kubernetesUpgrade
		if json.Unmarshal(data, &candidate) != nil {
			return fmt.Errorf("invalid Kubernetes upgrade journal")
		}
		if candidate.Phase != "completed" {
			journal = &candidate
			*c = candidate.Target
		}
	}
	pod, cleanup, err := k.job(ctx, *c, "")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, *c, pod, "theia", "maintenance", "resume"); err != nil {
		return err
	}
	if err := k.syncDatabaseSecret(ctx, *c, pod); err != nil {
		return err
	}
	op, err := k.operation(ctx, *c, pod)
	if err != nil {
		return err
	}
	if journal != nil {
		if op != nil && op.VerifiedReleaseTag == journal.Target.Release {
			*c = journal.Target
		} else if op != nil && op.VerifiedReleaseTag == journal.Original.Release {
			*c = journal.Original
		} else {
			return fmt.Errorf("verified operation does not identify a compatible release")
		}
	}
	cleanup()
	if err := k.save(*c); err != nil {
		return err
	}
	if err := k.helm(ctx, *c); err != nil {
		return err
	}
	if err := k.kubectl(ctx, *c, "exec", "deployment/"+c.Name+"-backend", "--", "rm", "-f", "/persist/control/public/deployment-maintenance"); err != nil {
		return err
	}
	if journal != nil {
		journal.Phase = "completed"
		return k.saveUpgrade(*journal)
	}
	return nil
}
