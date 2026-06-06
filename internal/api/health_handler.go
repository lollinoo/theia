package api

// This file defines health handler HTTP handler behavior and request/response boundaries.

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/version"
)

type statusProvider interface {
	Status() string
	PollingHealth() polling.HealthSnapshot
}

// HealthHandler provides the health check endpoint.
type HealthHandler struct {
	db     *sql.DB
	poller statusProvider
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB, poller statusProvider) *HealthHandler {
	return &HealthHandler{db: db, poller: poller}
}

// HandleHealth handles GET /api/v1/health
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := h.db.Ping(); err != nil {
		dbStatus = "error"
	}

	pollerStatus := "stopped"
	if h.poller != nil {
		pollerStatus = h.poller.Status()
	}
	pollingHealth := polling.HealthSnapshot{}
	if h.poller != nil {
		pollingHealth = h.poller.PollingHealth()
	}

	overallStatus := "ok"
	if dbStatus != "ok" {
		overallStatus = "degraded"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": overallStatus,
		"version": map[string]string{
			"version":    version.Version,
			"git_commit": version.GitCommit,
			"build_date": version.BuildDate,
		},
		"components": map[string]string{
			"db":          dbStatus,
			"db_dialect":  postgres.DialectPostgres,
			"snmp_poller": pollerStatus,
		},
		"polling": pollingHealth,
	})
}
