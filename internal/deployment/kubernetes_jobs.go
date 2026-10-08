package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
)

func (k *Kubernetes) capture(ctx context.Context, c KubernetesConfig, args ...string) ([]byte, error) {
	var buffer bytes.Buffer
	if len(args) > 1 && args[0] == "exec" {
		args = append([]string{args[0], args[1], "-c", "maintenance"}, args[2:]...)
	}
	if err := k.execute(ctx, "kubectl", append([]string{"--context", c.Context, "--namespace", c.Namespace}, args...), &buffer); err != nil {
		return nil, fmt.Errorf("Kubernetes inspection failed")
	}
	return buffer.Bytes(), nil
}

func (k *Kubernetes) job(ctx context.Context, c KubernetesConfig, recoverySecret string) (string, func(), error) {
	name := c.Name + "-maintenance-" + uuid.NewString()[:8]
	env := []map[string]string{{"name": "THEIA_INSTANCE_STATE", "value": "/persist/control/state.json"}, {"name": "THEIA_DATA_DIR", "value": "/persist/data"}, {"name": "THEIA_INSTANCE_BACKUP_DIR", "value": "/persist/data/instance-backups"}, {"name": "THEIA_DEPLOYMENT_ENV", "value": "production"}, {"name": "THEIA_RELEASE_TAG", "value": c.Release}}
	mounts := []map[string]any{{"name": "instance", "mountPath": "/persist"}}
	volumes := []map[string]any{{"name": "instance", "persistentVolumeClaim": map[string]string{"claimName": c.Name + "-instance"}}, {"name": "bootstrap", "secret": map[string]any{"secretName": c.Name + "-bootstrap", "optional": true, "defaultMode": 288}}}
	if recoverySecret != "" {
		mounts = append(mounts, map[string]any{"name": "recovery", "mountPath": "/run/recovery", "readOnly": true})
		volumes = append(volumes, map[string]any{"name": "recovery", "secret": map[string]any{"secretName": recoverySecret, "defaultMode": 288}})
	}
	bootstrap := map[string]any{"name": "bootstrap", "image": c.BackendImage, "command": []string{"sh", "-ec", `umask 077; mkdir -p /persist/control/public /persist/data; if [ ! -e /persist/control/state.json ]; then test -s /bootstrap/state.json; cp /bootstrap/state.json /persist/control/.bootstrap-state; chmod 600 /persist/control/.bootstrap-state; mv /persist/control/.bootstrap-state /persist/control/state.json; fi`}, "volumeMounts": []map[string]any{{"name": "instance", "mountPath": "/persist"}, {"name": "bootstrap", "mountPath": "/bootstrap", "readOnly": true}}}
	container := map[string]any{"name": "maintenance", "image": c.BackendImage, "command": []string{"sh", "-ec", "while [ ! -e /tmp/theia-finished ]; do sleep 2; done; exit $(cat /tmp/theia-finished)"}, "env": env, "volumeMounts": mounts, "readinessProbe": map[string]any{"exec": map[string]any{"command": []string{"test", "-s", "/persist/control/state.json"}}, "periodSeconds": 2}, "securityContext": map[string]any{"allowPrivilegeEscalation": false, "capabilities": map[string]any{"drop": []string{"ALL"}}}}
	term := map[string]any{"labelSelector": map[string]any{"matchLabels": map[string]string{"app": c.Name + "-frontend"}}, "topologyKey": "kubernetes.io/hostname"}
	affinity := map[string]any{"podAffinity": map[string]any{"preferredDuringSchedulingIgnoredDuringExecution": []any{map[string]any{"weight": 100, "podAffinityTerm": term}}}}
	podSpec := map[string]any{"restartPolicy": "Never", "automountServiceAccountToken": false, "securityContext": map[string]any{"runAsNonRoot": true, "runAsUser": 999, "runAsGroup": 999, "fsGroup": 999, "fsGroupChangePolicy": "OnRootMismatch"}, "initContainers": []any{bootstrap}, "containers": []any{container}, "volumes": volumes, "affinity": affinity}
	object := map[string]any{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]string{"name": name}, "spec": map[string]any{"backoffLimit": 0, "activeDeadlineSeconds": 1800, "ttlSecondsAfterFinished": 3600, "template": map[string]any{"metadata": map[string]any{"labels": map[string]string{"app": c.Name + "-maintenance"}}, "spec": podSpec}}}
	if err := k.applyObject(ctx, c, object); err != nil {
		return "", nil, err
	}
	cleanup := func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k.kubectl(clean, c, "delete", "job", name, "--ignore-not-found", "--wait=true")
	}
	for {
		data, err := k.capture(ctx, c, "get", "pod", "-l", "job-name="+name, "-o", "jsonpath={.items[0].metadata.name}")
		if err == nil && len(data) > 0 {
			pod := string(data)
			if err := k.kubectl(ctx, c, "wait", "--for=condition=Ready", "pod/"+pod, "--timeout=120s"); err != nil {
				cleanup()
				return "", nil, err
			}
			return pod, cleanup, nil
		}
		select {
		case <-ctx.Done():
			cleanup()
			return "", nil, ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func (k *Kubernetes) exec(ctx context.Context, c KubernetesConfig, pod string, args ...string) error {
	return k.kubectl(ctx, c, append([]string{"exec", pod, "-c", "maintenance", "--"}, args...)...)
}

// Offline stops the backend writer, runs the common native engine in a Job,
// and reopens HTTP only after verification. Failed mutations stay offline.
func (k *Kubernetes) Offline(ctx context.Context, action string, args ...string) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	if err := k.stop(ctx, *c); err != nil {
		return err
	}
	pod, cleanup, err := k.job(ctx, *c, "")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, *c, pod, append([]string{"theia", "maintenance", action}, args...)...); err != nil {
		return fmt.Errorf("maintenance failed; inspect status and run resume: %w", err)
	}
	if err := k.syncDatabaseSecret(ctx, *c, pod); err != nil {
		return err
	}
	cleanup()
	return k.start(ctx, *c)
}

func (k *Kubernetes) Backup(ctx context.Context, output string) error {
	if output != "" {
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			return fmt.Errorf("backup output already exists or cannot be inspected")
		}
	}
	c, err := k.Load()
	if err != nil {
		return err
	}
	if err := k.stop(ctx, *c); err != nil {
		return err
	}
	pod, cleanup, err := k.job(ctx, *c, "")
	if err != nil {
		return err
	}
	defer cleanup()
	data, err := k.capture(ctx, *c, "exec", pod, "--", "theia", "maintenance", "backup")
	if err != nil {
		return fmt.Errorf("backup incomplete; run resume before reopening writes")
	}
	var backup domain.InstanceBackup
	for _, line := range strings.Split(string(data), "\n") {
		var candidate domain.InstanceBackup
		if json.Unmarshal([]byte(line), &candidate) == nil && candidate.ID != uuid.Nil {
			backup = candidate
		}
	}
	if backup.Status != domain.InstanceBackupStatusSuccess || backup.ID == uuid.Nil || filepath.Base(backup.FileName) != backup.FileName {
		return fmt.Errorf("backup has no successful verification metadata")
	}
	if output != "" {
		temporary, err := os.CreateTemp(filepath.Dir(output), ".theia-backup-*")
		if err != nil {
			return err
		}
		path := temporary.Name()
		temporary.Close()
		defer os.Remove(path)
		source := pod + ":/persist/data/instance-backups/" + backup.ID.String() + "/" + backup.FileName
		if err := k.kubectl(ctx, *c, "cp", source, path); err != nil {
			return err
		}
		if err := os.Chmod(path, 0600); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		hash := sha256.New()
		size, readErr := io.Copy(hash, f)
		closeErr := errors.Join(f.Sync(), f.Close())
		if err := errors.Join(readErr, closeErr); err != nil {
			return err
		}
		if size != backup.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != backup.SHA256 {
			return fmt.Errorf("exported Kubernetes backup differs from verified bytes")
		}
		if err := os.Link(path, output); err != nil {
			return err
		}
	}
	if err := json.NewEncoder(k.Output).Encode(backup); err != nil {
		return err
	}
	return k.start(ctx, *c)
}

func (k *Kubernetes) Restore(ctx context.Context, archive, recoveryFile string) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	data, err := readInputFile(recoveryFile, 64<<10)
	if err != nil {
		return err
	}
	secret := c.Name + "-recovery-" + uuid.NewString()[:8]
	if err := k.applySecret(ctx, *c, secret, map[string]string{"recovery.txt": string(data)}); err != nil {
		return err
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k.kubectl(clean, *c, "delete", "secret", secret, "--ignore-not-found")
	}()
	pod, cleanup, err := k.job(ctx, *c, secret)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, *c, pod, "sh", "-ec", "umask 077; cp /run/recovery/recovery.txt /tmp/recovery.txt"); err != nil {
		return err
	}
	if err := k.kubectl(ctx, *c, "cp", archive, pod+":/tmp/archive.age"); err != nil {
		return err
	}
	args := []string{"-archive", "/tmp/archive.age", "-recovery-file", "/tmp/recovery.txt"}
	if err := k.exec(ctx, *c, pod, append([]string{"theia", "maintenance", "verify"}, args...)...); err != nil {
		return fmt.Errorf("archive rejected before stopping the active writer: %w", err)
	}
	if !c.PrepareRestore {
		if err := k.stop(ctx, *c); err != nil {
			return err
		}
	}
	if err := k.exec(ctx, *c, pod, append([]string{"theia", "maintenance", "restore"}, args...)...); err != nil {
		return err
	}
	c.PrepareRestore = false
	if err := k.saveMetadata(ctx, *c, pod); err != nil {
		return err
	}
	if err := k.save(*c); err != nil {
		return err
	}
	// Release the Job's exclusive lease before backend init maintenance runs.
	cleanup()
	return k.Up(ctx)
}

func (k *Kubernetes) Status(ctx context.Context) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	if err := k.kubectl(ctx, *c, "get", "deployments,pods,jobs,pvc"); err != nil {
		return err
	}
	return k.kubectl(ctx, *c, "exec", "deployment/"+c.Name+"-frontend", "--", "cat", "/status/status.json")
}

func (k *Kubernetes) Activation(ctx context.Context) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	return k.kubectl(ctx, *c, "exec", "deployment/"+c.Name+"-backend", "--", "theia", "instance", "activation", "-site", "https://"+c.Hostname)
}
