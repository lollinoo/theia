package api

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lollinoo/theia/internal/domain"
	"github.com/lollinoo/theia/internal/repository/postgres"
	"github.com/lollinoo/theia/internal/service"
	"github.com/lollinoo/theia/internal/vendor"
	"github.com/lollinoo/theia/internal/ws"
)

// routerOptions collects optional services that alter middleware or route behavior.
type routerOptions struct {
	security      SecurityConfig
	auth          authProvider
	bridgeService *service.BridgeService
	auditLogs     domain.AuditLogRepository
}

// RouterOption customizes router middleware behavior.
type RouterOption func(*routerOptions)

// WithSecurity configures operator authentication and browser origin policy.
func WithSecurity(config SecurityConfig) RouterOption {
	return func(options *routerOptions) {
		options.security = config
	}
}

// WithAuthService configures password-session authentication and RBAC.
func WithAuthService(authService *service.AuthService) RouterOption {
	return func(options *routerOptions) {
		options.auth = authService
	}
}

// WithBridgeService configures bridge launch and connector endpoints.
func WithBridgeService(bridgeService *service.BridgeService) RouterOption {
	return func(options *routerOptions) {
		options.bridgeService = bridgeService
	}
}

// WithAuditLogRepository configures audit logging for backup routes.
func WithAuditLogRepository(auditLogs domain.AuditLogRepository) RouterOption {
	return func(options *routerOptions) {
		options.auditLogs = auditLogs
	}
}

// withAuthProvider injects a test auth provider without requiring the concrete service.
func withAuthProvider(auth authProvider) RouterOption {
	return func(options *routerOptions) {
		options.auth = auth
	}
}

// NewRouter creates the HTTP handler with all /api/v1/ routes registered.
// Uses standard net/http (no framework needed at this scale).
func NewRouter(
	db *sql.DB,
	deviceService *service.DeviceService,
	linkRepo domain.LinkRepository,
	positionRepo domain.PositionRepository,
	canvasMapRepo domain.CanvasMapRepository,
	canvasMapPositionRepo domain.CanvasMapPositionRepository,
	settingsRepo domain.SettingsRepository,
	snmpProfileRepo domain.SNMPProfileRepository,
	credentialProfileRepo *postgres.CredentialProfileRepo,
	areaRepo domain.AreaRepository,
	backupService *service.BackupService,
	vendorRegistry *vendor.Registry,
	vendorConfigRepo domain.VendorConfigRepository,
	poller statusProvider,
	instanceBackupService *service.InstanceBackupService,
	restoreRestarter func(),
	bridgeBinariesDir string,
	runtimeSnapshotFunc func() (*ws.SnapshotPayload, uint64),
	wsHandler *ws.Handler,
	opts ...RouterOption,
) http.Handler {
	routerOpts := routerOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&routerOpts)
		}
	}

	mux := http.NewServeMux()
	authHandler := NewAuthHandler(routerOpts.auth)
	adminAuth, _ := routerOpts.auth.(adminProvider)
	adminHandler := NewAdminHandler(adminAuth)

	deviceHandler := NewDeviceHandler(
		deviceService,
		credentialProfileRepo,
		vendorRegistry,
		WithPrimaryCanvasMapMembership(canvasMapRepo, areaRepo, linkRepo),
	)
	linkHandler := NewLinkHandler(linkRepo, deviceService)
	positionHandler := NewPositionHandler(positionRepo, canvasMapRepo, canvasMapPositionRepo)
	canvasTopologyHandler := NewCanvasTopologyHandler(
		deviceService,
		linkRepo,
		positionRepo,
		areaRepo,
		vendorRegistry,
		runtimeSnapshotFunc,
	)
	canvasMapHandler := NewCanvasMapHandler(
		canvasMapRepo,
		canvasMapPositionRepo,
		positionRepo,
		canvasTopologyHandler,
		deviceService,
		linkRepo,
		areaRepo,
		runtimeSnapshotFunc,
	)
	settingsHandler := NewSettingsHandler(settingsRepo)
	grafanaDashboardHandler := NewGrafanaDashboardHandler(settingsRepo)
	snmpProfileHandler := NewSNMPProfileHandler(snmpProfileRepo)
	areaHandler := NewAreaHandler(areaRepo)
	backupHandlerOptions := []BackupHandlerOption{WithBackupAuditLogs(routerOpts.auditLogs)}
	if db != nil {
		bulkOperationLeaseRepo := postgres.NewBulkOperationLeaseRepo(db)
		if backupService != nil {
			backupService.SetBulkOperationLeaseRepository(bulkOperationLeaseRepo)
		}
		backupHandlerOptions = append(backupHandlerOptions, WithBulkDownloadLeaseRepository(bulkOperationLeaseRepo))
	}
	backupHandler := NewBackupHandler(backupService, settingsRepo, backupHandlerOptions...)
	credentialProfileHandler := NewCredentialProfileHandler(backupService, credentialProfileRepo)
	deviceCredHandler := NewDeviceCredentialProfileHandler(backupService, credentialProfileRepo)
	vendorHandler := NewVendorHandler(vendorRegistry, vendorConfigRepo)
	healthHandler := NewHealthHandler(db, poller)
	prometheusHandler := NewPrometheusHandler(settingsRepo)
	instanceBackupHandler := NewInstanceBackupHandlerWithRestarter(instanceBackupService, restoreRestarter)
	bridgeHandler := NewBridgeHandlerWithService(bridgeBinariesDir, routerOpts.bridgeService)
	userSettingsHandler := NewUserSettingsHandler(routerOpts.bridgeService, bridgeBinariesDir)

	mux.Handle("/api/v1/admin/", adminHandler)

	// Canvas topology read model route
	mux.HandleFunc("/api/v1/topology/canvas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		canvasTopologyHandler.HandleGet(w, r)
	})

	mux.HandleFunc("/api/v1/canvas", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		canvasTopologyHandler.HandleGetCanvas(w, r)
	})

	mux.HandleFunc("/api/v1/canvas/maps", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			canvasMapHandler.HandleList(w, r)
		case http.MethodPost:
			canvasMapHandler.HandleCreate(w, r)
		default:
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})

	mux.HandleFunc("/api/v1/canvas/maps/", func(w http.ResponseWriter, r *http.Request) {
		_, action, ok := parseCanvasMapRoute(r.URL.Path)
		if !ok {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		switch action {
		case "":
			switch r.Method {
			case http.MethodGet:
				canvasMapHandler.HandleGet(w, r)
			case http.MethodPatch:
				canvasMapHandler.HandlePatch(w, r)
			case http.MethodDelete:
				canvasMapHandler.HandleDelete(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		case "duplicate":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasMapHandler.HandleDuplicate(w, r)
		case "primary":
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasMapHandler.HandleSetPrimary(w, r)
		case "topology":
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasMapHandler.HandleTopology(w, r)
		case "bootstrap":
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasMapHandler.HandleBootstrap(w, r)
		case "positions":
			switch r.Method {
			case http.MethodGet:
				canvasMapHandler.HandleListPositions(w, r)
			case http.MethodPut:
				canvasMapHandler.HandleSavePositions(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		case "device-areas":
			if r.Method != http.MethodPut {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasMapHandler.HandleUpdateDeviceAreas(w, r)
		case "areas":
			switch r.Method {
			case http.MethodGet:
				canvasMapHandler.HandleListAreas(w, r)
			case http.MethodPost:
				canvasMapHandler.HandleCreateArea(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		default:
			if strings.HasPrefix(action, "areas/") {
				switch r.Method {
				case http.MethodPut:
					canvasMapHandler.HandleUpdateArea(w, r)
				case http.MethodDelete:
					canvasMapHandler.HandleDeleteArea(w, r)
				default:
					writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				}
				return
			}
			if strings.HasPrefix(action, "devices/") {
				switch r.Method {
				case http.MethodDelete:
					canvasMapHandler.HandleRemoveDevice(w, r)
				case http.MethodPatch:
					canvasMapHandler.HandlePatchDevice(w, r)
				case http.MethodPost:
					canvasMapHandler.HandleAddDevice(w, r)
				default:
					writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				}
				return
			}
			writeError(w, http.StatusNotFound, "not found")
		}
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

	mux.HandleFunc("/api/v1/devices/orphans", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		deviceHandler.HandleListOrphans(w, r)
	})

	// Device by ID routes (must be registered after /devices/batch and /devices/orphans to avoid conflicts)
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

		// Explicit WinBox credential reveal endpoint
		if isWinboxCredentialsRevealPath(r.URL.Path) {
			deviceCredHandler.HandleRevealWinboxCredentials(w, r)
			return
		}

		// Legacy WinBox credentials endpoint
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

	mux.HandleFunc("/api/v1/settings/me", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleMe(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleBridge(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge/secret", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleBridgeSecret(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge/secret/rotate", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleBridgeSecret(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge/secret/revoke", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleBridgeSecretRevoke(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge/connector/config", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleConnectorConfig(w, r)
	})

	mux.HandleFunc("/api/v1/settings/bridge/connector/download/", func(w http.ResponseWriter, r *http.Request) {
		userSettingsHandler.HandleConnectorDownload(w, r)
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

	mux.HandleFunc("/api/v1/grafana/dashboard-profiles", func(w http.ResponseWriter, r *http.Request) {
		grafanaDashboardHandler.HandleProfiles(w, r)
	})

	mux.HandleFunc("/api/v1/grafana/dashboard-profiles/", func(w http.ResponseWriter, r *http.Request) {
		grafanaDashboardHandler.HandleProfile(w, r)
	})

	mux.HandleFunc("/api/v1/grafana/device-overrides/", func(w http.ResponseWriter, r *http.Request) {
		grafanaDashboardHandler.HandleDeviceOverride(w, r)
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
		if strings.HasSuffix(r.URL.Path, "/reveal") {
			snmpProfileHandler.HandleReveal(w, r)
			return
		}
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
	mux.HandleFunc("/api/v1/backups/bulk/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleGetBulkOperationStatus(w, r)
	})

	mux.HandleFunc("/api/v1/backups/bulk-runs/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleGetLatestBulkBackupRun(w, r)
	})

	mux.HandleFunc("/api/v1/backups/bulk-runs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleStartBulkBackupRun(w, r)
	})

	mux.HandleFunc("/api/v1/backups/bulk-runs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pause") {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandlePauseBulkBackupRun(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/resume") {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleResumeBulkBackupRun(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleCancelBulkBackupRun(w, r)
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		backupHandler.HandleGetBulkBackupRun(w, r)
	})

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
	mux.HandleFunc("/api/v1/instance-backups/restore-status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		instanceBackupHandler.HandleRestoreStatus(w, r)
	})

	mux.HandleFunc("/api/v1/instance-backups/restore", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		instanceBackupHandler.HandleRestore(w, r)
	})

	mux.HandleFunc("/api/v1/instance-backups/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/v1/instance-backups/")
		parts := strings.Split(rest, "/")
		if rest == "" || len(parts) > 2 || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		if len(parts) == 2 {
			switch parts[1] {
			case "cancel":
				if r.Method != http.MethodPost {
					writeError(w, http.StatusMethodNotAllowed, "method not allowed")
					return
				}
				instanceBackupHandler.HandleCancel(w, r)
			case "download":
				if r.Method != http.MethodGet {
					writeError(w, http.StatusMethodNotAllowed, "method not allowed")
					return
				}
				instanceBackupHandler.HandleDownload(w, r)
			default:
				writeError(w, http.StatusNotFound, "not found")
			}
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

	mux.HandleFunc("/api/v1/bridge/launch-requests/", func(w http.ResponseWriter, r *http.Request) {
		bridgeHandler.HandleCreateLaunchRequest(w, r)
	})

	mux.HandleFunc("/api/v1/bridge/connector/launch", func(w http.ResponseWriter, r *http.Request) {
		bridgeHandler.HandleConnectorLaunch(w, r)
	})

	// Legacy bridge credential token endpoint.
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

	handler := applyMiddleware(mux, routerOpts.security, routerOpts.auth, true, 1<<20)
	downloadHandler := applyMiddleware(mux, routerOpts.security, routerOpts.auth, false, 0)
	publicAuthHandler := applyPublicMiddleware(authHandler, routerOpts.security, true, 16<<10)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAuthRoute(r.URL.Path) {
			publicAuthHandler.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/api/v1/bridge/connector/launch" {
			applyPublicMiddleware(http.HandlerFunc(bridgeHandler.HandleConnectorLaunch), routerOpts.security, true, 16<<10).ServeHTTP(w, r)
			return
		}

		// WebSocket upgrades must bypass the JSON/logger middleware chain because
		// the wrapped ResponseWriter does not expose the hijacker interface.
		if wsHandler != nil && r.URL.Path == "/api/v1/ws" {
			authenticatedRequest, user, _, ok := AuthenticateUserRequest(w, r, routerOpts.auth)
			if !ok {
				return
			}
			if user.User.User.MustChangePassword {
				writeAuthCodeError(w, http.StatusForbidden, "password_change_required", "password change required")
				return
			}
			if !requireAnyPermission(w, routerOpts.auth, user, []string{domain.PermissionTopologyRead}) {
				return
			}
			wsHandler.ServeHTTP(w, authenticatedRequest)
			return
		}

		// File download bypasses JSON content-type middleware
		if strings.HasSuffix(r.URL.Path, "/download") && strings.HasPrefix(r.URL.Path, "/api/v1/backup-files/") {
			downloadHandler.ServeHTTP(w, r)
			return
		}

		// Instance backup download bypasses JSON content-type and body size middleware
		if strings.HasSuffix(r.URL.Path, "/download") && strings.HasPrefix(r.URL.Path, "/api/v1/instance-backups/") {
			downloadHandler.ServeHTTP(w, r)
			return
		}

		// Instance backup restore bypasses JSON content-type but keeps a restore-specific body cap.
		if r.URL.Path == "/api/v1/instance-backups/restore" && r.Method == http.MethodPost {
			restoreLimit := service.DefaultRestoreArchiveLimits.MaxCompressedBytes
			if instanceBackupService != nil {
				restoreLimit = instanceBackupService.RestoreArchiveLimits().MaxCompressedBytes
			}
			applyMiddleware(mux, routerOpts.security, routerOpts.auth, false, restoreLimit+restoreMultipartEnvelopeOverheadBytes).ServeHTTP(w, r)
			return
		}

		// Bridge binary download bypasses JSON content-type and body size middleware
		if strings.HasPrefix(r.URL.Path, "/api/v1/bridge/download/") && r.Method == http.MethodGet {
			downloadHandler.ServeHTTP(w, r)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// isAuthRoute identifies public auth routes that bypass protected RBAC middleware.
func isAuthRoute(path string) bool {
	switch path {
	case "/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/me",
		"/api/v1/me",
		"/api/v1/auth/password/change",
		"/api/v1/auth/password/reset",
		"/api/v1/session":
		return true
	default:
		return false
	}
}

// parseCanvasMapRoute extracts the map ID and action suffix from saved-map routes.
func parseCanvasMapRoute(path string) (uuid.UUID, string, bool) {
	const prefix = "/api/v1/canvas/maps/"
	suffix, ok := strings.CutPrefix(path, prefix)
	if !ok || suffix == "" {
		return uuid.Nil, "", false
	}

	parts := strings.Split(suffix, "/")
	if len(parts) == 0 || parts[0] == "" {
		return uuid.Nil, "", false
	}
	mapID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", false
	}
	action := strings.Join(parts[1:], "/")
	if strings.Contains(action, "//") {
		return uuid.Nil, "", false
	}
	return mapID, action, true
}

// isWinboxCredentialsRevealPath matches the explicit credential reveal endpoint shape.
func isWinboxCredentialsRevealPath(path string) bool {
	const prefix = "/api/v1/devices/"
	suffix, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return false
	}
	parts := strings.Split(suffix, "/")
	return len(parts) == 3 &&
		parts[0] != "" &&
		parts[1] == "winbox-credentials" &&
		parts[2] == "reveal"
}

// indexOf returns the first byte offset of a substring or -1 when absent.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
