package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/instance"
)

func (k *Kubernetes) temporarySecret(ctx context.Context, c KubernetesConfig, values map[string]string) (string, func(), error) {
	name := c.Name + "-operator-" + uuid.NewString()[:8]
	if err := k.applySecret(ctx, c, name, values); err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = k.kubectl(context.Background(), c, "delete", "secret", name, "--ignore-not-found") }
	return name, cleanup, nil
}

func (k *Kubernetes) syncDatabaseSecret(ctx context.Context, c KubernetesConfig, pod string) error {
	if c.ExternalPostgres {
		return nil
	}
	if err := k.exec(ctx, c, pod, "theia", "instance", "database-password-file", "-output", "/tmp/theia-operator/database-password"); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "theia-password-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "password")
	if err := k.kubectl(ctx, c, "cp", pod+":/tmp/theia-operator/database-password", path); err != nil {
		return err
	}
	defer k.exec(context.Background(), c, pod, "rm", "-f", "/tmp/theia-operator/database-password")
	password, err := readInputFile(path, 64<<10)
	if err != nil {
		return err
	}
	return k.applySecret(ctx, c, c.Name+"-database", map[string]string{"password": string(password)})
}

func (k *Kubernetes) saveMetadata(ctx context.Context, c KubernetesConfig, pod string) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "theia-coordinates-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "coordinates.json")
	if err := instance.WritePrivateFile(path, data); err != nil {
		return err
	}
	if err := k.exec(ctx, c, pod, "sh", "-ec", "umask 077; mkdir -p /tmp/theia-operator"); err != nil {
		return err
	}
	if err := k.kubectl(ctx, c, "cp", path, pod+":/tmp/theia-operator/coordinates.json"); err != nil {
		return err
	}
	return k.exec(ctx, c, pod, "theia", "instance", "deployment", "-metadata-file", "/tmp/theia-operator/coordinates.json")
}

func (k *Kubernetes) ConfigureS3(ctx context.Context, destination instance.S3Config, accessFile, secretFile string) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	access, err := readInputFile(accessFile, 64<<10)
	if err != nil {
		return err
	}
	secret, err := readInputFile(secretFile, 64<<10)
	if err != nil {
		return err
	}
	destination.AccessKey = strings.TrimRight(string(access), "\r\n")
	destination.SecretKey = strings.TrimRight(string(secret), "\r\n")
	if _, err := instance.NewS3Destination(destination); err != nil {
		return err
	}
	name, remove, err := k.temporarySecret(ctx, *c, map[string]string{"access": destination.AccessKey, "secret": destination.SecretKey})
	if err != nil {
		return err
	}
	defer remove()
	if err := k.stop(ctx, *c); err != nil {
		return err
	}
	pod, cleanup, err := k.job(ctx, *c, name)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, *c, pod, "env", "THEIA_S3_ACCESS_KEY_FILE=/run/recovery/access", "THEIA_S3_SECRET_KEY_FILE=/run/recovery/secret", "theia", "instance", "s3", "-endpoint", destination.Endpoint, "-bucket", destination.Bucket, "-region", destination.Region, "-prefix", destination.Prefix); err != nil {
		return err
	}
	cleanup()
	return k.start(ctx, *c)
}

func (k *Kubernetes) RotateRecovery(ctx context.Context, original, output string) error {
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		return fmt.Errorf("recovery output already exists or cannot be inspected")
	}
	if err := outsideStorage(output, k.Dir); err != nil {
		return err
	}
	c, err := k.Load()
	if err != nil {
		return err
	}
	data, err := readInputFile(original, 64<<10)
	if err != nil {
		return err
	}
	name, remove, err := k.temporarySecret(ctx, *c, map[string]string{"recovery.txt": string(data)})
	if err != nil {
		return err
	}
	defer remove()
	if err := k.stop(ctx, *c); err != nil {
		return err
	}
	pod, cleanup, err := k.job(ctx, *c, name)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, *c, pod, "sh", "-ec", "umask 077; mkdir -p /tmp/theia-operator; cp /run/recovery/recovery.txt /tmp/theia-operator/original.txt"); err != nil {
		return err
	}
	result, err := k.capture(ctx, *c, "exec", pod, "--", "theia", "instance", "recovery-history", "-original-recovery-file", "/tmp/theia-operator/original.txt", "-output", "/tmp/theia-operator/new-recovery.txt")
	if err != nil {
		return err
	}
	var recipients struct{ Previous, Recipient string }
	if json.Unmarshal(result, &recipients) != nil || recipients.Previous == "" || recipients.Recipient == "" {
		return fmt.Errorf("invalid recovery proof metadata")
	}
	temporary, err := os.MkdirTemp(filepath.Dir(output), ".theia-recovery-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	path := filepath.Join(temporary, "recovery.txt")
	if err := k.kubectl(ctx, *c, "cp", pod+":/tmp/theia-operator/new-recovery.txt", path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	for _, recipient := range []string{recipients.Previous, recipients.Recipient} {
		if err := instance.VerifyRecoveryFile(path, recipient); err != nil {
			return err
		}
	}
	if err := os.Link(path, output); err != nil {
		return err
	}
	if err := k.exec(ctx, *c, pod, "theia", "instance", "recovery-replace", "-recovery-file", "/tmp/theia-operator/new-recovery.txt"); err != nil {
		return err
	}
	cleanup()
	return k.start(ctx, *c)
}
