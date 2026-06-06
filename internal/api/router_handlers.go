package api

// This file defines router handlers API routing, middleware, and permission policy behavior.

import (
	"net/http"
	"strings"

	"github.com/lollinoo/theia/internal/repository/postgres"
)

// buildRouteHandlers wires route metadata keys to their dispatch handlers.
func buildRouteHandlers(deps routerDependencies, routerOpts routerOptions) map[routeHandlerKey]http.Handler {
	authHandler := NewAuthHandler(routerOpts.auth)
	adminAuth, _ := routerOpts.auth.(adminProvider)
	adminHandler := NewAdminHandler(adminAuth)

	deviceHandler := NewDeviceHandler(
		deps.deviceService,
		deps.credentialProfileRepo,
		deps.vendorRegistry,
		WithPrimaryCanvasMapMembership(deps.canvasMapRepo, deps.areaRepo, deps.linkRepo),
	)
	linkHandler := NewLinkHandler(deps.linkRepo, deps.deviceService)
	positionHandler := NewPositionHandler(deps.positionRepo, deps.canvasMapRepo, deps.canvasMapPositionRepo)
	canvasTopologyHandler := NewCanvasTopologyHandler(
		deps.deviceService,
		deps.linkRepo,
		deps.positionRepo,
		deps.areaRepo,
		deps.vendorRegistry,
		deps.runtimeSnapshotFunc,
	)
	canvasMapHandler := NewCanvasMapHandler(
		deps.canvasMapRepo,
		deps.canvasMapPositionRepo,
		deps.positionRepo,
		canvasTopologyHandler,
		deps.deviceService,
		deps.linkRepo,
		deps.areaRepo,
		deps.runtimeSnapshotFunc,
	)
	settingsHandler := NewSettingsHandler(deps.settingsRepo)
	grafanaDashboardHandler := NewGrafanaDashboardHandler(deps.settingsRepo)
	snmpProfileHandler := NewSNMPProfileHandler(deps.snmpProfileRepo)
	areaHandler := NewAreaHandler(deps.areaRepo)
	backupHandlerOptions := []BackupHandlerOption{WithBackupAuditLogs(routerOpts.auditLogs)}
	if deps.db != nil {
		bulkOperationLeaseRepo := postgres.NewBulkOperationLeaseRepo(deps.db)
		if deps.backupService != nil {
			deps.backupService.SetBulkOperationLeaseRepository(bulkOperationLeaseRepo)
		}
		backupHandlerOptions = append(backupHandlerOptions, WithBulkDownloadLeaseRepository(bulkOperationLeaseRepo))
	}
	backupHandler := NewBackupHandler(deps.backupService, deps.settingsRepo, backupHandlerOptions...)
	credentialProfileHandler := NewCredentialProfileHandler(deps.backupService, deps.credentialProfileRepo)
	deviceCredHandler := NewDeviceCredentialProfileHandler(deps.backupService, deps.credentialProfileRepo)
	vendorHandler := NewVendorHandler(deps.vendorRegistry, deps.vendorConfigRepo)
	healthHandler := NewHealthHandler(deps.db, deps.poller, routerOpts.runtimeEnvironment)
	prometheusHandler := NewPrometheusHandler(deps.settingsRepo)
	instanceBackupHandler := NewInstanceBackupHandlerWithRestarter(deps.instanceBackupService, deps.restoreRestarter)
	bridgeHandler := NewBridgeHandlerWithService(deps.bridgeBinariesDir, routerOpts.bridgeService)
	userSettingsHandler := NewUserSettingsHandler(routerOpts.bridgeService, deps.bridgeBinariesDir)

	webSocketHandler := http.NotFoundHandler()
	if deps.wsHandler != nil {
		webSocketHandler = deps.wsHandler
	}
	return map[routeHandlerKey]http.Handler{
		routeHandlerAdmin: adminHandler,
		routeHandlerAuth:  authHandler,
		routeHandlerTopologyCanvas: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasTopologyHandler.HandleGet(w, r)
		}),
		routeHandlerCanvas: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			canvasTopologyHandler.HandleGetCanvas(w, r)
		}),
		routeHandlerCanvasMapCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				canvasMapHandler.HandleList(w, r)
			case http.MethodPost:
				canvasMapHandler.HandleCreate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerCanvasMapItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerDeviceCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				deviceHandler.HandleCreate(w, r)
			case http.MethodGet:
				deviceHandler.HandleList(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerDeviceBatch: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			deviceHandler.HandleBatchAdd(w, r)
		}),
		routeHandlerDeviceOrphans: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			deviceHandler.HandleListOrphans(w, r)
		}),
		routeHandlerDeviceItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/interfaces") && r.Method == http.MethodGet {
				linkHandler.HandleGetInterfaces(w, r)
				return
			}
			if len(r.URL.Path) > len("/api/v1/devices/") {
				pathSuffix := r.URL.Path[len("/api/v1/devices/"):]
				if idx := indexOf(pathSuffix, "/probe"); idx >= 0 && r.Method == http.MethodPost {
					deviceHandler.HandleProbe(w, r)
					return
				}
			}
			if strings.HasSuffix(r.URL.Path, "/snmp-test") && r.Method == http.MethodPost {
				deviceHandler.HandleTestSNMP(w, r)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/topology-discovery") && r.Method == http.MethodPost {
				deviceHandler.HandleRunTopologyDiscovery(w, r)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/ssh-credentials/test") && r.Method == http.MethodPost {
				backupHandler.HandleTestSSH(w, r)
				return
			}
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
			if strings.Contains(r.URL.Path, "/credential-profiles/") && r.Method == http.MethodDelete {
				deviceCredHandler.HandleUnassign(w, r)
				return
			}
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
			if isWinboxCredentialsRevealPath(r.URL.Path) {
				deviceCredHandler.HandleRevealWinboxCredentials(w, r)
				return
			}
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
		}),
		routeHandlerLinkCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				linkHandler.HandleList(w, r)
			case http.MethodPost:
				linkHandler.HandleCreate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerLinkItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPut:
				linkHandler.HandleUpdate(w, r)
			case http.MethodDelete:
				linkHandler.HandleDelete(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerPositionCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				positionHandler.HandleList(w, r)
			case http.MethodPut:
				positionHandler.HandleSaveAll(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerSettingsCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			settingsHandler.HandleGetAll(w, r)
		}),
		routeHandlerSettingsMe:                 http.HandlerFunc(userSettingsHandler.HandleMe),
		routeHandlerSettingsBridge:             http.HandlerFunc(userSettingsHandler.HandleBridge),
		routeHandlerSettingsBridgeSecret:       http.HandlerFunc(userSettingsHandler.HandleBridgeSecret),
		routeHandlerSettingsBridgeSecretRevoke: http.HandlerFunc(userSettingsHandler.HandleBridgeSecretRevoke),
		routeHandlerSettingsBridgeConnector:    http.HandlerFunc(userSettingsHandler.HandleConnectorConfig),
		routeHandlerSettingsBridgeDownload:     http.HandlerFunc(userSettingsHandler.HandleConnectorDownload),
		routeHandlerSettingsItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				settingsHandler.HandleGet(w, r)
			case http.MethodPut:
				settingsHandler.HandleUpdate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerGrafanaProfileCollection: http.HandlerFunc(grafanaDashboardHandler.HandleProfiles),
		routeHandlerGrafanaProfileItem:       http.HandlerFunc(grafanaDashboardHandler.HandleProfile),
		routeHandlerGrafanaDeviceOverride:    http.HandlerFunc(grafanaDashboardHandler.HandleDeviceOverride),
		routeHandlerSNMPProfileCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				snmpProfileHandler.HandleList(w, r)
			case http.MethodPost:
				snmpProfileHandler.HandleCreate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerSNMPProfileItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerAreaCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				areaHandler.HandleList(w, r)
			case http.MethodPost:
				areaHandler.HandleCreate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerAreaItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerCredentialProfileCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				credentialProfileHandler.HandleList(w, r)
			case http.MethodPost:
				credentialProfileHandler.HandleCreate(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerCredentialProfileItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerBackupBulkStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleGetBulkOperationStatus(w, r)
		}),
		routeHandlerBackupBulkRunLatest: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleGetLatestBulkBackupRun(w, r)
		}),
		routeHandlerBackupBulkRunCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleStartBulkBackupRun(w, r)
		}),
		routeHandlerBackupBulkRunItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerBackupBulk: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleBulkBackup(w, r)
		}),
		routeHandlerBackupBulkDownload: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			backupHandler.HandleBulkDownload(w, r)
		}),
		routeHandlerBackupJob: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				backupHandler.HandleGetBackupJob(w, r)
			case http.MethodDelete:
				backupHandler.HandleDeleteBackupJob(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerBackupFile: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerVendorCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			vendorHandler.HandleListVendors(w, r)
		}),
		routeHandlerVendorItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				vendorHandler.HandleGetVendor(w, r)
			case http.MethodPut:
				vendorHandler.HandleUpdateVendor(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerInstanceBackupCollection: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				instanceBackupHandler.HandleCreate(w, r)
			case http.MethodGet:
				instanceBackupHandler.HandleList(w, r)
			default:
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		}),
		routeHandlerInstanceBackupRestoreStatus: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			instanceBackupHandler.HandleRestoreStatus(w, r)
		}),
		routeHandlerInstanceBackupRestore: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			instanceBackupHandler.HandleRestore(w, r)
		}),
		routeHandlerInstanceBackupItem: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		}),
		routeHandlerBridgeDownload: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			bridgeHandler.HandleDownload(w, r)
		}),
		routeHandlerBridgeLaunchRequest:   http.HandlerFunc(bridgeHandler.HandleCreateLaunchRequest),
		routeHandlerBridgeConnectorLaunch: http.HandlerFunc(bridgeHandler.HandleConnectorLaunch),
		routeHandlerBridgeToken:           http.HandlerFunc(bridgeHandler.HandleBridgeToken),
		routeHandlerHealth:                http.HandlerFunc(healthHandler.HandleHealth),
		routeHandlerPrometheusHealth: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeError(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			prometheusHandler.HandleHealth(w, r)
		}),
		routeHandlerWebSocket: webSocketHandler,
	}
}
