package deployment

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/lollinoo/theia/internal/instance"
)

func (a *Admin) pause(ctx context.Context, frontend bool) error {
	if err := instance.WritePrivateFile(filepath.Join(a.Dir, "control", "public", "deployment-maintenance"), []byte("maintenance\n")); err != nil {
		return err
	}
	services := []string{"stop", "backend"}
	if frontend {
		services = append(services, "frontend")
	}
	return a.compose(ctx, services...)
}

func (a *Admin) startHTTP(ctx context.Context, c Config) error {
	if err := a.compose(ctx, "up", "-d", "--wait", "--wait-timeout", "120", "backend", "frontend"); err != nil {
		return err
	}
	check := a.ReadyCheck
	if check == nil {
		check = a.waitHTTPS
	}
	if err := check(ctx, c); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(a.Dir, "control", "public", "deployment-maintenance")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// waitHTTPS verifies the actual published proxy and certificate, rather than
// reporting success merely because a container process is running.
func (a *Admin) waitHTTPS(ctx context.Context, c Config) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	caReady := c.TLSMode != "internal"
	runner := a.Runner
	if runner == nil {
		runner = DockerRunner
	}
	transport := &http.Transport{Proxy: http.ProxyFromEnvironment, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		if !caReady {
			var ca bytes.Buffer
			args := []string{"compose", "-f", filepath.Join(a.Dir, "compose.yaml"), "exec", "-T", "frontend", "cat", "/data/caddy/pki/authorities/local/root.crt"}
			if runner(ctx, a.Dir, args, &ca) == nil {
				caReady = pool.AppendCertsFromPEM(ca.Bytes())
			}
		}
		if caReady {
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Origin()+"/readyz", nil)
			if err != nil {
				return err
			}
			response, err := client.Do(request)
			if err == nil {
				io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
				response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("published HTTPS readiness could not be verified; inspect the hostname, certificate and proxy, then run up: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
