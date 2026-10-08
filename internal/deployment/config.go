// Package deployment is the host adapter for the shared instance maintenance
// engine. It generates a self-contained Compose deployment without source tools.
package deployment

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lollinoo/theia/internal/instance"
)

const ConfigVersion = 1

// Config contains deployment coordinates, never passwords or private keys.
type Config struct {
	Version         int      `json:"version"`
	Project         string   `json:"project"`
	Release         string   `json:"release"`
	BackendImage    string   `json:"backend_image"`
	FrontendImage   string   `json:"frontend_image"`
	Hostname        string   `json:"hostname"`
	TLSMode         string   `json:"tls_mode"`
	HTTPPort        int      `json:"http_port"`
	HTTPSPort       int      `json:"https_port"`
	BindAddress     string   `json:"bind_address"`
	TrustedProxies  []string `json:"trusted_proxies,omitempty"`
	BundledPostgres bool     `json:"bundled_postgres"`
	PostgresMajor   int      `json:"postgres_major"`
	DataVolume      string   `json:"data_volume,omitempty"`
	PostgresVolume  string   `json:"postgres_volume,omitempty"`
}

var releasePattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`)
var imagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9./_:@-]*$`)
var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
var projectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (c Config) Validate() error {
	if c.Version != ConfigVersion || !projectPattern.MatchString(c.Project) {
		return fmt.Errorf("invalid deployment identity or version")
	}
	if !releasePattern.MatchString(c.Release) {
		return fmt.Errorf("an explicit versioned release is required, for example v1.8.0")
	}
	for _, image := range []string{c.BackendImage, c.FrontendImage} {
		parts := strings.Split(image, "/")
		if !imagePattern.MatchString(image) || !strings.Contains(parts[len(parts)-1], ":") || strings.HasSuffix(image, ":latest") || strings.HasSuffix(image, ":master") {
			return fmt.Errorf("deployment images must have explicit release tags or digests")
		}
	}
	if !hostnamePattern.MatchString(c.Hostname) && net.ParseIP(c.Hostname) == nil {
		return fmt.Errorf("hostname must be a DNS name or IP address")
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 || c.HTTPSPort < 1 || c.HTTPSPort > 65535 || c.HTTPPort == c.HTTPSPort {
		return fmt.Errorf("HTTP and HTTPS ports must be distinct valid ports")
	}
	if net.ParseIP(c.BindAddress) == nil {
		return fmt.Errorf("bind address must be an IP address")
	}
	switch c.TLSMode {
	case "auto", "internal", "external", "proxy":
	default:
		return fmt.Errorf("TLS mode must be auto, internal, external or proxy")
	}
	for _, cidr := range c.TrustedProxies {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("trusted proxy must be an explicit CIDR")
		}
	}
	if c.PostgresMajor != 17 && c.PostgresMajor != 18 {
		return fmt.Errorf("supported PostgreSQL versions are 17 and 18")
	}
	for _, volume := range []string{c.DataVolume, c.PostgresVolume} {
		if volume != "" && !projectPattern.MatchString(volume) {
			return fmt.Errorf("invalid existing volume name")
		}
	}
	return nil
}

func (c Config) Origin() string {
	host := c.Hostname
	if c.HTTPSPort != 443 {
		host = net.JoinHostPort(host, fmt.Sprint(c.HTTPSPort))
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return (&url.URL{Scheme: "https", Host: host}).String()
}

func configPath(dir string) string { return filepath.Join(dir, "control", "deployment.json") }
func StatePath(dir string) string  { return filepath.Join(dir, "control", "state.json") }

func SaveConfig(dir string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return instance.WritePrivateFile(configPath(dir), append(data, '\n'))
}

func LoadConfig(dir string) (*Config, error) {
	f, err := os.Open(configPath(dir))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var c Config
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("invalid deployment configuration")
	}
	return &c, c.Validate()
}

func selectTLS(hostname, mode string) string {
	if mode != "" {
		return mode
	}
	if net.ParseIP(hostname) != nil || !strings.Contains(hostname, ".") || strings.HasSuffix(hostname, ".local") || strings.HasSuffix(hostname, ".lan") || strings.HasSuffix(hostname, ".internal") {
		return "internal"
	}
	return "auto"
}
