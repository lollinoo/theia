package deployment

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	"github.com/lollinoo/theia/internal/instance"
	"gopkg.in/yaml.v3"
)

// RenderCompose produces a complete deployment. Operational secrets travel only
// through private persistent mounts, and neither the API nor PostgreSQL is published.
func RenderCompose(dir string, c Config) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if strings.Contains(dir, ":") || strings.Contains(dir, "$") {
		return nil, fmt.Errorf("deployment path cannot contain ':' or '$'")
	}
	control := filepath.Join(dir, "control")
	dataSource := filepath.Join(dir, "data")
	if c.DataVolume != "" {
		dataSource = c.DataVolume
	}
	volumes := map[string]any{}
	if c.DataVolume != "" {
		volumes[c.DataVolume] = map[string]any{"external": true, "name": c.DataVolume}
	}
	if c.PostgresVolume != "" {
		volumes[c.PostgresVolume] = map[string]any{"external": true, "name": c.PostgresVolume}
	}
	backend := map[string]any{
		"image": c.BackendImage, "entrypoint": []string{"theia"}, "restart": "unless-stopped", "init": true,
		"environment": map[string]string{"THEIA_INSTANCE_STATE": "/control/state.json", "THEIA_DATA_DIR": "/data", "THEIA_INSTANCE_BACKUP_DIR": "/data/instance-backups", "THEIA_DEPLOYMENT_ENV": "production", "THEIA_ALLOWED_ORIGINS": c.Origin(), "THEIA_LISTEN_ADDR": ":8080", "THEIA_RELEASE_TAG": c.Release},
		"volumes":     []string{control + ":/control", dataSource + ":/data"},
		"healthcheck": map[string]any{"test": []string{"CMD", "curl", "-fsS", "http://localhost:8080/readyz"}, "interval": "5s", "timeout": "4s", "retries": 12, "start_period": "10s"},
	}
	// Existing data volumes can carry certificates too. A subpath mount avoids
	// giving the frontend access to database artifacts or encrypted credentials.
	certificateMount := map[string]any{"type": "bind", "source": filepath.Join(dir, "data", "certificates"), "target": "/data"}
	if c.DataVolume != "" {
		certificateMount = map[string]any{"type": "volume", "source": c.DataVolume, "target": "/data", "volume": map[string]any{"subpath": "certificates"}}
	}
	address := strings.TrimPrefix(c.Origin(), "https://")
	tlsDirective := ""
	switch c.TLSMode {
	case "internal":
		tlsDirective = "tls internal"
	case "external":
		tlsDirective = "tls /data/external/cert.pem /data/external/key.pem"
	case "proxy":
		address = ":80"
	}
	ports := []string{net.JoinHostPort(c.BindAddress, fmt.Sprint(c.HTTPPort)) + ":80"}
	if c.TLSMode != "proxy" {
		ports = append(ports, net.JoinHostPort(c.BindAddress, fmt.Sprint(c.HTTPSPort))+":"+fmt.Sprint(c.HTTPSPort))
	}
	frontend := map[string]any{
		"image": c.FrontendImage, "restart": "unless-stopped", "ports": ports,
		"environment": map[string]string{"THEIA_SITE_ADDRESS": address, "THEIA_TLS_DIRECTIVE": tlsDirective},
		"volumes":     []any{certificateMount, control + "/public:/status:ro"},
		"depends_on":  map[string]any{"backend": map[string]string{"condition": "service_healthy"}},
	}
	if len(c.TrustedProxies) > 0 {
		frontend["environment"].(map[string]string)["THEIA_TRUSTED_PROXIES"] = strings.Join(c.TrustedProxies, " ")
	}
	services := map[string]any{"backend": backend, "frontend": frontend}
	if c.BundledPostgres {
		pgSource := filepath.Join(dir, fmt.Sprintf("postgres-%d", c.PostgresMajor))
		if c.PostgresVolume != "" {
			pgSource = c.PostgresVolume
		}
		pgTarget := "/var/lib/postgresql"
		if c.PostgresMajor == 17 {
			pgTarget += "/data"
		}
		services["postgres"] = map[string]any{"image": fmt.Sprintf("postgres:%d-bookworm", c.PostgresMajor), "restart": "unless-stopped", "environment": map[string]string{"POSTGRES_USER": "theia", "POSTGRES_DB": "theia", "POSTGRES_PASSWORD_FILE": "/run/secrets/postgres_password"}, "secrets": []string{"postgres_password"}, "volumes": []string{pgSource + ":" + pgTarget}, "healthcheck": map[string]any{"test": []string{"CMD", "pg_isready", "-U", "theia", "-d", "theia"}, "interval": "3s", "timeout": "3s", "retries": 30}}
		backend["depends_on"] = map[string]any{"postgres": map[string]string{"condition": "service_healthy"}}
	}
	compose := map[string]any{"name": c.Project, "services": services}
	if c.BundledPostgres {
		compose["secrets"] = map[string]any{"postgres_password": map[string]string{"file": filepath.Join(control, "postgres-password.txt")}}
	}
	if len(volumes) > 0 {
		compose["volumes"] = volumes
	}
	return yaml.Marshal(compose)
}

func WriteCompose(dir string, c Config) error {
	data, err := RenderCompose(dir, c)
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(filepath.Join(dir, "compose.yaml"), data)
}
