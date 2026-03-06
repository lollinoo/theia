package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/azmin/mikrotik-theia/internal/worker"
)

// HealthHandler provides the health check endpoint.
type HealthHandler struct {
	db     *sql.DB
	poller *worker.Poller
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(db *sql.DB, poller *worker.Poller) *HealthHandler {
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

	overallStatus := "ok"
	if dbStatus != "ok" {
		overallStatus = "degraded"
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": overallStatus,
		"components": map[string]string{
			"db":          dbStatus,
			"snmp_poller": pollerStatus,
		},
	})
}
