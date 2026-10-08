// Theia's standalone administration entrypoint needs only Docker and Compose on
// the target host. It uses the backend image for all database maintenance work.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lollinoo/theia/internal/deployment"
	"github.com/lollinoo/theia/internal/instance"
)

// Version is populated by the release build; development builds require -release.
var Version = ""

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: theia-admin install|import|up|upgrade|postgres-upgrade|backup|restore|convert-legacy|migrate|rotate-operational|rotate-recovery|s3|resume|activation|ca|status [options]")
	}
	f := flag.NewFlagSet("theia-admin "+args[0], flag.ContinueOnError)
	f.SetOutput(out)
	dir := f.String("dir", "./theia-instance", "Persistent instance directory")
	release := f.String("release", Version, "Pinned release, for example v1.8.0")
	hostname := f.String("hostname", "localhost", "LAN hostname, IP address or public domain")
	tlsMode := f.String("tls", "", "TLS: internal, auto, external or proxy (default selected from hostname)")
	bind := f.String("bind-address", "0.0.0.0", "Published frontend bind address")
	httpPort := f.Int("http-port", 80, "Published HTTP port")
	httpsPort := f.Int("https-port", 443, "Published HTTPS port")
	dsn := f.String("database-dsn-file", "", "External PostgreSQL connection file; bundled PostgreSQL by default")
	cert := f.String("tls-cert-file", "", "External certificate PEM file")
	key := f.String("tls-key-file", "", "External private key PEM file")
	trusted := f.String("trusted-proxies", "", "Comma-separated proxy CIDRs for external TLS termination")
	backend := f.String("backend-image", "", "Override pinned backend image")
	frontend := f.String("frontend-image", "", "Override pinned frontend image")
	archive := f.String("archive", "", "Encrypted instance backup")
	recovery := f.String("recovery-file", "", "Administrator recovery file, read transiently")
	output := f.String("output", "", "Output file for an encrypted backup or public CA certificate")
	rotate := f.Bool("rotate-credentials", false, "Force credential key rotation during migration")
	offline := f.Bool("offline", false, "Use images already loaded on this Docker host")
	postgresTarget := f.Int("postgres-target", 18, "Target bundled PostgreSQL major version")
	platform := f.String("platform", "docker", "Deployment platform: docker or kubernetes")
	kubeContext := f.String("kube-context", "", "Explicit configured Kubernetes context")
	namespace := f.String("namespace", "theia", "Kubernetes namespace")
	name := f.String("name", "theia", "Kubernetes Helm release name")
	ingressClass := f.String("ingress-class", "", "Cluster Ingress class")
	tlsSecret := f.String("tls-secret", "", "Existing or cluster-managed Ingress TLS Secret")
	storageClass := f.String("storage-class", "", "Kubernetes storage class (default cluster class)")
	storageSize := f.String("storage-size", "10Gi", "Persistent volume capacity")
	originalCompose := f.String("compose-file", "", "Original Compose file for legacy import")
	originalEnv := f.String("env-file", "", "Original Compose environment file")
	originalConfig := f.String("config", "config.yaml", "Original configuration path inside the backend container")
	s3Endpoint := f.String("endpoint", "", "S3 endpoint")
	s3Bucket := f.String("bucket", "", "S3 bucket")
	s3Region := f.String("region", "", "S3 region")
	s3Prefix := f.String("prefix", "theia", "S3 prefix")
	s3Access := f.String("access-key-file", "", "Private S3 access key file")
	s3Secret := f.String("secret-key-file", "", "Private S3 secret key file")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Minute)
	defer cancel()
	a := &deployment.Admin{Dir: *dir, Output: out, OfflineImages: *offline}
	c := deployment.Config{Release: *release, Hostname: *hostname, TLSMode: *tlsMode, BindAddress: *bind, HTTPPort: *httpPort, HTTPSPort: *httpsPort, BackendImage: *backend, FrontendImage: *frontend}
	if *trusted != "" {
		c.TrustedProxies = strings.Split(*trusted, ",")
	}
	installOptions := deployment.InstallOptions{Config: c, DatabaseDSNFile: *dsn, TLSCertFile: *cert, TLSKeyFile: *key}
	k := &deployment.Kubernetes{Dir: *dir, Output: out}
	kubeConfig, kubeErr := k.Load()
	if kubeErr != nil && !os.IsNotExist(kubeErr) {
		return kubeErr
	}
	if *platform == "kubernetes" || kubeErr == nil {
		_ = kubeConfig
		kc := deployment.KubernetesConfig{Name: *name, Namespace: *namespace, Context: *kubeContext, Release: *release, BackendImage: *backend, FrontendImage: *frontend, Hostname: *hostname, IngressClass: *ingressClass, TLSSecret: *tlsSecret, StorageClass: *storageClass, StorageSize: *storageSize, TrustedProxies: strings.Join(strings.Split(*trusted, ","), " ")}
		switch args[0] {
		case "install":
			return k.Install(ctx, kc, *dsn, false)
		case "up":
			return k.Up(ctx)
		case "status":
			return k.Status(ctx)
		case "activation":
			return k.Activation(ctx)
		case "backup":
			return k.Backup(ctx, *output)
		case "restore":
			if *archive == "" || *recovery == "" {
				return fmt.Errorf("-archive and -recovery-file are required")
			}
			if os.IsNotExist(kubeErr) {
				if err := k.Install(ctx, kc, *dsn, true); err != nil {
					return err
				}
			} else if kubeErr != nil {
				return kubeErr
			}
			return k.Restore(ctx, *archive, *recovery)
		case "migrate":
			if *rotate {
				return k.Offline(ctx, "migrate", "-rotate-credentials")
			}
			return k.Offline(ctx, "migrate")
		case "resume":
			return k.Resume(ctx)
		case "upgrade":
			return k.Upgrade(ctx, *release, *backend, *frontend)
		case "rotate-operational":
			return k.Offline(ctx, "rotate-operational")
		case "rotate-recovery":
			return k.RotateRecovery(ctx, *recovery, *output)
		case "s3":
			return k.ConfigureS3(ctx, instance.S3Config{Endpoint: *s3Endpoint, Bucket: *s3Bucket, Region: *s3Region, Prefix: *s3Prefix}, *s3Access, *s3Secret)
		default:
			return fmt.Errorf("unsupported Kubernetes administration command %q", args[0])
		}
	}
	if *platform != "docker" {
		return fmt.Errorf("platform must be docker or kubernetes")
	}
	switch args[0] {
	case "install":
		return a.Install(ctx, installOptions)
	case "import":
		if *originalCompose == "" || *output == "" {
			return fmt.Errorf("-compose-file and -output for the new operator recovery file are required")
		}
		return a.Import(ctx, deployment.ImportOptions{Config: c, ComposeFile: *originalCompose, EnvFile: *originalEnv, ConfigPath: *originalConfig, RecoveryOutput: *output})
	case "up":
		return a.Up(ctx)
	case "upgrade":
		return a.Upgrade(ctx, *release, *backend, *frontend)
	case "postgres-upgrade":
		return a.UpgradePostgres(ctx, *postgresTarget)
	case "backup":
		return a.Backup(ctx, *output)
	case "migrate":
		if *rotate {
			return a.Offline(ctx, "migrate", "-rotate-credentials")
		}
		return a.Offline(ctx, "migrate")
	case "rotate-operational":
		return a.Offline(ctx, "rotate-operational")
	case "convert-legacy":
		if *archive == "" || *output == "" {
			return fmt.Errorf("-archive and -output are required")
		}
		return a.ConvertLegacy(ctx, *archive, *output)
	case "s3":
		return a.ConfigureS3(ctx, instance.S3Config{Endpoint: *s3Endpoint, Bucket: *s3Bucket, Region: *s3Region, Prefix: *s3Prefix}, *s3Access, *s3Secret)
	case "rotate-recovery":
		if *recovery == "" || *output == "" {
			return fmt.Errorf("-recovery-file and -output are required")
		}
		return a.RotateRecovery(ctx, *recovery, *output)
	case "resume":
		return a.Resume(ctx)
	case "restore":
		if *archive == "" || *recovery == "" {
			return fmt.Errorf("-archive and -recovery-file are required")
		}
		if _, err := deployment.LoadConfig(*dir); os.IsNotExist(err) {
			installOptions.PrepareRestore = true
			if err := a.Install(ctx, installOptions); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		return a.Restore(ctx, *archive, *recovery)
	case "activation":
		return a.Activation(ctx)
	case "ca":
		if *output == "" {
			return fmt.Errorf("-output is required")
		}
		return a.ExportCA(ctx, *output)
	case "status":
		return a.Status(ctx)
	default:
		return fmt.Errorf("unknown administration command %q", args[0])
	}
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "theia-admin:", err)
		os.Exit(1)
	}
}
