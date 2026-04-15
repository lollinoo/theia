package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/metrics"
)

// PrometheusHandler provides HTTP handlers for Prometheus integration status.
type PrometheusHandler struct {
	settingsRepo domain.SettingsRepository
}

// NewPrometheusHandler creates a new PrometheusHandler.
func NewPrometheusHandler(settingsRepo domain.SettingsRepository) *PrometheusHandler {
	return &PrometheusHandler{settingsRepo: settingsRepo}
}

type prometheusHealthResponse struct {
	Enabled   bool   `json:"enabled"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
	Error     string `json:"error,omitempty"`
}

// HandleHealth handles GET /api/v1/prometheus/health
// It checks whether the configured Prometheus URL is set and reachable.
func (h *PrometheusHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	promURL, err := h.settingsRepo.Get(domain.SettingPrometheusURL)
	promURL = strings.TrimSpace(promURL)
	if err != nil || promURL == "" {
		json.NewEncoder(w).Encode(prometheusHealthResponse{
			Enabled:   false,
			Available: false,
			URL:       "",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	client := metrics.NewPromClient(promURL, nil)
	if err := client.CheckHealth(ctx); err != nil {
		json.NewEncoder(w).Encode(prometheusHealthResponse{
			Enabled:   true,
			Available: false,
			URL:       promURL,
			Error:     err.Error(),
		})
		return
	}

	json.NewEncoder(w).Encode(prometheusHealthResponse{
		Enabled:   true,
		Available: true,
		URL:       promURL,
	})
}
