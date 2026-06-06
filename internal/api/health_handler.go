package api

// This file defines health handler HTTP handler behavior and request/response boundaries.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/polling"
	"github.com/lollinoo/theia/internal/repository/postgres"
)

type statusProvider interface {
	Status() string
	PollingHealth() polling.HealthSnapshot
}

// HealthHandler provides the health check endpoint.
type HealthHandler struct {
	db          *sql.DB
	poller      statusProvider
	environment string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB, poller statusProvider, environment string) *HealthHandler {
	environment = strings.ToLower(strings.TrimSpace(environment))
	if environment == "" {
		environment = "development"
	}
	return &HealthHandler{db: db, poller: poller, environment: environment}
}

// HandleHealth handles GET /api/v1/health
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if h.db == nil {
		dbStatus = "error"
	} else if err := h.db.Ping(); err != nil {
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
		"status":      overallStatus,
		"environment": h.environment,
		"components": map[string]string{
			"db":          dbStatus,
			"db_dialect":  postgres.DialectPostgres,
			"snmp_poller": pollerStatus,
		},
		"polling": pollingHealth,
	})
}
