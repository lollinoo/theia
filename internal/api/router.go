package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/sqlite"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// NewRouter creates the HTTP handler with all /api/v1/ routes registered.
// Uses standard net/http (no framework needed at this scale).
func NewRouter(
	db *sql.DB,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	settingsRepo domain.SettingsRepository,
	snmpProfileRepo domain.SNMPProfileRepository,
	credentialProfileRepo *sqlite.CredentialProfileRepo,
	areaRepo domain.AreaRepository,
	backupService *service.BackupService,
	vendorRegistry *vendor.Registry,
	vendorConfigRepo domain.VendorConfigRepository,
	poller statusProvider,
	instanceBackupService *service.InstanceBackupService,
	bridgeBinariesDir string,
	wsHandler *ws.Handler,
) http.Handler {
	mux := http.NewServeMux()

	deviceHandler := NewDeviceHandler(deviceService, credentialProfileRepo, vendorRegistry)
	linkHandler := NewLinkHandler(linkRepo, deviceService)
	positionHandler := NewPositionHandler(positionRepo)
	canvasTopologyHandler := NewCanvasTopologyHandler(
		deviceService,
		linkRepo,
		positionRepo,
		areaRepo,
		vendorRegistry,
	)
	settingsHandler := NewSettingsHandler(settingsRepo)
	snmpProfileHandler := NewSNMPProfileHandler(snmpProfileRepo)
	areaHandler := NewAreaHandler(areaRepo)
	backupHandler := NewBackupHandler(backupService, settingsRepo)
	credentialProfileHandler := NewCredentialProfileHandler(backupService, credentialProfileRepo)
	deviceCredHandler := NewDeviceCredentialProfileHandler(backupService, credentialProfileRepo)
	vendorHandler := NewVendorHandler(vendorRegistry, vendorConfigRepo)
	healthHandler := NewHealthHandler(db, poller)
	prometheusHandler := NewPrometheusHandler(settingsRepo)
	instanceBackupHandler := NewInstanceBackupHandler(instanceBackupService)
	bridgeHandler := NewBridgeHandlerWithCredentials(bridgeBinariesDir, backupService, credentialProfileRepo)

	// Canvas topology read model route
	mux.HandleFunc("/api/v1/topology/canvas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		canvasTopologyHandler.HandleGet(w, r)
	})

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

		// SNMP test route
		if strings.HasSuffix(r.URL.Path, "/snmp-test") && r.Method == http.MethodPost {
			deviceHandler.HandleTestSNMP(w, r)
			return
		}

		if strings.HasSuffix(r.URL.Path, "/topology-discovery") && r.Method == http.MethodPost {
			deviceHandler.HandleRunTopologyDiscovery(w, r)
			return
		}

		// SSH test route (resolves credentials via profile)
		if strings.HasSuffix(r.URL.Path, "/ssh-credentials/test") && r.Method == http.MethodPost {
			backupHandler.HandleTestSSH(w, r)
			return
		}

		// Backup routes for devices
		if strings.HasSuffix(r.URL.Path, "/backups/latest") && r.Method == http.MethodGet {
			backupHandler.HandleGetLatestBackup(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/backups") {
			switch r.Method {
			case http.MethodGet:
				backupHandler.HandleListBackups(w, r)
			case http.MethodPost:
				backupHandler.HandleTriggerBackup(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}

		// Device credential profile assignment routes
		if strings.HasSuffix(r.URL.Path, "/credential-profiles") {
			switch r.Method {
			case http.MethodGet:
				deviceCredHandler.HandleListAssignments(w, r)
			case http.MethodPost:
				deviceCredHandler.HandleAssign(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}

		// Device credential profile unassign (DELETE with profileId in path)
		// Path: /api/v1/devices/{id}/credential-profiles/{profileId}
		if strings.Contains(r.URL.Path, "/credential-profiles/") && r.Method == http.MethodDelete {
			deviceCredHandler.HandleUnassign(w, r)
			return
		}

		// WinBox profile designation routes
		if strings.HasSuffix(r.URL.Path, "/winbox-profile") {
			switch r.Method {
			case http.MethodPut:
				deviceCredHandler.HandleSetWinbox(w, r)
			case http.MethodDelete:
				deviceCredHandler.HandleClearWinbox(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
			return
		}

		// WinBox credentials endpoint
		if strings.HasSuffix(r.URL.Path, "/winbox-credentials") && r.Method == http.MethodGet {
			deviceCredHandler.HandleGetWinboxCredentials(w, r)
			return
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
		switch r.Method {
		case http.MethodGet:
			settingsHandler.HandleGet(w, r)
		case http.MethodPut:
			settingsHandler.HandleUpdate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// SNMP credential profile routes
	mux.HandleFunc("/api/v1/snmp-profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			snmpProfileHandler.HandleList(w, r)
		case http.MethodPost:
			snmpProfileHandler.HandleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/api/v1/snmp-profiles/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			snmpProfileHandler.HandleGet(w, r)
		case http.MethodPut:
			snmpProfileHandler.HandleUpdate(w, r)
		case http.MethodDelete:
			snmpProfileHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Area routes
	mux.HandleFunc("/api/v1/areas", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			areaHandler.HandleList(w, r)
		case http.MethodPost:
			areaHandler.HandleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/api/v1/areas/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			areaHandler.HandleGet(w, r)
		case http.MethodPut:
			areaHandler.HandleUpdate(w, r)
		case http.MethodDelete:
			areaHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Credential profile routes
	mux.HandleFunc("/api/v1/credential-profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			credentialProfileHandler.HandleList(w, r)
		case http.MethodPost:
			credentialProfileHandler.HandleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/api/v1/credential-profiles/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/test") && r.Method == http.MethodPost {
			credentialProfileHandler.HandleTest(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			credentialProfileHandler.HandleGet(w, r)
		case http.MethodPut:
			credentialProfileHandler.HandleUpdate(w, r)
		case http.MethodDelete:
			credentialProfileHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Bulk backup routes
	mux.HandleFunc("/api/v1/backups/bulk", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleBulkBackup(w, r)
	})

	mux.HandleFunc("/api/v1/backups/bulk-download", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleBulkDownload(w, r)
	})

	// Backup job routes (by job ID)
	mux.HandleFunc("/api/v1/backup-jobs/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			backupHandler.HandleGetBackupJob(w, r)
		case http.MethodDelete:
			backupHandler.HandleDeleteBackupJob(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Backup file routes (download and content)
	mux.HandleFunc("/api/v1/backup-files/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/download") {
			backupHandler.HandleDownloadBackupFile(w, r)
		} else if strings.HasSuffix(r.URL.Path, "/content") {
			backupHandler.HandleGetBackupFileContent(w, r)
		} else {
			writeError(w, http.StatusNotFound, "not found")
		}
	})

	// Vendor config routes
	mux.HandleFunc("/api/v1/vendors", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		vendorHandler.HandleListVendors(w, r)
	})

	mux.HandleFunc("/api/v1/vendors/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			vendorHandler.HandleGetVendor(w, r)
		case http.MethodPut:
			vendorHandler.HandleUpdateVendor(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Instance backup routes
	mux.HandleFunc("/api/v1/instance-backups", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			instanceBackupHandler.HandleCreate(w, r)
		case http.MethodGet:
			instanceBackupHandler.HandleList(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Instance backup restore (multipart upload, bypass middleware)
	mux.HandleFunc("/api/v1/instance-backups/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		instanceBackupHandler.HandleRestore(w, r)
	})

	mux.HandleFunc("/api/v1/instance-backups/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") && r.Method == http.MethodGet {
			instanceBackupHandler.HandleDownload(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			instanceBackupHandler.HandleGet(w, r)
		case http.MethodDelete:
			instanceBackupHandler.HandleDelete(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	// Bridge binary download
	mux.HandleFunc("/api/v1/bridge/download/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		bridgeHandler.HandleDownload(w, r)
	})

	// Bridge credential token — encrypts WinBox credentials with the bridge's own secret
	mux.HandleFunc("/api/v1/bridge/token/", func(w http.ResponseWriter, r *http.Request) {
		bridgeHandler.HandleBridgeToken(w, r)
	})

	// Health endpoint
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		healthHandler.HandleHealth(w, r)
	})

	// Prometheus health endpoint
	mux.HandleFunc("/api/v1/prometheus/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		prometheusHandler.HandleHealth(w, r)
	})

	if wsHandler != nil {
		mux.Handle("/api/v1/ws", wsHandler)
	}

	// Apply middleware chain: CORS -> Logger -> MaxBodySize -> JSON Content-Type
	var handler http.Handler = mux
	handler = JSONContentType(handler)
	handler = MaxBodySize(1 << 20)(handler) // 1 MB limit
	handler = RequestLogger(handler)
	handler = CORS(handler)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WebSocket upgrades must bypass the JSON/logger middleware chain because
		// the wrapped ResponseWriter does not expose the hijacker interface.
		if wsHandler != nil && r.URL.Path == "/api/v1/ws" {
			wsHandler.ServeHTTP(w, r)
			return
		}

		// File download bypasses JSON content-type middleware
		if strings.HasSuffix(r.URL.Path, "/download") && strings.HasPrefix(r.URL.Path, "/api/v1/backup-files/") {
			CORS(RequestLogger(mux)).ServeHTTP(w, r)
			return
		}

		// Instance backup download bypasses JSON content-type and body size middleware
		if strings.HasSuffix(r.URL.Path, "/download") && strings.HasPrefix(r.URL.Path, "/api/v1/instance-backups/") {
			CORS(RequestLogger(mux)).ServeHTTP(w, r)
			return
		}

		// Instance backup restore bypasses body size and JSON content-type middleware (multipart upload)
		if r.URL.Path == "/api/v1/instance-backups/restore" && r.Method == http.MethodPost {
			CORS(RequestLogger(mux)).ServeHTTP(w, r)
			return
		}

		// Bridge binary download bypasses JSON content-type and body size middleware
		if strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/") && r.Method == http.MethodGet {
			CORS(RequestLogger(mux)).ServeHTTP(w, r)
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
