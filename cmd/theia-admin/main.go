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
)

// Version is populated by the release build; development builds require -release.
var Version = ""

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: theia-admin install|up|upgrade|backup|restore|migrate|resume|activation|ca|status [options]")
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
	switch args[0] {
	case "install":
		return a.Install(ctx, installOptions)
	case "up":
		return a.Up(ctx)
	case "upgrade":
		return a.Upgrade(ctx, *release, *backend, *frontend)
	case "backup":
		return a.Backup(ctx, *output)
	case "migrate":
		if *rotate {
			return a.Offline(ctx, "migrate", "-rotate-credentials")
		}
		return a.Offline(ctx, "migrate")
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
