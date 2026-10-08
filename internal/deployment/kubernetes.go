package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/instance"
	"gopkg.in/yaml.v3"
)

// KubernetesConfig contains only coordinates. Kubernetes PVC state remains
// authoritative; the host does not keep a second mutable copy of its secrets.
type KubernetesConfig struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	Context          string `json:"context"`
	Release          string `json:"release"`
	BackendImage     string `json:"backend_image"`
	FrontendImage    string `json:"frontend_image"`
	Hostname         string `json:"hostname"`
	IngressClass     string `json:"ingress_class"`
	TLSSecret        string `json:"tls_secret"`
	StorageClass     string `json:"storage_class"`
	StorageSize      string `json:"storage_size"`
	ExternalPostgres bool   `json:"external_postgres"`
	DisableIngress   bool   `json:"disable_ingress,omitempty"`
	PrepareRestore   bool   `json:"prepare_restore,omitempty"`
	TrustedProxies   string `json:"trusted_proxies,omitempty"`
}

var kubernetesName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func (c KubernetesConfig) Validate() error {
	if !kubernetesName.MatchString(c.Name) || !kubernetesName.MatchString(c.Namespace) || c.Context == "" {
		return fmt.Errorf("Kubernetes requires an explicit context and valid release and namespace names")
	}
	if !releasePattern.MatchString(c.Release) {
		return fmt.Errorf("a pinned Kubernetes release is required")
	}
	for _, image := range []string{c.BackendImage, c.FrontendImage} {
		parts := strings.Split(image, "/")
		if !imagePattern.MatchString(image) || !strings.Contains(parts[len(parts)-1], ":") || strings.HasSuffix(image, ":latest") || strings.HasSuffix(image, ":master") {
			return fmt.Errorf("invalid image")
		}
	}
	if !hostnamePattern.MatchString(c.Hostname) {
		return fmt.Errorf("a DNS hostname is required")
	}
	if !c.DisableIngress && !kubernetesName.MatchString(c.TLSSecret) {
		return fmt.Errorf("an existing or cluster-managed TLS Secret name is required")
	}
	return nil
}

// Kubernetes adapts maintenance to exclusive Jobs on persistent instance storage.
type Kubernetes struct {
	Dir    string
	Output io.Writer
	Runner func(context.Context, string, []string, io.Writer) error
}

func (k *Kubernetes) execute(ctx context.Context, program string, args []string, out io.Writer) error {
	if k.Runner != nil {
		return k.Runner(ctx, program, args, out)
	}
	command := exec.CommandContext(ctx, program, args...)
	command.Stdout = out
	command.Stderr = out
	return command.Run()
}

func (k *Kubernetes) kubectl(ctx context.Context, c KubernetesConfig, args ...string) error {
	return k.execute(ctx, "kubectl", append([]string{"--context", c.Context, "--namespace", c.Namespace}, args...), k.Output)
}

func (k *Kubernetes) save(c KubernetesConfig) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(filepath.Join(k.Dir, "control", "kubernetes.json"), data)
}

func (k *Kubernetes) Load() (*KubernetesConfig, error) {
	data, err := os.ReadFile(filepath.Join(k.Dir, "control", "kubernetes.json"))
	if err != nil {
		return nil, err
	}
	var c KubernetesConfig
	if json.Unmarshal(data, &c) != nil {
		return nil, fmt.Errorf("invalid Kubernetes configuration")
	}
	return &c, c.Validate()
}

func (k *Kubernetes) applyObject(ctx context.Context, c KubernetesConfig, object any) error {
	temporary, err := os.MkdirTemp("", "theia-kubernetes-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	data, err := json.Marshal(object)
	if err != nil {
		return err
	}
	file := filepath.Join(temporary, "object.json")
	if err := instance.WritePrivateFile(file, data); err != nil {
		return err
	}
	return k.kubectl(ctx, c, "apply", "-f", file)
}

func (k *Kubernetes) applySecret(ctx context.Context, c KubernetesConfig, name string, data map[string]string) error {
	return k.applyObject(ctx, c, map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]string{"name": name}, "type": "Opaque", "stringData": data})
}

func (k *Kubernetes) helm(ctx context.Context, c KubernetesConfig) error {
	temporary, err := os.MkdirTemp("", "theia-chart-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	chart := filepath.Join(temporary, "chart")
	if err := ExportChart(chart); err != nil {
		return err
	}
	values := map[string]any{"maintenanceMode": c.PrepareRestore, "releaseTag": c.Release, "backendImage": c.BackendImage, "frontendImage": c.FrontendImage, "bootstrapSecret": c.Name + "-bootstrap", "databaseSecret": c.Name + "-database", "persistence": map[string]string{"storageClass": c.StorageClass, "size": c.StorageSize}, "postgres": map[string]any{"bundled": !c.ExternalPostgres, "storageClass": c.StorageClass, "size": c.StorageSize}, "trustedProxies": c.TrustedProxies, "ingress": map[string]any{"enabled": !c.DisableIngress, "hostname": c.Hostname, "className": c.IngressClass, "tlsSecret": c.TLSSecret}}
	data, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	file := filepath.Join(temporary, "values.yaml")
	if err := instance.WritePrivateFile(file, data); err != nil {
		return err
	}
	args := []string{"upgrade", "--install", c.Name, chart, "--kube-context", c.Context, "--namespace", c.Namespace, "-f", file, "--timeout", "5m"}
	// WaitForFirstConsumer claims bind only when the bootstrap Job is created.
	if !c.PrepareRestore {
		args = append(args, "--wait")
	}
	return k.execute(ctx, "helm", args, k.Output)
}

// Install hands state to an optional one-time Secret. It never regenerates a
// missing PVC's keys from an old bootstrap copy after a successful installation.
func (k *Kubernetes) Install(ctx context.Context, c KubernetesConfig, dsnFile string, prepareRestore bool) error {
	if _, err := os.Lstat(filepath.Join(k.Dir, "control", "kubernetes.json")); !os.IsNotExist(err) {
		return fmt.Errorf("Kubernetes installation already exists; use up or resume")
	}
	if c.Name == "" {
		c.Name = "theia"
	}
	if c.Namespace == "" {
		c.Namespace = "theia"
	}
	if c.StorageSize == "" {
		c.StorageSize = "10Gi"
	}
	if c.BackendImage == "" {
		c.BackendImage = "ghcr.io/lollinoo/theia-backend:" + c.Release
	}
	if c.FrontendImage == "" {
		c.FrontendImage = "ghcr.io/lollinoo/theia-frontend:" + c.Release
	}
	c.ExternalPostgres = dsnFile != ""
	c.PrepareRestore = prepareRestore
	if err := c.Validate(); err != nil {
		return err
	}
	if err := k.applyObject(ctx, c, map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]string{"name": c.Namespace}}); err != nil {
		return err
	}
	// Refuse adoption of a pre-existing claim; import and restore are explicit.
	for _, claim := range []string{c.Name + "-instance", c.Name + "-postgres"} {
		data, err := k.capture(ctx, c, "get", "pvc", claim, "--ignore-not-found", "-o", "name")
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			return fmt.Errorf("instance PVC already exists; installation cannot replace its secrets")
		}
	}
	state, err := instance.Generate(time.Now())
	if err != nil {
		return err
	}
	if c.ExternalPostgres {
		data, err := readInputFile(dsnFile, 64<<10)
		if err != nil {
			return err
		}
		state.DBDSN = string(data)
	} else {
		connection := &url.URL{Scheme: "postgres", Host: c.Name + "-postgres:5432", Path: "/theia", User: url.UserPassword("theia", state.DatabasePassword), RawQuery: "sslmode=disable"}
		state.DBDSN = connection.String()
	}
	token, err := state.BeginActivation(time.Now())
	if err != nil {
		return err
	}
	state.DeploymentMetadata, err = json.Marshal(c)
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := k.applySecret(ctx, c, c.Name+"-bootstrap", map[string]string{"state.json": string(data)}); err != nil {
		return err
	}
	if !c.ExternalPostgres {
		if err := k.applySecret(ctx, c, c.Name+"-database", map[string]string{"password": state.DatabasePassword}); err != nil {
			return err
		}
	}
	if err := k.save(c); err != nil {
		return err
	}
	initial := c
	initial.PrepareRestore = true
	if err := k.helm(ctx, initial); err != nil {
		return fmt.Errorf("Kubernetes installation is incomplete; inspect init-container logs, then run up: %w", err)
	}
	if !c.ExternalPostgres {
		if err := k.kubectl(ctx, c, "rollout", "status", "deployment/"+c.Name+"-postgres", "--timeout=180s"); err != nil {
			return err
		}
	}
	pod, cleanup, err := k.job(ctx, c, "")
	if err != nil {
		return err
	}
	defer cleanup()
	if err := k.exec(ctx, c, pod, "theia", "instance", "status"); err != nil {
		return err
	}
	cleanup()
	if err := k.kubectl(ctx, c, "delete", "secret", c.Name+"-bootstrap", "--ignore-not-found"); err != nil {
		return err
	}
	if prepareRestore {
		return nil
	}
	if err := k.helm(ctx, c); err != nil {
		return err
	}
	_, err = fmt.Fprintf(k.Output, "Kubernetes instance is ready. Open https://%s/activate#token=%s within one hour. HTTPS is provided by the configured cluster Ingress and TLS Secret.\n", c.Hostname, token)
	return err
}

func (k *Kubernetes) Up(ctx context.Context) error {
	c, err := k.Load()
	if err != nil {
		return err
	}
	if err := k.helm(ctx, *c); err != nil {
		return err
	}
	return k.kubectl(ctx, *c, "delete", "secret", c.Name+"-bootstrap", "--ignore-not-found")
}

func (k *Kubernetes) stop(ctx context.Context, c KubernetesConfig) error {
	pods, err := k.capture(ctx, c, "get", "pods", "-l", "app="+c.Name+"-backend", "-o", "name")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(pods)) > 0 {
		if err := k.kubectl(ctx, c, "exec", "deployment/"+c.Name+"-backend", "-c", "backend", "--pod-running-timeout=10s", "--", "sh", "-ec", "umask 077; printf maintenance > /persist/control/public/deployment-maintenance"); err != nil {
			// A failed backend/init may already be offline; scaling still excludes it.
			fmt.Fprintln(k.Output, "Backend is unavailable; stopping any remaining writer before maintenance.")
		}
	}
	if err := k.kubectl(ctx, c, "scale", "deployment/"+c.Name+"-backend", "--replicas=0"); err != nil {
		return err
	}
	pods, err = k.capture(ctx, c, "get", "pods", "-l", "app="+c.Name+"-backend", "-o", "name")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(pods)) == 0 {
		return nil
	}
	return k.kubectl(ctx, c, "wait", "--for=delete", "pod", "-l", "app="+c.Name+"-backend", "--timeout=120s")
}

func (k *Kubernetes) start(ctx context.Context, c KubernetesConfig) error {
	if err := k.kubectl(ctx, c, "scale", "deployment/"+c.Name+"-backend", "--replicas=1"); err != nil {
		return err
	}
	if err := k.kubectl(ctx, c, "rollout", "status", "deployment/"+c.Name+"-backend", "--timeout=180s"); err != nil {
		return err
	}
	return k.kubectl(ctx, c, "exec", "deployment/"+c.Name+"-backend", "--", "rm", "-f", "/persist/control/public/deployment-maintenance")
}
