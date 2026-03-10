package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/azmin/mikrotik-theia/internal/domain"
	"github.com/azmin/mikrotik-theia/internal/service"
	"github.com/azmin/mikrotik-theia/internal/worker"
	"github.com/azmin/mikrotik-theia/internal/ws"
)

// NewRouter creates the HTTP handler with all /api/v1/ routes registered.
// Uses standard net/http (no framework needed at this scale).
func NewRouter(
	db *sql.DB,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	settingsRepo domain.SettingsRepository,
	poller *worker.Poller,
	wsHandler *ws.Handler,
) http.Handler {
	mux := http.NewServeMux()

	deviceHandler := NewDeviceHandler(deviceService)
	linkHandler := NewLinkHandler(linkRepo, deviceService)
	positionHandler := NewPositionHandler(positionRepo)
	settingsHandler := NewSettingsHandler(settingsRepo)
	healthHandler := NewHealthHandler(db, poller)

	// Device routes
	mux.HandleFunc("/api/v1/devices", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			deviceHandler.HandleCreate(w, r)
		case http.MethodGet:
			deviceHandler.HandleList(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/api/v1/devices/batch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		deviceHandler.HandleBatchAdd(w, r)
	})

	// Device by ID routes (must be registered after /devices/batch to avoid conflicts)
	mux.HandleFunc("/api/v1/devices/", func(w http.ResponseWriter, r *http.Request) {
		// Check if this is an interfaces request
		if strings.HasSuffix(r.URL.Path, "/interfaces") && r.Method == http.MethodGet {
			linkHandler.HandleGetInterfaces(w, r)
			return
		}

		// Check if this is a probe request
		if len(r.URL.Path) > len("/api/v1/devices/") {
			pathSuffix := r.URL.Path[len("/api/v1/devices/"):]
			if idx := indexOf(pathSuffix, "/probe"); idx >= 0 && r.Method == http.MethodPost {
				deviceHandler.HandleProbe(w, r)
				return
			}
		}

		switch r.Method {
		case http.MethodGet:
			deviceHandler.HandleGet(w, r)
		case http.MethodPut:
			deviceHandler.HandleUpdate(w, r)
		case http.MethodDelete:
			deviceHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Links routes
	mux.HandleFunc("/api/v1/links", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			linkHandler.HandleList(w, r)
		case http.MethodPost:
			linkHandler.HandleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Link by ID routes (PUT and DELETE)
	mux.HandleFunc("/api/v1/links/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			linkHandler.HandleUpdate(w, r)
		case http.MethodDelete:
			linkHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Position routes
	mux.HandleFunc("/api/v1/positions", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			positionHandler.HandleList(w, r)
		case http.MethodPut:
			positionHandler.HandleSaveAll(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Settings routes
	mux.HandleFunc("/api/v1/settings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		settingsHandler.HandleGetAll(w, r)
	})

	mux.HandleFunc("/api/v1/settings/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		settingsHandler.HandleUpdate(w, r)
	})

	// Health endpoint
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler.HandleHealth(w, r)
	})

	if wsHandler != nil {
		mux.Handle("/api/v1/ws", wsHandler)
	}

	// Apply middleware chain: CORS -> Logger -> JSON Content-Type
	var handler http.Handler = mux
	handler = JSONContentType(handler)
	handler = RequestLogger(handler)
	handler = CORS(handler)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket upgrades must bypass the JSON/logger middleware chain because
		// the wrapped ResponseWriter does not expose the hijacker interface.
		if wsHandler != nil && r.URL.Path == "/api/v1/ws" {
			wsHandler.ServeHTTP(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
